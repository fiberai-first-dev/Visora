package crawler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"visora-backend/internal/db"
)

// Helper to convert slice to JSON string
func toJson(arr []string) string {
	b, _ := json.Marshal(arr)
	if string(b) == "null" {
		return "[]"
	}
	return string(b)
}

// walkJsonLd walks a JSON-LD object to find @type, price, availability, and FAQ.
func walkJsonLd(node interface{}, types *[]string, price **string, availability **string, hasReviews *bool, hasProductSchema *bool, faqs *[]map[string]string) {
	switch v := node.(type) {
	case []interface{}:
		for _, child := range v {
			walkJsonLd(child, types, price, availability, hasReviews, hasProductSchema, faqs)
		}
	case map[string]interface{}:
		typeNames := []string{}
		if t, ok := v["@type"]; ok {
			switch typeVal := t.(type) {
			case string:
				typeNames = append(typeNames, typeVal)
			case []interface{}:
				for _, tv := range typeVal {
					if ts, ok2 := tv.(string); ok2 {
						typeNames = append(typeNames, ts)
					}
				}
			}
		}
		for _, typeVal := range typeNames {
			*types = append(*types, typeVal)
			if typeVal == "Product" {
				*hasProductSchema = true
			}
			if typeVal == "FAQPage" || typeVal == "Question" {
				extractFaqFromNode(v, faqs)
			}
		}

		if offers, ok := v["offers"]; ok && *hasProductSchema {
			offersList := []interface{}{}
			switch o := offers.(type) {
			case []interface{}:
				offersList = o
			case map[string]interface{}:
				offersList = append(offersList, o)
			}
			for _, o := range offersList {
				if om, ok := o.(map[string]interface{}); ok {
					if *price == nil && om["price"] != nil {
						pStr := fmt.Sprintf("%v", om["price"])
						if curr, ok := om["priceCurrency"].(string); ok {
							pStr = pStr + " " + curr
						}
						*price = &pStr
					}
					if *availability == nil {
						if avail, ok := om["availability"].(string); ok {
							parts := strings.Split(avail, "/")
							last := parts[len(parts)-1]
							*availability = &last
						}
					}
				}
			}
		}

		if v["aggregateRating"] != nil || v["review"] != nil {
			*hasReviews = true
		}

		if graph, ok := v["@graph"]; ok {
			walkJsonLd(graph, types, price, availability, hasReviews, hasProductSchema, faqs)
		}
		if main, ok := v["mainEntity"]; ok {
			walkJsonLd(main, types, price, availability, hasReviews, hasProductSchema, faqs)
		}
	}
}

func extractFaqFromNode(v map[string]interface{}, faqs *[]map[string]string) {
	if len(*faqs) >= 12 {
		return
	}
	q, _ := v["name"].(string)
	if q == "" {
		q, _ = v["question"].(string)
	}
	ans := ""
	if accepted, ok := v["acceptedAnswer"].(map[string]interface{}); ok {
		ans, _ = accepted["text"].(string)
	}
	if ans == "" {
		ans, _ = v["text"].(string)
	}
	q = strings.TrimSpace(q)
	ans = strings.TrimSpace(ans)
	if q == "" {
		return
	}
	if len(ans) > 600 {
		ans = ans[:600] + "…"
	}
	*faqs = append(*faqs, map[string]string{"q": q, "a": ans})
}

func cleanBodyText(raw string) string {
	raw = strings.Join(strings.Fields(raw), " ")
	raw = strings.TrimSpace(raw)
	const max = 8000
	if len(raw) > max {
		return raw[:max] + "…"
	}
	return raw
}

// ReadableExcerpt returns one or two real sentences. Mashed nav/UI dumps
// (no punctuation, dozens of words glued together) become empty so we never
// show that blob as "current copy".
func ReadableExcerpt(raw string, max int) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if max <= 0 {
		max = 220
	}
	var picked []string
	size := 0
	for _, line := range strings.Split(raw, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if len(strings.Fields(line)) < 6 {
			continue
		}
		picked = append(picked, line)
		size += len(line)
		if len(picked) >= 2 || size >= max {
			break
		}
	}
	out := strings.Join(picked, " ")
	if len(out) > max {
		cut := strings.LastIndex(out[:max], " ")
		if cut < max/2 {
			cut = max
		}
		return out[:cut] + "…"
	}
	return out
}

