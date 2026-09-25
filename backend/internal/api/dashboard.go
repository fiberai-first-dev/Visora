package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
)

// GetProjectSummary aggregates data for the frontend dashboard
func GetProjectSummary(c *gin.Context) {
	id := c.Param("id")

	var project db.Project
	if err := db.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// 1. Fetch Latest Metrics
	var seoMetric db.ProjectMetric
	db.DB.Where("project_id = ? AND seo_score IS NOT NULL", id).Order("id desc").First(&seoMetric)

	var geoMetric db.ProjectMetric
	db.DB.Where("project_id = ? AND geo_score IS NOT NULL", id).Order("id desc").First(&geoMetric)

	// 2. Fetch Latest Runs
	var latestCrawl db.CrawlRun
	db.DB.Where("project_id = ? AND competitor_id IS NULL", id).Order("id desc").First(&latestCrawl)

	var latestGeo db.GeoRun
	db.DB.Where("project_id = ?", id).Order("id desc").First(&latestGeo)

	// 3. Issue Counts
	var issueCounts struct {
		Critical int `json:"critical"`
		Warning  int `json:"warning"`
		Notice   int `json:"notice"`
	}
	if latestCrawl.ID != 0 {
		var results []struct {
			Severity string
			Count    int
		}
		db.DB.Model(&db.SeoIssue{}).Select("severity, count(*) as count").Where("crawl_run_id = ?", latestCrawl.ID).Group("severity").Scan(&results)
		for _, r := range results {
			if r.Severity == "critical" {
				issueCounts.Critical = r.Count
			} else if r.Severity == "warning" {
				issueCounts.Warning = r.Count
			} else if r.Severity == "notice" {
				issueCounts.Notice = r.Count
			}
		}
	}

	// 4. Counts
	var pageCount int64
	db.DB.Model(&db.Page{}).Where("project_id = ? AND competitor_id IS NULL", id).Count(&pageCount)

	var kwCount int64
	db.DB.Model(&db.Keyword{}).Where("project_id = ?", id).Count(&kwCount)

	var recCount int64
	db.DB.Model(&db.Recommendation{}).Where("project_id = ? AND status = 'open'", id).Count(&recCount)

	// 5. Active Jobs
	var activeJobs []map[string]interface{}
	db.DB.Model(&db.Job{}).Select("id, type, status").Where("status IN ?", []string{"queued", "running"}).Order("id desc").Limit(5).Find(&activeJobs)
	if activeJobs == nil {
		activeJobs = []map[string]interface{}{}
	}

	var avgResponseMs *int
	if latestCrawl.ID > 0 {
		var avg float64
		db.DB.Model(&db.Page{}).
			Where("crawl_run_id = ? AND response_ms > 0", latestCrawl.ID).
			Select("COALESCE(AVG(response_ms), 0)").
			Scan(&avg)
		if avg > 0 {
			v := int(avg)
			avgResponseMs = &v
		}
	}

	// Build the complex JSON payload expected by Next.js
	payload := gin.H{
		"project": project,
		"seo": gin.H{
			"score":           seoMetric.SeoScore,
			"computedAt":      seoMetric.ComputedAt,
			"breakdown":       jsonRawMessage(seoMetric.Breakdown),
			"avgResponseMs":   avgResponseMs,
			"latestRun": func() interface{} {
				if latestCrawl.ID == 0 {
					return nil
				}
				return latestCrawl
			}(),
			"issueCounts": issueCounts,
		},
		"geo": gin.H{
			"score":         geoMetric.GeoScore,
			"mentionRate":   geoMetric.MentionRate,
			"citationRate":  geoMetric.CitationRate,
			"competitorSov": geoMetric.CompetitorSov,
			"computedAt":    geoMetric.ComputedAt,
			"breakdown":     jsonRawMessage(geoMetric.Breakdown),
			"latestRun": func() interface{} {
				if latestGeo.ID == 0 {
					return nil
				}
				return latestGeo
			}(),
		},
		"counts": gin.H{
			"pages":               pageCount,
			"keywords":            kwCount,
			"openRecommendations": recCount,
		},
		"activeJobs": activeJobs,
	}

	c.JSON(http.StatusOK, payload)
}

