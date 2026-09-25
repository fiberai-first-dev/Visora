package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/competitor"
	"visora-backend/internal/db"
	"visora-backend/internal/seo"
)

// GetSeoAudit returns the technical audit of the latest crawl: the stored score,
// its breakdown, and the issues grouped per rule with real affected-page counts.
func GetSeoAudit(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	groups, runID, err := seo.SummarizeIssues(project.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to summarise issues"})
		return
	}

	bySeverity := map[string]int{"critical": 0, "warning": 0, "notice": 0}
	affectedTotal := 0
	for _, g := range groups {
		bySeverity[g.Severity]++
		affectedTotal += g.AffectedPages
	}

	var run db.CrawlRun
	if runID > 0 {
		db.DB.First(&run, runID)
	}

	payload := gin.H{
		"project_id":      project.ID,
		"score":           nil,
		"breakdown":       nil,
		"computed_at":     nil,
		"avg_response_ms": nil,
		"issues":          groups,
		"crawl_run":       run,
		"totals": gin.H{
			"issue_types":    len(groups),
			"affected_pages": affectedTotal,
			"by_severity":    bySeverity,
		},
	}

	if run.ID > 0 {
		var avgMs float64
		db.DB.Model(&db.Page{}).
			Where("crawl_run_id = ? AND response_ms > 0", run.ID).
			Select("COALESCE(AVG(response_ms), 0)").
			Scan(&avgMs)
		if avgMs > 0 {
			payload["avg_response_ms"] = int(avgMs)
		}
	}

	var metric db.ProjectMetric
	if err := db.DB.Where("project_id = ? AND seo_score IS NOT NULL", id).Order("id desc").First(&metric).Error; err == nil {
		payload["score"] = metric.SeoScore
		payload["breakdown"] = jsonRawMessage(metric.Breakdown)
		payload["computed_at"] = metric.ComputedAt
	}

	c.JSON(http.StatusOK, payload)
}

// GetIssueDetail returns one issue with the real pages it affects.
func GetIssueDetail(c *gin.Context) {
	issueID := c.Param("issueId")

	var issue db.SeoIssue
	if err := db.DB.First(&issue, issueID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Issue not found"})
		return
	}

	var related []db.SeoIssue
	db.DB.Where("crawl_run_id = ? AND rule_id = ?", issue.CrawlRunID, issue.RuleID).Find(&related)

	urls := make([]string, 0, len(related))
	seen := map[string]bool{}
	for _, r := range related {
		if r.URL == nil || seen[*r.URL] {
			continue
		}
		seen[*r.URL] = true
		urls = append(urls, *r.URL)
	}

	// Page-level evidence for the affected URL, when we have it.
	var page db.Page
	hasPage := false
	if issue.URL != nil {
		hasPage = db.DB.Where("project_id = ? AND url = ?", issue.ProjectID, *issue.URL).
			Order("id desc").First(&page).Error == nil
	}

	c.JSON(http.StatusOK, gin.H{
		"issue":         issue,
		"affected_urls": urls,
		"affected_count": len(urls),
		"page":          page,
		"has_page":      hasPage,
	})
}

// GetSearchIntentDetail returns the real Google SERP snapshot for one query.
func GetSearchIntentDetail(c *gin.Context) {
	intentID := c.Param("intentId")

	var intent db.SearchIntent
	if err := db.DB.First(&intent, intentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Search intent not found"})
		return
	}

	var results []db.SerpResult
	db.DB.Where("intent_id = ?", intent.ID).Order("position asc").Find(&results)
	if results == nil {
		results = []db.SerpResult{}
	}

	var ownPosition *int
	for _, r := range results {
		if r.IsOwnDomain {
			pos := r.Position
			ownPosition = &pos
			break
		}
	}

	var gap db.KeywordGap
	hasGap := db.DB.Where("project_id = ? AND query = ?", intent.ProjectID, intent.Keyword).
		Order("id desc").First(&gap).Error == nil

	payload := gin.H{
		"intent":       intent,
		"query":        intent.Keyword,
		"own_position": ownPosition,
		"results":      results,
	}
	if hasGap {
		payload["keyword_gap"] = gap
	}

	c.JSON(http.StatusOK, payload)
}

// GetSeoComparison compares the brand's site against every crawled competitor.
func GetSeoComparison(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	comparisons, err := competitor.CompareTechnical(project.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compare sites"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":  project.ID,
		"comparisons": comparisons,
	})
}

// UpdateProjectInput holds the editable project fields.
type UpdateProjectInput struct {
	Name        *string `json:"name"`
	Brand       *string `json:"brand"`
	Website     *string `json:"website"`
	Category    *string `json:"category"`
	Country     *string `json:"country"`
	Competitors *string `json:"competitors"`
}

// UpdateProject persists edits made in the settings screen. GORM is handed a map
// so explicit empty values are stored instead of being skipped.
func UpdateProject(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var input UpdateProjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.Brand != nil {
		updates["brand"] = *input.Brand
	}
	if input.Website != nil {
		updates["website"] = *input.Website
	}
	if input.Category != nil {
		updates["category"] = *input.Category
	}
	if input.Country != nil {
		updates["country"] = *input.Country
	}
	if input.Competitors != nil {
		updates["competitors"] = *input.Competitors
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nothing to update"})
		return
	}

	if err := db.DB.Model(&db.Project{}).Where("id = ?", project.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
		return
	}

	db.DB.First(&project, project.ID)
	c.JSON(http.StatusOK, project)
}

// UpdateRecommendationStatusInput is the new status for a recommendation.
type UpdateRecommendationStatusInput struct {
	Status string `json:"status" binding:"required,oneof=open dismissed done"`
}

