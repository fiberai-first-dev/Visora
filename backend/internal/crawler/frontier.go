package crawler

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"visora-backend/internal/db"
)

const (
	// userAgent identifies Visora honestly to every host we touch.
	userAgent = "VisoraBot/1.0 (+https://visora.app/bot)"
	// requestTimeout bounds a single page fetch.
	requestTimeout = 20 * time.Second
	// maxBodySize caps the HTML we are willing to parse (3 MB).
	maxBodySize = 3 << 20
	// maxSitemapURLs caps how many sitemap URLs are queued for one crawl.
	maxSitemapURLs = 2000
)

// CrawlOptions describes one crawl. All bounds are enforced strictly.
type CrawlOptions struct {
	Origin       string
	MaxPages     int
	MaxDepth     int
	Concurrency  int
	DelayMs      int
	ProjectID    uint
	CrawlRunID   uint
	CompetitorID *uint
	// SeedSitemap optionally points at an explicit sitemap the user supplied.
	SeedSitemap string
	// SeedURLs are fetched first (e.g. the rival pages that actually rank on Google).
	SeedURLs []string
}

// frontierItem is one queued URL plus the signals used to order the queue.
type frontierItem struct {
	url      string
	priority int
	depth    int
	seq      int
}

// frontier is a min-heap: lowest priority tier first, then shallowest page, then
// discovery order. This is what makes a crawl reproducible instead of accidental.
type frontier []frontierItem

func (f frontier) Len() int { return len(f) }

func (f frontier) Less(i, j int) bool {
	if f[i].priority != f[j].priority {
		return f[i].priority < f[j].priority
	}
	if f[i].depth != f[j].depth {
		return f[i].depth < f[j].depth
	}
	return f[i].seq < f[j].seq
}

func (f frontier) Swap(i, j int) { f[i], f[j] = f[j], f[i] }

func (f *frontier) Push(x interface{}) { *f = append(*f, x.(frontierItem)) }

func (f *frontier) Pop() interface{} {
	old := *f
	n := len(old)
	item := old[n-1]
	*f = old[:n-1]
	return item
}

// crawlStats is the truthful, machine readable summary stored on the CrawlRun.
type crawlStats struct {
	Requests       int            `json:"requests"`
	Pages          int            `json:"pages"`
	ErrorPages     int            `json:"error_pages"`
	Discovered     int            `json:"discovered"`
	Duplicates     int            `json:"duplicate_links"`
	Skipped        int            `json:"skipped_links"`
	SkippedReasons map[string]int `json:"skipped_reasons,omitempty"`
	Ignored        int            `json:"ignored_requests"`
	DepthBlocked   int            `json:"depth_blocked"`
	Errors         int            `json:"request_errors"`
	ByType         map[string]int `json:"pages_by_type"`
	Sitemaps       []string       `json:"sitemaps"`
	SitemapURLs    int            `json:"sitemap_urls"`
	RobotsFound    bool           `json:"robots_txt_found"`
	RobotsRules    int            `json:"robots_disallow_rules"`
	DurationMs     int64          `json:"duration_ms"`
	LastError      string         `json:"last_error,omitempty"`
}

// crawlState holds the shared frontier and counters. Every field is guarded by mu.
type crawlState struct {
	mu          sync.Mutex
	cond        *sync.Cond
	seen        map[string]bool
	queue       frontier
	seq         int
	requests    int
	pages       int
	errorPages  int
	inFlight    int
	duplicates  int
	skipped     int
	skipReasons map[string]int
	ignored     int
	depthStop   int
	errors      int
	byType      map[string]int
	lastErr     string
}

