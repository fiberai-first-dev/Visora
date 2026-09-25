package recommendations

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
	"visora-backend/internal/seo"
)

type aiRecommendation struct {
	Source      string `json:"source"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Action      string `json:"action"`
	PageURL     string `json:"page_url"`
	TargetField string `json:"target_field"`
	Before      string `json:"before"`
	After       string `json:"after"`
}

type aiRecList struct {
	Recommendations []aiRecommendation `json:"recommendations"`
	Items           []aiRecommendation `json:"items"`
	Actions         []aiRecommendation `json:"actions"`
	Edits           []aiRecommendation `json:"edits"`
}

func (l aiRecList) all() []aiRecommendation {
	for _, list := range [][]aiRecommendation{l.Recommendations, l.Items, l.Actions, l.Edits} {
		if len(list) > 0 {
			return list
		}
	}
	return nil
}

// parseRecommendations accepts the wrapped list we ask for, a bare array, a
// single item, or several items back to back — models return all of these.
func parseRecommendations(raw string) []aiRecommendation {
	body := strings.TrimSpace(raw)
	if strings.HasPrefix(body, "```") {
		body = strings.TrimSpace(strings.TrimSuffix(strings.TrimLeft(strings.TrimPrefix(body, "```"), "jsonJSON"), "```"))
	}
	if strings.HasPrefix(body, "[") {
		var arr []aiRecommendation
		if json.Unmarshal([]byte(body), &arr) == nil {
			return arr
		}
	}
	var out []aiRecommendation
	for _, obj := range topLevelObjects(body) {
		var list aiRecList
		if json.Unmarshal(obj, &list) == nil && len(list.all()) > 0 {
			out = append(out, list.all()...)
			continue
		}
		var one aiRecommendation
		if json.Unmarshal(obj, &one) == nil && strings.TrimSpace(one.Title) != "" {
			out = append(out, one)
		}
	}
	if len(out) == 0 {
		var list aiRecList
		if json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &list) == nil {
			out = list.all()
		}
	}
	return out
}

// topLevelObjects returns each balanced {...} value in s, skipping whatever
// separates them.
func topLevelObjects(s string) [][]byte {
	var out [][]byte
	depth, start := 0, -1
	inString, escape := false, false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			switch {
			case escape:
				escape = false
			case ch == '\\':
				escape = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			if depth > 0 {
				inString = true
			}
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					out = append(out, []byte(s[start:i+1]))
					start = -1
				}
			}
		}
	}
	return out
}

// Generate asks the LLM to write page-specific recommendations from crawl,
// Google, AI-answer and rival evidence. There is no template fallback: a
// failed call fails the job so the worker retries it.
func Generate(projectID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	events.PublishEvent(projectID, 12, "progress", "Writing ranked actions for your pages", nil)

	brief := buildEvidenceBrief(project)
	origin := "ai"
	recs, err := generateFromLLM(project, brief)
	if err != nil {
		return fmt.Errorf("recommendations AI failed: %w", err)
	}

	db.DB.Where("project_id = ? AND status = ?", projectID, "open").Delete(&db.Recommendation{})

	created := 0
	for _, rec := range recs {
		title := strings.TrimSpace(rec.Title)
		detail := strings.TrimSpace(rec.Detail)
		action := strings.TrimSpace(rec.Action)
		if title == "" || detail == "" || action == "" {
			continue
		}
		pageURL := strings.TrimSpace(rec.PageURL)
		field := normalizeField(rec.TargetField)
		before := strings.TrimSpace(rec.Before)
		page := loadProjectPage(projectID, pageURL)
		if page != nil {
			if pageURL == "" {
				pageURL = page.URL
			}
			if before == "" {
				before = currentFromPage(page, field)
			}
		}
		data, _ := json.Marshal(map[string]interface{}{
			"origin":       origin,
			"page_url":     pageURL,
			"target_field": field,
			"before":       before,
			"after":        strings.TrimSpace(rec.After),
		})
		db.DB.Create(&db.Recommendation{
			ProjectID: projectID,
			Source:    normalizeSource(rec.Source),
			Severity:  normalizeSeverity(rec.Severity),
			Title:     title,
			Detail:    detail,
			Action:    action,
			Status:    "open",
			Data:      string(data),
		})
		created++
	}

	if created == 0 {
		fmt.Printf("recommendations: nothing persisted for project %d\n", projectID)
	}
	return nil
}

