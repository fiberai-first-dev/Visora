package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
	"visora-backend/internal/noise"
)

// GetProjects lists all projects
func GetProjects(c *gin.Context) {
	userEmail := c.GetHeader("X-User-Email")
	if userEmail == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User email required"})
		return
	}

	var projects []db.Project
	if err := db.DB.Where("user_email = ?", userEmail).Order("created_at desc").Find(&projects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch projects"})
		return
	}
	c.JSON(http.StatusOK, projects)
}

// GetProject gets a specific project by ID
func GetProject(c *gin.Context) {
	userEmail := c.GetHeader("X-User-Email")
	if userEmail == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User email required"})
		return
	}

	id := c.Param("id")
	var project db.Project
	if err := db.DB.Where("id = ? AND user_email = ?", id, userEmail).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	c.JSON(http.StatusOK, project)
}

type CreateProjectInput struct {
	Website string `json:"website" binding:"required"`
	// MaxPages and SitemapURL come from the advanced options on the onboarding
	// screen. They are applied to the first crawl of the project.
	MaxPages   int    `json:"max_pages"`
	SitemapURL string `json:"sitemap_url"`
}

// defaultMaxPages and maxAllowedPages bound the crawl budget a user can request.
const (
	defaultMaxPages = 50
	maxAllowedPages = 500
)

// CreateProject creates a new project and queues its first full scan.
func CreateProject(c *gin.Context) {
	var input CreateProjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userEmail := c.GetHeader("X-User-Email")
	if userEmail == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User email required"})
		return
	}

	// Check if user already has a project
	var count int64
	db.DB.Model(&db.Project{}).Where("user_email = ?", userEmail).Count(&count)
	if count > 0 {
		var existing db.Project
		db.DB.Where("user_email = ?", userEmail).First(&existing)
		// If they already have a project, we can just return it so they get redirected to it
		c.JSON(http.StatusOK, existing)
		return
	}

	maxPages := input.MaxPages
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	if maxPages > maxAllowedPages {
		maxPages = maxAllowedPages
	}

	project := db.Project{
		Name:        "Pending Analysis",
		Brand:       "Pending",
		Website:     input.Website,
		Category:    "Unknown",
		Country:     "India",
		UserEmail:   userEmail,
		Competitors: "[]",
		ProfileData: "{}",
	}

	if err := db.DB.Create(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	run := db.CrawlRun{
		ProjectID: project.ID,
		Status:    "running",
		Trigger:   "full_scan",
		MaxPages:  maxPages,
		Stats:     "{}",
	}
	db.DB.Create(&run)

	payload, _ := json.Marshal(map[string]interface{}{
		"project_id":   project.ID,
		"crawl_run_id": run.ID,
		"start_url":    project.Website,
		"max_pages":    maxPages,
		"sitemap_url":  input.SitemapURL,
	})

	db.DB.Create(&db.Job{
		Type:    "crawl",
		Payload: string(payload),
		Status:  "queued",
	})

	c.JSON(http.StatusCreated, gin.H{
		"id":      project.PublicID,
		"project": project,
	})
}

// GetPages fetches crawled pages for a project (the brand's own site only)
func GetPages(c *gin.Context) {
	id := c.Param("id")

	var run db.CrawlRun
	if err := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", id).Order("id desc").First(&run).Error; err != nil {
		c.JSON(http.StatusOK, []db.Page{})
		return
	}

	var pages []db.Page
	db.DB.Where("project_id = ? AND crawl_run_id = ? AND competitor_id IS NULL", id, run.ID).Order("depth asc, id asc").Limit(100).Find(&pages)
	c.JSON(http.StatusOK, pages)
}