// PageOutline is the page as a reader sees it — headings then copy lines —
// capped for LLM prompts.
func PageOutline(p *db.Page, max int) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	if p.Title != nil && *p.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", *p.Title)
	}
	if p.MetaDescription != nil && *p.MetaDescription != "" {
		fmt.Fprintf(&b, "Meta: %s\n", *p.MetaDescription)
	}
	for _, field := range []struct{ label, raw string }{{"H1", p.H1}, {"H2", p.H2}} {
		var hs []string
		_ = json.Unmarshal([]byte(field.raw), &hs)
		if len(hs) > 0 {
			fmt.Fprintf(&b, "%s: %s\n", field.label, strings.Join(hs, " | "))
		}
	}
	b.WriteString("Copy:\n")
	b.WriteString(p.BodyText)
	out := b.String()
	if max > 0 && len(out) > max {
		out = out[:max] + "…"
	}
	return out
}

var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "br": true,
	"dd": true, "details": true, "dialog": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true, "h1": true,
	"h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true,
	"hr": true, "li": true, "main": true, "nav": true, "ol": true, "p": true,
	"pre": true, "section": true, "summary": true, "table": true, "td": true,
	"th": true, "tr": true, "ul": true, "label": true, "option": true,
}

// inlineGapTags sit side by side visually but have no whitespace between them in
// the markup (badges, buttons, links in a row), so they need a separator too.
var inlineGapTags = map[string]bool{"a": true, "button": true, "span": true, "img": true, "strong": true, "em": true, "b": true}