func generateFromLLM(project db.Project, brief string) ([]aiRecommendation, error) {
	prompt := fmt.Sprintf(`You are a senior SEO + GEO (AI-search) strategist.
Using ONLY the evidence below, write the edits that will most move this site up on Google and get it named by ChatGPT/Claude/Gemini.

Brand: %s
Website: %s
Category: %s
Country: %s

EVIDENCE:
%s

How to think:
1. Read what the site actually sells from its crawled copy (product names, features, who it is for).
2. For each lost search in HEAD-TO-HEAD, see what the rival page says that ours does not (the words in its title/H1, the use case it answers, proof, FAQs) and close that gap on OUR closest page.
3. For AI answers that named other brands, add the entity facts an AI needs to recommend us: a plain "Brand is a <category> for <audience> that <does X>" line, concrete features, pricing/plans if on the site, comparison vs the brands named, and FAQ answers that mirror the buyer questions.
4. Fix real technical issues only when they hurt a page that matters (never legal/privacy/terms/login pages).

Rules:
- Every item must name its evidence in "detail" (e.g. "rival.com ranks #2 with 'X' in its title; our /page title has no mention of X").
- page_url must be a URL from "Crawled page copy" or "Our closest page". Never invent paths, never legal pages.
- target_field: title | h1 | meta | body | faq | schema.
- before = the exact current text of that field from the evidence.
- after = finished copy to paste, using the site's real offer. title ≤ 60 chars incl. brand, meta ≤ 155 chars, body 2-4 sentences, faq = 2-3 "Q: … A: …" pairs, schema = JSON-LD.
- Banned filler: "see how it works", "compare options", "get started today", "look no further", "one-stop solution".
- No two items may target the same page + field.
- 6-10 items, highest impact first. source is one of technical_seo, search_visibility, content_gap, competitors, geo_intelligence.

Return ONLY JSON:
{"recommendations":[{"source":"search_visibility","severity":"high","title":"...","detail":"...","action":"...","page_url":"...","target_field":"title","before":"...","after":"..."}]}`,
		project.Brand, project.Website, project.Category, project.Country, brief)

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You write evidence-backed SEO/GEO page edits with exact paste-ready wording. Never invent URLs or facts not in the evidence. Reply with one JSON object whose \"recommendations\" key holds 6-10 items — never a single bare item.",
		},
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}
	raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), messages, 0.3)
	if err != nil {
		return nil, err
	}

	got := parseRecommendations(raw)
	recs := cleanRecommendations(project.ID, got)
	fmt.Printf("recommendations: project %d — AI returned %d, kept %d\n", project.ID, len(got), len(recs))

	// Cheap models sometimes return a single item; ask once for the rest.
	if len(recs) < minRecommendations {
		fmt.Printf("recommendations: short reply (brief %d chars, reply %d chars): %s\n", len(brief), len(raw), truncate(raw, 1200))
		messages = append(messages,
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: raw},
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(
				"That is only %d usable item(s). Return the full list: 6-10 items on different page + field pairs, following every rule, as {\"recommendations\":[...]}. Keep the items you already wrote.",
				len(recs))},
		)
		if more, err := llm.CompleteJSON(context.Background(), llm.GetModel(), messages, 0.3); err == nil {
			got = append(got, parseRecommendations(more)...)
			recs = cleanRecommendations(project.ID, got)
			fmt.Printf("recommendations: project %d — after follow-up, kept %d\n", project.ID, len(recs))
		}
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("AI returned no usable recommendations (%d raw; reply starts: %s)", len(got), truncate(raw, 300))
	}
	return recs, nil
}

const minRecommendations = 4

var fillerPhrases = []string{
	"see how it works", "compare options", "get started today", "look no further",
	"one-stop solution", "add a section on this page", "create or strengthen",
}

