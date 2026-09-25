package geo

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/llm"
)

const maxGeoPrompts = 4

var (
	reMarkdownHeading = regexp.MustCompile(`^#{1,6}\s*`)
	reMarkdownBullet  = regexp.MustCompile(`^(\*\s+|\*\*\s+|•\s+)`)
	reMarkdownItalic  = regexp.MustCompile(`(^|[^*\w])\*([^*\n]+)\*([^*\w]|$)`)
)
// RunMultiModelGeo executes GEO checks using the same search intents used for Google SERP.
func RunMultiModelGeo(projectID uint) (uint, error) {
	apiKey := os.Getenv("GEO_OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return 0, fmt.Errorf("GEO is not configured: set GEO_OPENAI_API_KEY or OPENAI_API_KEY")
	}

	if err := syncPromptsFromSearchIntents(projectID); err != nil {
		fmt.Printf("geo: intent sync warning for project %d: %v\n", projectID, err)
	}

	var prompts []db.GeoPrompt
	if err := db.DB.Where("project_id = ? AND active = ?", projectID, true).
		Order("id asc").
		Limit(maxGeoPrompts).
		Find(&prompts).Error; err != nil {
		return 0, err
	}

	if len(prompts) == 0 {
		return 0, fmt.Errorf("no active geo prompts found for project %d", projectID)
	}

	models := []struct {
		engine string
		model  string
		label  string
	}{
		// Spread across providers so one free-model RPM bucket cannot wipe all 4 cards.
		{"chatgpt", llm.CanonicalModel(getEnvOrDefault("GEO_MODEL_GPT", "openai/gpt-4o-mini")), "chatgpt"},
		{"claude", llm.CanonicalModel(getEnvOrDefault("GEO_MODEL_CLAUDE", "alibaba/qwen3.5-flash")), "claude"},
		{"gemini", llm.CanonicalModel(getEnvOrDefault("GEO_MODEL_GEMINI", "google/gemini-2.5-flash-lite")), "gemini"},
		{"grok", llm.CanonicalModel(getEnvOrDefault("GEO_MODEL_GROK", "alibaba/qwen3.5-flash")), "grok"},
	}

	started := time.Now()
	run := db.GeoRun{
		ProjectID:    projectID,
		Status:       "running",
		Model:        "multi-model",
		PromptsTotal: len(prompts) * len(models),
		StartedAt:    &started,
	}
	db.DB.Create(&run)

	concurrency := geoConcurrency()
	events.PublishEvent(projectID, 9, "progress", "Querying AI models for brand visibility", map[string]interface{}{
		"prompts":     len(prompts),
		"models":      len(models),
		"concurrency": concurrency,
	})

	var done, failed int64

	// One buyer question at a time; all four model cards fire in parallel for that question.
	for i, p := range prompts {
		events.PublishEvent(projectID, 9, "milestone",
			fmt.Sprintf("Asking models: %s", truncatePrompt(p.Text, 80)),
			map[string]interface{}{
				"prompt":        p.Text,
				"prompt_index":  i + 1,
				"prompts_total": len(prompts),
				"models":        []string{"chatgpt", "claude", "gemini", "grok"},
			})

		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		for _, m := range models {
			wg.Add(1)
			sem <- struct{}{}
			go func(prompt db.GeoPrompt, engine, modelID, label string, promptIndex int) {
				defer wg.Done()
				defer func() { <-sem }()
				queryVyce(projectID, run.ID, prompt, engine, modelID, label, promptIndex, len(prompts), &done, &failed)
			}(p, m.engine, m.model, m.label, i+1)
		}
		wg.Wait()
	}
	finished := time.Now()
	updated := map[string]interface{}{
		"status":         "done",
		"prompts_done":   int(atomic.LoadInt64(&done)),
		"prompts_failed": int(atomic.LoadInt64(&failed)),
		"finished_at":    finished,
	}
	if atomic.LoadInt64(&done) == 0 {
		updated["status"] = "error"
		updated["error"] = "every GEO request failed"
	}
	db.DB.Model(&db.GeoRun{}).Where("id = ?", run.ID).Updates(updated)

	events.PublishEvent(projectID, 9, "complete",
		fmt.Sprintf("Collected %d AI answers (%d failed)", atomic.LoadInt64(&done), atomic.LoadInt64(&failed)),
		map[string]interface{}{
			"geo_run_id":    run.ID,
			"responses":     int(atomic.LoadInt64(&done)),
			"failed":        int(atomic.LoadInt64(&failed)),
			"prompts_total": run.PromptsTotal,
		})

	if atomic.LoadInt64(&done) == 0 {
		return run.ID, fmt.Errorf("all GEO requests failed for project %d", projectID)
	}

	return run.ID, nil
}

// syncPromptsFromSearchIntents replaces active GEO prompts with the same
// product search intents used for Google SERP checks.
func syncPromptsFromSearchIntents(projectID uint) error {
	var intents []db.SearchIntent
	if err := db.DB.Where("project_id = ?", projectID).
		Order("priority desc, id asc").
		Limit(maxGeoPrompts).
		Find(&intents).Error; err != nil {
		return err
	}
	if len(intents) == 0 {
		return fmt.Errorf("no search intents")
	}

	db.DB.Model(&db.GeoPrompt{}).
		Where("project_id = ? AND active = ?", projectID, true).
		Update("active", false)

	for _, intent := range intents {
		text := intentToGeoPrompt(intent.Keyword)
		db.DB.Create(&db.GeoPrompt{
			ProjectID: projectID,
			Text:      text,
			Intent:    intent.Intent,
			Active:    true,
		})
	}
	return nil
}