// blockText reads text the way a person sees it: one line per block, spaces
// between adjacent inline pieces. Selection.Text() glues "Order desk" and
// "Every marketplace" into "Order deskEvery marketplace".
func blockText(sel *goquery.Selection) []string {
	var lines []string
	var cur strings.Builder
	flush := func() {
		line := strings.Join(strings.Fields(cur.String()), " ")
		cur.Reset()
		if line != "" {
			lines = append(lines, line)
		}
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			cur.WriteString(n.Data)
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			switch tag {
			case "script", "style", "noscript", "svg", "iframe", "template":
				return
			}
			if blockTags[tag] {
				flush()
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				flush()
				return
			}
			if inlineGapTags[tag] {
				cur.WriteByte(' ')
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			if inlineGapTags[tag] {
				cur.WriteByte(' ')
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range sel.Nodes {
		walk(n)
	}
	flush()
	return lines
}

func headingText(s *goquery.Selection) string {
	return strings.Join(blockText(s), " ")
}

func extractReadableBody(src *goquery.Selection) string {
	seen := map[string]bool{}
	var out []string
	total := 0
	for _, line := range blockText(src) {
		key := strings.ToLower(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, line)
		total += len(line) + 1
		if total > 12000 {
			break
		}
	}
	return strings.Join(out, "\n")
}

func ExtractPage(doc *goquery.Selection, pageUrl string, depth int) db.Page {
	page := db.Page{
		URL:       pageUrl,
		Depth:     depth,
		FetchedAt: time.Now(),
		PageType:  "other",
		H3:        "[]",
		FaqJson:   "[]",
		BodyText:  "",
	}

	title := strings.TrimSpace(doc.Find("head title").First().Text())
	if title != "" {
		page.Title = &title
	}

	if desc, exists := doc.Find("meta[name='description']").Attr("content"); exists {
		desc = strings.TrimSpace(desc)
		page.MetaDescription = &desc
	}

	if canonical, exists := doc.Find("link[rel='canonical']").Attr("href"); exists {
		page.Canonical = &canonical
	}

	if robots, exists := doc.Find("meta[name='robots']").Attr("content"); exists {
		page.RobotsMeta = &robots
	}

	if ogTitle, exists := doc.Find("meta[property='og:title']").Attr("content"); exists {
		page.OgTitle = &ogTitle
	}

	if ogDesc, exists := doc.Find("meta[property='og:description']").Attr("content"); exists {
		page.OgDescription = &ogDesc
	}

	if ogImage, exists := doc.Find("meta[property='og:image']").Attr("content"); exists {
		page.OgImage = &ogImage
	}

	var h1s []string
	doc.Find("h1").Each(func(i int, s *goquery.Selection) {
		text := headingText(s)
		if text != "" {
			h1s = append(h1s, text)
		}
	})
	page.H1 = toJson(h1s)

	var h2s []string
	doc.Find("h2").Each(func(i int, s *goquery.Selection) {
		text := headingText(s)
		if text != "" && len(h2s) < 20 {
			h2s = append(h2s, text)
		}
	})
	page.H2 = toJson(h2s)

	var h3s []string
	doc.Find("h3").Each(func(i int, s *goquery.Selection) {
		text := headingText(s)
		if text != "" && len(h3s) < 20 {
			h3s = append(h3s, text)
		}
	})
	page.H3 = toJson(h3s)

	// Prefer main/article copy; fall back to body. Store for AI wording suggestions.
	// Whole main region (not the first <article> card), minus site chrome.
	bodySrc := doc.Find("main, [role='main']").First()
	if bodySrc.Length() == 0 || len(strings.Fields(bodySrc.Text())) < 60 {
		bodySrc = doc.Find("body")
	}
	bodyClone := bodySrc.Clone()
	bodyClone.Find("script, style, noscript, nav, footer, iframe, form, svg, [aria-hidden='true'], [role='navigation']").Remove()
	bodyClone.Find("body > header").Remove()
	bodyText := extractReadableBody(bodyClone)
	page.BodyText = bodyText
	page.WordCount = len(strings.Fields(bodyText))

	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		page.Images++
		alt, _ := s.Attr("alt")
		if strings.TrimSpace(alt) == "" {
			page.ImagesMissingAlt++
		}
	})

	origin, _ := url.Parse(pageUrl)
	originHost := ""
	if origin != nil {
		originHost = origin.Host
	}

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			return
		}

		absURL, err := origin.Parse(href)
		if err != nil {
			return
		}

		if absURL.Host == originHost {
			page.InternalLinks++
		} else {
			page.ExternalLinks++
		}
	})

	var structuredDataTypes []string
	var faqs []map[string]string
	doc.Find("script[type='application/ld+json']").Each(func(i int, s *goquery.Selection) {
		raw := s.Text()
		var parsed interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			walkJsonLd(parsed, &structuredDataTypes, &page.Price, &page.Availability, &page.HasReviews, &page.HasProductSchema, &faqs)
		}
	})

	// DOM FAQ fallbacks (common accordion patterns).
	if len(faqs) == 0 {
		doc.Find("details, .faq-item, .accordion-item, [itemtype*='Question']").Each(func(i int, s *goquery.Selection) {
			if len(faqs) >= 8 {
				return
			}
			q := strings.TrimSpace(s.Find("summary, .faq-question, .accordion-title, [itemprop='name']").First().Text())
			a := strings.TrimSpace(s.Find(".faq-answer, .accordion-content, [itemprop='text']").First().Text())
			if q == "" {
				q = strings.TrimSpace(s.Find("h3, h4, strong").First().Text())
			}
			if q == "" {
				return
			}
			if len(a) > 400 {
				a = a[:400] + "…"
			}
			faqs = append(faqs, map[string]string{"q": q, "a": a})
		})
	}
	page.HasFaq = len(faqs) > 0
	page.FaqJson = toJsonPairs(faqs)

	typeMap := make(map[string]bool)
	var uniqueTypes []string
	for _, t := range structuredDataTypes {
		if !typeMap[t] {
			typeMap[t] = true
			uniqueTypes = append(uniqueTypes, t)
		}
	}
	page.StructuredDataTypes = toJson(uniqueTypes)

	if page.Price == nil {
		if metaPrice, exists := doc.Find("meta[property='product:price:amount']").Attr("content"); exists {
			page.Price = &metaPrice
		} else if ogPrice, exists := doc.Find("meta[property='og:price:amount']").Attr("content"); exists {
			page.Price = &ogPrice
		}
	}

	if !page.HasReviews {
		page.HasReviews = doc.Find("[itemprop='aggregateRating'], [itemprop='review'], #reviews, .product-reviews, .spr-review, .okeReviews-reviewsSummary, #shopify-product-reviews").Length() > 0
	}

	pagePath := ""
	if origin != nil {
		pagePath = strings.ToLower(origin.Path)
	}

	if pagePath == "/" || pagePath == "" {
		page.PageType = "home"
	} else if page.HasProductSchema || doc.Find("meta[property='og:type'][content='product']").Length() > 0 || strings.Contains(pagePath, "/product") {
		page.PageType = "product"
		page.IsProductPage = true
	} else if strings.Contains(pagePath, "/collection") || strings.Contains(pagePath, "/category") || strings.Contains(pagePath, "/shop") || strings.Contains(pagePath, "/solutions") {
		page.PageType = "category"
	} else if strings.Contains(pagePath, "/blog") || strings.Contains(pagePath, "/article") {
		page.PageType = "blog"
	}

	return page
}

func toJsonPairs(faqs []map[string]string) string {
	if len(faqs) == 0 {
		return "[]"
	}
	b, err := json.Marshal(faqs)
	if err != nil {
		return "[]"
	}
	return string(b)
}
