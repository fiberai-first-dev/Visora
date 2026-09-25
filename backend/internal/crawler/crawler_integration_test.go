package crawler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"visora-backend/internal/db"
)

// fakeShop serves a realistic Shopify-style D2C storefront: a robots.txt that
// advertises a sitemap index, a sitemap that reveals a product which is not linked
// anywhere, real product/collection/blog pages, and a large pile of URLs that a
// correct crawl must never follow.
type fakeShop struct {
	mu       sync.Mutex
	requests []string
	server   *httptest.Server
	base     string
}

// nonPagePaths are the crawl-support requests that are not pages.
var nonPagePaths = map[string]bool{
	"/robots.txt":           true,
	"/sitemap-index.xml":    true,
	"/sitemap-products.xml": true,
}

func (s *fakeShop) record(r *http.Request) {
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	s.mu.Lock()
	s.requests = append(s.requests, path)
	s.mu.Unlock()
}

// allRequests returns every request path in the order the server saw them.
func (s *fakeShop) allRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.requests))
	copy(out, s.requests)
	return out
}

// pageRequests returns only HTML page requests, in order.
func (s *fakeShop) pageRequests() []string {
	out := []string{}
	for _, p := range s.allRequests() {
		if nonPagePaths[p] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// newFakeShop builds the fixture. The handler closures read s.base lazily, which
// is only populated after the test server has started.
func newFakeShop(t *testing.T) *fakeShop {
	t.Helper()

	shop := &fakeShop{}

	html := func(title string, links ...string) string {
		var body strings.Builder
		body.WriteString("<!doctype html><html><head><title>")
		body.WriteString(title)
		body.WriteString("</title></head><body><h1>")
		body.WriteString(title)
		body.WriteString("</h1>")
		for _, l := range links {
			body.WriteString(`<a href="` + l + `">link</a> `)
		}
		body.WriteString("</body></html>")
		return body.String()
	}

	sendHTML := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}

	pages := map[string]string{
		"/": html("Home",
			"/collections/skincare",
			"/collections/hair",
			"/products/serum",
			"/products/toner?utm_source=fb&utm_campaign=spring",
			"/about-us",
			"/blog/post-1",
			"/cart",
			"/cart/add?id=9",
			"/checkout",
			"/account/login",
			"/wishlist",
			"/wp-admin/admin.php",
			"/search?q=serum",
			"/collections/all?orderby=price",
			"/collections/all?page=9",
			"/tag/skincare",
			"/assets/app.css",
			"/uploads/banner.jpg",
			"/private/secret",
			"/index.html",
			"https://external.example.com/",
			"mailto:hello@example.com",
			"#top",
		),
		"/products/serum":      html("Vitamin C Serum", "/products/mask", "/collections/skincare"),
		"/products/mask":       html("Clay Mask", "/products/serum"),
		"/products/toner":      html("Rose Toner"),
		"/collections/skincare": html("Skincare", "/products/serum", "/products/toner"),
		"/collections/hair":    html("Hair Care", "/collections/skincare"),
		"/about-us":            html("About Us"),
		"/blog/post-1":         html("Serum Guide", "/blog/post-1?ref=home"),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		shop.record(r)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "User-agent: *\nDisallow: /private/\nDisallow: /wp-admin/\nSitemap: %s/sitemap-index.xml\n", shop.base)
	})

	mux.HandleFunc("/sitemap-index.xml", func(w http.ResponseWriter, r *http.Request) {
		shop.record(r)
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/sitemap-products.xml</loc></sitemap>
</sitemapindex>`, shop.base)
	})

	mux.HandleFunc("/sitemap-products.xml", func(w http.ResponseWriter, r *http.Request) {
		shop.record(r)
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/products/toner</loc></url>
  <url><loc>%s/products/serum</loc></url>
  <url><loc>%s/products/gone</loc></url>
  <url><loc>%s/cart/thanks</loc></url>
</urlset>`, shop.base, shop.base, shop.base, shop.base)
	})

	mux.HandleFunc("/products/gone", func(w http.ResponseWriter, r *http.Request) {
		shop.record(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, html("Not found"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		shop.record(r)
		if body, ok := pages[r.URL.Path]; ok {
			sendHTML(w, body)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, html("Not found"))
	})

	shop.server = httptest.NewServer(mux)
	shop.base = shop.server.URL
	t.Cleanup(shop.server.Close)
	return shop
}

// eventRecord is a captured scan event.
type eventRecord struct {
	Stage     int
	EventType string
	Message   string
	Data      map[string]interface{}
}

// crawlOutcome captures everything a test needs to assert on. The crawl workers
// call the sinks concurrently, so every mutation is guarded.
type crawlOutcome struct {
	mu                 sync.Mutex
	Pages              []db.Page
	Events             []eventRecord
	Finished           crawlStats
	FinishedPages      int
	FinishedDiscovered int
	FinishedCalled     bool
	LastError          string
}

func (o *crawlOutcome) addPage(page db.Page) {
	o.mu.Lock()
	o.Pages = append(o.Pages, page)
	o.mu.Unlock()
}

func (o *crawlOutcome) addEvent(record eventRecord) {
	o.mu.Lock()
	o.Events = append(o.Events, record)
	o.mu.Unlock()
}

func (o *crawlOutcome) finish(pages, discovered int, stats crawlStats, lastErr string) {
	o.mu.Lock()
	o.Finished = stats
	o.FinishedPages = pages
	o.FinishedDiscovered = discovered
	o.FinishedCalled = true
	o.LastError = lastErr
	o.mu.Unlock()
}

func (o *crawlOutcome) pageList() []db.Page {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]db.Page, len(o.Pages))
	copy(out, o.Pages)
	return out
}

