package geo

import (
	"fmt"
	"regexp"
	"strings"
)

type PromptIdea struct {
	Text   string
	Intent string
	Topic  *string
}

// ClassifyIntent classifies a search keyword into a search intent
func ClassifyIntent(keyword string) string {
	k := strings.ToLower(keyword)
	
	transactionalRe := regexp.MustCompile(`^(buy|price|cheap|discount|order|shop|under ₹?\d+|coupon|deal)| (buy|price|under ₹?\d+|online)$`)
	commercialRe := regexp.MustCompile(`(best|top|vs|versus|compare|comparison|review|alternative|worth it|good)`)
	informationalRe := regexp.MustCompile(`\b(what|how|why|when|which|is|are|does|do|can|guide|tips)\b`)

	if transactionalRe.MatchString(k) {
		return "transactional"
	}
	if commercialRe.MatchString(k) {
		return "commercial"
	}
	if informationalRe.MatchString(k) {
		return "informational"
	}
	// Default to commercial for brand-like tokens
	return "commercial"
}

// GeneratePrompts creates realistic buyer-style prompts for AI search analysis
func GeneratePrompts(brand, category, country string, competitors, topics []string) []PromptIdea {
	cat := strings.ToLower(category)
	ctry := country
	if ctry == "" {
		ctry = "India"
	}
	
	// Limit topics to 5
	if len(topics) > 5 {
		topics = topics[:5]
	}

	topCompetitor := ""
	if len(competitors) > 0 {
		topCompetitor = competitors[0]
	}

	var prompts []PromptIdea

	add := func(text, intent string, topic *string) {
		prompts = append(prompts, PromptIdea{Text: text, Intent: intent, Topic: topic})
	}

	pCat := &cat
	pBrand := &brand

	add(fmt.Sprintf("What are the best %s brands in %s?", cat, ctry), "commercial", pCat)
	add(fmt.Sprintf("Top 10 %s brands in %s 2026", cat, ctry), "commercial", pCat)
	add(fmt.Sprintf("Which %s brand should I buy in %s?", cat, ctry), "commercial", pCat)
	add(fmt.Sprintf("Is %s a good %s brand?", brand, cat), "commercial", pBrand)
	add(fmt.Sprintf("What do reviews say about %s?", brand), "informational", pBrand)
	add(fmt.Sprintf("Is %s worth the price?", brand), "commercial", pBrand)
	add(fmt.Sprintf("%s vs other %s brands — which is better?", brand, cat), "commercial", pBrand)
	add(fmt.Sprintf("Where can I buy %s products in %s?", brand, ctry), "transactional", pBrand)
	add(fmt.Sprintf("Best affordable %s brands in %s", cat, ctry), "commercial", pCat)
	add(fmt.Sprintf("Best %s under ₹1000 in %s", cat, ctry), "transactional", pCat)
	add(fmt.Sprintf("%s recommendations for beginners", cat), "informational", pCat)
	add(fmt.Sprintf("What should I look for when buying %s?", cat), "informational", pCat)
	add(fmt.Sprintf("Which %s brands are popular in %s?", cat, ctry), "commercial", pCat)

	for _, topic := range topics {
		t := topic
		add(fmt.Sprintf("Best %s in %s", topic, ctry), "commercial", &t)
		add(fmt.Sprintf("Best %s under ₹2000", topic), "transactional", &t)
		add(fmt.Sprintf("%s for sensitive skin — what do you recommend?", topic), "commercial", &t)
	}

	if topCompetitor != "" {
		pComp := &topCompetitor
		add(fmt.Sprintf("%s vs %s — which should I choose?", brand, topCompetitor), "commercial", pBrand)
		add(fmt.Sprintf("Alternatives to %s in %s", topCompetitor, cat), "commercial", pComp)
	}

	// De-duplicate
	seen := make(map[string]bool)
	var uniquePrompts []PromptIdea
	for _, p := range prompts {
		k := strings.ToLower(p.Text)
		if !seen[k] {
			seen[k] = true
			uniquePrompts = append(uniquePrompts, p)
		}
	}

	return uniquePrompts
}
