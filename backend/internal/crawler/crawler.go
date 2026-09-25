package crawler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"visora-backend/internal/db"
	"visora-backend/internal/events"
)

// crawlDeps are the side effects of a crawl. They are injectable so the crawl can
// be driven end-to-end in tests without a database.
//
// Every function may be called concurrently from the crawl workers, so
// implementations must be safe for concurrent use. The defaults rely on GORM,
// which is goroutine safe.
type crawlDeps struct {
	startRun  func(opts CrawlOptions, at time.Time) error
	finishRun func(opts CrawlOptions, pages, discovered int, stats crawlStats, lastErr string) error
	publish   func(projectID uint, stage int, eventType, message string, data interface{})
	savePage  func(page db.Page) error
}

// defaultCrawlDeps wires the crawler to Postgres and the scan event stream.
func defaultCrawlDeps() crawlDeps {
	return crawlDeps{
		startRun: func(opts CrawlOptions, at time.Time) error {
			return markRunStarted(opts.CrawlRunID, at)
		},
		finishRun: func(opts CrawlOptions, pages, discovered int, stats crawlStats, lastErr string) error {
			return markRunFinished(opts.CrawlRunID, pages, discovered, stats, lastErr)
		},
		publish:  events.PublishEvent,
		savePage: func(page db.Page) error { return db.DB.Create(&page).Error },
	}
}

// CrawlSite performs a bounded, priority-ordered crawl of one site.
//
// Ordering is deterministic: the frontier is drained by (priority tier, depth,
// discovery order), so a limited page budget is always spent on product,
// collection and content pages before utility pages such as /about.
//
// Only HTML documents are stored. Functional URLs (cart, checkout, account,
// internal search, filtered/sorted views, deep pagination, assets) and off-site
// links never enter the queue. Sitemaps are parsed first, so products that are
// only reachable through the sitemap are still discovered.
func CrawlSite(opts CrawlOptions) error {
	return crawlSite(opts, defaultCrawlDeps())
}

func crawlSite(opts CrawlOptions, deps crawlDeps) error {
	if opts.MaxPages <= 0 {
		opts.MaxPages = 50
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 3
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.DelayMs < 0 {
		opts.DelayMs = 0
	}

	root, err := validOrigin(opts.Origin)
	if err != nil {
		return err
	}

	startedAt := time.Now()
	if err := deps.startRun(opts, startedAt); err != nil {
		fmt.Printf("crawler: could not mark run %d as running: %v\n", opts.CrawlRunID, err)
	}

	state := newCrawlState()
	deps.publish(opts.ProjectID, 1, "progress",
		"Crawling "+root.Host,
		map[string]interface{}{
			"url":              root.String(),
			"pages_crawled":    0,
			"pages_discovered": 1,
			"queue_depth":      0,
		})

	// 1. robots.txt and the sitemap first. This is how product pages that are not
	// hand-linked get found, and how disallowed paths are kept out of the budget.
	httpClient := NewHTTPClient()
	robots := FetchRobots(httpClient, root.String())
	policy := newURLPolicy(root, robots)

	seed := CollectSitemapURLs(httpClient, root.String(), robots.Sitemaps, opts.SeedSitemap, maxSitemapURLs)
	queuedFromSitemap := 0
	for _, raw := range seed.URLs {
		priority, normalized, reason := policy.classify(raw)
		if reason != "" {
			state.noteSkipped(reason)
			continue
		}
		if state.push(normalized, priority, 1) {
			queuedFromSitemap++
		}
	}
	// 2. Always crawl the site root, whatever the sitemap claims. It goes through
	// the same policy so it is normalized identically to any discovered link.
	if _, normalizedRoot, reason := policy.classify(root.String()); reason == "" {
		state.push(normalizedRoot, PriorityHome, 0)
	} else {
		state.push(root.String(), PriorityHome, 0)
	}
	for _, raw := range opts.SeedURLs {
		if _, normalized, reason := policy.classify(raw); reason == "" {
			state.push(normalized, PriorityHome, 1)
		}
	}

	deps.publish(opts.ProjectID, 1, "milestone",
		fmt.Sprintf("Discovered %d sitemap URLs across %d sitemap files", queuedFromSitemap, len(seed.Sitemaps)),
		map[string]interface{}{
			"sitemaps":          seed.Sitemaps,
			"sitemap_urls":      queuedFromSitemap,
			"robots_disallowed": robots.Disallowed,
			"pages_discovered":  state.queueDepth(),
		})

	allowedDomains := []string{root.Hostname()}
	if !strings.HasPrefix(root.Hostname(), "www.") {
		allowedDomains = append(allowedDomains, "www."+root.Hostname())
	}

	c := colly.NewCollector(
		colly.AllowedDomains(allowedDomains...),
		colly.UserAgent(userAgent),
		colly.ParseHTTPErrorResponse(),
	)
	c.MaxBodySize = maxBodySize
	c.AllowURLRevisit = false
	c.SetRequestTimeout(requestTimeout)

	if err := c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: opts.Concurrency,
		Delay:       time.Duration(opts.DelayMs) * time.Millisecond,
	}); err != nil {
		return fmt.Errorf("crawler: set rate limit: %w", err)
	}

	registerCallbacks(c, opts, state, deps, policy)
	runDispatcher(c, opts, state)

	return finalizeCrawl(opts, state, seed, root, startedAt, deps, robots)
}

