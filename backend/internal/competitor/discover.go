package competitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/db"
	"visora-backend/internal/llm"
)

const (
	maxKeepCandidates = 5
	classifyShortlist = 12
)

type DiscoveredCompetitor struct {
	Domain         string `json:"domain"`
	BrandName      string `json:"brand_name"`
	Classification string `json:"classification"`
	OverlapType    string `json:"overlap_type"`
}

type DiscoveryResult struct {
	Competitors []DiscoveredCompetitor `json:"competitors"`
}

// Discover is a no-op: candidates are the real domains seen in Google, and
// ClassifyAndMerge has the LLM decide which of them actually compete.
func Discover(projectID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}
	fmt.Printf("competitor: skipping LLM discovery for project %d — using SERP candidates\n", projectID)
	return nil
}

// ClassifyAndMerge scores SERP candidates, keeps the strongest ones, drops
// platform noise, lets the LLM decide which are real competitors, and queues
// a shallow crawl of those.
func ClassifyAndMerge(projectID uint) error {
	var candidates []db.CompetitorCandidate
	if err := db.DB.Where("project_id = ?", projectID).Find(&candidates).Error; err != nil {
		return err
	}

	type scored struct {
		candidate     db.CompetitorCandidate
		appearances   int
		sharedQueries int
		bestPosition  int
	}

	scoredList := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if isNoiseDomain(c.Domain) {
			db.DB.Delete(&c)
			continue
		}

		var results []db.SerpResult
		db.DB.Where("project_id = ? AND domain = ?", projectID, c.Domain).Find(&results)

		appearances := len(results)
		bestPosition := 999
		sharedQueries := map[string]bool{}
		for _, r := range results {
			if r.Position < bestPosition {
				bestPosition = r.Position
			}
			sharedQueries[r.Query] = true
		}

		scoredList = append(scoredList, scored{
			candidate:     c,
			appearances:   appearances,
			sharedQueries: len(sharedQueries),
			bestPosition:  bestPosition,
		})
	}

	for i := 0; i < len(scoredList); i++ {
		for j := i + 1; j < len(scoredList); j++ {
			a, b := scoredList[i], scoredList[j]
			if b.appearances > a.appearances ||
				(b.appearances == a.appearances && b.bestPosition < a.bestPosition) {
				scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
			}
		}
	}

	shortlist := scoredList
	if len(shortlist) > classifyShortlist {
		shortlist = shortlist[:classifyShortlist]
	}
	for _, item := range scoredList[len(shortlist):] {
		c := item.candidate
		db.DB.Delete(&c)
	}
	if len(shortlist) == 0 {
		return fmt.Errorf("no competitor candidates found in search results")
	}

	domains := make([]string, 0, len(shortlist))
	for _, item := range shortlist {
		domains = append(domains, item.candidate.Domain)
	}
	verdicts, err := classifyWithLLM(projectID, domains)
	if err != nil {
		return err
	}

	kept := 0
	for _, item := range shortlist {
		c := item.candidate
		v, ok := verdicts[strings.ToLower(c.Domain)]
		if !ok || !v.Competitor || kept >= maxKeepCandidates {
			db.DB.Delete(&c)
			continue
		}
		c.Appearances = item.appearances
		c.SharedQueryCount = item.sharedQueries
		if item.bestPosition != 999 {
			bp := item.bestPosition
			c.BestPosition = &bp
		}
		c.Classification = normalizeClassification(v.Classification)
		c.BrandName = firstNonEmpty(v.BrandName, c.BrandName, c.Domain)
		db.DB.Save(&c)
		kept++
	}
	if kept == 0 {
		return fmt.Errorf("AI found no real competitors among %d search results", len(shortlist))
	}

	fmt.Printf("competitor: AI kept %d of %d candidates for project %d\n", kept, len(candidates), projectID)

	payload, _ := json.Marshal(map[string]interface{}{"project_id": projectID})
	db.DB.Create(&db.Job{
		Type:    "competitor_crawl_run",
		Payload: string(payload),
		Status:  "queued",
	})
	return nil
}

type classifyVerdict struct {
	Domain         string `json:"domain"`
	BrandName      string `json:"brand_name"`
	Classification string `json:"classification"`
	Competitor     bool   `json:"competitor"`
}

// classifyWithLLM decides from real Google results which domains actually
// compete for the same buyers.
func classifyWithLLM(projectID uint, domains []string) (map[string]classifyVerdict, error) {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Our site: %s (%s)\nCategory: %s\n", project.Website, project.Brand, project.Category)
	if strings.TrimSpace(project.ProfileData) != "" {
		fmt.Fprintf(&b, "Our brand profile: %s\n", truncateText(project.ProfileData, 1200))
	}
	b.WriteString("\nDomains seen in Google for our buyer searches:\n")
	for _, d := range domains {
		var rows []db.SerpResult
		db.DB.Where("project_id = ? AND domain = ?", projectID, d).Order("position asc").Limit(3).Find(&rows)
		fmt.Fprintf(&b, "\nDOMAIN %s\n", d)
		for _, r := range rows {
			fmt.Fprintf(&b, "  #%d for %q: %s — %s\n", r.Position, r.Query, r.PageTitle, truncateText(r.PageSnippet, 180))
		}
	}

	raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: "You classify search competitors. A competitor sells something a buyer could choose INSTEAD of our site. " +
				"Directories, forums, Q&A sites, social networks, generic news and dictionary pages are not competitors. Reply with JSON only.",
		},
		{
			Role: openai.ChatMessageRoleUser,
			Content: b.String() + `
Return {"competitors":[{"domain":"","brand_name":"","classification":"direct_d2c|marketplace|retailer|publisher","competitor":true}]}
Include every domain listed. brand_name = the brand as buyers know it.`,
		},
	}, 0.1)
	if err != nil {
		return nil, fmt.Errorf("competitor classification AI call failed: %w", err)
	}

	var parsed struct {
		Competitors []classifyVerdict `json:"competitors"`
	}
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &parsed); err != nil {
		return nil, fmt.Errorf("competitor classification AI reply was not valid JSON: %w", err)
	}
	out := make(map[string]classifyVerdict, len(parsed.Competitors))
	for _, v := range parsed.Competitors {
		key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v.Domain), "www."))
		if key == "" {
			continue
		}
		out[key] = v
		out["www."+key] = v
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("competitor classification AI returned no domains")
	}
	return out, nil
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func normalizeClassification(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, " ", "_")
	switch v {
	case "direct_brand", "direct_d2c", "direct":
		return "direct_d2c"
	case "marketplace", "retailer", "publisher", "search":
		return v
	default:
		if strings.Contains(v, "direct") {
			return "direct_d2c"
		}
		if strings.Contains(v, "market") {
			return "marketplace"
		}
		return "search"
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
