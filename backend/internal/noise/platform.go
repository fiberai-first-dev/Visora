package noise

import "strings"

// Platform domains and marketplaces that show up in SERPs but are not
// real brand competitors for D2C / SaaS analysis.
var exact = map[string]bool{
	"youtube.com": true, "youtu.be": true, "reddit.com": true, "quora.com": true,
	"linkedin.com": true, "facebook.com": true, "fb.com": true, "meta.com": true,
	"instagram.com": true, "twitter.com": true, "x.com": true, "medium.com": true,
	"wikipedia.org": true, "pinterest.com": true, "tiktok.com": true,
	"play.google.com": true, "apps.apple.com": true, "apple.com": true,
	"google.com": true, "google.co.in": true, "google.co.uk": true,
	"shopify.com": true, "myshopify.com": true,
	"amazon.com": true, "amazon.in": true, "amazon.co.uk": true,
	"flipkart.com": true, "myntra.com": true, "ajio.com": true, "nykaa.com": true,
	"groww.in": true, "zerodha.com": true, "moneycontrol.com": true, "nseindia.com": true,
	"indiamart.com": true, "justdial.com": true,
	"capterra.com": true, "capterra.in": true, "g2.com": true,
	"softwareadvice.com": true, "getapp.com": true, "producthunt.com": true,
	"microsoft.com": true, "bing.com": true, "yahoo.com": true,
	"wordpress.com": true, "wix.com": true, "squarespace.com": true, "webflow.com": true,
	"github.com": true, "gitlab.com": true, "stackoverflow.com": true,
	"craigslist.org": true, "ebay.com": true, "etsy.com": true,
	"whatsapp.com": true, "telegram.org": true, "discord.com": true,
	"notion.so": true, "notion.site": true, "canva.com": true,
}

// Parent suffixes — any subdomain is noise (apps.shopify.com, maps.google.com, …).
var parents = []string{
	"google.com", "google.co.in", "google.co.uk",
	"shopify.com", "myshopify.com",
	"facebook.com", "fb.com", "meta.com", "instagram.com",
	"amazon.com", "amazon.in", "amazon.co.uk",
	"apple.com", "microsoft.com",
	"youtube.com", "youtu.be",
	"linkedin.com", "twitter.com", "x.com",
}

// IsPlatform reports whether domain is a marketplace, platform, social, or
// app-store host that must never be treated as a brand competitor.
func IsPlatform(domain string) bool {
	d := normalize(domain)
	if d == "" {
		return false
	}
	if exact[d] {
		return true
	}
	for _, p := range parents {
		if d == p || strings.HasSuffix(d, "."+p) {
			return true
		}
	}
	// Common platform subdomains even on other hosts.
	if strings.HasPrefix(d, "apps.") || strings.HasPrefix(d, "help.") ||
		strings.HasPrefix(d, "support.") || strings.HasPrefix(d, "docs.") ||
		strings.HasPrefix(d, "play.") || strings.HasPrefix(d, "store.") ||
		strings.HasPrefix(d, "accounts.") || strings.HasPrefix(d, "login.") ||
		strings.HasPrefix(d, "developers.") || strings.HasPrefix(d, "developer.") {
		return true
	}
	return false
}

func normalize(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	return d
}
