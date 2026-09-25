package serp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/noise"
)

// CheckProjectIntents fetches SERPs for all SearchIntents of a project.
func CheckProjectIntents(projectID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	var intents []db.SearchIntent
	if err := db.DB.Where("project_id = ?", projectID).Find(&intents).Error; err != nil {
		return err
	}

	events.PublishEvent(projectID, 5, "progress", "Starting SERP checks", map[string]int{"intents_count": len(intents)})

	projectDomain := extractDomain(project.Website)
	concurrency := envInt("SERP_CONCURRENCY", 4)
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(intents) && len(intents) > 0 {
		concurrency = len(intents)
	}

	var (
		top10    int64
		top50    int64
		notFound int64
		checked  int64
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, concurrency)

	for _, intent := range intents {
		intent := intent
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			entries, err := CheckQuery(intent.Keyword)
			if err != nil {
				atomic.AddInt64(&notFound, 1)
				n := atomic.AddInt64(&checked, 1)
				events.PublishEvent(projectID, 5, "error", fmt.Sprintf("Failed to check SERP for %s", intent.Keyword), map[string]interface{}{
					"query":           intent.Keyword,
					"queries_checked": n,
					"intents_count":   len(intents),
				})
				return
			}

			brandPosition := 0
			topResults := make([]map[string]interface{}, 0, 8)

			for _, entry := range entries {
				isOwnDomain := false
				entryDomain := extractDomain(entry.URL)
				if entryDomain == projectDomain {
					isOwnDomain = true
					if brandPosition == 0 || entry.Position < brandPosition {
						brandPosition = entry.Position
					}
				}

				serpRes := db.SerpResult{
					ProjectID:   projectID,
					IntentID:    intent.ID,
					Query:       intent.Keyword,
					Domain:      entryDomain,
					Position:    entry.Position,
					PageTitle:   entry.Title,
					PageURL:     entry.URL,
					PageSnippet: entry.Snippet,
					IsOwnDomain: isOwnDomain,
					CheckedAt:   time.Now(),
				}
				db.DB.Create(&serpRes)

				if len(topResults) < 8 {
					topResults = append(topResults, map[string]interface{}{
						"position": entry.Position,
						"domain":   entryDomain,
						"title":    entry.Title,
						"url":      entry.URL,
						"snippet":  entry.Snippet,
						"own":      isOwnDomain,
					})
				}

				if !isOwnDomain && entryDomain != "" && !noise.IsPlatform(entryDomain) {
					var existing db.CompetitorCandidate
					if err := db.DB.Where("project_id = ? AND domain = ?", projectID, entryDomain).First(&existing).Error; err != nil {
						db.DB.Create(&db.CompetitorCandidate{
							ProjectID: projectID,
							Domain:    entryDomain,
							Status:    "pending",
						})
					}
				}
			}

			foundInTop50 := brandPosition > 0 && brandPosition <= 50
			if brandPosition > 0 && brandPosition <= 10 {
				atomic.AddInt64(&top10, 1)
			}
			if foundInTop50 {
				atomic.AddInt64(&top50, 1)
			} else {
				atomic.AddInt64(&notFound, 1)
			}

			n := atomic.AddInt64(&checked, 1)
			events.PublishEvent(projectID, 5, "milestone",
				fmt.Sprintf("Checked “%s”", intent.Keyword),
				map[string]interface{}{
					"query":              intent.Keyword,
					"brand_position":     brandPosition,
					"found_in_top_50":    foundInTop50,
					"results":            topResults,
					"queries_checked":    n,
					"top_10_appearances": atomic.LoadInt64(&top10),
					"top_50_appearances": atomic.LoadInt64(&top50),
					"not_found":          atomic.LoadInt64(&notFound),
					"intents_count":      len(intents),
				})
		}()
	}
	wg.Wait()

	events.PublishEvent(projectID, 5, "complete", "Finished SERP checks", map[string]interface{}{
		"intents_count":      len(intents),
		"queries_checked":    len(intents),
		"top_10_appearances": atomic.LoadInt64(&top10),
		"top_50_appearances": atomic.LoadInt64(&top50),
		"not_found":          atomic.LoadInt64(&notFound),
		"keywords": func() []string {
			out := make([]string, 0, len(intents))
			for _, intent := range intents {
				out = append(out, intent.Keyword)
			}
			return out
		}(),
	})

	// AI answers only need the buyer searches, so they run before the slow
	// competitor crawl; the GEO job then continues the pipeline.
	payload, _ := json.Marshal(map[string]interface{}{
		"project_id": projectID,
		"pipeline":   true,
	})
	db.DB.Create(&db.Job{
		Type:    "geo_run",
		Payload: string(payload),
		Status:  "queued",
	})

	return nil
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func extractDomain(u string) string {
	if !strings.HasPrefix(u, "http") {
		u = "https://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	host := parsed.Hostname()
	return strings.TrimPrefix(host, "www.")
}
