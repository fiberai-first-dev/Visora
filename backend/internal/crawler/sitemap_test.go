package crawler

import "testing"

const urlsetFixture = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
    <lastmod>2026-01-02</lastmod>
    <priority>1.0</priority>
  </url>
  <url>
    <loc>https://example.com/products/vitamin-c-serum</loc>
    <lastmod>2026-01-03</lastmod>
  </url>
  <url><loc>https://example.com/collections/skincare</loc></url>
</urlset>`

const sitemapIndexFixture = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://example.com/sitemap-products.xml</loc></sitemap>
  <sitemap><loc>https://example.com/sitemap-blogs.xml</loc></sitemap>
</sitemapindex>`

func TestParseSitemapURLSet(t *testing.T) {
	sitemaps, pages, err := ParseSitemap([]byte(urlsetFixture))
	if err != nil {
		t.Fatalf("ParseSitemap returned error: %v", err)
	}
	if len(sitemaps) != 0 {
		t.Fatalf("expected no child sitemaps, got %v", sitemaps)
	}
	want := []string{
		"https://example.com/",
		"https://example.com/products/vitamin-c-serum",
		"https://example.com/collections/skincare",
	}
	if len(pages) != len(want) {
		t.Fatalf("expected %d page URLs, got %d (%v)", len(want), len(pages), pages)
	}
	for i, w := range want {
		if pages[i] != w {
			t.Fatalf("page URL %d = %q, want %q", i, pages[i], w)
		}
	}
}

func TestParseSitemapIndex(t *testing.T) {
	sitemaps, pages, err := ParseSitemap([]byte(sitemapIndexFixture))
	if err != nil {
		t.Fatalf("ParseSitemap returned error: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("expected no page URLs from a sitemap index, got %v", pages)
	}
	if len(sitemaps) != 2 {
		t.Fatalf("expected 2 child sitemaps, got %d (%v)", len(sitemaps), sitemaps)
	}
	if sitemaps[0] != "https://example.com/sitemap-products.xml" {
		t.Fatalf("unexpected first child sitemap %q", sitemaps[0])
	}
}

func TestParseSitemapWithoutNamespace(t *testing.T) {
	raw := `<?xml version="1.0"?>
<urlset>
  <url><loc>https://example.com/a</loc></url>
  <url><loc>https://example.com/b</loc></url>
</urlset>`
	_, pages, err := ParseSitemap([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSitemap returned error: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 page URLs, got %d (%v)", len(pages), pages)
	}
}

func TestParseSitemapCDATALoc(t *testing.T) {
	raw := `<?xml version="1.0"?>
<urlset>
  <url><loc><![CDATA[https://example.com/products/x]]></loc></url>
</urlset>`
	_, pages, err := ParseSitemap([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSitemap returned error: %v", err)
	}
	if len(pages) != 1 || pages[0] != "https://example.com/products/x" {
		t.Fatalf("CDATA loc was not parsed, got %v", pages)
	}
}

func TestParseSitemapHTMLResponseIsNotConfused(t *testing.T) {
	// A 404 served as an HTML error page must not produce URLs.
	_, pages, _ := ParseSitemap([]byte(`<html><body><h1>Not found</h1></body></html>`))
	if len(pages) != 0 {
		t.Fatalf("HTML error page produced page URLs: %v", pages)
	}
}

func TestParseSitemapTrimsWhitespaceInLoc(t *testing.T) {
	raw := `<?xml version="1.0"?>
<urlset>
  <url>
    <loc>
      https://example.com/products/spaced
    </loc>
  </url>
</urlset>`
	_, pages, err := ParseSitemap([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSitemap returned error: %v", err)
	}
	if len(pages) != 1 || pages[0] != "https://example.com/products/spaced" {
		t.Fatalf("whitespace around <loc> was not trimmed, got %v", pages)
	}
}