func (o *crawlOutcome) eventList() []eventRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]eventRecord, len(o.Events))
	copy(out, o.Events)
	return out
}

// runTestCrawl drives the real crawl engine with in-memory sinks.
func runTestCrawl(t *testing.T, opts CrawlOptions) (*crawlOutcome, error) {
	t.Helper()

	outcome := &crawlOutcome{}
	deps := crawlDeps{
		startRun: func(CrawlOptions, time.Time) error { return nil },
		finishRun: func(o CrawlOptions, pages, discovered int, stats crawlStats, lastErr string) error {
			outcome.finish(pages, discovered, stats, lastErr)
			return nil
		},
		publish: func(projectID uint, stage int, eventType, message string, data interface{}) {
			record := eventRecord{Stage: stage, EventType: eventType, Message: message}
			if data != nil {
				if b, err := json.Marshal(data); err == nil {
					var m map[string]interface{}
					if json.Unmarshal(b, &m) == nil {
						record.Data = m
					}
				}
			}
			outcome.addEvent(record)
		},
		savePage: func(page db.Page) error {
			outcome.addPage(page)
			return nil
		},
	}

	err := crawlSite(opts, deps)
	return outcome, err
}

// pagePaths lists the paths of the pages the crawl actually stored.
func pagePaths(pages []db.Page) []string {
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		out = append(out, displayPath(p.URL))
	}
	return out
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

// TestCrawlNeverFollowsJunkURLs is the regression guard for "it crawls random
// pages": none of the cart, account, search, filter, asset, off-site, robots
// disallowed or index-file URLs may ever be requested.
func TestCrawlNeverFollowsJunkURLs(t *testing.T) {
	shop := newFakeShop(t)

	outcome, err := runTestCrawl(t, baseCrawlOptions(shop, 50))
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	forbidden := []string{
		"/cart",
		"/cart/add?id=9",
		"/cart/thanks",
		"/checkout",
		"/account/login",
		"/wishlist",
		"/wp-admin/admin.php",
		"/search?q=serum",
		"/tag/skincare",
		"/collections/all?orderby=price",
		"/collections/all?page=9",
		"/assets/app.css",
		"/uploads/banner.jpg",
		"/private/secret",
		"/index.html",
	}

	requested := shop.allRequests()
	for _, bad := range forbidden {
		if containsPath(requested, bad) {
			t.Errorf("crawl requested a URL it should have refused: %s", bad)
		}
	}

	for _, r := range requested {
		if strings.Contains(r, "external.example.com") {
			t.Errorf("crawl left the site: %s", r)
		}
	}

	// No page may be fetched twice, in any of its duplicate forms.
	counts := map[string]int{}
	for _, p := range shop.pageRequests() {
		counts[p]++
	}
	for path, n := range counts {
		if n > 1 {
			t.Errorf("page fetched %d times: %s", n, path)
		}
	}

	if outcome.Finished.Pages == 0 {
		t.Fatal("no pages were stored")
	}
	t.Logf("all server requests: %v", shop.allRequests())
	t.Logf("crawl stats: %+v", outcome.Finished)
}

