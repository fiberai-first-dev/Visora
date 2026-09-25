package api

import (
	"encoding/json"
	"strings"

	"visora-backend/internal/db"
)

// competitorInsights is computed entirely from stored crawl/SERP/GEO evidence.
// Nothing here is estimated or invented: unavailable metrics are returned as null.
type competitorInsights struct {
	KeywordOverlap   *float64 `json:"keyword_overlap"`
	SharedQueryCount int      `json:"shared_query_count"`
	OwnQueryCount    int      `json:"own_query_count"`
	AiVisibility     *float64 `json:"ai_visibility"`
	AiMentions       int      `json:"ai_mentions"`
	AiPromptsChecked int      `json:"ai_prompts_checked"`
	TopSharedTopics  []string `json:"top_shared_topics"`
	TrackedPages     int      `json:"tracked_pages"`
	CrawlStatus      string   `json:"crawl_status"`
	TrackedQueries   int      `json:"tracked_queries"`
}

// buildCompetitorInsights derives real overlap metrics for one competitor.
func buildCompetitorInsights(projectID uint, comp db.CompetitorCandidate) competitorInsights {
	out := competitorInsights{
		CrawlStatus:     comp.CrawlStatus,
		TopSharedTopics: []string{},
	}

	// 1. Keyword overlap, measured against the brand's own ranking queries.
	var ownQueries []string
	db.DB.Model(&db.SerpResult{}).
		Where("project_id = ? AND is_own_domain = ?", projectID, true).
		Distinct().Pluck("query", &ownQueries)
	out.OwnQueryCount = len(ownQueries)

	ownSet := make(map[string]bool, len(ownQueries))
	for _, q := range ownQueries {
		ownSet[strings.ToLower(q)] = true
	}

	var competitorQueries []string
	db.DB.Model(&db.SerpResult{}).
		Where("project_id = ? AND domain = ?", projectID, comp.Domain).
		Distinct().Pluck("query", &competitorQueries)
	out.TrackedQueries = len(competitorQueries)

	shared := 0
	for _, q := range competitorQueries {
		if ownSet[strings.ToLower(q)] {
			shared++
		}
	}
	out.SharedQueryCount = shared
	if len(ownQueries) > 0 {
		pct := float64(shared) / float64(len(ownQueries)) * 100
		out.KeywordOverlap = &pct
	}

	// 2. AI visibility, measured from the most recent GEO run.
	var latestGeo db.GeoRun
	if err := db.DB.Where("project_id = ?", projectID).Order("id desc").First(&latestGeo).Error; err == nil {
		out.AiPromptsChecked = latestGeo.PromptsTotal

		brand := strings.TrimSpace(comp.BrandName)
		if brand == "" {
			brand = comp.Domain
		}

		var mentions int64
		db.DB.Model(&db.GeoMention{}).
			Where("run_id = ? AND mentioned = ? AND (brand = ? OR brand = ?)", latestGeo.ID, true, brand, comp.Domain).
			Count(&mentions)
		out.AiMentions = int(mentions)

		if latestGeo.PromptsTotal > 0 {
			pct := float64(mentions) / float64(latestGeo.PromptsTotal) * 100
			if pct > 100 {
				pct = 100
			}
			out.AiVisibility = &pct
		}
	}

	// 3. Shared topics taken from the real content gap clusters.
	var gaps []db.ContentGap
	db.DB.Where("project_id = ?", projectID).Order("id desc").Find(&gaps)
	for _, gap := range gaps {
		var domains []string
		if err := json.Unmarshal([]byte(gap.CompetitorDomains), &domains); err != nil {
			continue
		}
		for _, d := range domains {
			if strings.EqualFold(strings.TrimSpace(d), comp.Domain) {
				out.TopSharedTopics = append(out.TopSharedTopics, gap.Topic)
				break
			}
		}
		if len(out.TopSharedTopics) >= 8 {
			break
		}
	}

	// 4. How much of the competitor site we actually crawled.
	var run db.CrawlRun
	if err := db.DB.Where("project_id = ? AND competitor_id = ? AND status = 'done'", projectID, comp.ID).
		Order("id desc").First(&run).Error; err == nil {
		var pages int64
		db.DB.Model(&db.Page{}).Where("crawl_run_id = ? AND competitor_id = ?", run.ID, comp.ID).Count(&pages)
		out.TrackedPages = int(pages)
	}

	return out
}
