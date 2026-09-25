package crawler

import (
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Crawl priority tiers. A lower number means "crawl this page first".
//
// The crawler always drains the frontier in priority order, so a 50 page budget
// is spent on products and collections before it is spent on /about or /contact.
const (
	PriorityHome       = 0
	PriorityProduct    = 1
	PriorityCollection = 2
	PriorityContent    = 3
	PriorityUtility    = 4
	PriorityOther      = 5
)

// maxCrawlPageNumber is the highest ?page=N value that is still crawled. Anything
// above it is considered pagination spam and is skipped.
const maxCrawlPageNumber = 3

// trackingParams are query parameters that never change the rendered content of a
// page. They are stripped during normalization so that campaign links, ad clicks
// and analytics wrappers all dedupe onto the same canonical URL.
var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true, "utm_term": true,
	"utm_content": true, "utm_id": true, "utm_name": true, "utm_reader": true,
	"utm_referrer": true, "utm_social": true, "utm_social-type": true,
	"fbclid": true, "gclid": true, "gbraid": true, "wbraid": true,
	"msclkid": true, "dclid": true, "yclid": true, "twclid": true, "igshid": true,
	"mc_cid": true, "mc_eid": true, "_ga": true, "_gl": true, "_hsenc": true,
	"_hsmi": true, "oly_anon_id": true, "oly_enc_id": true, "sc_cid": true,
	"s_kwcid": true, "srsltid": true, "ref": true, "ref_src": true,
	"referrer": true, "aff": true, "affid": true, "affiliate": true,
}

// skipQueryKeys are query parameters that turn a URL into a functional view
// (cart action, sort order, filter facet, internal search) instead of content.
var skipQueryKeys = map[string]bool{
	"add-to-cart": true, "add_to_cart": true, "remove_item": true, "quantity": true,
	"orderby": true, "sort": true, "sort_by": true, "sortby": true,
	"filter": true, "filters": true, "min_price": true, "max_price": true,
	"search": true, "s": true, "q": true, "query": true, "keyword": true,
	"replytocom": true, "preview": true, "preview_nonce": true, "wpmp_switcher": true,
	"variant": true, "view": true, "grid_list": true, "page_size": true,
	"per_page": true, "color": true, "colour": true, "size": true,
	"material": true, "redirect_to": true, "redirect": true, "return_url": true,
}

// paginationQueryKeys are the query parameters used for archive pagination. Only
// the first few pages are worth crawling, the rest is duplicate content.
var paginationQueryKeys = []string{"page", "paged", "page_num", "pagenumber", "pageindex", "page_number"}

// skipSegments are path segments that never contain indexable content.
var skipSegments = map[string]bool{
	"cart": true, "checkout": true, "basket": true, "bag": true,
	"account": true, "accounts": true, "my-account": true, "profile": true,
	"login": true, "logout": true, "signin": true, "signup": true, "register": true,
	"wishlist": true, "compare": true, "user": true, "orders": true,
	"wp-admin": true, "wp-json": true, "wp-content": true, "wp-includes": true,
	"xmlrpc.php": true, "feed": true, "rss": true, "atom": true, "cdn-cgi": true,
	"admin": true, "api": true, "graphql": true, "search": true, "tag": true,
	"tags": true, "author": true, "authors": true, "print": true, "amp": true,
	"share": true, "track": true, "out": true, "go": true, "unsubscribe": true,
	"preferences": true, "webhooks": true, "webhook": true, "payments": true,
	"payment": true, "session": true, "sessions": true, "email": true, "invite": true,
	"gift_cards": true, "gift-cards": true, "gift_card": true,
}

