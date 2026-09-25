package brand

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/crawler"
	"visora-backend/internal/db"
	"visora-backend/internal/llm"
)

type SearchQueryExtraction struct {
	Query  string `json:"query"`
	Intent string `json:"intent"` // informational, commercial, transactional
}

type BrandProfile struct {
	BrandName      string                  `json:"brand_name"`
	Category       string                  `json:"category"`
	TargetAudience string                  `json:"target_audience"`
	USPs           []string                `json:"usps"`
	Topics         []string                `json:"topics"`
	Products       []string                `json:"products"`
	Entities       []string                `json:"entities"`
	SearchQueries  []SearchQueryExtraction `json:"search_queries"`
}

// Analyze asks the LLM to profile the brand from the latest crawl. There is
// no non-AI fallback: a failed call fails the job so the worker retries it.
func Analyze(projectID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	pages := crawler.LatestSitePages(projectID, 40)
	if len(pages) == 0 {
		return fmt.Errorf("no pages found to analyze")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Website: %s\nCountry: %s\n\n", project.Website, project.Country)
	used := 0
	for i := range pages {
		if used >= 12 || crawler.IsBoilerplatePage(pages[i].URL) {
			continue
		}
		budget := 600
		if used == 0 {
			budget = 2500
		}
		fmt.Fprintf(&b, "PAGE %s\n%s\n\n", pages[i].URL, crawler.PageOutline(&pages[i], budget))
		used++
	}

	raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: "You are a brand analyst. Read the crawled pages and describe what this business actually sells, " +
				"to whom, and why buyers pick it. Use only facts from the pages. Reply with JSON only.",
		},
		{
			Role: openai.ChatMessageRoleUser,
			Content: b.String() + `Return JSON:
{"brand_name":"","category":"","target_audience":"","usps":[""],"topics":[""],"products":[""],"entities":[""],
 "search_queries":[{"query":"","intent":"commercial|transactional|informational"}]}
products = the real offerings named on the site. search_queries = 6-10 searches a buyer who does not know the brand yet would type.`,
		},
	}, 0.2)
	if err != nil {
		return fmt.Errorf("brand profile AI call failed: %w", err)
	}

	var profile BrandProfile
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &profile); err != nil {
		return fmt.Errorf("brand profile AI reply was not valid JSON: %w", err)
	}
	if strings.TrimSpace(profile.BrandName) == "" {
		profile.BrandName = project.Brand
	}
	if strings.TrimSpace(profile.BrandName) == "" || len(profile.Products)+len(profile.Topics) == 0 {
		return fmt.Errorf("brand profile AI reply had no brand or offerings")
	}
	return persistProfile(projectID, &project, profile)
}

func persistProfile(projectID uint, project *db.Project, profile BrandProfile) error {
	raw, _ := json.Marshal(profile)
	project.ProfileData = string(raw)
	if profile.BrandName != "" {
		project.Brand = profile.BrandName
		project.Name = profile.BrandName
	}
	if profile.Category != "" {
		project.Category = profile.Category
	}
	db.DB.Save(project)

	for _, t := range profile.Topics {
		var existing db.Topic
		if err := db.DB.Where("project_id = ? AND name = ?", projectID, t).First(&existing).Error; err != nil {
			db.DB.Create(&db.Topic{ProjectID: projectID, Name: t})
		}
	}

	for _, sq := range profile.SearchQueries {
		query := strings.TrimSpace(sq.Query)
		if query == "" {
			continue
		}
		var kw db.Keyword
		if db.DB.Where("project_id = ? AND keyword = ?", projectID, query).First(&kw).Error != nil {
			db.DB.Create(&db.Keyword{
				ProjectID: projectID,
				Keyword:   query,
				Intent:    sq.Intent,
				Source:    "ai_discovery",
				Status:    "tracked",
			})
		}
	}

	return nil
}
