package competitor

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"visora-backend/internal/crawler"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/noise"
)

const (
	maxCompetitorsToCrawl   = 8
	competitorMaxPages      = 12
	competitorMaxDepth      = 2
	competitorCrawlParallel = 4
)

// CrawlTopCompetitors crawls the top 5 rival sites in parallel.
func CrawlTopCompetitors(projectID uint) error {
	candidates := selectTopCompetitors(projectID, maxCompetitorsToCrawl)

	events.PublishEvent(projectID, 7, "progress", "Checking rival homepages", map[string]interface{}{
		"competitors_to_crawl": len(candidates),
		"max_pages_each":       competitorMaxPages,
	})

	var (
		mu      sync.Mutex
		crawled int
		wg      sync.WaitGroup
	)
	sem := make(chan struct{}, competitorCrawlParallel)

	for i := range candidates {
		c := candidates[i]
		wg.Add(1)
		go func(c db.CompetitorCandidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			events.PublishEvent(projectID, 7, "milestone", "Checking rival: "+c.Domain, map[string]interface{}{
				"domain": c.Domain,
			})

			c.CrawlStatus = "crawling"
			db.DB.Save(&c)

			competitorID := c.ID
			run := db.CrawlRun{
				ProjectID:    projectID,
				CompetitorID: &competitorID,
				Status:       "running",
				Trigger:      "competitor_crawl_" + c.Domain,
				MaxPages:     competitorMaxPages,
				Stats:        "{}",
			}
			db.DB.Create(&run)

			opts := crawler.CrawlOptions{
				Origin:       "https://" + c.Domain,
				MaxPages:     competitorMaxPages,
				MaxDepth:     competitorMaxDepth,
				Concurrency:  2,
				DelayMs:      0,
				ProjectID:    projectID,
				CrawlRunID:   run.ID,
				CompetitorID: &competitorID,
				SeedURLs:     rankingURLs(projectID, c.Domain, 8),
			}

			if err := crawler.CrawlSite(opts); err != nil {
				c.CrawlStatus = "skipped"
				db.DB.Save(&c)
				fmt.Printf("competitor: crawl %s failed: %v\n", c.Domain, err)
				return
			}

			var finalRun db.CrawlRun
			if db.DB.First(&finalRun, run.ID).Error == nil {
				c.PagesCrawled = finalRun.PagesCrawled
			}

			c.CrawlStatus = "crawled"
			now := time.Now()
			c.CrawledAt = &now
			db.DB.Save(&c)

			mu.Lock()
			crawled++
			mu.Unlock()

			events.PublishEvent(projectID, 7, "milestone",
				"Checked "+c.Domain, map[string]interface{}{
					"domain":        c.Domain,
					"pages_crawled": c.PagesCrawled,
				})
		}(c)
	}
	wg.Wait()

	events.PublishEvent(projectID, 7, "complete", "Finished rival checks", map[string]interface{}{
		"competitors_crawled": crawled,
	})

	payload, _ := json.Marshal(map[string]interface{}{"project_id": projectID})
	db.DB.Create(&db.Job{
		Type:    "competitor_compare_run",
		Payload: string(payload),
		Status:  "queued",
	})

	return nil
}

func selectTopCompetitors(projectID uint, limit int) []db.CompetitorCandidate {
	var all []db.CompetitorCandidate
	db.DB.Where("project_id = ?", projectID).
		Order("appearances DESC, id ASC").
		Find(&all)

	out := make([]db.CompetitorCandidate, 0, limit)
	for _, c := range all {
		if noise.IsPlatform(c.Domain) {
			continue
		}
		class := strings.ToLower(c.Classification)
		if class == "publisher" || class == "marketplace" {
			continue
		}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}

	if len(out) == 0 {
		for _, c := range all {
			if noise.IsPlatform(c.Domain) {
				continue
			}
			out = append(out, c)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// rankingURLs are the rival's pages Google actually showed for our buyer
// searches — the copy we have to beat.
func rankingURLs(projectID uint, domain string, limit int) []string {
	var rows []db.SerpResult
	db.DB.Where("project_id = ? AND (domain = ? OR domain = ?) AND is_own_domain = false", projectID, domain, "www."+domain).
		Order("position asc, id desc").
		Limit(limit * 4).
		Find(&rows)
	seen := map[string]bool{}
	var urls []string
	for _, r := range rows {
		if r.PageURL == "" || seen[r.PageURL] {
			continue
		}
		seen[r.PageURL] = true
		urls = append(urls, r.PageURL)
		if len(urls) >= limit {
			break
		}
	}
	return urls
}

func isNoiseDomain(domain string) bool {
	return noise.IsPlatform(domain)
}

func latestCompetitorRun(projectID, competitorID uint) (db.CrawlRun, bool) {
	var run db.CrawlRun
	err := db.DB.Where("project_id = ? AND competitor_id = ? AND status = 'done'", projectID, competitorID).
		Order("id desc").First(&run).Error
	return run, err == nil
}
