package seo

import (
	"fmt"
	"sort"

	"visora-backend/internal/db"
	"visora-backend/internal/events"
)

// severityPriority ranks severities for the opportunity list. Higher wins.
var severityPriority = map[string]int{
	"critical": 4,
	"warning":  3,
	"notice":   2,
}

// IssueGroup is one real SEO rule aggregated across the site: how many pages it
// affects, where to look, and what to do about it.
type IssueGroup struct {
	RuleID           string `json:"rule_id"`
	Title            string `json:"title"`
	Severity         string `json:"severity"`
	Category         string `json:"category"`
	Detail           string `json:"detail"`
	Recommendation   string `json:"recommendation"`
	AffectedPages    int    `json:"affected_pages"`
	ExampleURL       string `json:"example_url"`
	Priority         int    `json:"priority"`
	MatchedURLs      []string `json:"matched_urls"`
}

// SummarizeIssues aggregates the SEO issues of the latest brand crawl into one
// entry per rule, ranked by severity and blast radius. This is the single source
// of truth for the technical SEO screen, the opportunities list and the
// recommendation engine.
func SummarizeIssues(projectID uint) ([]IssueGroup, uint, error) {
	var run db.CrawlRun
	if err := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", projectID).
		Order("id desc").First(&run).Error; err != nil {
		return []IssueGroup{}, 0, nil
	}

	var issues []db.SeoIssue
	if err := db.DB.Where("crawl_run_id = ?", run.ID).Order("rule_id asc, id asc").Find(&issues).Error; err != nil {
		return nil, run.ID, err
	}

	type bucket struct {
		group IssueGroup
		urls  map[string]bool
	}
	buckets := map[string]*bucket{}
	order := []string{}

	for _, issue := range issues {
		b, ok := buckets[issue.RuleID]
		if !ok {
			b = &bucket{
				group: IssueGroup{
					RuleID:         issue.RuleID,
					Title:          issue.Title,
					Severity:       issue.Severity,
					Category:       issue.Category,
					Detail:         issue.Detail,
					Recommendation: issue.Recommendation,
					Priority:       severityPriority[issue.Severity],
				},
				urls: map[string]bool{},
			}
			buckets[issue.RuleID] = b
			order = append(order, issue.RuleID)
		}
		if issue.URL != nil && *issue.URL != "" {
			if !b.urls[*issue.URL] {
				b.urls[*issue.URL] = true
			}
			if b.group.ExampleURL == "" {
				b.group.ExampleURL = *issue.URL
			}
		}
	}

	groups := make([]IssueGroup, 0, len(order))
	for _, ruleID := range order {
		b := buckets[ruleID]
		b.group.AffectedPages = len(b.urls)
		b.group.MatchedURLs = make([]string, 0, len(b.urls))
		for url := range b.urls {
			b.group.MatchedURLs = append(b.group.MatchedURLs, url)
		}
		sort.Strings(b.group.MatchedURLs)
		if len(b.group.MatchedURLs) > 10 {
			b.group.MatchedURLs = b.group.MatchedURLs[:10]
		}
		groups = append(groups, b.group)
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Priority != groups[j].Priority {
			return groups[i].Priority > groups[j].Priority
		}
		if groups[i].AffectedPages != groups[j].AffectedPages {
			return groups[i].AffectedPages > groups[j].AffectedPages
		}
		return groups[i].RuleID < groups[j].RuleID
	})

	return groups, run.ID, nil
}

// PrioritizeIssues ranks the real issues for a project and publishes the stage 11
// summary that the scan UI displays.
func PrioritizeIssues(projectID uint) error {
	groups, runID, err := SummarizeIssues(projectID)
	if err != nil {
		return err
	}

	counts := map[string]int{"critical": 0, "warning": 0, "notice": 0}
	for _, g := range groups {
		counts[g.Severity]++
	}

	top := make([]map[string]interface{}, 0, 5)
	for i, g := range groups {
		if i == 5 {
			break
		}
		top = append(top, map[string]interface{}{
			"rule_id":        g.RuleID,
			"title":          g.Title,
			"severity":       g.Severity,
			"affected_pages": g.AffectedPages,
			"example_url":    g.ExampleURL,
		})
	}

	events.PublishEvent(projectID, 11, "complete",
		fmt.Sprintf("Prioritized %d issues affecting %d pages", len(groups), affectedPageCount(runID)),
		map[string]interface{}{
			"crawl_run_id": runID,
			"issue_types":  len(groups),
			"by_severity":  counts,
			"top_issues":   top,
		})

	fmt.Printf("Prioritized %d issue types for project %d\n", len(groups), projectID)
	return nil
}

// affectedPageCount counts the unique URLs that carry at least one issue.
func affectedPageCount(runID uint) int {
	var count int64
	db.DB.Model(&db.SeoIssue{}).
		Where("crawl_run_id = ? AND url IS NOT NULL", runID).
		Distinct("url").
		Count(&count)
	return int(count)
}
