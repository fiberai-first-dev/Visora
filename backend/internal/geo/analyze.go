package geo

import (
	"encoding/json"
	"regexp"
	"strings"

	"visora-backend/internal/db"
	"visora-backend/internal/noise"
)

var (
	Marketplaces = []string{"amazon", "flipkart", "myntra", "nykaa", "ajio", "purplle", "bigbasket", "zepto", "blinkit", "walmart", "ebay"}
	ReviewSites  = []string{"trustpilot", "trustradius", "g2.com", "consumerreports", "mouthshut", "sitejabber", "productreview"}
	NewsHints    = []string{"timesofindia", "indiatimes", "ndtv", "hindustantimes", "thehindu", "economictimes", "forbes", "vogue", "elle", "cosmopolitan", "bloomberg", "reuters", "yourstory", "inc42", "moneycontrol", "livemint", "news18", "indianexpress", "vogue.in", "bustle", "allure", "byrdie", "healthline", "verywellmind", "verywellhealth", "medicalnewstoday", "webmd", "cnn", "bbc"}

	reListItem = regexp.MustCompile(`(?m)^\s*(?:[-*•]|\d+[.)])\s+(.+)$`)
	// Split "Brand: desc", "Brand - desc", "Brand — desc"
	reSplitSep = regexp.MustCompile(`\s*[—–\-|:]\s+`)
)

func ClassifySource(domain, brandDomain string) string {
	d := strings.ToLower(domain)
	bd := strings.ToLower(brandDomain)
	if d == bd || (bd != "" && strings.HasSuffix(d, "."+bd)) {
		return "brand-website"
	}
	if strings.Contains(d, "reddit.") {
		return "reddit"
	}
	if strings.Contains(d, "youtube.") || strings.Contains(d, "youtu.be") {
		return "youtube"
	}
	for _, m := range Marketplaces {
		if strings.Contains(d, m) {
			return "marketplace"
		}
	}
	for _, m := range ReviewSites {
		if strings.Contains(d, m) {
			return "review-site"
		}
	}
	if strings.Contains(d, "quora.") || strings.Contains(d, "forum") || strings.Contains(d, "community") {
		return "forum"
	}
	if strings.Contains(d, "medium.") || strings.Contains(d, "substack.") || strings.Contains(d, "blogspot.") || strings.Contains(d, "wordpress.com") {
		return "blog"
	}
	for _, n := range NewsHints {
		if strings.Contains(d, n) {
			return "news"
		}
	}
	if strings.HasSuffix(d, ".news") || strings.HasSuffix(d, ".press") || strings.Contains(d, "blog") {
		return "news"
	}
	return "other"
}

func extractUrls(text string) []string {
	re := regexp.MustCompile(`https?:\/\/[^\s<>"')\]]+`)
	matches := re.FindAllString(text, -1)
	out := []string{}
	seen := make(map[string]bool)
	for _, m := range matches {
		urlStr := strings.TrimRight(m, ".,;:")
		if !seen[urlStr] {
			seen[urlStr] = true
			out = append(out, urlStr)
		}
	}
	return out
}

func textMentionsBrand(text, brand string) bool {
	brand = strings.TrimSpace(brand)
	if brand == "" || strings.EqualFold(brand, "Pending") {
		return false
	}
	escaped := regexp.QuoteMeta(brand)
	re := regexp.MustCompile(`(?i)\b` + escaped + `s?\b`)
	return re.MatchString(text)
}

func findContextSentence(text, brand string) string {
	parts := regexp.MustCompile(`[.!?]+`).Split(text, -1)
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if textMentionsBrand(s, brand) {
			if len(s) > 300 {
				return s[:300]
			}
			return s
		}
	}
	return ""
}

// domainStem turns "upfluence.com" / "www.fybud.com" into "upfluence" / "fybud".
func domainStem(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "www.")
	host = strings.Split(host, "/")[0]
	if host == "" {
		return ""
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return parts[0]
	}
	return host
}

