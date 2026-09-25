package fixes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/crawler"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/llm"
)

type structuredFix struct {
	PageURL     string `json:"page_url"`
	TargetField string `json:"target_field"`
	Before      string `json:"before"`
	After       string `json:"after"`
	Note        string `json:"note"`
}

// GenerateForProject has the LLM write page-grounded fixes for open
// recommendations. There is no template fallback: if the AI writes nothing the
// job fails so the worker retries it.
func GenerateForProject(projectID uint) error {
	var recommendations []db.Recommendation
	if err := db.DB.Where("project_id = ? AND status = 'open'", projectID).
		Order("CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END, id asc").
		Limit(12).
		Find(&recommendations).Error; err != nil {
		return err
	}

	events.PublishEvent(projectID, 13, "progress", "Writing exact wording for each page", map[string]int{
		"recommendations_to_fix": len(recommendations),
	})

	if len(recommendations) == 0 {
		events.PublishEvent(projectID, 13, "complete", "No open recommendations to fix", nil)
		return nil
	}

	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	type pendingFix struct {
		rec  db.Recommendation
		fix  structuredFix
		meta recMeta
		page *db.Page
	}
	written := make([]pendingFix, 0, len(recommendations))
	var firstErr error
	for _, rec := range recommendations {
		meta := parseRecData(rec.Data)
		query := queryFromRec(rec)
		page := resolveTargetPage(projectID, meta.PageURL, query)
		if page != nil && (meta.PageURL == "" || crawler.IsBoilerplatePage(meta.PageURL) && query != "") {
			meta.PageURL = page.URL
			meta.Before = ""
		}
		if page != nil && meta.Before == "" {
			meta.Before = currentTextForField(page, meta.TargetField)
		}
		field := strings.ToLower(firstNonEmpty(meta.TargetField, "body"))
		pageContext := formatPageContext(page, meta)
		searchLine := ""
		if query != "" {
			searchLine = fmt.Sprintf("\nBuyer search to win: %q.\n", query)
			if rival := crawler.RivalContext(projectID, query, 1200); rival != "" {
				searchLine += "\nWHAT RANKS NOW (beat this, do not copy it):\n" + rival + "\n"
			}
		}
		draft := ""
		if d := strings.TrimSpace(meta.After); d != "" {
			draft = "\nStrategist's rough draft (improve it, do not paste it blindly):\n" + d + "\n"
		}
		prompt := fmt.Sprintf(`Brand: %s
Website: %s
Country: %s
%s
Recommendation:
Title: %s
Detail: %s
Action: %s
%s
Target page: %s
Field to rewrite: %s
Current text of that field: %q

FULL PAGE AS CRAWLED (headings + copy, one block per line):
%s

Rewrite ONLY the %s of this page so it ranks for the buyer search and so an AI assistant can recommend this brand.
Use what the page actually sells — product names, features, audience — from the copy above.

Return JSON only:
{"page_url":"...","target_field":"%s","before":"...","after":"...","note":"..."}
Rules:
- before: the current text above, copied exactly.
- after: NEW paste-ready copy, never the current page text repeated.
  title: 45-60 chars, "<Product name>: <what it does> for <who> | %s" style, reads like a human wrote it, not a keyword list.
  meta: 120-155 chars, one sentence with the offer, the audience and one concrete proof point from the page.
  h1: under 70 chars, the product's promise in plain words.
  body: 2-4 sentences, one paragraph, names the product and says who it is for and what it does.
  faq: 2-3 lines of "Q: … A: …" answering real buyer questions from the page.
  schema: a JSON-LD object for the page.
- No filler ("see how it works", "compare options", "get started today", "look no further").
- note: one short line saying where on the page to put it.`,
			project.Brand, project.Website, project.Country, searchLine,
			rec.Title, rec.Detail, rec.Action, draft,
			meta.PageURL, field, meta.Before,
			pageContext, field, field, project.Brand)

		wrote, err := askFix(prompt, func(f structuredFix) string {
			return copyProblem(f.After, meta.Before, field, page)
		}, func(f structuredFix) structuredFix {
			f.After = shortenCopy(f.After, field)
			return f
		})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			fmt.Printf("fixes: AI could not write recommendation %d: %v\n", rec.ID, err)
			continue
		}
		wrote.TargetField = field
		wrote.PageURL = meta.PageURL
		if page != nil && (wrote.PageURL == "" || crawler.IsBoilerplatePage(wrote.PageURL)) {
			wrote.PageURL = page.URL
		}
		written = append(written, pendingFix{rec: rec, fix: wrote, meta: meta, page: page})
	}

	if len(written) == 0 {
		if firstErr != nil {
			return fmt.Errorf("AI wrote no page fixes: %w", firstErr)
		}
		return fmt.Errorf("AI wrote no page fixes")
	}

	// Only replace the previous fixes once the new AI copy exists.
	db.DB.Where("project_id = ? AND status = ?", projectID, "pending").Delete(&db.GeneratedFix{})

	generated := 0
	for _, w := range written {
		rec, fix, meta, page := w.rec, w.fix, w.meta, w.page
		pageURL := strings.TrimSpace(fix.PageURL)
		if pageURL == "" {
			pageURL = meta.PageURL
		}
		if pageURL == "" && page != nil {
			pageURL = page.URL
		}

		// Re-resolve page for the final URL (LLM may change it).
		finalPage := loadPage(projectID, pageURL)
		if finalPage == nil {
			finalPage = page
		}
		field := firstNonEmpty(fix.TargetField, meta.TargetField, "body")
		before := firstNonEmpty(
			strings.TrimSpace(fix.Before),
			strings.TrimSpace(meta.Before),
			currentTextForField(finalPage, field),
		)
		if field == "title" || field == "meta" || field == "h1" {
			// Single-line fields: show the page's real text, not the model's copy of it.
			before = firstNonEmpty(currentTextForField(finalPage, field), before)
		}

		// Keep the recommendation's copy in step with the polished fix.
		var recData map[string]interface{}
		if json.Unmarshal([]byte(rec.Data), &recData) == nil && recData != nil {
			recData["after"] = fix.After
			recData["before"] = before
			recData["page_url"] = pageURL
			if b, err := json.Marshal(recData); err == nil {
				db.DB.Model(&db.Recommendation{}).Where("id = ?", rec.ID).Update("data", string(b))
			}
		}

		content, _ := json.Marshal(map[string]string{
			"page_url":     pageURL,
			"target_field": field,
			"before":       before,
			"after":        fix.After,
			"note":         fix.Note,
		})

		db.DB.Create(&db.GeneratedFix{
			ProjectID:        projectID,
			RecommendationID: &rec.ID,
			FixType:          fixTypeFor(field, rec.Source),
			PageURL:          pageURL,
			Title:            fmt.Sprintf("%s · %s", displayField(field), rec.Title),
			Content:          string(content),
			Status:           "pending",
		})
		generated++

		events.PublishEvent(projectID, 13, "milestone",
			fmt.Sprintf("Fix ready: %s", truncate(rec.Title, 60)),
			map[string]interface{}{
				"page_url":     pageURL,
				"target_field": field,
				"fixes_done":   generated,
			})
	}

	events.PublishEvent(projectID, 13, "complete",
		fmt.Sprintf("AI wrote %d page fixes", generated),
		map[string]interface{}{"fixes": generated})

	return nil
}

