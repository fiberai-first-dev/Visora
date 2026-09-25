package search_intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/brand"
	"visora-backend/internal/crawler"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/llm"
)

const maxIntents = 5

type GeneratedIntent struct {
	Keyword  string `json:"keyword"`
	Intent   string `json:"intent"`
	Source   string `json:"source"`
	Priority int    `json:"priority"`
}

// loose shapes â€” Vyce/DeepSeek often ignores JSON Schema and returns queries/query.
type looseIntent struct {
	Keyword  string `json:"keyword"`
	Query    string `json:"query"`
	Intent   string `json:"intent"`
	Source   string `json:"source"`
	Product  string `json:"product"`
	Priority int    `json:"priority"`
}

type looseList struct {
	Intents []looseIntent `json:"intents"`
	Queries []looseIntent `json:"queries"`
}

// Generate asks the LLM for buyer-search intents. There is no template
// fallback: a failed or empty reply fails the job so the worker retries it.
func Generate(projectID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	events.PublishEvent(projectID, 4, "progress", "Finding buyer searches", nil)

	intents, err := generateFromLLM(project)
	if err != nil {
		return fmt.Errorf("buyer search AI call failed: %w", err)
	}
	source := "ai_discovery"

	intents = validIntents(productFocused(intents, project))
	if len(intents) > maxIntents {
		intents = intents[:maxIntents]
	}
	if len(intents) == 0 {
		return fmt.Errorf("AI returned no usable buyer searches")
	}

	db.DB.Where("project_id = ?", projectID).Delete(&db.SearchIntent{})

	keywords := make([]string, 0, len(intents))
	for _, intent := range intents {
		keyword := strings.TrimSpace(intent.Keyword)
		db.DB.Create(&db.SearchIntent{
			ProjectID: projectID,
			Keyword:   keyword,
			Intent:    defaultString(intent.Intent, "commercial"),
			Source:    defaultString(intent.Source, source),
			Priority:  defaultPriority(intent.Priority),
			Status:    "opportunity",
		})
		keywords = append(keywords, keyword)
	}

	events.PublishEvent(projectID, 4, "complete",
		fmt.Sprintf("Prepared %d buyer searches", len(keywords)),
		map[string]interface{}{
			"count":    len(keywords),
			"keywords": keywords,
			"source":   source,
		})

	payload, _ := json.Marshal(map[string]interface{}{"project_id": projectID})
	db.DB.Create(&db.Job{
		Type:    "serp_check_run",
		Payload: string(payload),
		Status:  "queued",
	})

	return nil
}

func generateFromLLM(project db.Project) ([]GeneratedIntent, error) {
	var profile brand.BrandProfile
	_ = json.Unmarshal([]byte(project.ProfileData), &profile)

	products := profile.Products
	topics := profile.Topics
	category := profile.Category
	audience := profile.TargetAudience
	brandName := profile.BrandName
	if brandName == "" {
		brandName = project.Brand
	}
	if category == "" {
		category = project.Category
	}
	country := strings.TrimSpace(project.Country)
	if country == "" {
		country = "the site's home market"
	}

	var suggested []string
	for _, q := range profile.SearchQueries {
		if s := strings.TrimSpace(q.Query); s != "" {
			suggested = append(suggested, s)
		}
	}

	var pagesText strings.Builder
	if pages := crawler.LatestSitePages(project.ID, 40); len(pages) > 0 {
		used := 0
		for i := range pages {
			if used >= 6 || crawler.IsBoilerplatePage(pages[i].URL) {
				continue
			}
			fmt.Fprintf(&pagesText, "PAGE %s\n%s\n\n", pages[i].URL, crawler.PageOutline(&pages[i], 500))
			used++
		}
	}

	prompt := fmt.Sprintf(`You are a search-intent strategist.

Brand: %s
Website: %s
Category: %s
Market: %s
Products: %v
Topics: %v
Audience: %s
Searches the brand analyst suggested: %v

What the site says:
%s
Write 8-10 Google searches a real buyer in %s would type to FIND WHAT THIS SITE SELLS, before they know the brand.
Never the brand or domain alone. Prefer commercial / transactional searches. Only add a place, platform or audience
modifier when the pages above show it matters to this business.

Return ONLY a JSON object with this exact shape:
{"intents":[{"keyword":"...","intent":"commercial","source":"product","priority":1}]}

Keys must be "intents" and "keyword" (not "queries" / "query"). No markdown, no extra text.`,
		brandName, project.Website, category, country, products, topics, audience, suggested, pagesText.String(), country)

	raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}, 0.3)
	if err != nil {
		return nil, err
	}

	parsed := parseLooseIntents(raw)
	if len(parsed) == 0 {
		return nil, fmt.Errorf("AI returned empty intents array")
	}
	return parsed, nil
}