// cleanRecommendations points every item at a real, non-legal crawled page,
// cuts filler sentences out of the new copy, and drops duplicate page+field
// pairs.
func cleanRecommendations(projectID uint, in []aiRecommendation) []aiRecommendation {
	seen := map[string]bool{}
	out := make([]aiRecommendation, 0, len(in))
	var spare []aiRecommendation
	for i, r := range in {
		r.Title = strings.TrimSpace(r.Title)
		r.PageURL = strings.TrimSpace(r.PageURL)
		if r.Title == "" {
			fmt.Printf("recommendations: drop #%d — no title\n", i)
			continue
		}
		if strings.TrimSpace(r.Action) == "" {
			r.Action = firstNonEmptyStr(r.Detail, r.Title)
		}
		if strings.TrimSpace(r.Detail) == "" {
			r.Detail = r.Action
		}

		if crawler.IsBoilerplatePage(r.PageURL) {
			fmt.Printf("recommendations: drop %q — targets utility page %s\n", r.Title, r.PageURL)
			continue
		}
		moved := false
		page := crawler.FindPage(projectID, r.PageURL)
		if page == nil {
			// The AI named a page we never crawled: its advice was written for
			// that page, so it only stands in on our closest page when nothing
			// else survives.
			page = crawler.BestPageFor(projectID, r.Title+" "+r.Detail)
			r.Before = ""
			moved = true
		}
		if page == nil {
			fmt.Printf("recommendations: drop %q — no crawled page to attach to\n", r.Title)
			continue
		}
		r.PageURL = page.URL
		r.After = stripFiller(r.After)
		if moved {
			spare = append(spare, r)
			continue
		}

		key := r.PageURL + "|" + normalizeField(r.TargetField)
		if seen[key] {
			fmt.Printf("recommendations: drop %q — duplicate %s\n", r.Title, key)
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	if len(out) == 0 {
		for _, r := range spare {
			key := r.PageURL + "|" + normalizeField(r.TargetField)
			if !seen[key] {
				seen[key] = true
				out = append(out, r)
			}
		}
	} else if len(spare) > 0 {
		fmt.Printf("recommendations: dropped %d items aimed at pages we never crawled\n", len(spare))
	}
	return out
}

// stripFiller removes sentences that are generic filler; the rest of the
// copy is kept.
func stripFiller(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for li, line := range lines {
		sentences := strings.SplitAfter(line, ". ")
		kept := sentences[:0]
		for _, sentence := range sentences {
			lower := strings.ToLower(sentence)
			bad := false
			for _, f := range fillerPhrases {
				if strings.Contains(lower, f) {
					bad = true
					break
				}
			}
			if !bad {
				kept = append(kept, sentence)
			}
		}
		lines[li] = strings.TrimSpace(strings.Join(kept, ""))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func buildEvidenceBrief(project db.Project) string {
	var b strings.Builder
	projectID := project.ID

	groups, _, _ := seo.SummarizeIssues(projectID)
	fmt.Fprintf(&b, "SEO issues (with example pages):\n")
	for i, g := range groups {
		if i >= 8 {
			break
		}
		fmt.Fprintf(&b, "- [%s] %s (%d pages) example=%s action=%s\n",
			g.Severity, g.Title, g.AffectedPages, g.ExampleURL, g.Recommendation)
		for _, u := range g.MatchedURLs {
			if u == g.ExampleURL {
				continue
			}
			fmt.Fprintf(&b, "  also: %s\n", u)
			break
		}
	}

	fmt.Fprintf(&b, "\nCrawled page copy (use these URLs and wording; never target legal/privacy pages):\n")
	shown := 0
	for _, p := range crawler.LatestSitePages(projectID, 60) {
		if crawler.IsBoilerplatePage(p.URL) {
			continue
		}
		budget := 700
		if shown == 0 {
			budget = 2500
		}
		fmt.Fprintf(&b, "- URL: %s | type=%s | words=%d | faq=%v\n%s\n",
			p.URL, p.PageType, p.WordCount, p.HasFaq, indent(crawler.PageOutline(&p, budget)))
		shown++
		if shown >= 14 {
			break
		}
	}

	writeHeadToHead(&b, projectID)
	writeAIVisibility(&b, project)

	var missing, lagging []db.KeywordGap
	db.DB.Where("project_id = ? AND gap_type = ?", projectID, "missing").Limit(10).Find(&missing)
	db.DB.Where("project_id = ? AND gap_type = ?", projectID, "lagging").Limit(6).Find(&lagging)
	fmt.Fprintf(&b, "\nKeyword gaps — missing (%d):\n", len(missing))
	for _, g := range missing {
		fmt.Fprintf(&b, "- query=%q rival=%s\n", g.Query, g.BestCompetitor)
	}
	fmt.Fprintf(&b, "Keyword gaps — lagging (%d):\n", len(lagging))
	for _, g := range lagging {
		pos := 0
		if g.BrandPosition != nil {
			pos = *g.BrandPosition
		}
		fmt.Fprintf(&b, "- query=%q brand_pos=%d rival=%s\n", g.Query, pos, g.BestCompetitor)
	}

	var contentGaps []db.ContentGap
	db.DB.Where("project_id = ? AND status = ?", projectID, "open").Order("priority asc").Limit(5).Find(&contentGaps)
	fmt.Fprintf(&b, "\nContent gap topics:\n")
	for _, g := range contentGaps {
		fmt.Fprintf(&b, "- topic=%q intent=%s priority=%s\n", g.Topic, g.Intent, g.Priority)
	}

	var candidates []db.CompetitorCandidate
	db.DB.Where("project_id = ?", projectID).Order("appearances desc").Limit(6).Find(&candidates)
	fmt.Fprintf(&b, "\nCompetitors in SERPs:\n")
	for _, c := range candidates {
		fmt.Fprintf(&b, "- %s appearances=%d\n", c.Domain, c.Appearances)
	}

	var latestGeo db.GeoRun
	if err := db.DB.Where("project_id = ? AND status IN ?", projectID, []string{"done", "error"}).Order("id desc").First(&latestGeo).Error; err == nil {
		var responses []db.GeoResponse
		db.DB.Where("run_id = ?", latestGeo.ID).Find(&responses)
		mentioned := 0
		brand := strings.ToLower(project.Brand)
		for _, r := range responses {
			if brand != "" && strings.Contains(strings.ToLower(r.ResponseText), brand) {
				mentioned++
			}
		}
		fmt.Fprintf(&b, "\nAI visibility: %d/%d answers mentioned %q\n", mentioned, len(responses), project.Brand)
	}

	var intents []db.SearchIntent
	db.DB.Where("project_id = ?", projectID).Order("priority desc").Limit(10).Find(&intents)
	fmt.Fprintf(&b, "\nBuyer search intents:\n")
	for _, intent := range intents {
		fmt.Fprintf(&b, "- %s (%s)\n", intent.Keyword, intent.Intent)
	}

	return b.String()
}

// writeHeadToHead puts, for each lost buyer search, the rival page Google
// ranked next to our closest page — so the LLM compares real copy, not guesses.
func writeHeadToHead(b *strings.Builder, projectID uint) {
	var gaps []db.KeywordGap
	db.DB.Where("project_id = ?", projectID).
		Order("CASE gap_type WHEN 'missing' THEN 0 ELSE 1 END, id asc").
		Limit(6).Find(&gaps)
	if len(gaps) == 0 {
		return
	}
	fmt.Fprintf(b, "\nHEAD-TO-HEAD on lost buyer searches (rival page that ranks vs our closest page):\n")
	for _, g := range gaps {
		fmt.Fprintf(b, "\n### Search %q — we are %s, best rival %s\n", g.Query, positionLabel(g.BrandPosition), g.BestCompetitor)

		if rival := crawler.RivalContext(projectID, g.Query, 900); rival != "" {
			fmt.Fprintf(b, "%s\n", indent(rival))
		}
		if ours := crawler.BestPageFor(projectID, g.Query); ours != nil {
			fmt.Fprintf(b, "Our closest page %s:\n%s\n", ours.URL, indent(crawler.PageOutline(ours, 700)))
		}
	}
}

func positionLabel(p *int) string {
	if p == nil || *p <= 0 {
		return "not in top 50"
	}
	return fmt.Sprintf("#%d", *p)
}

// writeAIVisibility lists what AIs were asked and which brands they named
// instead of ours — the input for GEO fixes.
func writeAIVisibility(b *strings.Builder, project db.Project) {
	var run db.GeoRun
	if err := db.DB.Where("project_id = ? AND status IN ?", project.ID, []string{"done", "error"}).
		Order("id desc").First(&run).Error; err != nil {
		return
	}
	var responses []db.GeoResponse
	db.DB.Where("run_id = ?", run.ID).Find(&responses)
	if len(responses) == 0 {
		return
	}
	promptText := map[uint]string{}
	var prompts []db.GeoPrompt
	db.DB.Where("project_id = ?", project.ID).Find(&prompts)
	for _, p := range prompts {
		promptText[p.ID] = p.Text
	}
	brand := strings.ToLower(strings.TrimSpace(project.Brand))
	fmt.Fprintf(b, "\nAI ANSWERS (GEO):\n")
	for _, r := range responses {
		named := strings.Contains(strings.ToLower(r.ResponseText), brand) && brand != ""
		var others []string
		db.DB.Model(&db.GeoMention{}).
			Where("response_id = ? AND is_target = false AND mentioned = true", r.ID).
			Limit(6).Pluck("brand", &others)
		fmt.Fprintf(b, "- [%s] Q: %q → mentions us: %v; brands named instead: %s\n",
			r.Model, promptText[r.PromptID], named, strings.Join(others, ", "))
	}
	var cites []db.GeoCitation
	db.DB.Where("run_id = ? AND is_brand_domain = false", run.ID).Limit(10).Find(&cites)
	if len(cites) > 0 {
		var domains []string
		for _, c := range cites {
			domains = append(domains, c.Domain)
		}
		fmt.Fprintf(b, "Sources the AIs cited: %s\n", strings.Join(domains, ", "))
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n  ")
}

func loadProjectPage(projectID uint, pageURL string) *db.Page {
	if p := crawler.FindPage(projectID, pageURL); p != nil {
		return p
	}
	return crawler.HomePage(projectID)
}

func currentFromPage(page *db.Page, field string) string {
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
	default:
		return crawler.ReadableExcerpt(page.BodyText, 220)
	}
	return crawler.ReadableExcerpt(page.BodyText, 220)
}

func normalizeSource(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "technical_seo", "search_visibility", "content_gap", "competitors", "geo_intelligence":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "ai_strategy"
	}
}

func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "high", "warning", "notice":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "warning"
	}
}

func normalizeField(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "title", "h1", "meta", "body", "faq", "schema", "links", "general":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "general"
	}
}