// askFix asks the copywriter LLM for one page edit. When the reply is broken
// or fails check, the problem is fed back and it tries again.
// fit gets one last chance to repair the final reply (e.g. trim overlong copy).
func askFix(prompt string, check func(structuredFix) string, fit func(structuredFix) structuredFix) (structuredFix, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a senior SEO/GEO copywriter. You rewrite one field of a real page using the page's real offer. JSON only.",
		},
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}
	var lastErr error
	var lastFix structuredFix
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), messages, 0.4)
		if err != nil {
			lastErr = err
			continue
		}
		var fix structuredFix
		if err := json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &fix); err != nil {
			lastErr = fmt.Errorf("reply was not valid JSON: %w", err)
			continue
		}
		fix.After = strings.TrimSpace(fix.After)
		problem := check(fix)
		if problem == "" {
			return fix, nil
		}
		lastErr = fmt.Errorf("rejected copy: %s", problem)
		lastFix = fix
		messages = append(messages,
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: raw},
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "That copy is not usable: " + problem + ". Write it again following every rule. JSON only."},
		)
	}
	if lastFix.After != "" && fit != nil {
		if fixed := fit(lastFix); check(fixed) == "" {
			return fixed, nil
		}
	}
	return structuredFix{}, lastErr
}

var fieldLimits = map[string]int{"title": 70, "meta": 170, "h1": 80}