func parseLooseIntents(raw string) []GeneratedIntent {
	var list looseList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	rows := list.Intents
	if len(rows) == 0 {
		rows = list.Queries
	}
	out := make([]GeneratedIntent, 0, len(rows))
	for _, row := range rows {
		keyword := strings.TrimSpace(row.Keyword)
		if keyword == "" {
			keyword = strings.TrimSpace(row.Query)
		}
		if keyword == "" {
			continue
		}
		source := strings.TrimSpace(row.Source)
		if source == "" {
			source = "product"
		}
		intent := strings.TrimSpace(row.Intent)
		if intent == "" {
			intent = "commercial"
		}
		out = append(out, GeneratedIntent{
			Keyword:  keyword,
			Intent:   intent,
			Source:   source,
			Priority: defaultPriority(row.Priority),
		})
	}
	return out
}

// productFocused drops brand-only / domain / navigational junk from AI output.
func productFocused(in []GeneratedIntent, project db.Project) []GeneratedIntent {
	var profile brand.BrandProfile
	_ = json.Unmarshal([]byte(project.ProfileData), &profile)
	brandName := profile.BrandName
	if brandName == "" {
		brandName = project.Brand
	}

	out := make([]GeneratedIntent, 0, len(in))
	for _, item := range in {
		q := cleanQuery(item.Keyword)
		q = stripBrandPrefix(q, brandName, project.Brand)
		if !looksLikeProductQuery(q, brandName, project.Brand) {
			continue
		}
		item.Keyword = q
		if item.Intent == "navigational" {
			item.Intent = "commercial"
		}
		out = append(out, item)
	}
	return out
}

func looksLikeProductQuery(q, brandName, projectBrand string) bool {
	q = strings.TrimSpace(q)
	if len(q) < 3 {
		return false
	}
	lower := strings.ToLower(q)
	if strings.Contains(lower, ".com") || strings.Contains(lower, ".in") || strings.Contains(lower, "www.") {
		return false
	}
	for _, b := range []string{brandName, projectBrand} {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		bl := strings.ToLower(b)
		if lower == bl || lower == bl+" review" || lower == bl+" alternative" || lower == bl+" login" {
			return false
		}
	}
	return true
}

func stripBrandPrefix(q string, brands ...string) string {
	q = strings.TrimSpace(q)
	lower := strings.ToLower(q)
	for _, b := range brands {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		bl := strings.ToLower(b)
		if strings.HasPrefix(lower, bl+" ") {
			q = strings.TrimSpace(q[len(b):])
			lower = strings.ToLower(q)
		}
		if strings.HasPrefix(lower, bl+"'s ") {
			q = strings.TrimSpace(q[len(b)+2:])
		}
	}
	return strings.TrimSpace(q)
}

func validIntents(in []GeneratedIntent) []GeneratedIntent {
	out := make([]GeneratedIntent, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		keyword := cleanQuery(item.Keyword)
		if keyword == "" || seen[strings.ToLower(keyword)] {
			continue
		}
		seen[strings.ToLower(keyword)] = true
		item.Keyword = keyword
		out = append(out, item)
	}
	return out
}

func cleanQuery(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	if len(raw) < 2 {
		return ""
	}
	return raw
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultPriority(value int) int {
	if value <= 0 {
		return 2
	}
	return value
}
