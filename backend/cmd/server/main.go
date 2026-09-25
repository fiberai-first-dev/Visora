package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/api"
	visoraauth "visora-backend/internal/auth"
	"visora-backend/internal/db"
)

func main() {
	fmt.Println("Starting Visora Go Backend API...")

	db.Connect()

	r := gin.Default()

	// ── CORS ──────────────────────────────────────────────────────────────────
	// Allow the frontend origin and credentials (cookies)
	r.Use(func(c *gin.Context) {
		frontendURL := os.Getenv("FRONTEND_URL")
		if frontendURL == "" {
			frontendURL = "http://localhost:3000"
		}
		origin := c.Request.Header.Get("Origin")
		if origin == frontendURL {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", frontendURL)
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-Email")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// ── JWT middleware (runs on all routes, sets user_email if valid cookie) ──
	r.Use(visoraauth.AuthMiddleware())

	// ── Health ────────────────────────────────────────────────────────────────
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Visora Go Backend is running"})
	})

	// ── Google OAuth routes (no auth required) ────────────────────────────────
	r.GET("/auth/google", visoraauth.GoogleLogin)
	r.GET("/auth/google/callback", visoraauth.GoogleCallback)
	r.GET("/auth/logout", visoraauth.Logout)
	r.GET("/auth/me", visoraauth.RequireAuth(), visoraauth.Me)

	// ── API routes (all require auth) ─────────────────────────────────────────
	apiGroup := r.Group("/api", visoraauth.RequireAuth(), api.ResolveProjectID())
	{
		// Projects
		apiGroup.GET("/projects", api.GetProjects)
		apiGroup.POST("/projects", api.CreateProject)
		apiGroup.GET("/projects/:id", api.GetProject)
		apiGroup.PUT("/projects/:id", api.UpdateProject)
		apiGroup.PATCH("/projects/:id", api.UpdateProject)
		apiGroup.GET("/projects/:id/summary", api.GetProjectSummary)
		apiGroup.GET("/projects/:id/pages", api.GetPages)
		apiGroup.GET("/projects/:id/pages/:pageId", api.GetPageDetail)
		apiGroup.GET("/projects/:id/issues", api.GetIssues)
		apiGroup.GET("/projects/:id/issues/:issueId", api.GetIssueDetail)
		apiGroup.GET("/projects/:id/seo", api.GetSeoAudit)
		apiGroup.GET("/projects/:id/performance", api.GetPerformance)
		apiGroup.GET("/projects/:id/recommendations", api.GetRecommendations)
		apiGroup.PATCH("/projects/:id/recommendations/:recId", api.UpdateRecommendationStatus)
		apiGroup.GET("/projects/:id/competitors", api.GetCompetitors)
		apiGroup.GET("/projects/:id/competitors/:compId", api.GetCompetitorDetail)
		apiGroup.GET("/projects/:id/seo-comparison", api.GetSeoComparison)
		apiGroup.GET("/projects/:id/keywords", api.GetKeywords)
		apiGroup.GET("/projects/:id/live-status", api.GetLiveStatus)
		apiGroup.GET("/projects/:id/preview", api.PreviewPage)

		// Intelligence & Scan
		apiGroup.POST("/projects/:id/scan", api.TriggerFullScan)
		apiGroup.GET("/projects/:id/scan-stream", api.StreamScanEvents)
		apiGroup.GET("/projects/:id/search-intents", api.GetSearchIntents)
		apiGroup.GET("/projects/:id/search-intents/:intentId", api.GetSearchIntentDetail)
		apiGroup.GET("/projects/:id/keyword-gaps", api.GetKeywordGaps)
		apiGroup.GET("/projects/:id/content-gaps", api.GetContentGaps)
		apiGroup.GET("/projects/:id/monitoring", api.GetMonitoringEvents)
		apiGroup.GET("/projects/:id/fixes", api.GetFixes)
		apiGroup.POST("/projects/:id/fixes/:fixId/export", api.ExportFix)
		apiGroup.GET("/projects/:id/metrics/history", api.GetMetricsHistory)
		apiGroup.POST("/projects/:id/recrawl", api.TriggerRecrawl)

		// Crawl
		apiGroup.GET("/projects/:id/crawl", api.GetLatestCrawl)
		apiGroup.POST("/projects/:id/crawl", api.TriggerCrawl)

		// GEO
		apiGroup.GET("/projects/:id/geo/prompts", api.GetGeoPrompts)
		apiGroup.POST("/projects/:id/geo/run", api.TriggerGeoRun)
		apiGroup.GET("/projects/:id/geo/responses", api.GetGeoResponses)
		apiGroup.GET("/projects/:id/geo/overview", api.GetGeoOverview)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server listening on port %s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
