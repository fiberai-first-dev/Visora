package metrics

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"visora-backend/internal/db"
)

// Severity weights for the SEO score penalty. A page that fails a critical rule
// costs far more than a page with a cosmetic notice.
var severityWeights = map[string]float64{
	"critical": 45,
	"warning":  25,
	"notice":   10,
}

// Result is the computed snapshot, returned so callers can emit events or logs.
type Result struct {
	SeoScore      *float32
	GeoScore      *float32
	MentionRate   *float32
	CitationRate  *float32
	CompetitorSov *float32
	PagesAnalyzed int
	IssuesBySev   map[string]int
	AffectedPages map[string]int
}

// Compute derives and persists a fresh ProjectMetric snapshot for a project.
//
// Every number comes from stored evidence (crawl pages, SEO issues, SERP results
// and GEO mentions). When a data source is missing the corresponding metric is
// stored as NULL rather than guessed, so the UI can show "not measured yet"
// instead of an invented score.
func Compute(projectID uint) (*Result, error) {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return nil, err
	}

	result := &Result{
		IssuesBySev:   map[string]int{},
		AffectedPages: map[string]int{},
	}
	breakdown := map[string]interface{}{}

	// ---- SEO score from the latest brand crawl ----
	var latestCrawl db.CrawlRun
	hasCrawl := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", projectID).
		Order("id desc").First(&latestCrawl).Error == nil

	if hasCrawl {
		seoScore, seoBreakdown := computeSeoScore(projectID, latestCrawl)
		result.SeoScore = seoScore
		for k, v := range seoBreakdown {
			breakdown[k] = v
		}
		if v, ok := seoBreakdown["issues_by_severity"].(map[string]int); ok {
			result.IssuesBySev = v
		}
		if v, ok := seoBreakdown["affected_pages"].(map[string]int); ok {
			result.AffectedPages = v
		}
		if v, ok := seoBreakdown["pages_analyzed"].(int); ok {
			result.PagesAnalyzed = v
		}
	}

	// ---- GEO score from the latest completed GEO run ----
	geoMetrics := computeGeoMetrics(projectID)
	result.GeoScore = geoMetrics.GeoScore
	result.MentionRate = geoMetrics.MentionRate
	result.CitationRate = geoMetrics.CitationRate
	result.CompetitorSov = geoMetrics.CompetitorSov
	for k, v := range geoMetrics.Breakdown {
		breakdown[k] = v
	}

	breakdownJSON, err := json.Marshal(breakdown)
	if err != nil {
		breakdownJSON = []byte("{}")
	}

	previous := latestMetric(projectID)

	metric := db.ProjectMetric{
		ProjectID:     projectID,
		SeoScore:      result.SeoScore,
		GeoScore:      result.GeoScore,
		MentionRate:   result.MentionRate,
		CitationRate:  result.CitationRate,
		CompetitorSov: result.CompetitorSov,
		ComputedAt:    time.Now(),
		Breakdown:     string(breakdownJSON),
	}
	if err := db.DB.Create(&metric).Error; err != nil {
		return result, fmt.Errorf("failed to store project metric: %w", err)
	}

	writeHistory(projectID, result.SeoScore)
	emitMonitorEvents(project, previous, result)

	fmt.Printf("metrics: project %d — seo=%.1f geo=%.1f mention=%.2f citation=%.2f (pages=%d)\n",
		projectID, deref(result.SeoScore), deref(result.GeoScore), deref(result.MentionRate),
		deref(result.CitationRate), result.PagesAnalyzed)

	return result, nil
}

// latestMetric returns the most recent snapshot, if any.
func latestMetric(projectID uint) *db.ProjectMetric {
	var metric db.ProjectMetric
	if err := db.DB.Where("project_id = ?", projectID).Order("id desc").First(&metric).Error; err != nil {
		return nil
	}
	return &metric
}

// writeHistory appends the improvement-tracking row used by the reports page.
func writeHistory(projectID uint, seoScore *float32) {
	var keywordsRanking, topTen, competitors int64

	db.DB.Model(&db.SerpResult{}).
		Where("project_id = ? AND is_own_domain = ?", projectID, true).
		Count(&keywordsRanking)
	db.DB.Model(&db.SerpResult{}).
		Where("project_id = ? AND is_own_domain = ? AND position <= 10", projectID, true).
		Count(&topTen)
	db.DB.Model(&db.CompetitorCandidate{}).
		Where("project_id = ?", projectID).
		Count(&competitors)

	db.DB.Create(&db.ProjectMetricsHistory{
		ProjectID:       projectID,
		SeoScore:        seoScore,
		KeywordsRanking: int(keywordsRanking),
		TopTenCount:     int(topTen),
		CompetitorCount: int(competitors),
		ComputedAt:      time.Now(),
	})
}