// registerCallbacks wires extraction, link discovery and error recording.
func registerCallbacks(c *colly.Collector, opts CrawlOptions, state *crawlState, deps crawlDeps, policy urlPolicy) {
	c.OnResponse(func(r *colly.Response) {
		contentType := ""
		if r.Headers != nil {
			contentType = r.Headers.Get("Content-Type")
		}
		if isHTMLContentType(contentType) && len(r.Body) > 0 {
			// HTML extraction happens in OnHTML so the document is parsed once.
			return
		}
		if r.StatusCode >= 400 {
			if err := deps.savePage(buildErrorPage(opts, r.Request.URL.String(), r.StatusCode, contentType,
				describeNonHTML(contentType, len(r.Body)))); err != nil {
				state.addError(err)
				return
			}
			state.recordErrorPage()
			return
		}
		// A 2xx non-HTML response is an asset-like endpoint, not a page.
		state.addSkipped(1)
	})

	c.OnHTML("html", func(e *colly.HTMLElement) {
		pageURL := e.Request.URL.String()
		depth := depthFromContext(e.Request.Ctx)

		page := ExtractPage(e.DOM, pageURL, depth)
		page.ProjectID = opts.ProjectID
		page.CrawlRunID = opts.CrawlRunID
		page.CompetitorID = opts.CompetitorID

		status := e.Response.StatusCode
		page.Status = &status
		if e.Response.Headers != nil {
			if ct := e.Response.Headers.Get("Content-Type"); ct != "" {
				page.ContentType = &ct
			}
		}
		page.ResponseBytes = int64(len(e.Response.Body))
		page.ResponseMs = elapsedMs(e.Request.Ctx)

		if err := deps.savePage(page); err != nil {
			state.addError(err)
		} else {
			state.recordPage(page.PageType)
		}

		links, refused, depthBlocked := enqueueLinks(e, opts.MaxDepth, policy)
		for reason, count := range refused {
			for i := 0; i < count; i++ {
				state.noteSkipped(reason)
			}
		}
		for i := 0; i < depthBlocked; i++ {
			state.addDepthBlocked()
		}
		for _, link := range links {
			state.push(link.url, link.priority, link.depth)
		}

		publishPageEvent(opts, state, deps, page)
	})

	c.OnError(func(r *colly.Response, err error) {
		// ParseHTTPErrorResponse is enabled, so this only fires for real transport
		// or parse failures. They are recorded as crawl evidence.
		if err == nil {
			return
		}
		pageURL := ""
		if r != nil && r.Request != nil {
			pageURL = r.Request.URL.String()
		}
		if saveErr := deps.savePage(buildErrorPage(opts, pageURL, 0, "", err.Error())); saveErr != nil {
			state.addError(saveErr)
			return
		}
		state.recordErrorPage()
	})
}

