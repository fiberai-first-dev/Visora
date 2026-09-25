package contentgap

import (
	"strings"

	"visora-backend/internal/db"
)

// pageIndexEntry is one real page of the brand's own site, used to match an LLM
// topic cluster back to content the site already has.
type pageIndexEntry struct {
	URL   string
	Title string
	Text  string // lowercased URL + title + H1, used for keyword matching
}

// loadBrandPages builds an index of the pages from the latest completed brand
// crawl. Competitor pages are excluded: content gaps are about the brand's site.
func loadBrandPages(projectID uint) []pageIndexEntry {
	var run db.CrawlRun
	if err := db.DB.Where("project_id = ? AND status = 'done' AND competitor_id IS NULL", projectID).
		Order("id desc").First(&run).Error; err != nil {
		return nil
	}

	var pages []db.Page
	db.DB.Where("project_id = ? AND crawl_run_id = ? AND competitor_id IS NULL", projectID, run.ID).
		Order("depth asc, id asc").Limit(500).Find(&pages)

	index := make([]pageIndexEntry, 0, len(pages))
	for _, p := range pages {
		title := ""
		if p.Title != nil {
			title = *p.Title
		}
		entry := pageIndexEntry{
			URL:   p.URL,
			Title: title,
		}
		entry.Text = strings.ToLower(p.URL + " " + title + " " + p.H1)
		index = append(index, entry)
	}
	return index
}

// matchPages finds the real pages that already relate to a topic cluster so the
// UI can show "you have 2 of 5 relevant pages" instead of a placeholder.
func matchPages(index []pageIndexEntry, topic string, evidence []string, limit int) []map[string]string {
	keywords := topicKeywords(topic, evidence)
	out := []map[string]string{}
	if len(keywords) == 0 {
		return out
	}

	type scored struct {
		entry pageIndexEntry
		hits  int
	}
	var candidates []scored
	for _, entry := range index {
		hits := 0
		for _, kw := range keywords {
			if strings.Contains(entry.Text, kw) {
				hits++
			}
		}
		if hits > 0 {
			candidates = append(candidates, scored{entry: entry, hits: hits})
		}
	}

	// Best matches first, then shallowest URL as a stable tie break.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0; j-- {
			if candidates[j].hits > candidates[j-1].hits {
				candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
				continue
			}
			break
		}
	}

	for _, c := range candidates {
		if len(out) >= limit {
			break
		}
		out = append(out, map[string]string{
			"url":   c.entry.URL,
			"title": c.entry.Title,
		})
	}
	return out
}

// topicKeywords derives the search terms used to match existing content. Words
// shorter than four characters (prepositions, articles) are ignored.
func topicKeywords(topic string, evidence []string) []string {
	seen := map[string]bool{}
	var keywords []string

	add := func(text string) {
		for _, raw := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
		}) {
			if len(raw) < 4 || seen[raw] {
				continue
			}
			seen[raw] = true
			keywords = append(keywords, raw)
		}
	}

	add(topic)
	for _, q := range evidence {
		add(q)
	}
	if len(keywords) > 12 {
		keywords = keywords[:12]
	}
	return keywords
}
