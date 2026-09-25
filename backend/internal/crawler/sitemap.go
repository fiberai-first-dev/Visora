package crawler

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// sitemapDefaults are the conventional sitemap locations. They are only used when
// robots.txt does not advertise any sitemap.
var sitemapDefaults = []string{
	"/sitemap.xml",
	"/sitemap_index.xml",
	"/sitemap-index.xml",
	"/wp-sitemap.xml",
	"/sitemap/sitemap.xml",
	"/sitemap1.xml",
}

const (
	// sitemapMaxBytes caps a single sitemap document we are willing to read.
	sitemapMaxBytes = 12 << 20
	// sitemapMaxDepth bounds how deep a <sitemapindex> chain is followed.
	sitemapMaxDepth = 3
)

// SitemapSeed is the outcome of sitemap discovery for one origin.
type SitemapSeed struct {
	// Sitemaps lists the sitemap documents that were successfully read.
	Sitemaps []string `json:"sitemaps"`
	// URLs lists the raw page URLs advertised by those sitemaps.
	URLs []string `json:"-"`
	// Errors records per-sitemap failures so the crawl stats stay honest.
	Errors []string `json:"errors,omitempty"`
}

// NewHTTPClient builds the plain HTTP client used for robots.txt and sitemap
// fetching. It is separate from the Colly collector so that sitemap discovery
// never consumes the page crawl budget.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: requestTimeout}
}

// rootURL converts any URL on the site into its scheme+host root.
func rootURL(origin string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Host == "" {
		return nil, false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, false
	}
	return &url.URL{Scheme: scheme, Host: u.Host}, true
}

// ParseSitemap extracts every <loc> from a sitemap document. It accepts both
// <sitemapindex> (which points at further sitemaps) and <urlset> (which points at
// pages) and is namespace agnostic so it works with every real world sitemap.
func ParseSitemap(data []byte) (childSitemaps []string, pageURLs []string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	var (
		stack  []string
		locBuf strings.Builder
		inLoc  bool
	)

	for {
		tok, tokErr := dec.Token()
		if tokErr == io.EOF {
			break
		}
		if tokErr != nil {
			return childSitemaps, pageURLs, tokErr
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			stack = append(stack, name)
			if name == "loc" {
				locBuf.Reset()
				inLoc = true
			}
		case xml.CharData:
			if inLoc {
				locBuf.Write(t)
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if name != "loc" {
				continue
			}
			inLoc = false
			loc := strings.TrimSpace(locBuf.String())
			locBuf.Reset()
			// After popping <loc> the new top of the stack is its parent:
			// <sitemap> inside a sitemap index, or <url> inside a urlset.
			owner := ""
			if len(stack) > 0 {
				owner = stack[len(stack)-1]
			}
			if loc == "" {
				continue
			}
			switch owner {
			case "sitemap":
				childSitemaps = append(childSitemaps, loc)
			case "url":
				pageURLs = append(pageURLs, loc)
			}
		}
	}

	return childSitemaps, pageURLs, nil
}

// CollectSitemapURLs performs full sitemap discovery for an origin. It starts from
// the sitemaps advertised in robots.txt, follows <sitemapindex> chains recursively,
// and finally falls back to conventional sitemap paths when nothing usable was
// advertised.
func CollectSitemapURLs(client *http.Client, origin string, advertisedSitemaps []string, extraSitemap string, maxURLs int) SitemapSeed {
	seed := SitemapSeed{}
	if maxURLs <= 0 {
		return seed
	}

	type node struct {
		url   string
		depth int
	}

	seen := map[string]bool{}
	queue := make([]node, 0, 8)

	push := func(raw string, depth int) {
		if raw == "" {
			return
		}
		norm, ok := NormalizeURL(raw)
		if !ok || seen[norm] {
			return
		}
		seen[norm] = true
		queue = append(queue, node{norm, depth})
	}

	root, hasRoot := rootURL(origin)

	drain := func() {
		for i := 0; i < len(queue) && len(seed.URLs) < maxURLs; i++ {
			item := queue[i]
			body, err := fetchBytes(client, item.url)
			if err != nil {
				seed.Errors = append(seed.Errors, item.url+": "+err.Error())
				continue
			}
			childSitemaps, pageURLs, parseErr := ParseSitemap(body)
			if parseErr != nil {
				seed.Errors = append(seed.Errors, item.url+": "+parseErr.Error())
				continue
			}
			seed.Sitemaps = append(seed.Sitemaps, item.url)
			for _, p := range pageURLs {
				if len(seed.URLs) >= maxURLs {
					break
				}
				if norm, ok := NormalizeURL(p); ok {
					seed.URLs = append(seed.URLs, norm)
				}
			}
			if item.depth < sitemapMaxDepth {
				for _, c := range childSitemaps {
					push(c, item.depth+1)
				}
			}
		}
		queue = queue[:0]
	}

	for _, s := range advertisedSitemaps {
		push(s, 0)
	}
	if extraSitemap != "" {
		push(extraSitemap, 0)
	}
	drain()

	if len(seed.URLs) == 0 && hasRoot {
		for _, p := range sitemapDefaults {
			push(root.JoinPath(p).String(), 0)
		}
		drain()
	}

	return seed
}

// fetchBytes performs a polite GET and transparently decompresses gzip payloads,
// which is how most large sitemaps are served.
func fetchBytes(client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/xml,text/xml,text/plain,*/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, sitemapMaxBytes))
	if err != nil {
		return nil, err
	}
	return maybeGunzip(body)
}

// maybeGunzip inflates a gzip payload when the body looks compressed.
func maybeGunzip(body []byte) ([]byte, error) {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, sitemapMaxBytes))
}
