package competitor

import (
	"strings"

	"visora-backend/internal/db"
)

// SiteComparison is a like-for-like structural comparison between the brand's own
// site and one crawled competitor. Every field is aggregated from real crawled
// pages, so the numbers are verifiable.
type SiteComparison struct {
	Domain               string         `json:"domain"`
	BrandName            string         `json:"brand_name"`
	Classification       string         `json:"classification"`
	RelationshipType     string         `json:"relationship_type"`
	IsBrand              bool           `json:"is_brand"`
	CrawlRunID           uint           `json:"crawl_run_id"`
	PagesCrawled         int            `json:"pages_crawled"`
	ProductPages         int            `json:"product_pages"`
	CollectionPages      int            `json:"collection_pages"`
	ContentPages         int            `json:"content_pages"`
	PageTypes            map[string]int `json:"page_types"`
	PagesWithProductData int            `json:"pages_with_product_schema"`
	SchemaCoverage       float64        `json:"schema_coverage"`
	AvgWordCount         float64        `json:"avg_word_count"`
	MissingTitles        int            `json:"missing_titles"`
	MissingMeta          int            `json:"missing_meta_descriptions"`
	ImagesMissingAlt     int            `json:"images_missing_alt"`
	BrokenPages          int            `json:"broken_pages"`
}

// pageAggregate is the SQL projection used for one crawl run.
type pageAggregate struct {
	Pages            int     `gorm:"column:pages"`
	ProductPages     int     `gorm:"column:product_pages"`
	CollectionPages  int     `gorm:"column:collection_pages"`
	ContentPages     int     `gorm:"column:content_pages"`
	ProductSchema    int     `gorm:"column:product_schema"`
	AvgWords         float64 `gorm:"column:avg_words"`
	MissingTitles    int     `gorm:"column:missing_titles"`
	MissingMeta      int     `gorm:"column:missing_meta"`
	ImagesMissingAlt int     `gorm:"column:images_missing_alt"`
	BrokenPages      int     `gorm:"column:broken_pages"`
}

const pageAggregateSQL = `
SELECT
  COUNT(*)                                                                   AS pages,
  COALESCE(SUM(CASE WHEN page_type = 'product'  THEN 1 ELSE 0 END), 0)       AS product_pages,
  COALESCE(SUM(CASE WHEN page_type = 'category' THEN 1 ELSE 0 END), 0)       AS collection_pages,
  COALESCE(SUM(CASE WHEN page_type = 'blog'     THEN 1 ELSE 0 END), 0)       AS content_pages,
  COALESCE(SUM(CASE WHEN has_product_schema      THEN 1 ELSE 0 END), 0)      AS product_schema,
  COALESCE(AVG(NULLIF(word_count, 0)), 0)                                    AS avg_words,
  COALESCE(SUM(CASE WHEN title IS NULL OR title = '' THEN 1 ELSE 0 END), 0)  AS missing_titles,
  COALESCE(SUM(CASE WHEN meta_description IS NULL OR meta_description = '' THEN 1 ELSE 0 END), 0) AS missing_meta,
  COALESCE(SUM(images_missing_alt), 0)                                       AS images_missing_alt,
  COALESCE(SUM(CASE WHEN status IS NOT NULL AND status >= 400 THEN 1 ELSE 0 END), 0) AS broken_pages
FROM pages
WHERE crawl_run_id = ?`

// aggregateRun summarises the real pages of one crawl run.
func aggregateRun(crawlRunID uint) pageAggregate {
	var agg pageAggregate
	db.DB.Raw(pageAggregateSQL, crawlRunID).Scan(&agg)
	return agg
}

// pageTypesForRun returns the page type breakdown recorded during the crawl.
func pageTypesForRun(crawlRunID uint) map[string]int {
	var rows []struct {
		PageType string
		Total    int
	}
	db.DB.Raw(`SELECT page_type, COUNT(*) AS total FROM pages WHERE crawl_run_id = ? GROUP BY page_type`, crawlRunID).Scan(&rows)

	out := map[string]int{}
	for _, r := range rows {
		out[r.PageType] = r.Total
	}
	return out
}

// CompareTechnical compares the brand's own site against every competitor we have
// actually crawled, using real page-level evidence on both sides.
func CompareTechnical(projectID uint) ([]SiteComparison, error) {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return nil, err
	}

	out := []SiteComparison{}

	// The brand's own site.
	var brandRun db.CrawlRun
	if err := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", projectID).
		Order("id desc").First(&brandRun).Error; err == nil {
		agg := aggregateRun(brandRun.ID)
		out = append(out, buildComparison(project.Brand, project.Website, "own_site", "own_site", true, brandRun, agg))
	}

	// Every competitor we actually crawled.
	var candidates []db.CompetitorCandidate
	db.DB.Where("project_id = ? AND crawl_status = 'crawled'", projectID).
		Order("appearances DESC, id ASC").Find(&candidates)

	for _, c := range candidates {
		run, ok := latestCompetitorRun(projectID, c.ID)
		if !ok {
			continue
		}
		agg := aggregateRun(run.ID)

		var rel db.CompetitorRelationship
		db.DB.Where("project_id = ? AND competitor_id = ?", projectID, c.ID).First(&rel)

		name := c.BrandName
		if name == "" {
			name = c.Domain
		}
		out = append(out, buildComparison(name, c.Domain, c.Classification, rel.Type, false, run, agg))
	}

	return out, nil
}

// buildComparison assembles one comparison row.
func buildComparison(name, domainURL, classification, relationship string, isBrand bool, run db.CrawlRun, agg pageAggregate) SiteComparison {
	comparison := SiteComparison{
		Domain:               domainOf(domainURL),
		BrandName:            name,
		Classification:       classification,
		RelationshipType:     relationship,
		IsBrand:              isBrand,
		CrawlRunID:           run.ID,
		PagesCrawled:         agg.Pages,
		ProductPages:         agg.ProductPages,
		CollectionPages:      agg.CollectionPages,
		ContentPages:         agg.ContentPages,
		PageTypes:            pageTypesForRun(run.ID),
		PagesWithProductData: agg.ProductSchema,
		AvgWordCount:         agg.AvgWords,
		MissingTitles:        agg.MissingTitles,
		MissingMeta:          agg.MissingMeta,
		ImagesMissingAlt:     agg.ImagesMissingAlt,
		BrokenPages:          agg.BrokenPages,
	}

	if agg.ProductPages > 0 {
		comparison.SchemaCoverage = float64(agg.ProductSchema) / float64(agg.ProductPages) * 100
	}
	return comparison
}

// domainOf extracts a bare host from a stored website URL or domain.
func domainOf(raw string) string {
	d := strings.TrimSpace(raw)
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	return strings.TrimPrefix(strings.ToLower(d), "www.")
}