// skipPrefixes are path prefixes that never contain indexable content. The check
// is boundary aware (see pathHasAnyPrefix) so "/shop" does not match
// "/shopping-guide".
var skipPrefixes = []string{
	"/wp-admin", "/wp-json", "/wp-login", "/wp-content", "/wp-includes",
	"/cdn-cgi", "/cart", "/checkout", "/account", "/my-account", "/login",
	"/register", "/signup", "/signin", "/wishlist", "/tools/", "/admin",
	"/api/", "/graphql", "/search", "/tag/", "/tags/", "/author/", "/authors/",
	"/feed", "/rss", "/amp/", "/print/", "/share/", "/track/", "/out/", "/go/",
	"/unsubscribe", "/preferences", "/webhook", "/payment", "/gift_card",
	"/session", "/invite", "/404", "/500", "/error", "/cgi-bin", "/.well-known",
}

// skipExtensions are file extensions that are never HTML documents.
var skipExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".avif": true, ".svg": true, ".ico": true, ".bmp": true, ".tif": true,
	".tiff": true, ".jfif": true, ".heic": true, ".css": true, ".js": true,
	".mjs": true, ".cjs": true, ".json": true, ".xml": true, ".rss": true,
	".atom": true, ".txt": true, ".csv": true, ".pdf": true, ".zip": true,
	".rar": true, ".gz": true, ".tar": true, ".7z": true, ".doc": true,
	".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".wmv": true,
	".webm": true, ".ogg": true, ".m4a": true, ".wav": true, ".swf": true,
	".exe": true, ".dmg": true, ".apk": true, ".ipa": true, ".ics": true,
	".vcf": true, ".sql": true, ".map": true,
}

// indexFiles are default documents that are equivalent to the directory URL.
var indexFiles = []string{
	"/index.html", "/index.htm", "/index.php", "/index.asp", "/index.aspx",
	"/index.jsp", "/default.html", "/default.aspx",
}

var productPrefixes = []string{
	"/products", "/product", "/p", "/item", "/items", "/dp", "/pd", "/buy",
	"/product-details", "/shop-product", "/store/products",
}

var collectionPrefixes = []string{
	"/collections", "/collection", "/categories", "/category", "/c", "/shop",
	"/store", "/range", "/browse", "/department", "/all-products", "/all",
	"/shop-all", "/product-category", "/product-categories",
}

var contentPrefixes = []string{
	"/blogs", "/blog", "/articles", "/article", "/guides", "/guide", "/learn",
	"/faq", "/faqs", "/help", "/news", "/recipes", "/recipe", "/magazine",
	"/journal", "/insights", "/resources", "/blog-post", "/posts", "/post",
	"/pages", "/knowledge", "/academy", "/glossary",
}

var utilityPrefixes = []string{
	"/about", "/contact", "/support", "/shipping", "/returns", "/refund",
	"/exchange", "/policy", "/policies", "/privacy", "/terms", "/conditions",
	"/careers", "/jobs", "/stores", "/locations", "/sitemap", "/track-order",
	"/order-status", "/wholesale", "/bulk", "/distributor", "/investor",
	"/press", "/media", "/csr", "/sustainability", "/security", "/accessibility",
}

// NormalizeURL canonicalizes a URL for dedupe and queueing. It returns ok=false
// when the URL can never be a crawlable HTML page (unsupported scheme, missing
// host, embedded credentials).
func NormalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if u.Host == "" || u.User != nil {
		return "", false
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	host = strings.TrimPrefix(host, "www.")
	// A trailing dot is a valid but non-canonical FQDN form.
	host = strings.TrimSuffix(host, ".")

	if port := u.Port(); port != "" {
		if !((scheme == "http" && port == "80") || (scheme == "https" && port == "443")) {
			host = host + ":" + port
		}
	}

	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	// Collapse repeated slashes produced by template bugs.
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	// /index.html and friends are the same resource as the directory itself.
	lowerPath := strings.ToLower(p)
	for _, idx := range indexFiles {
		if strings.HasSuffix(lowerPath, idx) {
			p = p[:len(p)-len(idx)]
			break
		}
	}
	if p == "" {
		p = "/"
	}
	// Drop the trailing slash so /shop and /shop/ dedupe onto one entry.
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		p = "/"
	}

	cleaned := url.Values{}
	for k, vals := range u.Query() {
		key := strings.ToLower(k)
		if trackingParams[key] {
			continue
		}
		for _, v := range vals {
			cleaned.Add(key, strings.TrimSpace(v))
		}
	}

	out := scheme + "://" + host + p
	if len(cleaned) > 0 {
		// url.Values.Encode sorts by key, which keeps normalization deterministic.
		out += "?" + cleaned.Encode()
	}
	return out, true
}