// shortenCopy asks the LLM to tighten overlong single-line copy, falling back
// to fitToLimit when the reply is still too long.
func shortenCopy(text, field string) string {
	limit, ok := fieldLimits[field]
	if !ok || len([]rune(text)) <= limit {
		return text
	}
	target := limit - 10
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You tighten SEO copy without losing its keywords or meaning. JSON only."},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(
			"Rewrite this page %s so it is at most %d characters, one line, complete words and sentences, same product and keywords.\n\n%s\n\nReturn {\"after\": \"...\"}",
			field, target, text)},
	}
	if raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), messages, 0.2); err == nil {
		var out struct {
			After string `json:"after"`
		}
		if json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &out) == nil {
			if s := strings.TrimSpace(out.After); s != "" && len([]rune(s)) <= limit {
				return s
			}
		}
	}
	return fitToLimit(text, field)
}

// fitToLimit shortens single-line copy to its field limit, cutting at a
// sentence end when possible, else at a word, never mid-word.
func fitToLimit(text, field string) string {
	limit, ok := fieldLimits[field]
	text = strings.TrimSpace(text)
	if !ok || len([]rune(text)) <= limit {
		return text
	}
	cut := string([]rune(text)[:limit])
	if field == "meta" {
		if i := strings.LastIndexAny(cut, ".!?"); i >= limit/2 {
			return strings.TrimSpace(cut[:i+1])
		}
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	cut = strings.TrimRight(cut, " ,;:-–—|&")
	for _, w := range []string{" and", " or", " for", " with", " to", " of", " the", " a", " in"} {
		cut = strings.TrimSuffix(cut, w)
	}
	cut = strings.TrimRight(cut, " ,;:-–—|&")
	if field == "meta" && !strings.HasSuffix(cut, ".") {
		cut += "."
	}
	return cut
}

// copyProblem says why new copy cannot be pasted, or "" when it is fine.
func copyProblem(after, before, field string, page *db.Page) string {
	after = strings.TrimSpace(after)
	if after == "" {
		return "the after field is empty"
	}
	norm := func(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
	if before != "" && norm(after) == norm(before) {
		return "after is identical to the current text"
	}
	if strings.ContainsAny(after, "←→") {
		return "after contains navigation text copied from the page"
	}
	if limit, ok := fieldLimits[field]; ok {
		if len([]rune(after)) > limit {
			return fmt.Sprintf("the %s is %d characters; keep it under %d", field, len([]rune(after)), limit)
		}
		if strings.Contains(after, "\n") {
			return fmt.Sprintf("the %s must be a single line", field)
		}
	}
	if words := strings.Fields(strings.TrimRight(after, ".!?")); len(words) > 0 {
		switch strings.ToLower(words[len(words)-1]) {
		case "a", "an", "the", "and", "or", "for", "from", "with", "to", "of", "in", "on", "one", "your", "our", "&":
			return "the copy is cut off mid-phrase; end with a complete sentence"
		}
	}
	if field == "title" && len(strings.Fields(after)) < 5 {
		return "the title is a bare keyword list; name the product and what it does"
	}
	lower := strings.ToLower(after)
	for _, f := range []string{"see how it works", "compare options", "get started today", "look no further"} {
		if strings.Contains(lower, f) {
			return fmt.Sprintf("it uses the filler phrase %q", f)
		}
	}
	if page != nil && (field == "body" || field == "faq") {
		body := norm(page.BodyText)
		lines := strings.Split(after, "\n")
		copied := 0
		for _, l := range lines {
			if l = norm(l); len(l) > 20 && strings.Contains(body, l) {
				copied++
			}
		}
		if len(lines) > 0 && copied*2 >= len(lines) {
			return "most of it is the existing page text pasted back; write new copy"
		}
		if len(lines) > 10 {
			return "it is too long; write one short paragraph"
		}
	}
	return ""
}

type recMeta struct {
	PageURL     string
	TargetField string
	Before      string
	After       string
	Origin      string
}

func resolveTargetPage(projectID uint, pageURL, query string) *db.Page {
	if query != "" && (pageURL == "" || crawler.IsBoilerplatePage(pageURL)) {
		return crawler.BestPageFor(projectID, query)
	}
	if p := crawler.FindPage(projectID, pageURL); p != nil {
		return p
	}
	if query != "" {
		return crawler.BestPageFor(projectID, query)
	}
	return crawler.HomePage(projectID)
}

func parseRecData(raw string) recMeta {
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &m)
	get := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	return recMeta{
		PageURL:     get("page_url"),
		TargetField: get("target_field"),
		Before:      get("before"),
		After:       get("after"),
		Origin:      get("origin"),
	}
}

func loadPage(projectID uint, pageURL string) *db.Page {
	if p := crawler.FindPage(projectID, pageURL); p != nil {
		return p
	}
	return crawler.HomePage(projectID)
}

func currentTextForField(page *db.Page, field string) string {
	if page == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "title":
		if page.Title != nil {
			return strings.TrimSpace(*page.Title)
		}
	case "meta":
		if page.MetaDescription != nil {
			return strings.TrimSpace(*page.MetaDescription)
		}
	case "h1":
		var h1s []string
		_ = json.Unmarshal([]byte(page.H1), &h1s)
		if len(h1s) > 0 {
			return strings.TrimSpace(h1s[0])
		}
	case "faq":
		if strings.TrimSpace(page.FaqJson) != "" && page.FaqJson != "[]" {
			return truncate(page.FaqJson, 900)
		}
	case "body", "content", "general", "":
		return crawler.ReadableExcerpt(page.BodyText, 220)
	}
	if excerpt := crawler.ReadableExcerpt(page.BodyText, 220); excerpt != "" {
		return excerpt
	}
	if page.Title != nil && strings.TrimSpace(*page.Title) != "" {
		return strings.TrimSpace(*page.Title)
	}
	return ""
}