// emitMonitorEvents records only genuine changes between two snapshots.
func emitMonitorEvents(project db.Project, previous *db.ProjectMetric, current *Result) {
	if previous == nil {
		db.DB.Create(&db.MonitorEvent{
			ProjectID: project.ID,
			EventType: "baseline",
			Title:     "Baseline measurement captured",
			Detail: jsonString(map[string]interface{}{
				"seo_score":      current.SeoScore,
				"geo_score":      current.GeoScore,
				"pages_analyzed": current.PagesAnalyzed,
			}),
			Severity: "notice",
		})
		return
	}

	if previous.SeoScore != nil && current.SeoScore != nil {
		delta := *current.SeoScore - *previous.SeoScore
		if delta <= -5 {
			db.DB.Create(&db.MonitorEvent{
				ProjectID: project.ID,
				EventType: "technical_regression",
				Title:     fmt.Sprintf("Technical SEO score dropped %.0f points", -delta),
				Detail: jsonString(map[string]interface{}{
					"from": *previous.SeoScore, "to": *current.SeoScore,
					"issues_by_severity": current.IssuesBySev,
				}),
				Severity: "warning",
			})
		} else if delta >= 5 {
			db.DB.Create(&db.MonitorEvent{
				ProjectID: project.ID,
				EventType: "technical_improvement",
				Title:     fmt.Sprintf("Technical SEO score improved %.0f points", delta),
				Detail: jsonString(map[string]interface{}{
					"from": *previous.SeoScore, "to": *current.SeoScore,
				}),
				Severity: "notice",
			})
		}
	}

	if previous.MentionRate != nil && current.MentionRate != nil {
		delta := *current.MentionRate - *previous.MentionRate
		if delta <= -0.1 || delta >= 0.1 {
			direction := "up"
			severity := "notice"
			if delta < 0 {
				direction = "down"
				severity = "warning"
			}
			db.DB.Create(&db.MonitorEvent{
				ProjectID: project.ID,
				EventType: "ranking_change",
				Title:     fmt.Sprintf("AI mention rate moved %s %.0f points", direction, abs(delta)*100),
				Detail: jsonString(map[string]interface{}{
					"from": *previous.MentionRate, "to": *current.MentionRate,
				}),
				Severity: severity,
			})
		}
	}
}