func intentToGeoPrompt(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return keyword
	}
	lower := strings.ToLower(keyword)
	if strings.HasPrefix(lower, "what ") || strings.HasPrefix(lower, "how ") ||
		strings.HasPrefix(lower, "which ") || strings.Contains(keyword, "?") {
		return keyword
	}
	return fmt.Sprintf("What are the best options for %s?", keyword)
}

func geoUserMessage(question string) string {
	q := strings.TrimSpace(question)
	return q + "\n\nReply in under 80 words. Name at most 4 options as plain lines starting with - . No markdown stars or headings."
}

func trimGeoAnswer(text string) string {
	text = stripMarkdownNoise(text)
	if text == "" {
		return text
	}
	const max = 700
	if len(text) <= max {
		return text
	}
	cut := text[:max]
	if i := strings.LastIndexAny(cut, "\n."); i > max/2 {
		return strings.TrimSpace(cut[:i+1])
	}
	return strings.TrimSpace(cut) + "…"
}

// stripMarkdownNoise turns raw LLM markdown into clean chat copy so * / ** / ####
// never leak into the UI (that looks like an unstyled model dump).
func stripMarkdownNoise(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		// Headings #### Title → Title
		s = reMarkdownHeading.ReplaceAllString(s, "")
		// Leading markdown bullets
		s = reMarkdownBullet.ReplaceAllString(s, "- ")
		// Bold / italic markers
		s = strings.ReplaceAll(s, "**", "")
		s = strings.ReplaceAll(s, "__", "")
		s = reMarkdownItalic.ReplaceAllString(s, "$1$2$3")
		s = strings.ReplaceAll(s, "*", "")
		// Inline code / leftover hashes
		s = strings.ReplaceAll(s, "`", "")
		s = strings.TrimSpace(s)
		if s == "" || s == "-" {
			continue
		}
		out = append(out, s)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// geoConcurrency caps parallel model calls per buyer question.
// Default 1 — free gateway models often allow only ~5 RPM; parallel 4× kills the step.
func geoConcurrency() int {
	raw := strings.TrimSpace(os.Getenv("GEO_CONCURRENCY"))
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}

func truncatePrompt(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max-1] + "…"
}

func saveResponse(projectID, runID, promptID uint, model, responseText string) {
	db.DB.Create(&db.GeoResponse{
		ProjectID:    projectID,
		RunID:        runID,
		PromptID:     promptID,
		Model:        model,
		ResponseText: responseText,
	})
}

func queryVyce(
	projectID, runID uint,
	prompt db.GeoPrompt,
	engineName, modelID, label string,
	promptIndex, promptsTotal int,
	done, failed *int64,
) {
	if os.Getenv("GEO_OPENAI_API_KEY") == "" && os.Getenv("OPENAI_API_KEY") == "" {
		atomic.AddInt64(failed, 1)
		return
	}
	if modelID == "" {
		modelID = llm.GetModel()
	}

	var (
		text string
		err  error
	)
	modelsToTry := []string{llm.CanonicalModel(modelID)}

	for _, tryModel := range modelsToTry {
		modelID = tryModel
		text = ""
		err = nil
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			text, err = llm.Complete(ctx, modelID, []openai.ChatCompletionMessage{
				{
					Role: openai.ChatMessageRoleSystem,
					Content: "You are a concise shopping assistant writing for a chat UI. " +
						"Answer in under 80 words. At most 4 options as short lines. " +
						"Plain text only — never use markdown: no *, **, #, ####, backticks, or links in brackets.",
				},
				{Role: openai.ChatMessageRoleUser, Content: geoUserMessage(prompt.Text)},
			}, 0.35, 220)
			cancel()
			if err == nil && strings.TrimSpace(text) != "" {
				break
			}
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			// Don't burn retries on hard model access errors — try the fallback model instead.
			if strings.Contains(msg, "403") || strings.Contains(msg, "404") || strings.Contains(msg, "Forbidden") || strings.Contains(msg, "Not Found") {
				fmt.Printf("geo: %s model %s unavailable (%s) — trying next\n", engineName, modelID, truncatePrompt(msg, 80))
				break
			}
			if attempt < 5 && (strings.Contains(msg, "429") || strings.Contains(msg, "Too Many") || strings.Contains(msg, "rate_limit") || strings.Contains(msg, "concurrent")) {
				wait := time.Duration(8*(attempt+1)) * time.Second
				if strings.Contains(msg, "Retry after") {
					wait = time.Duration(20+attempt*15) * time.Second
				}
				fmt.Printf("geo: %s rate-limited, retry in %s (attempt %d)\n", engineName, wait, attempt+1)
				time.Sleep(wait)
				continue
			}
			break
		}
		if err == nil && strings.TrimSpace(text) != "" {
			break
		}
	}

	if err != nil || strings.TrimSpace(text) == "" {
		atomic.AddInt64(failed, 1)
		fmt.Printf("geo: %s failed for prompt %d: %v\n", engineName, prompt.ID, err)
		events.PublishEvent(projectID, 9, "milestone",
			fmt.Sprintf("%s failed", label),
			map[string]interface{}{
				"prompt":        prompt.Text,
				"prompt_index":  promptIndex,
				"prompts_total": promptsTotal,
				"model":         label,
				"engine":        engineName,
				"failed":        true,
			})
		return
	}

	text = trimGeoAnswer(text)
	saveResponse(projectID, runID, prompt.ID, engineName, text)
	atomic.AddInt64(done, 1)

	events.PublishEvent(projectID, 9, "milestone",
		fmt.Sprintf("%s answered", label),
		map[string]interface{}{
			"prompt":        prompt.Text,
			"prompt_index":  promptIndex,
			"prompts_total": promptsTotal,
			"model":         label,
			"engine":        engineName,
			"response":      text,
			"failed":        false,
		})
}