// GetPageDetail fetches a specific page and its related issues
func GetPageDetail(c *gin.Context) {
	pageId := c.Param("pageId")
	
	var page db.Page
	if err := db.DB.First(&page, pageId).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Page not found"})
		return
	}
	
	var issues []db.SeoIssue
	db.DB.Where("project_id = ? AND url = ?", page.ProjectID, page.URL).Find(&issues)
	
	c.JSON(http.StatusOK, gin.H{
		"page": page,
		"issues": issues,
	})
}

// GetIssues fetches SEO issues for a project's own site
func GetIssues(c *gin.Context) {
	id := c.Param("id")

	var run db.CrawlRun
	if err := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", id).Order("id desc").First(&run).Error; err != nil {
		c.JSON(http.StatusOK, []db.SeoIssue{})
		return
	}

	var issues []db.SeoIssue
	db.DB.Where("project_id = ? AND crawl_run_id = ?", id, run.ID).Order("id desc").Limit(500).Find(&issues)
	c.JSON(http.StatusOK, issues)
}

// GetRecommendations fetches actionable recommendations for a project
func GetRecommendations(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var recs []db.Recommendation
	db.DB.Where("project_id = ?", id).Order("created_at desc").Limit(50).Find(&recs)
	c.JSON(http.StatusOK, recs)
}

// GetCompetitors fetches auto-discovered competitors for a project
func GetCompetitors(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var candidates []db.CompetitorCandidate
	db.DB.Where("project_id = ?", id).Order("id desc").Find(&candidates)
	
	// Join with relationships
	var rels []db.CompetitorRelationship
	db.DB.Where("project_id = ?", id).Find(&rels)
	
	type CompResponse struct {
		ID               uint   `json:"id"`
		Domain           string `json:"domain"`
		Status           string `json:"status"`
		Type             string `json:"type"`
		BrandName        string `json:"brand_name"`
		Classification   string `json:"classification"`
		Appearances      int    `json:"appearances"`
		BestPosition     *int   `json:"best_position"`
		SharedQueryCount int    `json:"shared_query_count"`
		CrawlStatus      string `json:"crawl_status"`
		PagesCrawled     int    `json:"pages_crawled"`
	}
	
	var res []CompResponse
	for _, cand := range candidates {
		if noise.IsPlatform(cand.Domain) {
			continue
		}
		relType := "unknown"
		for _, r := range rels {
			if r.CompetitorID == cand.ID {
				relType = r.Type
				break
			}
		}
		res = append(res, CompResponse{
			ID:               cand.ID,
			Domain:           cand.Domain,
			Status:           cand.Status,
			Type:             relType,
			BrandName:        cand.BrandName,
			Classification:   cand.Classification,
			Appearances:      cand.Appearances,
			BestPosition:     cand.BestPosition,
			SharedQueryCount: cand.SharedQueryCount,
			CrawlStatus:      cand.CrawlStatus,
			PagesCrawled:     cand.PagesCrawled,
		})
	}

	c.JSON(http.StatusOK, res)
}