func jsonRawMessage(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

// GetGeoOverview fetches detailed GEO analytics
func GetGeoOverview(c *gin.Context) {
	id := c.Param("id")

	var run db.GeoRun
	if err := db.DB.Where("project_id = ?", id).Order("id desc").First(&run).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"latestRun":          nil,
			"metrics":            gin.H{},
			"competitorMentions": []interface{}{},
			"citationSources":    []interface{}{},
			"sentiment":          gin.H{},
			"perPrompt":          []interface{}{},
			"modelStats":         []interface{}{},
			"promptsChecked":     0,
			"promptsMentioned":   0,
			"answersCollected":   0,
		})
		return
	}

	var metric db.ProjectMetric
	db.DB.Where("project_id = ? AND geo_score IS NOT NULL", id).Order("id desc").First(&metric)

	var competitorMentions []map[string]interface{}
	db.DB.Model(&db.GeoMention{}).
		Select("brand, count(*) as count").
		Where("run_id = ? AND is_target = false AND mentioned = true", run.ID).
		Group("brand").
		Order("count desc").
		Limit(12).
		Find(&competitorMentions)
	if competitorMentions == nil {
		competitorMentions = []map[string]interface{}{}
	}

	var citationSources []map[string]interface{}
	db.DB.Model(&db.GeoCitation{}).
		Select("source_type, count(*) as total, sum(case when is_brand_domain then 1 else 0 end) as brand").
		Where("run_id = ?", run.ID).
		Group("source_type").
		Order("total desc").
		Find(&citationSources)
	if citationSources == nil {
		citationSources = []map[string]interface{}{}
	}

	var sentiment []struct {
		Sentiment string
		Count     int
	}
	db.DB.Model(&db.GeoMention{}).
		Select("sentiment, count(*) as count").
		Where("run_id = ? AND is_target = true AND mentioned = true", run.ID).
		Group("sentiment").
		Scan(&sentiment)

	sentimentMap := make(map[string]int)
	for _, s := range sentiment {
		sentimentMap[s.Sentiment] = s.Count
	}

	var prompts []db.GeoPrompt
	db.DB.Where("project_id = ? AND active = ?", id, true).Order("id asc").Find(&prompts)

	var responses []db.GeoResponse
	db.DB.Where("run_id = ?", run.ID).Find(&responses)

	var mentions []db.GeoMention
	db.DB.Where("run_id = ?", run.ID).Find(&mentions)

	responsesByPrompt := map[uint][]db.GeoResponse{}
	for _, r := range responses {
		responsesByPrompt[r.PromptID] = append(responsesByPrompt[r.PromptID], r)
	}
	mentionByResponse := map[uint][]db.GeoMention{}
	for _, m := range mentions {
		mentionByResponse[m.ResponseID] = append(mentionByResponse[m.ResponseID], m)
	}

	type answerRow struct {
		Model      string   `json:"model"`
		Text       string   `json:"text"`
		NamedYou   bool     `json:"namedYou"`
		NamedBrands []string `json:"namedBrands"`
	}

	type perPromptRow struct {
		PromptID           uint        `json:"promptId"`
		Text               string      `json:"text"`
		Intent             string      `json:"intent"`
		Topic              string      `json:"topic"`
		ModelsChecked      []string    `json:"modelsChecked"`
		ModelsMentioned    []string    `json:"modelsMentioned"`
		Mentioned          bool        `json:"mentioned"`
		MentionCount       int         `json:"mentionCount"`
		BestCompetitor     string      `json:"bestCompetitor"`
		CompetitorMentions int         `json:"competitorMentions"`
		Gap                string      `json:"gap"`
		Answers            []answerRow `json:"answers"`
	}

	modelOrder := []string{"ChatGPT", "Claude", "Gemini", "Grok"}
	modelStatsMap := map[string]struct{ checked, named int }{}
	for _, m := range modelOrder {
		modelStatsMap[m] = struct{ checked, named int }{}
	}

	perPrompt := make([]perPromptRow, 0, len(prompts))
	mentionedPrompts := 0
	answersWithBrand := 0
	for _, p := range prompts {
		resps := responsesByPrompt[p.ID]
		modelsChecked := []string{}
		modelsMentioned := []string{}
		seenModel := map[string]bool{}
		compCounts := map[string]int{}
		mentioned := false
		answers := make([]answerRow, 0, len(resps))

		for _, r := range resps {
			model := friendlyGeoModel(r.Model)
			if !seenModel[model] {
				seenModel[model] = true
				modelsChecked = append(modelsChecked, model)
			}
			namedYou := false
			namedBrands := []string{}
			seenBrand := map[string]bool{}
			for _, m := range mentionByResponse[r.ID] {
				if m.IsTarget && m.Mentioned {
					namedYou = true
					mentioned = true
					if !containsString(modelsMentioned, model) {
						modelsMentioned = append(modelsMentioned, model)
					}
				}
				if !m.IsTarget && m.Mentioned && m.Brand != "" {
					compCounts[m.Brand]++
					if !seenBrand[strings.ToLower(m.Brand)] {
						seenBrand[strings.ToLower(m.Brand)] = true
						namedBrands = append(namedBrands, m.Brand)
					}
				}
			}
			if namedYou {
				answersWithBrand++
			}
			st := modelStatsMap[model]
			st.checked++
			if namedYou {
				st.named++
			}
			modelStatsMap[model] = st

			snippet := strings.TrimSpace(r.ResponseText)
			if len(snippet) > 420 {
				snippet = strings.TrimSpace(snippet[:420]) + "…"
			}
			answers = append(answers, answerRow{
				Model:       model,
				Text:        snippet,
				NamedYou:    namedYou,
				NamedBrands: namedBrands,
			})
		}

		// Stable model order in UI
		rank := map[string]int{"ChatGPT": 0, "Claude": 1, "Gemini": 2, "Grok": 3}
		sort.SliceStable(answers, func(i, j int) bool {
			ri, okI := rank[answers[i].Model]
			rj, okJ := rank[answers[j].Model]
			if !okI {
				ri = 99
			}
			if !okJ {
				rj = 99
			}
			return ri < rj
		})
		modelsChecked = orderedModels(modelsChecked, modelOrder)
		modelsMentioned = orderedModels(modelsMentioned, modelOrder)

		bestComp := ""
		bestCompN := 0
		totalComp := 0
		for brand, n := range compCounts {
			totalComp += n
			if n > bestCompN {
				bestCompN = n
				bestComp = brand
			}
		}

		gap := "missing"
		if mentioned && len(modelsMentioned) >= len(modelsChecked) && len(modelsChecked) > 0 {
			gap = "mentioned"
		} else if mentioned {
			gap = "partial"
		}
		if mentioned {
			mentionedPrompts++
		}

		topic := ""
		if p.Topic != nil {
			topic = *p.Topic
		}

		perPrompt = append(perPrompt, perPromptRow{
			PromptID:           p.ID,
			Text:               p.Text,
			Intent:             p.Intent,
			Topic:              topic,
			ModelsChecked:      modelsChecked,
			ModelsMentioned:    modelsMentioned,
			Mentioned:          mentioned,
			MentionCount:       len(modelsMentioned),
			BestCompetitor:     bestComp,
			CompetitorMentions: totalComp,
			Gap:                gap,
			Answers:            answers,
		})
	}

	modelStats := make([]gin.H, 0, len(modelOrder))
	for _, name := range modelOrder {
		st := modelStatsMap[name]
		rate := float32(0)
		if st.checked > 0 {
			rate = float32(st.named) / float32(st.checked)
		}
		modelStats = append(modelStats, gin.H{
			"model":   name,
			"checked": st.checked,
			"named":   st.named,
			"rate":    rate,
		})
	}

	// Live metrics from this run (prefer over stale project_metrics when present).
	totalAnswers := len(responses)
	liveMentionRate := float32(0)
	if totalAnswers > 0 {
		liveMentionRate = float32(answersWithBrand) / float32(totalAnswers)
	}
	var targetN, rivalN int64
	db.DB.Model(&db.GeoMention{}).Where("run_id = ? AND is_target = ? AND mentioned = ?", run.ID, true, true).Count(&targetN)
	db.DB.Model(&db.GeoMention{}).Where("run_id = ? AND is_target = ? AND mentioned = ?", run.ID, false, true).Count(&rivalN)
	liveSov := float32(0)
	if targetN+rivalN > 0 {
		liveSov = float32(rivalN) / float32(targetN+rivalN)
	}
	liveOwn := float32(0)
	if targetN+rivalN > 0 {
		liveOwn = float32(targetN) / float32(targetN+rivalN)
	}
	liveScore := float32(100) * (0.55*liveMentionRate + 0.20*liveOwn)
	if targetN == 0 && rivalN == 0 {
		liveScore = 0
	}
	// Blend in stored citation rate if available
	if metric.CitationRate != nil {
		liveScore = float32(100)*(0.55*liveMentionRate+0.25*(*metric.CitationRate)+0.20*liveOwn)
		if targetN == 0 && rivalN == 0 && *metric.CitationRate == 0 {
			liveScore = 0
		}
	}

	mentionRate := liveMentionRate
	competitorSov := liveSov
	geoScore := liveScore
	citationRate := float32(0)
	if metric.CitationRate != nil {
		citationRate = *metric.CitationRate
	}

	c.JSON(http.StatusOK, gin.H{
		"latestRun": run,
		"metrics": gin.H{
			"mentionRate":   mentionRate,
			"citationRate":  citationRate,
			"competitorSov": competitorSov,
			"geoScore":      geoScore,
			"breakdown":     jsonRawMessage(metric.Breakdown),
		},
		"competitorMentions": competitorMentions,
		"citationSources":    citationSources,
		"sentiment":          sentimentMap,
		"perPrompt":          perPrompt,
		"modelStats":         modelStats,
		"promptsChecked":     len(perPrompt),
		"promptsMentioned":   mentionedPrompts,
		"answersCollected":   totalAnswers,
		"answersMentioned":   answersWithBrand,
	})
}

func orderedModels(have []string, order []string) []string {
	out := make([]string, 0, len(have))
	seen := map[string]bool{}
	for _, want := range order {
		for _, h := range have {
			if h == want && !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	for _, h := range have {
		if !seen[h] {
			out = append(out, h)
		}
	}
	return out
}

func friendlyGeoModel(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(s, "claude"):
		return "Claude"
	case strings.Contains(s, "gemini"):
		return "Gemini"
	case strings.Contains(s, "grok") || strings.Contains(s, "xai") || strings.Contains(s, "spacex"):
		return "Grok"
	case strings.Contains(s, "perplexity") || strings.Contains(s, "sonar"):
		return "Grok" // legacy rows: Perplexity slot is now Grok
	case strings.Contains(s, "gpt") || strings.Contains(s, "chatgpt") || strings.Contains(s, "openai"):
		return "ChatGPT"
	default:
		if raw == "" {
			return "AI"
		}
		return raw
	}
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
