package main

import (
	"fmt"
	"os"

	"visora-backend/internal/db"
	"visora-backend/internal/geo"
)

// One-shot: re-run GEO mention extraction for the latest run of a project.
// Usage: go run ./cmd/reanalyze-geo <projectID>
func main() {
	projectID := uint(1)
	if len(os.Args) > 1 {
		var n uint
		fmt.Sscanf(os.Args[1], "%d", &n)
		if n > 0 {
			projectID = n
		}
	}
	db.Connect()
	var run db.GeoRun
	if err := db.DB.Where("project_id = ?", projectID).Order("id desc").First(&run).Error; err != nil {
		fmt.Println("no geo run:", err)
		os.Exit(1)
	}
	fmt.Printf("reanalyzing project %d run %d…\n", projectID, run.ID)
	if err := geo.AnalyzeGeoRun(projectID, run.ID); err != nil {
		fmt.Println("failed:", err)
		os.Exit(1)
	}
	var mentions int64
	db.DB.Model(&db.GeoMention{}).Where("run_id = ?", run.ID).Count(&mentions)
	fmt.Printf("done — %d mention rows\n", mentions)
}