func newCrawlState() *crawlState {
	s := &crawlState{
		seen:        make(map[string]bool),
		byType:      make(map[string]int),
		skipReasons: make(map[string]int),
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// push queues a URL unless it has already been seen. Returns true when queued.
func (s *crawlState) push(rawURL string, priority, depth int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[rawURL] {
		s.duplicates++
		return false
	}
	s.seen[rawURL] = true
	s.seq++
	heap.Push(&s.queue, frontierItem{url: rawURL, priority: priority, depth: depth, seq: s.seq})
	s.cond.Broadcast()
	return true
}

// reserve pops the next URL and reserves one unit of the page budget. It returns
// ok=false when the crawl is finished (queue drained or the budget is exhausted).
func (s *crawlState) reserve(maxPages, concurrency int) (frontierItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		queueEmpty := len(s.queue) == 0
		if queueEmpty && s.inFlight == 0 {
			return frontierItem{}, false
		}
		if s.requests >= maxPages && s.inFlight == 0 {
			return frontierItem{}, false
		}
		if !queueEmpty && s.inFlight < concurrency && s.requests < maxPages {
			item := heap.Pop(&s.queue).(frontierItem)
			s.requests++
			s.inFlight++
			return item, true
		}
		s.cond.Wait()
	}
}

// release marks one reserved request as finished and wakes the dispatcher.
func (s *crawlState) release() {
	s.mu.Lock()
	s.inFlight--
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *crawlState) recordPage(pageType string) {
	if pageType == "" {
		pageType = "other"
	}
	s.mu.Lock()
	s.pages++
	s.byType[pageType]++
	s.mu.Unlock()
}

// recordErrorPage counts a page row that only exists because the URL could not be
// served as an HTML page, so a completely broken site can never look like a
// successful crawl.
func (s *crawlState) recordErrorPage() {
	s.mu.Lock()
	s.pages++
	s.errorPages++
	s.byType["error"]++
	s.mu.Unlock()
}

func (s *crawlState) addSkipped(n int) {
	s.mu.Lock()
	s.skipped += n
	s.mu.Unlock()
}

// noteSkipped records why a link was refused so the crawl stats can explain it.
func (s *crawlState) noteSkipped(reason string) {
	if reason == "" {
		reason = "unknown"
	}
	s.mu.Lock()
	s.skipped++
	s.skipReasons[reason]++
	s.mu.Unlock()
}

// noteIgnored records a request Colly refused for us (duplicate or disallowed),
// which is not a failure but is worth reporting.
func (s *crawlState) noteIgnored(err error) {
	s.mu.Lock()
	s.ignored++
	if err != nil {
		s.lastErr = "ignored: " + err.Error()
	}
	s.mu.Unlock()
}

func (s *crawlState) addDepthBlocked() {
	s.mu.Lock()
	s.depthStop++
	s.mu.Unlock()
}

func (s *crawlState) addError(err error) {
	s.mu.Lock()
	s.errors++
	if err != nil {
		s.lastErr = err.Error()
	}
	s.mu.Unlock()
}

// counters is an immutable snapshot of the crawl counters.
type counters struct {
	Requests      int
	Pages         int
	ErrorPages    int
	Discovered    int
	Duplicates    int
	Skipped       int
	SkippedReason map[string]int
	Ignored       int
	DepthStop     int
	Errors        int
	ByType        map[string]int
	LastError     string
}

// Extracted is the number of HTML pages that were parsed and stored, which is the
// number a user means by "pages crawled".
func (c counters) Extracted() int {
	extracted := c.Pages - c.ErrorPages
	if extracted < 0 {
		return 0
	}
	return extracted
}

// snapshot copies the counters for reporting.
func (s *crawlState) snapshot() counters {
	s.mu.Lock()
	defer s.mu.Unlock()

	byType := make(map[string]int, len(s.byType))
	for k, v := range s.byType {
		byType[k] = v
	}

	return counters{
		Requests:      s.requests,
		Pages:         s.pages,
		ErrorPages:    s.errorPages,
		Discovered:    len(s.seen),
		Duplicates:    s.duplicates,
		Skipped:       s.skipped,
		SkippedReason: copyCounts(s.skipReasons),
		Ignored:       s.ignored,
		DepthStop:     s.depthStop,
		Errors:        s.errors,
		ByType:        byType,
		LastError:     s.lastErr,
	}
}

// copyCounts clones a map[string]int.
func copyCounts(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// queueDepth reports how many URLs are still waiting.
func (s *crawlState) queueDepth() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// shouldIgnoreRequestError reports whether a Colly request error is an expected
// "we deliberately did not fetch this" signal rather than a real failure.
func shouldIgnoreRequestError(err error) bool {
	if err == nil {
		return true
	}
	var alreadyVisited *colly.AlreadyVisitedError
	if errors.As(err, &alreadyVisited) {
		return true
	}
	return errors.Is(err, colly.ErrForbiddenDomain) ||
		errors.Is(err, colly.ErrForbiddenURL) ||
		errors.Is(err, colly.ErrNoURLFiltersMatch) ||
		errors.Is(err, colly.ErrRobotsTxtBlocked) ||
		errors.Is(err, colly.ErrMaxDepth)
}

// depthFromContext reads the crawl depth that was attached to the request.
func depthFromContext(ctx *colly.Context) int {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.GetAny("depth").(int); ok {
		return v
	}
	return 0
}

// jsonOrEmpty marshals a value, falling back to "{}".
func jsonOrEmpty(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "{}"
	}
	return string(b)
}

// validOrigin normalizes a crawl origin and guarantees it is an http(s) site root.
func validOrigin(origin string) (*url.URL, error) {
	norm, ok := NormalizeURL(origin)
	if !ok {
		return nil, fmt.Errorf("invalid crawl origin %q", origin)
	}
	u, err := url.Parse(norm)
	if err != nil {
		return nil, fmt.Errorf("invalid crawl origin %q: %w", origin, err)
	}
	// Always crawl from the site root, no matter which deep link was supplied.
	u.Path = "/"
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

// savePage stores an extracted page and returns the persisted row.
func savePage(page db.Page) *db.Page {
	if err := db.DB.Create(&page).Error; err != nil {
		fmt.Printf("crawler: failed to store page %s: %v\n", page.URL, err)
		return nil
	}
	return &page
}

// saveErrorPage records a page we could not fetch as real crawl evidence so the
// technical SEO rules can act on it later.
func saveErrorPage(opts CrawlOptions, rawURL string, status int, contentType, message string) {
	db.DB.Create(buildErrorPage(opts, rawURL, status, contentType, message))
}

// buildErrorPage constructs the Page row used to record an unreachable or
// non-HTML URL. Kept separate so the crawl can be exercised without a database.
func buildErrorPage(opts CrawlOptions, rawURL string, status int, contentType, message string) db.Page {
	page := db.Page{
		ProjectID:    opts.ProjectID,
		CrawlRunID:   opts.CrawlRunID,
		CompetitorID: opts.CompetitorID,
		URL:          rawURL,
		PageType:     "other",
		FetchedAt:    time.Now(),
	}
	if status > 0 {
		page.Status = &status
	}
	if contentType != "" {
		page.ContentType = &contentType
	}
	if message != "" {
		page.Error = &message
	}
	return page
}
