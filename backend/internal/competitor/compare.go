package competitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/crawler"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/llm"
)

type contentInsight struct {
	Topic       string `json:"topic"`
	Intent      string `json:"intent"`
	RivalDomain string `json:"rival_domain"`
	WhyItWins   string `json:"why_it_wins"`
	YourGap     string `json:"your_gap"`
	Action      string `json:"action"`
}

type compareResponse struct {
	Insights []contentInsight `json:"insights"`
	Summary  string           `json:"summary"`
}

// Compare runs a single fast LLM contrast of your homepage vs crawled rival
// homepages. Heavy multi-page LLM gap analysis was removed because SERP
// keyword gaps + technical SEO compare already cover that ground — this call
// only adds the narrative "what they say that you don't" layer (~5–15s).
func Compare(projectID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	brandPage := crawler.HomePage(projectID)
	rivals := selectTopCompetitors(projectID, maxCompetitorsToCrawl)

	type rivalSnap struct {
		Domain string
		Page   *db.Page
	}
	snaps := make([]rivalSnap, 0, len(rivals))
	for _, c := range rivals {
		if c.CrawlStatus != "crawled" {
			continue
		}
		cid := c.ID
		p := loadHomepage(projectID, &cid)
		if p == nil {
			continue
		}
		snaps = append(snaps, rivalSnap{Domain: c.Domain, Page: p})
	}

	events.PublishEvent(projectID, 8, "progress", "Contrasting your homepage with rivals", map[string]interface{}{
		"rivals_with_pages": len(snaps),
	})

	if brandPage == nil || len(snaps) == 0 {
		fmt.Printf("competitor: compare skipped for project %d — need brand + rival pages\n", projectID)
		events.PublishEvent(projectID, 8, "progress", "Not enough rival pages to contrast yet", nil)
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "BRAND: %s (%s)\n%s\n\n", project.Brand, project.Website, crawler.PageOutline(brandPage, 2500))
	for _, s := range snaps {
		fmt.Fprintf(&b, "RIVAL: %s\n%s\n\n", s.Domain, crawler.PageOutline(s.Page, 1200))
	}

	prompt := b.String() + `Compare the brand homepage to the rival homepages.
Return JSON only:
{"summary":"one sentence","insights":[{"topic":"...","intent":"commercial|informational","rival_domain":"...","why_it_wins":"...","your_gap":"...","action":"concrete fix"}]}
Give 3-5 insights. Prefer specific wording/positioning gaps over generic SEO advice.`

	raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a D2C competitive content analyst. Reply with JSON only.",
		},
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}, 0.2)
	if err != nil {
		return fmt.Errorf("rival comparison AI call failed: %w", err)
	}

	var parsed compareResponse
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &parsed); err != nil {
		return fmt.Errorf("rival comparison AI reply was not valid JSON: %w", err)
	}
	if len(parsed.Insights) == 0 {
		return fmt.Errorf("rival comparison AI returned no insights")
	}

	db.DB.Where("project_id = ?", projectID).Delete(&db.ContentOpportunity{})
	db.DB.Where("project_id = ? AND source = ?", projectID, "competitors").
		Where("status = ?", "open").
		Delete(&db.Recommendation{})

	saved := 0
	insightCards := make([]map[string]interface{}, 0, len(parsed.Insights))
	for _, in := range parsed.Insights {
		topic := strings.TrimSpace(in.Topic)
		if topic == "" {
			continue
		}
		intent := strings.TrimSpace(in.Intent)
		if intent == "" {
			intent = "commercial"
		}

		db.DB.Create(&db.ContentOpportunity{
			ProjectID: projectID,
			Topic:     topic,
			Intent:    intent,
			Status:    "open",
		})

		detail := strings.TrimSpace(in.YourGap)
		if why := strings.TrimSpace(in.WhyItWins); why != "" {
			if detail != "" {
				detail = detail + " — " + why
			} else {
				detail = why
			}
		}
		if rival := strings.TrimSpace(in.RivalDomain); rival != "" {
			detail = fmt.Sprintf("[%s] %s", rival, detail)
		}
		action := strings.TrimSpace(in.Action)
		if action == "" {
			continue
		}

		data, _ := json.Marshal(map[string]string{
			"origin":       "ai",
			"rival_domain": strings.TrimSpace(in.RivalDomain),
			"topic":        topic,
		})
		db.DB.Create(&db.Recommendation{
			ProjectID: projectID,
			Source:    "competitors",
			Severity:  "high",
			Title:     topic,
			Detail:    detail,
			Action:    action,
			Status:    "open",
			Data:      string(data),
		})
		saved++
		insightCards = append(insightCards, map[string]interface{}{
			"topic":        topic,
			"rival_domain": in.RivalDomain,
			"action":       action,
		})
	}

	summary := strings.TrimSpace(parsed.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Found %d content gaps vs rivals", saved)
	}
	events.PublishEvent(projectID, 8, "milestone", summary, map[string]interface{}{
		"insights":       insightCards,
		"insights_count": saved,
	})
	fmt.Printf("competitor: LLM compare saved %d insights for project %d\n", saved, projectID)
	return nil
}

func loadHomepage(projectID uint, competitorID *uint) *db.Page {
	var page db.Page
	q := db.DB.Where("project_id = ?", projectID).Order("depth asc, id asc")
	if competitorID == nil {
		q = q.Where("competitor_id IS NULL")
	} else {
		q = q.Where("competitor_id = ?", *competitorID)
	}
	if err := q.First(&page).Error; err != nil {
		return nil
	}
	return &page
}

func formatPageEvidence(p *db.Page) string {
	title := ""
	if p.Title != nil {
		title = *p.Title
	}
	meta := ""
	if p.MetaDescription != nil {
		meta = *p.MetaDescription
	}
	body := strings.TrimSpace(p.BodyText)
	if len(body) > 900 {
		body = body[:900] + "…"
	}
	return fmt.Sprintf("url=%s\ntitle=%s\nh1=%s\nmeta=%s\nwords=%d\nbody=%s",
		p.URL, title, p.H1, meta, p.WordCount, body)
}