// brandAliases returns matchable names for the target brand (display name + domain stem).
func brandAliases(brand, website string) []string {
	out := []string{}
	seen := map[string]bool{}
	push := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.EqualFold(s, "Pending") || seen[strings.ToLower(s)] {
			return
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
	}
	push(brand)
	push(domainStem(website))
	return out
}

// extractListedBrands pulls product/brand names from bullet-style AI answers.
func extractListedBrands(text string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, match := range reListItem.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 {
			continue
		}
		line := strings.TrimSpace(match[1])
		if line == "" {
			continue
		}
		name := line
		if parts := reSplitSep.Split(line, 2); len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			name = strings.TrimSpace(parts[0])
		}
		name = strings.Trim(name, " \"'`*._")
		// Drop trailing parentheticals: "Zendesk (support)"
		if i := strings.Index(name, "("); i > 2 {
			name = strings.TrimSpace(name[:i])
		}
		words := strings.Fields(name)
		if len(words) == 0 || len(words) > 4 {
			continue
		}
		if len(name) < 2 || len(name) > 48 {
			continue
		}
		lower := strings.ToLower(name)
		if lower == "none" || lower == "n/a" || strings.HasPrefix(lower, "http") {
			continue
		}
		// Skip sentence-y lines that aren't brand names.
		if strings.ContainsAny(name, ",.?!") {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		out = append(out, name)
	}
	return out
}

type BrandMention struct {
	Brand    string `json:"brand"`
	Position *int   `json:"position"`
	Context  string `json:"context"`
}

type GeoAnalysisResult struct {
	BrandsMentioned  []BrandMention
	Citations        []string
	Sentiment        string
	ReasoningSummary string
}

func AnalyzeResponse(responseText, targetBrand string, competitors []string, brandDomain string) GeoAnalysisResult {
	var finalBrands []BrandMention
	brandSeen := make(map[string]bool)

	pushBrand := func(name string, context string) {
		k := strings.ToLower(strings.TrimSpace(name))
		if k == "" || brandSeen[k] {
			return
		}
		brandSeen[k] = true
		finalBrands = append(finalBrands, BrandMention{
			Brand:   strings.TrimSpace(name),
			Context: context,
		})
	}

	aliases := brandAliases(targetBrand, brandDomain)
	targetKeys := map[string]bool{}
	for _, a := range aliases {
		targetKeys[strings.ToLower(a)] = true
	}

	// 1) Known target aliases
	for _, alias := range aliases {
		if textMentionsBrand(responseText, alias) {
			pushBrand(targetBrand, findContextSentence(responseText, alias))
			break
		}
	}

	// 2) Known competitor names / domain stems
	for _, candidate := range competitors {
		if textMentionsBrand(responseText, candidate) {
			pushBrand(candidate, findContextSentence(responseText, candidate))
		}
	}

	// 3) Brands listed in the answer (Grin, Upfluence, …) even if not in our DB yet
	for _, listed := range extractListedBrands(responseText) {
		lower := strings.ToLower(listed)
		if targetKeys[lower] {
			pushBrand(targetBrand, findContextSentence(responseText, listed))
			continue
		}
		if noise.IsPlatform(lower) || noise.IsPlatform(lower+".com") {
			continue
		}
		pushBrand(listed, findContextSentence(responseText, listed))
	}

	citations := extractUrls(responseText)
	targetMentioned := false
	for k := range brandSeen {
		if targetKeys[k] {
			targetMentioned = true
			break
		}
	}
	// Also if we pushed under canonical target brand name
	if brandSeen[strings.ToLower(strings.TrimSpace(targetBrand))] {
		targetMentioned = true
	}

	sentiment := "none"
	reasoning := "Brand not mentioned in answer."
	if targetMentioned {
		reasoning = "Brand mentioned in answer."
		sentiment = "neutral"
	}

	return GeoAnalysisResult{
		BrandsMentioned:  finalBrands,
		Citations:        citations,
		Sentiment:        sentiment,
		ReasoningSummary: reasoning,
	}
}