// GetCompetitorDetail fetches detailed intelligence for a specific competitor
func GetCompetitorDetail(c *gin.Context) {
	compId := c.Param("compId")
	projectID := c.Param("id")
	
	var comp db.CompetitorCandidate
	if err := db.DB.First(&comp, compId).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Competitor not found"})
		return
	}
	
	var rel db.CompetitorRelationship
	db.DB.Where("competitor_id = ?", compId).First(&rel)

	// All insights come from stored evidence: SERP snapshots, GEO mentions and the
	// competitor's own crawl. Missing evidence is reported as null, never faked.
	insights := buildCompetitorInsights(comp.ProjectID, comp)

	// Quotes and gaps found for this competitor, straight from the content gaps.
	var gaps []db.ContentGap
	db.DB.Where("project_id = ?", comp.ProjectID).Order("id desc").Find(&gaps)

	var gapItems []gin.H
	for _, gap := range gaps {
		var domains []string
		if err := json.Unmarshal([]byte(gap.CompetitorDomains), &domains); err != nil {
			continue
		}
		matched := false
		for _, d := range domains {
			if strings.EqualFold(strings.TrimSpace(d), comp.Domain) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		gapItems = append(gapItems, gin.H{
			"id":        gap.ID,
			"topic":     gap.Topic,
			"intent":    gap.Intent,
			"priority":  gap.Priority,
			"status":    gap.Status,
			"evidence":  jsonRawMessage(gap.EvidenceQueries),
			"yourPages": jsonRawMessage(gap.ExistingRelatedPages),
		})
	}
	if gapItems == nil {
		gapItems = []gin.H{}
	}

	// Real pages we crawled from this competitor.
	var run db.CrawlRun
	var pages []db.Page
	if db.DB.Where("project_id = ? AND competitor_id = ? AND status = 'done'", comp.ProjectID, comp.ID).
		Order("id desc").First(&run).Error == nil {
		db.DB.Where("crawl_run_id = ? AND competitor_id = ?", run.ID, comp.ID).
			Order("depth asc, id asc").Limit(25).Find(&pages)
	}

	c.JSON(http.StatusOK, gin.H{
		"competitor":   comp,
		"relationship": rel,
		"insights":     insights,
		"content_gaps": gapItems,
		"crawl_run":    run,
		"pages":        pages,
		"project_id":   projectID,
	})
}

// GetKeywords fetches keyword opportunities for a project
func GetKeywords(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var keywords []db.Keyword
	db.DB.Where("project_id = ?", id).Order("id desc").Find(&keywords)
	if keywords == nil {
		keywords = []db.Keyword{}
	}

	c.JSON(http.StatusOK, keywords)
}

// GetLiveStatus returns highly detailed real-time data for the cinematic loading UI
func GetLiveStatus(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// 1. Crawl Run Data
	var crawlRun db.CrawlRun
	db.DB.Where("project_id = ?", id).Order("id desc").First(&crawlRun)

	var latestPages []db.Page
	if crawlRun.ID > 0 {
		db.DB.Where("crawl_run_id = ?", crawlRun.ID).Order("id desc").Limit(5).Find(&latestPages)
	}

	var latestIssues []db.SeoIssue
	if crawlRun.ID > 0 {
		db.DB.Where("crawl_run_id = ?", crawlRun.ID).Order("id desc").Limit(3).Find(&latestIssues)
	}

	// 2. Active Jobs — cast project_id so numeric JSON matches the path param.
	var activeJobs []db.Job
	db.DB.Where("(payload->>'project_id')::int = ? AND status IN ('queued', 'running')", id).Find(&activeJobs)

	// Recent failures help the scan UI explain a paused pipeline.
	var recentErrors []db.Job
	db.DB.Where("(payload->>'project_id')::int = ? AND status = 'error'", id).
		Order("id desc").Limit(5).Find(&recentErrors)

	errorSummaries := make([]gin.H, 0, len(recentErrors))
	for _, job := range recentErrors {
		msg := ""
		if job.Error != nil {
			msg = *job.Error
		}
		errorSummaries = append(errorSummaries, gin.H{
			"id":      job.ID,
			"type":    job.Type,
			"status":  job.Status,
			"error":   msg,
			"finished": job.FinishedAt,
		})
	}

	// 3. GEO Run Data
	var geoRun db.GeoRun
	db.DB.Where("project_id = ?", id).Order("id desc").First(&geoRun)

	var latestGeoResponses []db.GeoResponse
	if geoRun.ID > 0 {
		db.DB.Where("run_id = ?", geoRun.ID).Order("id desc").Limit(3).Find(&latestGeoResponses)
	}

	c.JSON(http.StatusOK, gin.H{
		"project":              project,
		"active_jobs":          activeJobs,
		"recent_errors":        errorSummaries,
		"crawl_run":            crawlRun,
		"latest_pages":         latestPages,
		"latest_issues":        latestIssues,
		"geo_run":              geoRun,
		"latest_geo_responses": latestGeoResponses,
	})
}
