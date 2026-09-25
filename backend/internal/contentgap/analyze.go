package contentgap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
	"visora-backend/internal/llm"
	"visora-backend/internal/noise"
)

type TopicCluster struct {
	Topic           string   `json:"topic"`
	Intent          string   `json:"intent"`
	EvidenceQueries []string `json:"evidence_queries"`
}

type ClusterResponse struct {
	Clusters []TopicCluster `json:"clusters"`
}

// Analyze produces ContentGap and KeywordGap records.
// Clustering is LLM-only; a failed call fails the job so the worker retries it.
func Analyze(projectID uint) error {
	var intents []db.SearchIntent
	if err := db.DB.Where("project_id = ?", projectID).Find(&intents).Error; err != nil {
		return err
	}

	events.PublishEvent(projectID, 8, "progress", "Starting Gap Analysis", nil)

	db.DB.Where("project_id = ?", projectID).Delete(&db.KeywordGap{})
	db.DB.Where("project_id = ?", projectID).Delete(&db.ContentGap{})

	var allGaps []db.KeywordGap

	var kept []db.CompetitorCandidate
	db.DB.Where("project_id = ?", projectID).Find(&kept)
	confirmed := make(map[string]bool, len(kept))
	for _, c := range kept {
		confirmed[strings.TrimPrefix(strings.ToLower(c.Domain), "www.")] = true
	}

	for _, intent := range intents {
		var brandRes db.SerpResult
		db.DB.Where("project_id = ? AND intent_id = ? AND is_own_domain = ?", projectID, intent.ID, true).First(&brandRes)

		var compRes []db.SerpResult
		db.DB.Where("project_id = ? AND intent_id = ? AND is_own_domain = ?", projectID, intent.ID, false).Order("position ASC").Find(&compRes)

		bestComp, ok := firstRealCompetitor(compRes, confirmed)
		if !ok {
			continue
		}

		brandPos := brandRes.Position
		if brandRes.ID == 0 {
			gap := db.KeywordGap{
				ProjectID:              projectID,
				Query:                  intent.Keyword,
				BestCompetitor:         bestComp.Domain,
				BestCompetitorPosition: &bestComp.Position,
				GapType:                "missing",
			}
			db.DB.Create(&gap)
			allGaps = append(allGaps, gap)
		} else if brandPos > 10 && bestComp.Position <= 5 {
			gap := db.KeywordGap{
				ProjectID:              projectID,
				Query:                  intent.Keyword,
				BrandPosition:          &brandPos,
				BestCompetitor:         bestComp.Domain,
				BestCompetitorPosition: &bestComp.Position,
				GapType:                "lagging",
			}
			db.DB.Create(&gap)
			allGaps = append(allGaps, gap)
		}
	}

	if len(allGaps) > 0 {
		gapByQuery := map[string]db.KeywordGap{}
		queries := make([]string, 0, len(allGaps))
		for _, g := range allGaps {
			key := strings.ToLower(strings.TrimSpace(g.Query))
			if key == "" {
				continue
			}
			gapByQuery[key] = g
			queries = append(queries, g.Query)
		}

		pageIndex := loadBrandPages(projectID)

		prompt := fmt.Sprintf(`Group these buyer searches (where rivals beat us on Google) into 2-5 topic clusters
that each could be won by one page on our site.
Queries: %v
Return ONLY JSON: {"clusters":[{"topic":"...","intent":"commercial","evidence_queries":["..."]}]}`, queries)

		raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		}, 0.2)
		if err != nil {
			return fmt.Errorf("gap clustering AI call failed: %w", err)
		}
		var cr ClusterResponse
		if err := json.Unmarshal([]byte(llm.ExtractJSONObject(raw)), &cr); err != nil {
			return fmt.Errorf("gap clustering AI reply was not valid JSON: %w", err)
		}
		usable := make([]TopicCluster, 0, len(cr.Clusters))
		for _, c := range cr.Clusters {
			c.Topic = strings.TrimSpace(c.Topic)
			c.Intent = strings.TrimSpace(c.Intent)
			if c.Topic == "" || len(c.EvidenceQueries) == 0 {
				continue
			}
			if c.Intent == "" {
				c.Intent = "commercial"
			}
			usable = append(usable, c)
		}
		if len(usable) == 0 {
			return fmt.Errorf("gap clustering AI returned no clusters")
		}
		persistClusters(projectID, usable, gapByQuery, pageIndex)
	}

	events.PublishEvent(projectID, 8, "complete", "Finished gap analysis", map[string]interface{}{
		"keyword_gaps": len(allGaps),
	})

	payload, _ := json.Marshal(map[string]interface{}{"project_id": projectID})
	db.DB.Create(&db.Job{
		Type:    "prioritize_issues_run",
		Payload: string(payload),
		Status:  "queued",
	})

	return nil
}

func persistClusters(
	projectID uint,
	clusters []TopicCluster,
	gapByQuery map[string]db.KeywordGap,
	pageIndex []pageIndexEntry,
) {
	for _, cluster := range clusters {
		domains := map[string]bool{}
		hasMissing := false
		hasLagging := false
		for _, q := range cluster.EvidenceQueries {
			gap, ok := gapByQuery[strings.ToLower(strings.TrimSpace(q))]
			if !ok {
				continue
			}
			if gap.BestCompetitor != "" {
				domains[gap.BestCompetitor] = true
			}
			switch gap.GapType {
			case "missing":
				hasMissing = true
			case "lagging":
				hasLagging = true
			}
		}

		priority := "medium"
		if hasMissing {
			priority = "critical"
		} else if hasLagging {
			priority = "high"
		}

		domainList := make([]string, 0, len(domains))
		for d := range domains {
			domainList = append(domainList, d)
		}
		sort.Strings(domainList)

		eqBytes, _ := json.Marshal(cluster.EvidenceQueries)
		epBytes, _ := json.Marshal(matchPages(pageIndex, cluster.Topic, cluster.EvidenceQueries, 5))
		cdBytes, _ := json.Marshal(domainList)

		db.DB.Create(&db.ContentGap{
			ProjectID:            projectID,
			Topic:                cluster.Topic,
			Intent:               cluster.Intent,
			CompetitorDomains:    string(cdBytes),
			EvidenceQueries:      string(eqBytes),
			ExistingRelatedPages: string(epBytes),
			Priority:             priority,
			Status:               "open",
		})
	}
}

// firstRealCompetitor prefers domains the AI confirmed as competitors, then
// skips Shopify / Google / Facebook / app-store noise.
func firstRealCompetitor(rows []db.SerpResult, confirmed map[string]bool) (db.SerpResult, bool) {
	for _, row := range rows {
		if confirmed[strings.TrimPrefix(strings.ToLower(row.Domain), "www.")] {
			return row, true
		}
	}
	for _, row := range rows {
		if noise.IsPlatform(row.Domain) {
			continue
		}
		return row, true
	}
	return db.SerpResult{}, false
}
