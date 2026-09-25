package crawler

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"empty", "", "", false},
		{"mailto", "mailto:hello@example.com", "", false},
		{"javascript", "javascript:void(0)", "", false},
		{"tel", "tel:+911234567890", "", false},
		{"relative", "/products/serum", "", false},
		{"credentials", "https://user:pass@example.com/p", "", false},
		{"uppercase host and default port", "https://WWW.Example.com:443/Shop/", "https://example.com/Shop", true},
		{"http default port dropped", "http://example.com:80/x", "http://example.com/x", true},
		{"custom port kept", "https://example.com:8080/x", "https://example.com:8080/x", true},
		{"index file removed", "http://example.com/index.html", "http://example.com/", true},
		{"nested index file removed", "https://example.com/shop/index.php", "https://example.com/shop", true},
		{"duplicate slashes collapsed", "https://example.com/a//b///c", "https://example.com/a/b/c", true},
		{"trailing slash dropped", "https://example.com/shop/", "https://example.com/shop", true},
		{"root slash kept", "https://example.com", "https://example.com/", true},
		{"fragment dropped", "https://example.com/p#reviews", "https://example.com/p", true},
		{"tracking params stripped and rest sorted", "https://www.example.com/Shop/?utm_source=x&b=2&a=1#frag", "https://example.com/Shop?a=1&b=2", true},
		{"fbclid stripped", "https://example.com/p?fbclid=abc", "https://example.com/p", true},
		{"meaningful params kept", "https://example.com/index.php?p=123", "https://example.com/?p=123", true},
		{"trailing dot host", "https://example.com./x", "https://example.com/x", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeURL(tc.in)
			if ok != tc.ok {
				t.Fatalf("NormalizeURL(%q) ok=%v, want %v (got %q)", tc.in, ok, tc.ok, got)
			}
			if ok && got != tc.want {
				t.Fatalf("NormalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeURLIsIdempotent(t *testing.T) {
	inputs := []string{
		"https://www.Example.com:443/Shop/?utm_source=x&b=2&a=1#frag",
		"https://example.com/a//b/",
		"http://example.com/index.html",
	}
	for _, in := range inputs {
		once, ok := NormalizeURL(in)
		if !ok {
			t.Fatalf("NormalizeURL(%q) was rejected", in)
		}
		twice, ok := NormalizeURL(once)
		if !ok {
			t.Fatalf("NormalizeURL(%q) rejected the normalized value %q", in, once)
		}
		if once != twice {
			t.Fatalf("normalization is not idempotent: %q -> %q -> %q", in, once, twice)
		}
	}
}

// TestClassifyURL locks in the crawl policy: what we crawl, in what order, and
// what we deliberately refuse to crawl.
func TestClassifyURL(t *testing.T) {
	skipCases := []string{
		"https://example.com/cart",
		"https://example.com/cart/add?id=1",
		"https://example.com/checkout",
		"https://example.com/account/login",
		"https://example.com/my-account",
		"https://example.com/wishlist",
		"https://example.com/wp-admin/admin.php",
		"https://example.com/wp-json/wp/v2/posts",
		"https://example.com/feed",
		"https://example.com/cdn-cgi/scripts/x",
		"https://example.com/search?q=serum",
		"https://example.com/tag/skincare",
		"https://example.com/author/jane",
		"https://example.com/products/serum.jpg",
		"https://example.com/assets/app.css",
		"https://example.com/uploads/catalog.pdf",
		"https://example.com/collections/all?add-to-cart=5",
		"https://example.com/collections/all?orderby=price",
		"https://example.com/collections/all?filter.v.price=100",
		"https://example.com/products/serum?sort=price",
		"https://example.com/blog?page=9",
		"https://example.com/blogs/news?paged=14",
		"https://example.com/gift_cards",
		"https://example.com/404",
	}

	priorityCases := []struct {
		url  string
		want int
	}{
		{"https://example.com/", PriorityHome},
		{"https://example.com/products/vitamin-c-serum", PriorityProduct},
		{"https://example.com/product/vitamin-c-serum", PriorityProduct},
		{"https://example.com/p/serum-30ml", PriorityProduct},
		{"https://example.com/pd/serum", PriorityProduct},
		{"https://example.com/collections/skincare", PriorityCollection},
		{"https://example.com/collection/new-arrivals", PriorityCollection},
		{"https://example.com/category/hair-care", PriorityCollection},
		{"https://example.com/shop", PriorityCollection},
		{"https://example.com/store", PriorityCollection},
		{"https://example.com/all-products", PriorityCollection},
		{"https://example.com/blogs/news/serum-guide", PriorityContent},
		{"https://example.com/blog/serum-guide", PriorityContent},
		{"https://example.com/faq", PriorityContent},
		{"https://example.com/pages/contact", PriorityContent},
		{"https://example.com/about-us", PriorityUtility},
		{"https://example.com/contact", PriorityUtility},
		{"https://example.com/policies/refund-policy", PriorityUtility},
		{"https://example.com/some/deep/unknown-path", PriorityOther},
		// Boundary safety: these must NOT be mistaken for short prefixes.
		{"https://example.com/shopping-guide", PriorityOther},
		{"https://example.com/press-releases", PriorityUtility},
		// Pagination inside the allowed range is still crawled.
		{"https://example.com/collections/all?page=2", PriorityCollection},
	}

	for _, url := range skipCases {
		t.Run("skip "+url, func(t *testing.T) {
			priority, reason := ClassifyURL(url)
			if reason == "" {
				t.Fatalf("ClassifyURL(%q) was allowed with priority %d, want it skipped", url, priority)
			}
		})
	}

	for _, tc := range priorityCases {
		t.Run("priority "+tc.url, func(t *testing.T) {
			priority, reason := ClassifyURL(tc.url)
			if reason != "" {
				t.Fatalf("ClassifyURL(%q) was skipped (%s), want priority %d", tc.url, reason, tc.want)
			}
			if priority != tc.want {
				t.Fatalf("ClassifyURL(%q) priority = %d, want %d", tc.url, priority, tc.want)
			}
		})
	}
}
