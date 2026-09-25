package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"visora-backend/internal/brand"
	"visora-backend/internal/competitor"
	"visora-backend/internal/contentgap"
	"visora-backend/internal/crawler"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/fixes"
	"visora-backend/internal/geo"
	"visora-backend/internal/metrics"
	"visora-backend/internal/recommendations"
	"visora-backend/internal/search_intent"
	"visora-backend/internal/seo"
	"visora-backend/internal/serp"
)

// Shared job payload fields. JSON tags are required — without them every
// handler receives ProjectID=0 and the pipeline dies after crawl.
type jobPayload struct {
	ProjectID  uint   `json:"project_id"`
	CrawlRunID uint   `json:"crawl_run_id"`
	GeoRunID   uint   `json:"geo_run_id"`
	Origin     string `json:"start_url"`
	MaxPages   int    `json:"max_pages"`
	SitemapURL string `json:"sitemap_url"`
	// Pipeline marks jobs that belong to a full scan (vs a manual GEO re-run).
	Pipeline bool `json:"pipeline"`
}

// Crawls write pages as they go, so a retry would duplicate them.
var noRetryJobs = map[string]bool{
	"crawl":                true,
	"competitor_crawl_run": true,
}

func parsePayload(job db.Job) (jobPayload, error) {
	var p jobPayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return p, fmt.Errorf("invalid job payload: %w", err)
	}
	if p.ProjectID == 0 {
		return p, fmt.Errorf("job payload missing project_id")
	}
	return p, nil
}

func main() {
	fmt.Println("Starting Visora Background Worker...")

	db.Connect()

	stuckAfter := envDurationMinutes("WORKER_STUCK_MINUTES", 25)
	maxAttempts := envInt("WORKER_MAX_ATTEMPTS", 3)

	for {
		requeueStuckJobs(stuckAfter, maxAttempts)

		job, ok := claimNextJob()
		if !ok {
			time.Sleep(2 * time.Second)
			continue
		}

		fmt.Printf("Processing job %d of type %s (attempt %d)\n", job.ID, job.Type, job.Attempts)

		var processErr error
		switch job.Type {
		case "crawl":
			processErr = executeCrawlJob(job)
		case "website_analysis_run":
			processErr = executeWebsiteAnalysisJob(job)
		case "brand_understand_run":
			processErr = executeBrandUnderstandJob(job)
		case "search_intent_run":
			processErr = executeSearchIntentJob(job)
		case "serp_check_run":
			processErr = executeSerpCheckJob(job)
		case "competitor_classify_run":
			processErr = executeCompetitorClassifyJob(job)
		case "competitor_crawl_run":
			processErr = executeCompetitorCrawlJob(job)
		case "competitor_compare_run":
			processErr = executeCompetitorCompareJob(job)
		case "gap_analysis_run":
			processErr = executeGapAnalysisJob(job)
		case "geo_run":
			processErr = executeGeoRunJob(job)
		case "geo_analyze_run":
			processErr = executeGeoAnalyzeJob(job)
		case "prioritize_issues_run":
			processErr = executePrioritizeIssuesJob(job)
		case "recommendations_run":
			processErr = executeRecommendationsJob(job)
		case "fix_generation_run":
			processErr = executeFixGenerationJob(job)
		case "competitor_discovery_run":
			processErr = executeCompetitorDiscoveryJob(job)
		case "seo_comparison_run":
			processErr = executeSeoComparisonJob(job)
		default:
			processErr = fmt.Errorf("unknown job type: %s", job.Type)
		}

		finishedAt := time.Now()
		job.FinishedAt = &finishedAt
		if processErr != nil {
			errMsg := processErr.Error()
			if job.Attempts < maxAttempts && !noRetryJobs[job.Type] {
				// started_at doubles as the retry backoff clock in claimNextJob.
				db.DB.Model(&job).Updates(map[string]interface{}{
					"status":     "queued",
					"error":      errMsg,
					"started_at": time.Now(),
				})
				fmt.Printf("Job %d (%s) failed on attempt %d, retrying: %s\n", job.ID, job.Type, job.Attempts, errMsg)
				continue
			}
			job.Status = "error"
			job.Error = &errMsg
			fmt.Printf("Job %d (%s) failed: %s\n", job.ID, job.Type, errMsg)
			if p, err := parsePayload(job); err == nil {
				events.PublishEvent(p.ProjectID, 0, "error", "Scan stopped: "+errMsg, map[string]interface{}{
					"job_type": job.Type,
				})
			}
		} else {
			job.Status = "done"
			job.Error = nil
		}
		db.DB.Save(&job)
	}
}