// elapsedMs returns how long the request that produced this context has taken.
func elapsedMs(ctx *colly.Context) int {
	if ctx == nil {
		return 0
	}
	if started, ok := ctx.GetAny("started").(time.Time); ok {
		ms := time.Since(started).Milliseconds()
		if ms < 0 {
			return 0
		}
		return int(ms)
	}
	return 0
}

// describeNonHTML explains why a response was not treated as a page.
func describeNonHTML(contentType string, bodyLength int) string {
	if bodyLength == 0 {
		return "empty response body"
	}
	if contentType == "" {
		return "response has no content type"
	}
	return "non-html response: " + contentType
}

// discoveredLink is a policy-approved URL ready for the frontier.
type discoveredLink struct {
	url      string
	priority int
	depth    int
}

// enqueueLinks applies the URL policy to every anchor on a page. It returns the
// links worth crawling plus a tally of the reasons everything else was refused.
func enqueueLinks(e *colly.HTMLElement, maxDepth int, policy urlPolicy) (links []discoveredLink, refused map[string]int, depthBlocked int) {
	nextDepth := depthFromContext(e.Request.Ctx) + 1
	refused = map[string]int{}

	refuse := func(reason string) {
		refused[reason]++
	}

	e.DOM.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		abs := e.Request.AbsoluteURL(href)
		if abs == "" {
			refuse("unresolvable link")
			return
		}

		priority, normalized, reason := policy.classify(abs)
		if reason != "" {
			refuse(reason)
			return
		}

		if nextDepth > maxDepth {
			depthBlocked++
			return
		}

		links = append(links, discoveredLink{url: normalized, priority: priority, depth: nextDepth})
	})

	return links, refused, depthBlocked
}

// runDispatcher drains the frontier with a fixed number of workers. Colly is used
// synchronously (one request per worker) so the page budget is reserved up front
// and can never be exceeded, and so callbacks never race on shared counters.
func runDispatcher(c *colly.Collector, opts CrawlOptions, state *crawlState) {
	jobs := make(chan frontierItem)
	var workers sync.WaitGroup

	for i := 0; i < opts.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				ctx := colly.NewContext()
				ctx.Put("depth", item.depth)
				// Timed so every stored page carries a real response time.
				ctx.Put("started", time.Now())

				if err := c.Request(http.MethodGet, item.url, nil, ctx, nil); err != nil {
					if shouldIgnoreRequestError(err) {
						state.noteIgnored(err)
					} else {
						state.addError(err)
					}
				}
				state.release()
			}
		}()
	}

	for {
		item, ok := state.reserve(opts.MaxPages, opts.Concurrency)
		if !ok {
			break
		}
		jobs <- item
	}

	close(jobs)
	workers.Wait()
}

// publishPageEvent streams one real crawled URL to the scan UI.
func publishPageEvent(opts CrawlOptions, state *crawlState, deps crawlDeps, page db.Page) {
	snap := state.snapshot()
	status := 0
	if page.Status != nil {
		status = *page.Status
	}
	title := ""
	if page.Title != nil {
		title = *page.Title
	}
	ogImage := ""
	if page.OgImage != nil {
		ogImage = *page.OgImage
	}
	desc := ""
	if page.MetaDescription != nil {
		desc = *page.MetaDescription
	} else if page.OgDescription != nil {
		desc = *page.OgDescription
	}
	h1 := ""
	var h1s []string
	if err := json.Unmarshal([]byte(page.H1), &h1s); err == nil && len(h1s) > 0 {
		h1 = h1s[0]
	}
	deps.publish(opts.ProjectID, 1, "progress",
		fmt.Sprintf("%s  %d", displayPath(page.URL), status),
		map[string]interface{}{
			"url":              page.URL,
			"path":             displayPath(page.URL),
			"page_type":        page.PageType,
			"status":           status,
			"depth":            page.Depth,
			"title":            title,
			"description":      desc,
			"h1":               h1,
			"og_image":         ogImage,
			"response_ms":      page.ResponseMs,
			"word_count":       page.WordCount,
			"pages_crawled":    snap.Extracted(),
			"requests":         snap.Requests,
			"pages_discovered": snap.Discovered,
			"links_skipped":    snap.Skipped,
			"queue_depth":      state.queueDepth(),
			"pages_by_type":    snap.ByType,
			"products":         snap.ByType["product"],
			"collections":      snap.ByType["collection"],
			"articles":         snap.ByType["article"] + snap.ByType["blog"],
		})
}

