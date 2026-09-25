package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
)

// TriggerCrawl starts a new crawl job
func TriggerCrawl(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Create Crawl Run
	run := db.CrawlRun{
		ProjectID: project.ID,
		Status:    "queued",
		Trigger:   "manual",
		MaxPages:  50,
		Stats:     "{}",
	}

	if err := db.DB.Create(&run).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create crawl run"})
		return
	}

	// Create Job. The payload key must be "start_url": that is what the worker reads.
	payload := map[string]interface{}{
		"project_id":   project.ID,
		"crawl_run_id": run.ID,
		"start_url":    project.Website,
		"max_pages":    run.MaxPages,
	}
	payloadBytes, _ := json.Marshal(payload)

	job := db.Job{
		Type:    "crawl",
		Payload: string(payloadBytes),
	}

	if err := db.DB.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Crawl job queued",
		"crawl_run_id": run.ID,
		"job_id":       job.ID,
	})
}

// GetLatestCrawl fetches the latest crawl run for a project
func GetLatestCrawl(c *gin.Context) {
	id := c.Param("id")

	var run db.CrawlRun
	if err := db.DB.Where("project_id = ?", id).Order("created_at desc").First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No crawl runs found"})
		return
	}

	var issues []db.SeoIssue
	db.DB.Where("crawl_run_id = ?", run.ID).Find(&issues)

	c.JSON(http.StatusOK, gin.H{
		"run":    run,
		"issues": issues,
	})
}