// ClassifyURL decides whether a normalized URL should be crawled and, if so, how
// urgently. When the URL is not worth crawling, reason is a non-empty, human
// readable explanation that is surfaced in the crawl stats.
func ClassifyURL(normalized string) (priority int, reason string) {
	u, err := url.Parse(normalized)
	if err != nil {
		return 0, "unparseable url"
	}

	lowerPath := strings.ToLower(u.Path)
	if lowerPath == "" || lowerPath == "/" {
		return PriorityHome, ""
	}

	if ext := strings.ToLower(path.Ext(lowerPath)); ext != "" && skipExtensions[ext] {
		return 0, "non-html file " + ext
	}

	for _, seg := range strings.Split(strings.Trim(lowerPath, "/"), "/") {
		if skipSegments[seg] {
			return 0, "excluded path segment /" + seg
		}
	}

	if prefix, blocked := pathHasAnyPrefix(lowerPath, skipPrefixes); blocked {
		return 0, "excluded path prefix " + prefix
	}

	query := u.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if skipQueryKeys[lower] {
			return 0, "excluded query parameter " + key
		}
		// Shopify/Liquid faceted filters: filter.v.price, filter.p.vendor, filter[...]
		if strings.HasPrefix(lower, "filter.") || strings.HasPrefix(lower, "filter[") || strings.HasPrefix(lower, "facet") {
			return 0, "excluded filter facet " + key
		}
	}
	for _, key := range paginationQueryKeys {
		if v := query.Get(key); v != "" {
			if n, convErr := strconv.Atoi(v); convErr == nil && n > maxCrawlPageNumber {
				return 0, "deep pagination"
			}
		}
	}

	switch {
	case pathHasAnyPrefixBool(lowerPath, productPrefixes):
		return PriorityProduct, ""
	case pathHasAnyPrefixBool(lowerPath, collectionPrefixes):
		return PriorityCollection, ""
	case pathHasAnyPrefixBool(lowerPath, contentPrefixes):
		return PriorityContent, ""
	case pathHasAnyPrefixBool(lowerPath, utilityPrefixes):
		return PriorityUtility, ""
	}
	return PriorityOther, ""
}

// pathHasAnyPrefixBool is the boolean form of pathHasAnyPrefix.
func pathHasAnyPrefixBool(p string, prefixes []string) bool {
	_, ok := pathHasAnyPrefix(p, prefixes)
	return ok
}

// pathHasAnyPrefix reports whether p starts with one of the prefixes, respecting
// path boundaries ("/shop" matches "/shop" and "/shop/x", not "/shopping-guide").
func pathHasAnyPrefix(p string, prefixes []string) (string, bool) {
	for _, pre := range prefixes {
		if !strings.HasPrefix(p, pre) {
			continue
		}
		if len(p) == len(pre) || strings.HasSuffix(pre, "/") {
			return pre, true
		}
		switch p[len(pre)] {
		case '/', '.', '-':
			return pre, true
		}
	}
	return "", false
}

// isHTMLContentType reports whether an HTTP Content-Type header describes an HTML
// document. An empty header is treated as HTML so servers which omit it are still
// parsed.
func isHTMLContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return true
	}
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "text/html", "application/xhtml+xml", "application/xhtml", "text/xhtml":
		return true
	}
	return false
}
