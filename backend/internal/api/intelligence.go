package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
	"visora-backend/internal/noise"
)

func GetSearchIntents(c *gin.Context) {
	id := c.Param("id")
	var intents []db.SearchIntent
	if err := db.DB.Where("project_id = ?", id).Find(&intents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch search intents"})
		return
	}
	
	type IntentWithPosition struct {
		db.SearchIntent
		Position *int `json:"position"`
	}
	
	var result []IntentWithPosition
	for _, intent := range intents {
		var serpRes db.SerpResult
		db.DB.Where("project_id = ? AND intent_id = ? AND is_own_domain = ?", id, intent.ID, true).First(&serpRes)
		
		var pos *int
		if serpRes.ID != 0 {
			p := serpRes.Position
			pos = &p
		}
		
		result = append(result, IntentWithPosition{
			SearchIntent: intent,
			Position:     pos,
		})
	}
	
	c.JSON(http.StatusOK, result)
}

func GetKeywordGaps(c *gin.Context) {
	id := c.Param("id")
	var gaps []db.KeywordGap
	if err := db.DB.Where("project_id = ?", id).Order("id desc").Find(&gaps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch keyword gaps"})
		return
	}
	out := make([]db.KeywordGap, 0, len(gaps))
	for _, g := range gaps {
		if !noise.IsPlatform(g.BestCompetitor) {
			out = append(out, g)
			continue
		}
		// Stored rival was a platform — swap in the next real brand from SERP.
		var intent db.SearchIntent
		if err := db.DB.Where("project_id = ? AND LOWER(keyword) = ?", id, strings.ToLower(g.Query)).First(&intent).Error; err != nil {
			continue
		}
		var rows []db.SerpResult
		db.DB.Where("project_id = ? AND intent_id = ? AND is_own_domain = ?", id, intent.ID, false).
			Order("position ASC").Find(&rows)
		replaced := false
		for _, row := range rows {
			if noise.IsPlatform(row.Domain) {
				continue
			}
			pos := row.Position
			g.BestCompetitor = row.Domain
			g.BestCompetitorPosition = &pos
			out = append(out, g)
			replaced = true
			break
		}
		if !replaced {
			// Still a gap, but no real rival named.
			g.BestCompetitor = ""
			g.BestCompetitorPosition = nil
			out = append(out, g)
		}
	}
	c.JSON(http.StatusOK, out)
}

func GetContentGaps(c *gin.Context) {
	id := c.Param("id")
	var gaps []db.ContentGap
	if err := db.DB.Where("project_id = ?", id).Order("id desc").Find(&gaps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch content gaps"})
		return
	}
	c.JSON(http.StatusOK, gaps)
}

func GetMonitoringEvents(c *gin.Context) {
	id := c.Param("id")
	var events []db.MonitorEvent
	if err := db.DB.Where("project_id = ?", id).Order("created_at desc").Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch monitor events"})
		return
	}
	c.JSON(http.StatusOK, events)
}

func GetFixes(c *gin.Context) {
	id := c.Param("id")
	projectID := uint(0)
	if stored, ok := c.Get("project"); ok {
		if p, ok := stored.(*db.Project); ok {
			projectID = p.ID
		}
	}
	if projectID == 0 {
		var project db.Project
		if err := db.DB.First(&project, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
			return
		}
		projectID = project.ID
	}

	var rows []db.GeneratedFix
	if err := db.DB.Where("project_id = ?", projectID).Order("id asc").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch generated fixes"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func ExportFix(c *gin.Context) {
	fixID := c.Param("fixId")
	var fix db.GeneratedFix
	if err := db.DB.First(&fix, fixID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fix not found"})
		return
	}
	
	fix.Status = "exported"
	db.DB.Save(&fix)
	
	c.JSON(http.StatusOK, gin.H{"content": fix.Content})
}

func GetMetricsHistory(c *gin.Context) {
	id := c.Param("id")
	var history []db.ProjectMetricsHistory
	if err := db.DB.Where("project_id = ?", id).Order("computed_at asc").Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch metrics history"})
		return
	}
	c.JSON(http.StatusOK, history)
}

func TriggerRecrawl(c *gin.Context) {
	TriggerFullScan(c)
}