// TestCrawlFollowsSitemapOnlyProducts proves products that are not linked from any
// page are still discovered, which is the whole point of parsing the sitemap.
func TestCrawlFollowsSitemapOnlyProducts(t *testing.T) {
	shop := newFakeShop(t)

	outcome, err := runTestCrawl(t, baseCrawlOptions(shop, 50))
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	paths := pagePaths(outcome.pageList())
	for _, want := range []string{"/", "/products/toner", "/products/serum", "/products/mask", "/collections/skincare"} {
		if !containsPath(paths, want) {
			t.Errorf("expected %s to be crawled, got %v", want, paths)
		}
	}

	if outcome.Finished.SitemapURLs == 0 {
		t.Error("sitemap discovery found no URLs")
	}
	if len(outcome.Finished.Sitemaps) == 0 {
		t.Error("no sitemap documents were read")
	}
}

// TestCrawlSpendsBudgetOnProductsFirst is the regression guard for "pages are
// random": with a small budget the crawl must spend it on products, never on
// /about-us or the blog.
func TestCrawlSpendsBudgetOnProductsFirst(t *testing.T) {
	shop := newFakeShop(t)

	outcome, err := runTestCrawl(t, baseCrawlOptions(shop, 4))
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	paths := pagePaths(outcome.pageList())
	t.Logf("budget=4 requested: %v", shop.pageRequests())
	t.Logf("budget=4 stored:    %v", paths)

	if len(paths) < 3 {
		t.Fatalf("expected the crawl to use its page budget, got %d: %v", len(paths), paths)
	}
	if paths[0] != "/" {
		t.Errorf("expected the homepage to be crawled first, got %v", paths)
	}
	if !containsPath(paths, "/products/toner") || !containsPath(paths, "/products/serum") {
		t.Errorf("expected both products to be crawled, got %v", paths)
	}
	if containsPath(paths, "/about-us") || containsPath(paths, "/blog/post-1") {
		t.Errorf("crawl spent its budget on low value pages: %v", paths)
	}
	if outcome.Finished.Requests > 4 {
		t.Errorf("crawl made %d requests with a budget of 4", outcome.Finished.Requests)
	}
}

// TestCrawlRespectsMaxPages hammers the budget and the duplicate guard together.
func TestCrawlRespectsMaxPages(t *testing.T) {
	for _, maxPages := range []int{1, 2, 3, 6} {
		t.Run(fmt.Sprintf("max_pages=%d", maxPages), func(t *testing.T) {
			shop := newFakeShop(t)

			outcome, err := runTestCrawl(t, baseCrawlOptions(shop, maxPages))
			if err != nil {
				t.Fatalf("crawl failed: %v", err)
			}

			if outcome.Finished.Requests > maxPages {
				t.Errorf("made %d requests with a budget of %d", outcome.Finished.Requests, maxPages)
			}
			if len(outcome.pageList()) > maxPages {
				t.Errorf("stored %d pages with a budget of %d", len(outcome.pageList()), maxPages)
			}
			if !outcome.FinishedCalled {
				t.Error("crawl run was never finalised")
			}
		})
	}
}

// TestCrawlReportsRealProgress proves the scan UI can be fed real URLs and counts
// instead of the random strings it used to animate.
func TestCrawlReportsRealProgress(t *testing.T) {
	shop := newFakeShop(t)

	outcome, err := runTestCrawl(t, baseCrawlOptions(shop, 50))
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	foundToner := false
	for _, ev := range outcome.eventList() {
		if ev.Stage != 1 {
			t.Errorf("crawl emitted an event for stage %d", ev.Stage)
		}
		if ev.Data == nil {
			continue
		}
		url, _ := ev.Data["url"].(string)
		if strings.HasSuffix(url, "/products/toner") {
			foundToner = true
			if status, _ := ev.Data["status"].(float64); status != 200 {
				t.Errorf("expected status 200 for /products/toner, got %v", ev.Data["status"])
			}
			if ev.Data["page_type"] != "product" {
				t.Errorf("expected page_type product, got %v", ev.Data["page_type"])
			}
			if crawled, _ := ev.Data["pages_crawled"].(float64); crawled < 1 {
				t.Errorf("expected a real pages_crawled count, got %v", ev.Data["pages_crawled"])
			}
		}
	}
	if !foundToner {
		t.Error("no scan event reported the real /products/toner URL")
	}

	last := outcome.eventList()[len(outcome.eventList())-1]
	if last.EventType != "complete" || last.Stage != 1 {
		t.Errorf("expected a stage 1 complete event last, got %+v", last)
	}
}

