package seo

import (
	"encoding/json"
	"fmt"
	"strings"

	"visora-backend/internal/db"
	"visora-backend/internal/events"
)

func analyzePage(page db.Page) []db.SeoIssue {
	var issues []db.SeoIssue

	currentTitle := ""
	if page.Title != nil {
		currentTitle = *page.Title
	}
	currentMeta := ""
	if page.MetaDescription != nil {
		currentMeta = *page.MetaDescription
	}
	var h1s []string
	_ = json.Unmarshal([]byte(page.H1), &h1s)
	currentH1 := ""
	if len(h1s) > 0 {
		currentH1 = h1s[0]
	}

	addIssue := func(ruleID, severity, category, title, detail, action, targetField string) {
		data, _ := json.Marshal(map[string]interface{}{
			"target_field":   targetField,
			"current_title":  currentTitle,
			"current_meta":   currentMeta,
			"current_h1":     currentH1,
			"page_type":      page.PageType,
			"word_count":     page.WordCount,
			"body_excerpt":   truncateRunes(page.BodyText, 400),
		})
		issues = append(issues, db.SeoIssue{
			ProjectID:      page.ProjectID,
			CrawlRunID:     page.CrawlRunID,
			PageID:         &page.ID,
			URL:            &page.URL,
			RuleID:         ruleID,
			Severity:       severity,
			Category:       category,
			Title:          title,
			Detail:         detail,
			Recommendation: action,
			Data:           string(data),
		})
	}

	if page.Status != nil && *page.Status >= 400 {
		addIssue(
			"http_error",
			"critical",
			"crawlability",
			fmt.Sprintf("Page returned HTTP %d", *page.Status),
			"Search engines cannot access this page, which will prevent it from being indexed.",
			"Ensure the page exists or set up a proper 301 redirect.",
			"url",
		)
		return issues
	}

	if page.RobotsMeta != nil && strings.Contains(strings.ToLower(*page.RobotsMeta), "noindex") {
		addIssue(
			"noindex_tag",
			"warning",
			"indexability",
			"Page is blocked from indexing (noindex)",
			"A noindex directive was found. If this is intentional for utility pages, ignore this.",
			"Remove the noindex tag if you want this page to appear in Google.",
			"robots",
		)
	}

	if page.Title == nil || strings.TrimSpace(*page.Title) == "" {
		addIssue(
			"missing_title",
			"critical",
			"metadata",
			"Missing Title Tag",
			"The page does not have a title tag.",
			"Add a descriptive, unique title tag under 60 characters.",
			"title",
		)
	} else if len(*page.Title) > 70 {
		addIssue(
			"long_title",
			"notice",
			"metadata",
			"Title tag is too long",
			fmt.Sprintf("Title is %d characters long, which may be truncated in search results.", len(*page.Title)),
			"Keep title tags under 60 characters for optimal display.",
			"title",
		)
	}

	if page.MetaDescription == nil || strings.TrimSpace(*page.MetaDescription) == "" {
		addIssue(
			"missing_description",
			"warning",
			"metadata",
			"Missing Meta Description",
			"No meta description found. Search engines may generate a suboptimal snippet.",
			"Add a compelling meta description between 120-160 characters.",
			"meta",
		)
	}

	if len(h1s) == 0 {
		addIssue(
			"missing_h1",
			"warning",
			"content",
			"Missing H1 Heading",
			"The page lacks a primary H1 heading.",
			"Add exactly one H1 heading describing the page's main topic.",
			"h1",
		)
	} else if len(h1s) > 1 {
		addIssue(
			"multiple_h1",
			"notice",
			"content",
			"Multiple H1 Headings",
			fmt.Sprintf("Found %d H1 tags. While allowed in HTML5, one clear H1 is best practice.", len(h1s)),
			"Use a single H1 for the page title and H2/H3 for sub-sections.",
			"h1",
		)
	}

	if page.WordCount < 150 && page.PageType != "home" {
		addIssue(
			"thin_content",
			"notice",
			"content",
			"Thin Content",
			fmt.Sprintf("Page has only %d words of content.", page.WordCount),
			"Expand the page copy to provide more value and context to search engines.",
			"body",
		)
	}

	if page.ImagesMissingAlt > 0 {
		addIssue(
			"missing_image_alt",
			"notice",
			"images",
			"Images missing Alt Text",
			fmt.Sprintf("%d images are missing descriptive alt text.", page.ImagesMissingAlt),
			"Add descriptive alt text to improve accessibility and image search rankings.",
			"alt",
		)
	}

	if page.InternalLinks == 0 && page.PageType != "home" {
		addIssue(
			"orphan_page",
			"warning",
			"links",
			"No Outgoing Internal Links",
			"This page links to nowhere else on your site (dead end).",
			"Add internal links to related products, categories, or articles.",
			"links",
		)
	}

	if page.IsProductPage || page.PageType == "product" {
		if !page.HasProductSchema {
			addIssue(
				"missing_product_schema",
				"critical",
				"structured-data",
				"Product Schema Missing",
				"This is a product page but lacks valid Product structured data.",
				"Add Product schema containing the name, image, price, and availability.",
				"schema",
			)
		}
		if page.Price == nil {
			addIssue(
				"missing_product_price",
				"warning",
				"d2c-product",
				"Product Price Missing",
				"Could not detect a price for this product.",
				"Ensure the price is clearly visible in the DOM and structured data.",
				"schema",
			)
		}
		if !page.HasFaq && page.WordCount < 400 {
			addIssue(
				"missing_faq",
				"notice",
				"geo",
				"No FAQ block for AI answers",
				"Product pages with clear FAQ copy are more likely to be quoted by ChatGPT and Google.",
				"Add 4-6 buyer questions and short answers, plus FAQPage schema.",
				"faq",
			)
		}
	}

	return issues
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// AnalyzeRun audits all pages for a specific crawl run and generates SEO issues.
// Issues from a previous run of the same crawl are replaced, so re-running the
// analysis never double counts.
func AnalyzeRun(projectID uint, crawlRunID uint) error {
	fmt.Printf("Starting SEO analysis for Run %d\n", crawlRunID)

	var pages []db.Page
	if err := db.DB.Where("crawl_run_id = ?", crawlRunID).Find(&pages).Error; err != nil {
		return err
	}

	events.PublishEvent(projectID, 2, "progress", "Auditing technical health", map[string]int{"pages": len(pages)})

	db.DB.Where("crawl_run_id = ?", crawlRunID).Delete(&db.SeoIssue{})

	var allIssues []db.SeoIssue
	for _, page := range pages {
		allIssues = append(allIssues, analyzePage(page)...)
	}

	if len(allIssues) > 0 {
		if err := db.DB.CreateInBatches(&allIssues, 200).Error; err != nil {
			return err
		}
	}

	counts := map[string]int{}
	for _, issue := range allIssues {
		counts[issue.Severity]++
	}

	// Real technical signals from the crawl + pages (no invented scores).
	tech := collectTechnicalSignals(projectID, crawlRunID, pages, allIssues)

	events.PublishEvent(projectID, 2, "complete",
		fmt.Sprintf("Audited %d pages and found %d issues", len(pages), len(allIssues)),
		map[string]interface{}{
			"pages_audited":      len(pages),
			"issues_total":       len(allIssues),
			"issues_by_severity": counts,
			"technical":          tech,
			"avg_response_ms":    tech["avg_response_ms"],
			"https":              tech["https"],
			"sitemap":            tech["sitemap"],
			"robots_txt":         tech["robots_txt"],
			"missing_titles":     tech["missing_titles"],
			"missing_meta":       tech["missing_meta"],
			"missing_h1":         tech["missing_h1"],
			"missing_alt":        tech["missing_alt"],
			"slow_pages":         tech["slow_pages"],
		})

	fmt.Printf("Completed SEO analysis for Run %d. Audited %d pages, generated %d issues.\n", crawlRunID, len(pages), len(allIssues))
	return nil
}

func collectTechnicalSignals(projectID, crawlRunID uint, pages []db.Page, issues []db.SeoIssue) map[string]interface{} {
	var project db.Project
	_ = db.DB.First(&project, projectID)

	httpsOK := strings.HasPrefix(strings.ToLower(project.Website), "https://")
	var crawl db.CrawlRun
	_ = db.DB.First(&crawl, crawlRunID)

	robotsFound := false
	sitemapFound := false
	var stats map[string]interface{}
	if crawl.Stats != "" {
		_ = json.Unmarshal([]byte(crawl.Stats), &stats)
		if v, ok := stats["robots_txt_found"].(bool); ok {
			robotsFound = v
		}
		if n, ok := stats["sitemap_urls"].(float64); ok && n > 0 {
			sitemapFound = true
		}
		if arr, ok := stats["sitemaps"].([]interface{}); ok && len(arr) > 0 {
			sitemapFound = true
		}
	}

	totalMs := 0
	slow := 0
	missingTitle := 0
	missingMeta := 0
	missingH1 := 0
	missingAlt := 0
	for _, page := range pages {
		totalMs += page.ResponseMs
		if page.ResponseMs >= 2500 {
			slow++
		}
		if page.Title == nil || strings.TrimSpace(*page.Title) == "" {
			missingTitle++
		}
		if page.MetaDescription == nil || strings.TrimSpace(*page.MetaDescription) == "" {
			missingMeta++
		}
		var h1s []string
		_ = json.Unmarshal([]byte(page.H1), &h1s)
		if len(h1s) == 0 {
			missingH1++
		}
		missingAlt += page.ImagesMissingAlt
	}
	avgMs := 0
	if len(pages) > 0 {
		avgMs = totalMs / len(pages)
	}
	_ = issues // severity totals already published separately

	return map[string]interface{}{
		"https":           httpsOK,
		"sitemap":         sitemapFound,
		"robots_txt":      robotsFound,
		"avg_response_ms": avgMs,
		"slow_pages":      slow,
		"missing_titles":  missingTitle,
		"missing_meta":    missingMeta,
		"missing_h1":      missingH1,
		"missing_alt":     missingAlt,
		"pages_checked":   len(pages),
	}
}