func CitationToRow(rawUrl, brandDomain string) db.GeoCitation {
	domain := rawUrl
	if strings.Contains(rawUrl, "://") {
		parts := strings.Split(rawUrl, "://")
		if len(parts) > 1 {
			domain = strings.Split(parts[1], "/")[0]
		}
	}
	bd := strings.TrimPrefix(brandDomain, "www.")
	sourceType := ClassifySource(domain, bd)
	isBrand := domain == bd || strings.HasSuffix(domain, "."+bd)

	return db.GeoCitation{
		URL:           rawUrl,
		Domain:        domain,
		SourceType:    sourceType,
		IsBrandDomain: isBrand,
	}
}

// competitorNamesForProject builds a match list from project JSON + crawled candidates.
func competitorNamesForProject(project db.Project) []string {
	var comps []string
	_ = json.Unmarshal([]byte(project.Competitors), &comps)

	var candidates []db.CompetitorCandidate
	db.DB.Where("project_id = ?", project.ID).Order("appearances desc, id asc").Limit(40).Find(&candidates)

	seen := map[string]bool{}
	out := []string{}
	push := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[strings.ToLower(s)] {
			return
		}
		if noise.IsPlatform(s) || noise.IsPlatform(strings.ToLower(s)+".com") {
			return
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
	}
	for _, c := range comps {
		push(c)
		push(domainStem(c))
	}
	for _, c := range candidates {
		push(c.BrandName)
		push(domainStem(c.Domain))
		// Prefer printable brand over raw domain when BrandName is just the domain
		if c.BrandName == "" || strings.Contains(c.BrandName, ".") {
			stem := domainStem(c.Domain)
			if len(stem) >= 2 {
				push(strings.ToUpper(stem[:1]) + stem[1:])
			}
		}
	}
	return out
}

// AnalyzeGeoRun analyzes all responses generated in a specific GEO run.
func AnalyzeGeoRun(projectID, geoRunID uint) error {
	var project db.Project
	if err := db.DB.First(&project, projectID).Error; err != nil {
		return err
	}

	comps := competitorNamesForProject(project)

	bd := project.Website
	if strings.Contains(bd, "://") {
		bd = strings.Split(bd, "://")[1]
		bd = strings.Split(bd, "/")[0]
	}
	bd = strings.TrimPrefix(bd, "www.")

	var responses []db.GeoResponse
	if err := db.DB.Where("run_id = ?", geoRunID).Find(&responses).Error; err != nil {
		return err
	}

	// Fresh analysis for this run — drop prior mention/citation rows so re-runs stay correct.
	db.DB.Where("run_id = ?", geoRunID).Delete(&db.GeoMention{})
	db.DB.Where("run_id = ?", geoRunID).Delete(&db.GeoCitation{})

	targetKeys := map[string]bool{}
	for _, a := range brandAliases(project.Brand, project.Website) {
		targetKeys[strings.ToLower(a)] = true
	}
	targetKeys[strings.ToLower(strings.TrimSpace(project.Brand))] = true

	for _, resp := range responses {
		analysis := AnalyzeResponse(resp.ResponseText, project.Brand, comps, bd)

		analysisJson, _ := json.Marshal(analysis)
		resp.Analysis = string(analysisJson)
		db.DB.Save(&resp)

		for _, m := range analysis.BrandsMentioned {
			isTarget := targetKeys[strings.ToLower(strings.TrimSpace(m.Brand))]
			db.DB.Create(&db.GeoMention{
				ProjectID:  projectID,
				RunID:      geoRunID,
				ResponseID: resp.ID,
				Brand:      m.Brand,
				IsTarget:   isTarget,
				Mentioned:  true,
				Position:   m.Position,
				Context:    &m.Context,
				Sentiment:  analysis.Sentiment,
			})
		}

		for _, rawUrl := range analysis.Citations {
			row := CitationToRow(rawUrl, bd)
			row.ProjectID = projectID
			row.RunID = geoRunID
			row.ResponseID = resp.ID
			db.DB.Create(&row)
		}
	}

	return nil
}