// TestCrawlRecordsHttpErrorsAsEvidence proves 404s found via the sitemap become
// real page rows with a status, so the technical audit can flag them.
func TestCrawlRecordsHttpErrorsAsEvidence(t *testing.T) {
	shop := newFakeShop(t)

	outcome, err := runTestCrawl(t, baseCrawlOptions(shop, 50))
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}

	found := false
	for _, p := range outcome.pageList() {
		if !strings.HasSuffix(p.URL, "/products/gone") {
			continue
		}
		found = true
		if p.Status == nil || *p.Status != 404 {
			t.Errorf("expected status 404 for /products/gone, got %v", p.Status)
		}
	}
	if !found {
		t.Error("the sitemap-only 404 page was not recorded as crawl evidence")
	}
}

// TestCrawlIsDeterministicAcrossConcurrency proves the crawled set is identical
// regardless of worker scheduling. (The frontier is always drained in priority
// order; only the tie-break among equal-priority URLs is concurrency sensitive.)
func TestCrawlIsDeterministicAcrossConcurrency(t *testing.T) {
	sequentialShop := newFakeShop(t)
	sequential, err := runTestCrawl(t, baseCrawlOptions(sequentialShop, 50))
	if err != nil {
		t.Fatalf("sequential crawl failed: %v", err)
	}

	concurrentShop := newFakeShop(t)
	opts := baseCrawlOptions(concurrentShop, 50)
	opts.Concurrency = 4
	concurrent, err := runTestCrawl(t, opts)
	if err != nil {
		t.Fatalf("concurrent crawl failed: %v", err)
	}

	seqPaths := pagePaths(sequential.Pages)
	conPaths := pagePaths(concurrent.Pages)
	if len(seqPaths) != len(conPaths) {
		t.Fatalf("crawl size differs by concurrency: %v vs %v", seqPaths, conPaths)
	}
	for _, p := range seqPaths {
		if !containsPath(conPaths, p) {
			t.Errorf("concurrent crawl missed %s (got %v)", p, conPaths)
		}
	}

	if sequential.Finished.Ignored != 0 {
		t.Errorf("sequential crawl had %d ignored requests, expected 0", sequential.Finished.Ignored)
	}
	if concurrent.Finished.Ignored != 0 {
		t.Errorf("concurrent crawl had %d ignored requests, expected 0", concurrent.Finished.Ignored)
	}
}

// TestCrawlFailsLoudlyWhenNothingIsCrawlable makes sure a broken origin is not
// silently reported as a successful crawl.
func TestCrawlFailsLoudlyWhenNothingIsCrawlable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	outcome, err := runTestCrawl(t, CrawlOptions{
		Origin:      server.URL,
		MaxPages:    5,
		MaxDepth:    2,
		Concurrency: 1,
		ProjectID:   1,
		CrawlRunID:  1,
	})
	if err == nil {
		t.Fatal("expected an error when no page can be crawled")
	}
	if outcome.Finished.Pages != 0 {
		t.Errorf("expected zero stored pages, got %d", outcome.Finished.Pages)
	}
	if outcome.LastError == "" {
		t.Error("expected the failure reason to be recorded on the run")
	}
}

// baseCrawlOptions builds the default crawl configuration for the fixture.
func baseCrawlOptions(shop *fakeShop, maxPages int) CrawlOptions {
	return CrawlOptions{
		Origin:      shop.base + "/",
		MaxPages:    maxPages,
		MaxDepth:    3,
		Concurrency: 1,
		DelayMs:     0,
		ProjectID:   1,
		CrawlRunID:  1,
	}
}