// displayPath renders the crawlable part of a URL for logs and the scan UI.
func displayPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}

// finalizeCrawl persists the real crawl statistics and emits the completion event.
func finalizeCrawl(opts CrawlOptions, state *crawlState, seed SitemapSeed, root *url.URL, startedAt time.Time, deps crawlDeps, robots RobotsInfo) error {
	snap := state.snapshot()
	extracted := snap.Extracted()

	stats := crawlStats{
		Requests:       snap.Requests,
		Pages:          extracted,
		ErrorPages:     snap.ErrorPages,
		Discovered:     snap.Discovered,
		Duplicates:     snap.Duplicates,
		Skipped:        snap.Skipped,
		SkippedReasons: snap.SkippedReason,
		Ignored:        snap.Ignored,
		DepthBlocked:   snap.DepthStop,
		Errors:         snap.Errors,
		ByType:         snap.ByType,
		Sitemaps:       seed.Sitemaps,
		SitemapURLs:    len(seed.URLs),
		RobotsFound:    robots.Found,
		RobotsRules:    len(robots.Disallowed),
		DurationMs:     time.Since(startedAt).Milliseconds(),
		LastError:      snap.LastError,
	}

	// Record an explanatory failure reason on the run when nothing was crawled.
	failureReason := snap.LastError
	if extracted == 0 && failureReason == "" {
		if snap.ErrorPages > 0 {
			failureReason = fmt.Sprintf("no HTML pages were served by %s (%d non-HTML or error responses)", root.Host, snap.ErrorPages)
		} else {
			failureReason = "no crawlable HTML pages were discovered on " + root.Host
		}
	}

	if err := deps.finishRun(opts, extracted, snap.Discovered, stats, failureReason); err != nil {
		fmt.Printf("crawler: could not finalise run %d: %v\n", opts.CrawlRunID, err)
	}

	eventType := "complete"
	message := fmt.Sprintf("Crawled %d pages from %s", extracted, root.Host)
	if extracted == 0 {
		eventType = "error"
		message = "No HTML pages could be crawled from " + root.Host
	}
	deps.publish(opts.ProjectID, 1, eventType, message, map[string]interface{}{
		"pages_crawled":    extracted,
		"pages_discovered": snap.Discovered,
		"requests":         snap.Requests,
		"links_skipped":    snap.Skipped,
		"errors":           snap.Errors,
		"error_pages":      snap.ErrorPages,
		"pages_by_type":    snap.ByType,
		"sitemaps":         seed.Sitemaps,
		"duration_ms":      stats.DurationMs,
	})

	fmt.Printf("crawler: %s finished — pages=%d discovered=%d requests=%d skipped=%d errors=%d duration=%dms\n",
		root.Host, extracted, snap.Discovered, snap.Requests, snap.Skipped, snap.Errors, stats.DurationMs)

	if extracted == 0 {
		return fmt.Errorf("%s", failureReason)
	}
	return nil
}

// markRunStarted flips the crawl run into the running state with a real timestamp.
func markRunStarted(runID uint, at time.Time) error {
	if runID == 0 {
		return nil
	}
	return db.DB.Model(&db.CrawlRun{}).Where("id = ?", runID).
		Updates(map[string]interface{}{"status": "running", "started_at": at}).Error
}

// markRunFinished writes the final status, counts and stats for a crawl run.
func markRunFinished(runID uint, pages, discovered int, stats crawlStats, lastErr string) error {
	if runID == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"status":           "done",
		"pages_crawled":    pages,
		"pages_discovered": discovered,
		"finished_at":      time.Now(),
		"stats":            jsonOrEmpty(stats),
	}
	if pages == 0 {
		updates["status"] = "error"
		msg := lastErr
		if msg == "" {
			msg = "no pages could be crawled"
		}
		updates["error"] = msg
	}
	return db.DB.Model(&db.CrawlRun{}).Where("id = ?", runID).Updates(updates).Error
}