// claimNextJob atomically picks the oldest queued job using SKIP LOCKED so
// multiple workers cannot double-process the same row.
func claimNextJob() (db.Job, bool) {
	var job db.Job
	tx := db.DB.Begin()
	if tx.Error != nil {
		return job, false
	}

	err := tx.Raw(`
		UPDATE jobs
		SET status = 'running',
		    attempts = attempts + 1,
		    started_at = NOW(),
		    error = NULL
		WHERE id = (
			SELECT id FROM jobs
			WHERE status = 'queued'
			  AND (started_at IS NULL OR started_at < NOW() - INTERVAL '15 seconds')
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING *`).Scan(&job).Error

	if err != nil || job.ID == 0 {
		tx.Rollback()
		return job, false
	}
	if err := tx.Commit().Error; err != nil {
		return db.Job{}, false
	}
	return job, true
}

func requeueStuckJobs(after time.Duration, maxAttempts int) {
	cutoff := time.Now().Add(-after)
	var stuck []db.Job
	db.DB.Where("status = ? AND started_at IS NOT NULL AND started_at < ?", "running", cutoff).Find(&stuck)
	for _, job := range stuck {
		if job.Attempts >= maxAttempts {
			msg := fmt.Sprintf("abandoned after %d attempts (stuck running)", job.Attempts)
			finished := time.Now()
			db.DB.Model(&job).Updates(map[string]interface{}{
				"status":      "error",
				"error":       msg,
				"finished_at": finished,
			})
			fmt.Printf("worker: abandoned stuck job %d (%s)\n", job.ID, job.Type)
			continue
		}
		db.DB.Model(&job).Updates(map[string]interface{}{
			"status":     "queued",
			"started_at": nil,
			"error":      "requeued after worker interrupt",
		})
		fmt.Printf("worker: requeued stuck job %d (%s)\n", job.ID, job.Type)
	}
}

func envDurationMinutes(key string, fallback int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return time.Duration(fallback) * time.Minute
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func executeCrawlJob(job db.Job) error {
	payload, err := parsePayload(job)
	if err != nil {
		return err
	}

	opts := crawler.CrawlOptions{
		Origin:      payload.Origin,
		MaxPages:    payload.MaxPages,
		MaxDepth:    3,
		Concurrency: envInt("CRAWL_CONCURRENCY", 8),
		DelayMs:     envInt("CRAWL_DELAY_MS", 50),
		ProjectID:   payload.ProjectID,
		CrawlRunID:  payload.CrawlRunID,
		SeedSitemap: payload.SitemapURL,
	}

	if err := crawler.CrawlSite(opts); err != nil {
		return err
	}

	var run db.CrawlRun
	if err := db.DB.First(&run, payload.CrawlRunID).Error; err == nil {
		run.Status = "done"
		finished := time.Now()
		run.FinishedAt = &finished
		db.DB.Save(&run)
	}

	nextPayload, _ := json.Marshal(map[string]interface{}{
		"project_id":   payload.ProjectID,
		"crawl_run_id": payload.CrawlRunID,
	})
	// Site-quality signals are near-instant — run them before brand so the UI
	// has real metrics on step 2, then unlock buyer-search / Google.
	db.DB.Create(&db.Job{
		Type:    "website_analysis_run",
		Payload: string(nextPayload),
		Status:  "queued",
	})
	db.DB.Create(&db.Job{
		Type:    "brand_understand_run",
		Payload: string(nextPayload),
		Status:  "queued",
	})

	return nil
}

func executeWebsiteAnalysisJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	if p.CrawlRunID == 0 {
		return fmt.Errorf("job payload missing crawl_run_id")
	}
	if err := seo.AnalyzeRun(p.ProjectID, p.CrawlRunID); err != nil {
		return err
	}
	if _, err := metrics.Compute(p.ProjectID); err != nil {
		fmt.Printf("worker: metrics after SEO audit failed: %v\n", err)
	}
	return nil
}

func executeBrandUnderstandJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	if err := brand.Analyze(p.ProjectID); err != nil {
		return err
	}

	var project db.Project
	if db.DB.First(&project, p.ProjectID).Error == nil {
		var profile brand.BrandProfile
		if json.Unmarshal([]byte(project.ProfileData), &profile) == nil {
			events.PublishEvent(p.ProjectID, 3, "milestone", "Brand profile: "+profile.BrandName, map[string]interface{}{
				"brand_name":      profile.BrandName,
				"category":        profile.Category,
				"target_audience": profile.TargetAudience,
				"products":        profile.Products,
				"topics":          profile.Topics,
				"entities":        profile.Entities,
				"usps":            profile.USPs,
			})
		}
	}

	nextPayload, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	db.DB.Create(&db.Job{Type: "search_intent_run", Payload: string(nextPayload), Status: "queued"})
	return nil
}

func executeSearchIntentJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	if err := search_intent.Generate(p.ProjectID); err != nil {
		return err
	}
	return nil
}

func executeSerpCheckJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	return serp.CheckProjectIntents(p.ProjectID)
}

func executeCompetitorDiscoveryJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}

	events.PublishEvent(p.ProjectID, 6, "progress", "Discovering competitors from the search landscape", nil)

	if err := competitor.Discover(p.ProjectID); err != nil {
		return err
	}

	var candidates []db.CompetitorCandidate
	db.DB.Where("project_id = ?", p.ProjectID).Order("id asc").Find(&candidates)

	domains := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		domains = append(domains, candidate.Domain)
	}

	events.PublishEvent(p.ProjectID, 6, "complete",
		fmt.Sprintf("Identified %d candidate competitors", len(candidates)),
		map[string]interface{}{
			"competitors_discovered": len(candidates),
			"domains":                domains,
		})

	nextPayload, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	db.DB.Create(&db.Job{Type: "competitor_classify_run", Payload: string(nextPayload), Status: "queued"})
	return nil
}

func executeCompetitorClassifyJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}

	if err := competitor.ClassifyAndMerge(p.ProjectID); err != nil {
		return err
	}

	var candidates []db.CompetitorCandidate
	db.DB.Where("project_id = ?", p.ProjectID).Order("appearances DESC, id asc").Find(&candidates)

	classified := make([]map[string]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		classified = append(classified, map[string]interface{}{
			"domain":         candidate.Domain,
			"brand_name":     candidate.BrandName,
			"classification": candidate.Classification,
			"appearances":    candidate.Appearances,
			"best_position":  candidate.BestPosition,
		})
	}

	events.PublishEvent(p.ProjectID, 6, "milestone",
		fmt.Sprintf("Classified %d competitors", len(candidates)),
		map[string]interface{}{
			"competitors": classified,
		})

	return nil
}

func executeCompetitorCrawlJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	return competitor.CrawlTopCompetitors(p.ProjectID)
}

func executeCompetitorCompareJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}

	events.PublishEvent(p.ProjectID, 8, "progress", "Comparing rival positioning with your site", nil)

	if err := competitor.Compare(p.ProjectID); err != nil {
		return err
	}

	next, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	db.DB.Create(&db.Job{Type: "seo_comparison_run", Payload: string(next), Status: "queued"})
	return nil
}

func executeGapAnalysisJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	return contentgap.Analyze(p.ProjectID)
}

func executeSeoComparisonJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}

	comparisons, err := competitor.CompareTechnical(p.ProjectID)
	if err != nil {
		return err
	}

	summary := make([]map[string]interface{}, 0, len(comparisons))
	for _, c := range comparisons {
		summary = append(summary, map[string]interface{}{
			"domain":             c.Domain,
			"is_brand":           c.IsBrand,
			"pages_crawled":      c.PagesCrawled,
			"product_pages":      c.ProductPages,
			"collection_pages":   c.CollectionPages,
			"content_pages":      c.ContentPages,
			"schema_coverage":    c.SchemaCoverage,
			"avg_word_count":     c.AvgWordCount,
			"missing_titles":     c.MissingTitles,
			"missing_meta":       c.MissingMeta,
			"images_missing_alt": c.ImagesMissingAlt,
			"broken_pages":       c.BrokenPages,
		})
	}

	events.PublishEvent(p.ProjectID, 8, "progress",
		fmt.Sprintf("Compared %d crawled sites page-for-page", len(comparisons)), map[string]interface{}{
			"sites_compared": len(comparisons),
			"comparison":     summary,
		})

	next, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	db.DB.Create(&db.Job{Type: "gap_analysis_run", Payload: string(next), Status: "queued"})
	return nil
}

func executeGeoRunJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	id, err := geo.RunMultiModelGeo(p.ProjectID)
	if err != nil {
		return err
	}
	next, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID, "geo_run_id": id, "pipeline": p.Pipeline})
	db.DB.Create(&db.Job{Type: "geo_analyze_run", Payload: string(next), Status: "queued"})
	return nil
}

// queueAfterGeo continues a full scan into competitor discovery; a manual
// GEO re-run only refreshes recommendations.
func queueAfterGeo(p jobPayload) {
	next, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	nextType := "prioritize_issues_run"
	if p.Pipeline {
		nextType = "competitor_discovery_run"
	}
	db.DB.Create(&db.Job{Type: nextType, Payload: string(next), Status: "queued"})
}

func executeGeoAnalyzeJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	if err := geo.AnalyzeGeoRun(p.ProjectID, p.GeoRunID); err != nil {
		return err
	}
	events.PublishEvent(p.ProjectID, 10, "complete", "AI answers analysed", map[string]interface{}{
		"geo_run_id": p.GeoRunID,
	})
	if _, err := metrics.Compute(p.ProjectID); err != nil {
		fmt.Printf("worker: metrics after GEO failed: %v\n", err)
	}
	queueAfterGeo(p)
	return nil
}

func executePrioritizeIssuesJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	if err := seo.PrioritizeIssues(p.ProjectID); err != nil {
		return err
	}
	next, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	db.DB.Create(&db.Job{Type: "recommendations_run", Payload: string(next), Status: "queued"})
	return nil
}

func executeRecommendationsJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}
	if err := recommendations.Generate(p.ProjectID); err != nil {
		return err
	}

	var openRecs []db.Recommendation
	db.DB.Where("project_id = ? AND status = 'open'", p.ProjectID).
		Order("CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END, id asc").
		Find(&openRecs)

	seen := map[string]bool{}
	summary := make([]map[string]interface{}, 0, 8)
	for _, rec := range openRecs {
		key := strings.ToLower(strings.TrimSpace(rec.Title))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true

		meta := map[string]string{}
		_ = json.Unmarshal([]byte(rec.Data), &meta)

		summary = append(summary, map[string]interface{}{
			"title":        rec.Title,
			"severity":     rec.Severity,
			"source":       rec.Source,
			"action":       firstNonEmpty(rec.Action, meta["note"]),
			"page_url":     firstNonEmpty(meta["page_url"], ""),
			"target_field": firstNonEmpty(meta["target_field"], ""),
			"before":       firstNonEmpty(meta["before"], ""),
			"after":        firstNonEmpty(meta["after"], meta["patch"]),
		})
		if len(summary) >= 8 {
			break
		}
	}

	events.PublishEvent(p.ProjectID, 12, "complete",
		fmt.Sprintf("Generated %d recommendations", len(openRecs)), map[string]interface{}{
			"recommendations": len(openRecs),
			"top":             summary,
		})

	next, _ := json.Marshal(map[string]interface{}{"project_id": p.ProjectID})
	db.DB.Create(&db.Job{Type: "fix_generation_run", Payload: string(next), Status: "queued"})
	return nil
}

func executeFixGenerationJob(job db.Job) error {
	p, err := parsePayload(job)
	if err != nil {
		return err
	}

	if err := fixes.GenerateForProject(p.ProjectID); err != nil {
		return err
	}

	if _, err := metrics.Compute(p.ProjectID); err != nil {
		fmt.Printf("worker: final metrics computation failed: %v\n", err)
	}

	events.PublishEvent(p.ProjectID, 16, "complete", "Full scan complete", map[string]interface{}{
		"message": "All 16 pipeline stages finished",
	})
	return nil
}
