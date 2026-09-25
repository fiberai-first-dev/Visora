package crawler

import (
	"fmt"
	"net/url"
	"strings"

	"visora-backend/internal/db"
)

// LatestSitePages returns the brand's own pages from its most recent crawl.
// Older runs stay in the table, so ordering by depth/id alone can return a
// stale /privacy page instead of the homepage.
func LatestSitePages(projectID uint, limit int) []db.Page {
	var run db.CrawlRun
	q := db.DB.Where("project_id = ? AND competitor_id IS NULL", projectID)
	if err := db.DB.Where("project_id = ? AND competitor_id IS NULL AND pages_crawled > 0", projectID).
		Order("id desc").First(&run).Error; err == nil {
		q = q.Where("crawl_run_id = ?", run.ID)
	}
	var pages []db.Page
	if limit <= 0 {
		limit = 200
	}
	q.Where("(status IS NULL OR status < 400)").Order("depth asc, id asc").Limit(limit).Find(&pages)
	return pages
}

func pagePath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := strings.TrimRight(u.Path, "/")
	if p == "" {
		return "/"
	}
	return strings.ToLower(p)
}

var boilerplatePaths = []string{
	"/privacy", "/privacy-policy", "/terms", "/terms-of-service", "/tos", "/legal",
	"/cookie", "/cookies", "/refund", "/refund-policy", "/shipping-policy",
	"/login", "/signin", "/signup", "/register", "/cart", "/checkout", "/account",
	"/contact", "/careers", "/sitemap",
}

// IsBoilerplatePage is legal/account/utility pages we must never suggest
// ranking copy for.
func IsBoilerplatePage(raw string) bool {
	p := pagePath(raw)
	for _, b := range boilerplatePaths {
		if p == b || strings.HasPrefix(p, b+"/") || strings.HasSuffix(p, b) {
			return true
		}
	}
	return false
}

// HomePage is the "/" page of the latest crawl, else the shallowest non-legal page.
func HomePage(projectID uint) *db.Page {
	pages := LatestSitePages(projectID, 200)
	for i := range pages {
		if pagePath(pages[i].URL) == "/" {
			return &pages[i]
		}
	}
	for i := range pages {
		if !IsBoilerplatePage(pages[i].URL) {
			return &pages[i]
		}
	}
	if len(pages) > 0 {
		return &pages[0]
	}
	return nil
}

// FindPage resolves a URL (or path) to the latest crawled copy of that page.
func FindPage(projectID uint, raw string) *db.Page {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	want := raw
	if !strings.HasPrefix(want, "http") {
		want = "https://x" + want
	}
	wantPath := pagePath(want)
	for _, p := range LatestSitePages(projectID, 300) {
		if p.URL == raw || pagePath(p.URL) == wantPath {
			cp := p
			return &cp
		}
	}
	return nil
}

// CompetitorPage is the crawled copy of a rival page (exact URL, else that
// rival's shallowest page).
func CompetitorPage(projectID uint, pageURL, domain string) *db.Page {
	var page db.Page
	if pageURL != "" {
		if err := db.DB.Where("project_id = ? AND competitor_id IS NOT NULL AND url = ?", projectID, pageURL).
			Order("id desc").First(&page).Error; err == nil {
			return &page
		}
	}
	host := strings.TrimPrefix(strings.TrimSpace(domain), "www.")
	if host == "" {
		return nil
	}
	if err := db.DB.Where("project_id = ? AND competitor_id IS NOT NULL AND (url LIKE ? OR url LIKE ?)",
		projectID, "https://"+host+"%", "https://www."+host+"%").
		Order("depth asc, id desc").First(&page).Error; err == nil {
		return &page
	}
	return nil
}

// RivalContext describes who ranks for a query and what their page says.
func RivalContext(projectID uint, query string, max int) string {
	var serp []db.SerpResult
	db.DB.Where("project_id = ? AND query = ? AND is_own_domain = false", projectID, query).
		Order("position asc").Limit(3).Find(&serp)
	if len(serp) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range serp {
		fmt.Fprintf(&b, "Google #%d %s\n  title: %s\n  snippet: %s\n", r.Position, r.PageURL, r.PageTitle, r.PageSnippet)
	}
	if rival := CompetitorPage(projectID, serp[0].PageURL, serp[0].Domain); rival != nil {
		b.WriteString("Top rival page as crawled:\n")
		b.WriteString(PageOutline(rival, max))
	}
	return b.String()
}

var stopWords = map[string]bool{
	"the": true, "for": true, "and": true, "best": true, "top": true, "online": true,
	"india": true, "in": true, "of": true, "to": true, "a": true, "with": true,
	"app": true, "tool": true, "software": true, "near": true, "me": true, "free": true,
}

// BestPageFor picks the crawled page whose title/headings/copy match a buyer
// search best. Falls back to the homepage; never returns legal pages.
func BestPageFor(projectID uint, query string) *db.Page {
	pages := LatestSitePages(projectID, 300)
	var terms []string
	for _, w := range strings.Fields(strings.ToLower(query)) {
		w = strings.Trim(w, ".,!?\"'()")
		if len(w) < 3 || stopWords[w] {
			continue
		}
		terms = append(terms, w)
	}
	var best *db.Page
	bestScore := 0
	for i := range pages {
		p := &pages[i]
		if IsBoilerplatePage(p.URL) {
			continue
		}
		title := ""
		if p.Title != nil {
			title = strings.ToLower(*p.Title)
		}
		head := strings.ToLower(p.H1 + " " + p.H2)
		path := pagePath(p.URL)
		body := strings.ToLower(p.BodyText)
		score := 0
		for _, t := range terms {
			if strings.Contains(path, t) {
				score += 5
			}
			if strings.Contains(title, t) {
				score += 4
			}
			if strings.Contains(head, t) {
				score += 3
			}
			if strings.Contains(body, t) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	if best != nil && bestScore >= 4 {
		return best
	}
	return HomePage(projectID)
}
