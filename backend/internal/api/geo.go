package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
)

// GetGeoPrompts fetches existing prompts or generates new ones if none exist
func GetGeoPrompts(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Prompts come only from the scan's AI-written buyer searches.
	existing := []db.GeoPrompt{}
	db.DB.Where("project_id = ? AND active = ?", project.ID, true).Order("id asc").Find(&existing)
	c.JSON(http.StatusOK, existing)
}

// TriggerGeoRun starts a new GEO analysis job
func TriggerGeoRun(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Get active prompts
	var prompts []db.GeoPrompt
	db.DB.Where("project_id = ? AND active = ?", project.ID, true).Find(&prompts)

	if len(prompts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active prompts found for this project"})
		return
	}

	// The worker creates the GeoRun and chains analysis + recommendations.
	runBytes, _ := json.Marshal(map[string]interface{}{"project_id": project.ID})
	if err := db.DB.Create(&db.Job{Type: "geo_run", Payload: string(runBytes), Status: "queued"}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue GEO run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "GEO run queued"})
}

// GetGeoResponses fetches the latest GEO responses and mentions
func GetGeoResponses(c *gin.Context) {
	id := c.Param("id")

	// Get latest run
	var geoRun db.GeoRun
	if err := db.DB.Where("project_id = ?", id).Order("id desc").First(&geoRun).Error; err != nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	var responses []db.GeoResponse
	db.DB.Where("run_id = ?", geoRun.ID).Find(&responses)

	var mentions []db.GeoMention
	db.DB.Where("run_id = ?", geoRun.ID).Find(&mentions)

	// Join them for the frontend
	type GeoItem struct {
		Response db.GeoResponse `json:"response"`
		Mention  *db.GeoMention `json:"mention"`
	}

	var results []GeoItem
	for _, r := range responses {
		var matchedMention *db.GeoMention
		for _, m := range mentions {
			if m.ResponseID == r.ID && m.IsTarget { // only include the target brand mention for now
				matchedMention = &m
				break
			}
		}
		// Also fetch the prompt text to join
		var p db.GeoPrompt
		db.DB.First(&p, r.PromptID)
		
		// Let's just shove it in the ResponseText field for now so the frontend doesn't need to join
		r.ResponseText = p.Text + "|||" + r.ResponseText

		results = append(results, GeoItem{
			Response: r,
			Mention:  matchedMention,
		})
	}

	c.JSON(http.StatusOK, results)
}