// UpdateRecommendationStatus lets the UI resolve or dismiss a recommendation.
func UpdateRecommendationStatus(c *gin.Context) {
	recID := c.Param("recId")

	var rec db.Recommendation
	if err := db.DB.First(&rec, recID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Recommendation not found"})
		return
	}

	var input UpdateRecommendationStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.DB.Model(&db.Recommendation{}).Where("id = ?", rec.ID).
		Update("status", input.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update recommendation"})
		return
	}

	db.DB.First(&rec, rec.ID)
	c.JSON(http.StatusOK, rec)
}

// performanceTotals is the aggregate measured timing for one crawl run.
type performanceTotals struct {
	Pages            int     `gorm:"column:pages" json:"pages"`
	AvgMs            float64 `gorm:"column:avg_ms" json:"avg_ms"`
	MedianMs         float64 `gorm:"column:median_ms" json:"median_ms"`
	P90Ms            float64 `gorm:"column:p90_ms" json:"p90_ms"`
	MaxMs            int     `gorm:"column:max_ms" json:"max_ms"`
	TotalBytes       int64   `gorm:"column:total_bytes" json:"total_bytes"`
	Images           int     `gorm:"column:images" json:"images"`
	ImagesMissingAlt int     `gorm:"column:images_missing_alt" json:"images_missing_alt"`
	BrokenPages      int     `gorm:"column:broken_pages" json:"broken_pages"`
}

const performanceTotalsSQL = `
SELECT
  COUNT(*)                                                                        AS pages,
  COALESCE(AVG(NULLIF(response_ms, 0)), 0)                                        AS avg_ms,
  COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY response_ms), 0)            AS median_ms,
  COALESCE(percentile_cont(0.9) WITHIN GROUP (ORDER BY response_ms), 0)            AS p90_ms,
  COALESCE(MAX(response_ms), 0)                                                    AS max_ms,
  COALESCE(SUM(response_bytes), 0)                                                 AS total_bytes,
  COALESCE(SUM(images), 0)                                                         AS images,
  COALESCE(SUM(images_missing_alt), 0)                                             AS images_missing_alt,
  COALESCE(SUM(CASE WHEN status IS NOT NULL AND status >= 400 THEN 1 ELSE 0 END), 0) AS broken_pages
FROM pages
WHERE crawl_run_id = ?`

// GetPerformance reports the response times actually measured while crawling.
//
// Core Web Vitals (LCP, CLS, INP) require a browser based audit, so they are
// explicitly reported as not measured rather than estimated.
func GetPerformance(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var run db.CrawlRun
	if err := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", project.ID).
		Order("id desc").First(&run).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"project_id":      project.ID,
			"crawl_run":       nil,
			"totals":          performanceTotals{},
			"by_page_type":    []gin.H{},
			"slowest_pages":   []gin.H{},
			"crawl_started_at": nil,
			"crawl_finished_at": nil,
			"measurement_note": "Run a scan to measure real response times.",
		})
		return
	}

	var totals performanceTotals
	db.DB.Raw(performanceTotalsSQL, run.ID).Scan(&totals)

	var byType []struct {
		PageType   string  `gorm:"column:page_type"`
		Pages      int     `gorm:"column:pages"`
		AvgMs      float64 `gorm:"column:avg_ms"`
		MaxMs      int     `gorm:"column:max_ms"`
		TotalBytes int64   `gorm:"column:total_bytes"`
	}
	db.DB.Raw(`SELECT page_type, COUNT(*) AS pages, COALESCE(AVG(NULLIF(response_ms,0)),0) AS avg_ms,
	                  COALESCE(MAX(response_ms),0) AS max_ms, COALESCE(SUM(response_bytes),0) AS total_bytes
	           FROM pages WHERE crawl_run_id = ? GROUP BY page_type ORDER BY avg_ms DESC`, run.ID).Scan(&byType)

	byTypePayload := make([]gin.H, 0, len(byType))
	for _, row := range byType {
		byTypePayload = append(byTypePayload, gin.H{
			"page_type":   row.PageType,
			"pages":       row.Pages,
			"avg_ms":      row.AvgMs,
			"max_ms":      row.MaxMs,
			"total_bytes": row.TotalBytes,
		})
	}

	var slowest []struct {
		URL           string `gorm:"column:url"`
		PageType      string `gorm:"column:page_type"`
		Status        *int   `gorm:"column:status"`
		ResponseMs    int    `gorm:"column:response_ms"`
		ResponseBytes int64  `gorm:"column:response_bytes"`
		Title         string `gorm:"column:title"`
	}
	db.DB.Raw(`SELECT url, page_type, status, response_ms, response_bytes, COALESCE(title,'') AS title
	           FROM pages WHERE crawl_run_id = ? ORDER BY response_ms DESC LIMIT 20`, run.ID).Scan(&slowest)

	slowestPayload := make([]gin.H, 0, len(slowest))
	for _, row := range slowest {
		slowestPayload = append(slowestPayload, gin.H{
			"url":            row.URL,
			"page_type":      row.PageType,
			"status":         row.Status,
			"response_ms":    row.ResponseMs,
			"response_bytes": row.ResponseBytes,
			"title":          row.Title,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":        project.ID,
		"crawl_run":         run,
		"totals":            totals,
		"by_page_type":      byTypePayload,
		"slowest_pages":     slowestPayload,
		"crawl_started_at":  run.StartedAt,
		"crawl_finished_at": run.FinishedAt,
		"measurement_note":  "Response time is measured server-side during the crawl. Core Web Vitals (LCP, CLS, INP) need a browser based audit and are not measured.",
	})
}