func formatPageContext(page *db.Page, meta recMeta) string {
	if page == nil {
		return "No crawled page matched. Use recommendation context only."
	}
	return fmt.Sprintf("URL: %s\nType: %s\n%s\n\nHint field: %s\nHint current text: %s",
		page.URL, page.PageType, crawler.PageOutline(page, 5000), meta.TargetField, meta.Before)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}


func fixTypeFor(field, source string) string {
	switch strings.ToLower(field) {
	case "title", "meta":
		return "meta"
	case "schema":
		return "schema"
	case "faq":
		return "faq"
	case "body", "h1":
		return "content"
	}
	switch source {
	case "geo_intelligence":
		return "geo"
	case "technical_seo":
		return "meta"
	default:
		return "content"
	}
}

func displayField(field string) string {
	switch strings.ToLower(field) {
	case "title":
		return "Title"
	case "h1":
		return "Headline"
	case "meta":
		return "Meta"
	case "body":
		return "Copy"
	case "faq":
		return "FAQ"
	case "schema":
		return "Schema"
	default:
		return "Fix"
	}
}

// queryFromRec finds which of the project's real buyer searches this
// recommendation is about, so the rival page ranking for it can be shown.
func queryFromRec(rec db.Recommendation) string {
	title := strings.TrimSpace(rec.Title)
	for _, prefix := range []string{"Rank for: ", "Rank for:", "Missing: "} {
		if strings.HasPrefix(title, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	var keywords []string
	db.DB.Model(&db.SearchIntent{}).Where("project_id = ?", rec.ProjectID).Pluck("keyword", &keywords)
	text := strings.ToLower(rec.Title + " " + rec.Detail + " " + rec.Action)
	best := ""
	for _, k := range keywords {
		if k = strings.TrimSpace(k); k != "" && strings.Contains(text, strings.ToLower(k)) && len(k) > len(best) {
			best = k
		}
	}
	return best
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