func jsonString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func deref(v *float32) float64 {
	if v == nil {
		return -1
	}
	return float64(*v)
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// computeSeoScore turns the real SEO issues of a crawl run into a 0-100 score.
//
// The penalty for a severity level is its weight scaled by the share of analyzed
// pages that are actually affected, so a handful of bad pages on a large site is
// treated far more leniently than the same issue on a small site.
func computeSeoScore(projectID uint, run db.CrawlRun) (*float32, map[string]interface{}) {
	var pagesAnalyzed int64
	db.DB.Model(&db.Page{}).
		Where("crawl_run_id = ? AND competitor_id IS NULL", run.ID).
		Count(&pagesAnalyzed)

	breakdown := map[string]interface{}{
		"crawl_run_id":   run.ID,
		"pages_crawled":  run.PagesCrawled,
		"pages_analyzed": int(pagesAnalyzed),
	}

	if pagesAnalyzed == 0 {
		breakdown["note"] = "no pages analyzed yet"
		return nil, breakdown
	}

	var issues []db.SeoIssue
	db.DB.Where("crawl_run_id = ?", run.ID).Find(&issues)

	type ruleStat struct {
		RuleID   string `json:"rule_id"`
		Title    string `json:"title"`
		Severity string `json:"severity"`
		Pages    int    `json:"pages"`
	}

	issuesBySeverity := map[string]int{}
	affectedBySeverity := map[string]map[string]bool{
		"critical": {}, "warning": {}, "notice": {},
	}
	rulePages := map[string]map[string]bool{}
	ruleMeta := map[string]ruleStat{}

	for _, issue := range issues {
		sev := issue.Severity
		if _, known := severityWeights[sev]; !known {
			sev = "notice"
		}
		issuesBySeverity[sev]++
		key := ""
		if issue.URL != nil {
			key = *issue.URL
		} else if issue.PageID != nil {
			key = fmt.Sprintf("page:%d", *issue.PageID)
		}
		if key == "" {
			key = fmt.Sprintf("issue:%d", issue.ID)
		}
		affectedBySeverity[sev][key] = true

		if rulePages[issue.RuleID] == nil {
			rulePages[issue.RuleID] = map[string]bool{}
		}
		rulePages[issue.RuleID][key] = true
		if _, seen := ruleMeta[issue.RuleID]; !seen {
			ruleMeta[issue.RuleID] = ruleStat{
				RuleID:   issue.RuleID,
				Title:    issue.Title,
				Severity: sev,
			}
		}
	}

	penalty := 0.0
	penaltyBySeverity := map[string]float64{}
	affectedCounts := map[string]int{}
	for sev, weight := range severityWeights {
		total := len(affectedBySeverity[sev])
		affectedCounts[sev] = total
		rate := float64(total) / float64(pagesAnalyzed)
		if rate > 1 {
			rate = 1
		}
		penaltyBySeverity[sev] = weight * rate
		penalty += penaltyBySeverity[sev]
	}

	score := 100 - penalty
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	score32 := float32(score)

	worst := make([]ruleStat, 0, len(ruleMeta))
	for ruleID, meta := range ruleMeta {
		meta.Pages = len(rulePages[ruleID])
		worst = append(worst, meta)
	}
	sort.Slice(worst, func(i, j int) bool {
		if worst[i].Pages != worst[j].Pages {
			return worst[i].Pages > worst[j].Pages
		}
		return worst[i].RuleID < worst[j].RuleID
	})
	if len(worst) > 8 {
		worst = worst[:8]
	}

	breakdown["total_issues"] = len(issues)
	breakdown["issues_by_severity"] = issuesBySeverity
	breakdown["affected_pages"] = affectedCounts
	breakdown["penalty_by_severity"] = penaltyBySeverity
	breakdown["top_issues"] = worst

	return &score32, breakdown
}

// geoScoreResult holds the GEO metrics derived from a completed GEO run.
type geoScoreResult struct {
	GeoScore      *float32
	MentionRate   *float32
	CitationRate  *float32
	CompetitorSov *float32
	Breakdown     map[string]interface{}
}

// computeGeoMetrics aggregates the latest GEO run. Every value stays NULL when
// there is no GEO evidence to measure.
func computeGeoMetrics(projectID uint) geoScoreResult {
	out := geoScoreResult{Breakdown: map[string]interface{}{}}

	var run db.GeoRun
	if err := db.DB.Where("project_id = ? AND status = 'done'", projectID).Order("id desc").First(&run).Error; err != nil {
		out.Breakdown["geo_run_id"] = nil
		out.Breakdown["note"] = "no completed GEO run"
		return out
	}

	var responses int64
	db.DB.Model(&db.GeoResponse{}).Where("run_id = ?", run.ID).Count(&responses)

	out.Breakdown["geo_run_id"] = run.ID
	out.Breakdown["geo_responses"] = int(responses)
	out.Breakdown["geo_models"] = run.Model

	if responses == 0 {
		out.Breakdown["note"] = "GEO run produced no responses"
		return out
	}

	var targetMentions int64
	db.DB.Model(&db.GeoMention{}).
		Where("run_id = ? AND is_target = ? AND mentioned = ?", run.ID, true, true).
		Count(&targetMentions)

	var competitorMentions int64
	db.DB.Model(&db.GeoMention{}).
		Where("run_id = ? AND is_target = ? AND mentioned = ?", run.ID, false, true).
		Count(&competitorMentions)

	var citations, brandCitations int64
	db.DB.Model(&db.GeoCitation{}).Where("run_id = ?", run.ID).Count(&citations)
	db.DB.Model(&db.GeoCitation{}).Where("run_id = ? AND is_brand_domain = ?", run.ID, true).Count(&brandCitations)

	mentionRate := float32(targetMentions) / float32(responses)
	citationRate := float32(brandCitations) / float32(responses)
	if citationRate > 1 {
		citationRate = 1
	}

	var competitorSov float32
	total := targetMentions + competitorMentions
	ownSov := float32(0)
	if total > 0 {
		competitorSov = float32(competitorMentions) / float32(total)
		ownSov = float32(targetMentions) / float32(total)
	}

	// No free points when nobody was named — empty GEO stays near zero.
	score := float32(100) * (0.55*mentionRate + 0.25*citationRate + 0.20*ownSov)
	if targetMentions == 0 && competitorMentions == 0 {
		score = 0
	}

	out.MentionRate = &mentionRate
	out.CitationRate = &citationRate
	out.CompetitorSov = &competitorSov
	out.GeoScore = &score

	var competitorBrands []string
	db.DB.Model(&db.GeoMention{}).
		Distinct().Where("run_id = ? AND is_target = ? AND mentioned = ?", run.ID, false, true).
		Limit(10).Pluck("brand", &competitorBrands)

	out.Breakdown["mentions"] = int(targetMentions)
	out.Breakdown["competitor_mentions"] = int(competitorMentions)
	out.Breakdown["competitor_brands"] = competitorBrands
	out.Breakdown["citations"] = int(citations)
	out.Breakdown["brand_citations"] = int(brandCitations)

	return out
}
