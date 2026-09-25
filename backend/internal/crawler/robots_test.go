package crawler

import (
	"net/url"
	"testing"
)

func TestRobotsPatternMatching(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/private/", "/private/secret", true},
		{"/private/", "/public/secret", false},
		{"/admin", "/admin", true},
		{"/admin", "/administrator", true},
		{"/admin$", "/admin", true},
		{"/admin$", "/admin/users", false},
		{"/*.pdf", "/guides/catalog.pdf", true},
		{"/*.pdf", "/guides/catalog.html", false},
		{"/*?", "/shop?color=red", true},
		{"", "/anything", false},
	}

	for _, tc := range cases {
		if got := robotsPatternMatches(tc.pattern, tc.path); got != tc.want {
			t.Errorf("robotsPatternMatches(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestRobotsBlocksPrecedence(t *testing.T) {
	info := RobotsInfo{
		Found:      true,
		Disallowed: []string{"/"},
		Allowed:    []string{"/products/", "/blog"},
	}

	cases := []struct {
		path string
		want bool
	}{
		// The longer Allow rule wins over the global Disallow.
		{"/products/serum", false},
		{"/blog/post-1", false},
		// Everything else is blocked by the global Disallow.
		{"/about-us", true},
		{"/cart", true},
	}

	for _, tc := range cases {
		if got := info.Blocks(tc.path); got != tc.want {
			t.Errorf("Blocks(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestRobotsBlocksWithoutRules(t *testing.T) {
	// A robots.txt that only advertises a sitemap disallows nothing.
	info := RobotsInfo{Found: true, Sitemaps: []string{"https://example.com/sitemap.xml"}}
	if info.Blocks("/anything") {
		t.Error("expected no path to be blocked")
	}
	// If robots.txt could not be read we must not block anything either.
	if (&RobotsInfo{}).Blocks("/anything") {
		t.Error("expected no blocking when robots.txt is missing")
	}
}

func TestRobotsAgentMatching(t *testing.T) {
	cases := map[string]bool{
		"*":                  true,
		"VisoraBot":          true,
		"visorabot/1.0":      true,
		"Googlebot":          false,
		"":                   false,
	}
	for agent, want := range cases {
		if got := robotsAgentApplies(agent); got != want {
			t.Errorf("robotsAgentApplies(%q) = %v, want %v", agent, got, want)
		}
	}
}

func TestURLPolicyRejectsForeignHosts(t *testing.T) {
	root, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	policy := newURLPolicy(root, RobotsInfo{})

	cases := []struct {
		raw  string
		ok   bool
		host string
	}{
		{"https://example.com/products/x", true, "example.com"},
		{"https://www.example.com/products/x", true, "example.com"},
		{"http://example.com/products/x", true, "example.com"},
		{"https://competitor.example.net/products/x", false, ""},
		{"https://evil.com/", false, ""},
		{"https://sub.example.com/x", false, ""},
	}

	for _, tc := range cases {
		_, normalized, reason := policy.classify(tc.raw)
		if tc.ok && reason != "" {
			t.Errorf("classify(%q) was refused: %s", tc.raw, reason)
			continue
		}
		if !tc.ok {
			if reason == "" {
				t.Errorf("classify(%q) was allowed but should not be", tc.raw)
			}
			continue
		}
		parsed, parseErr := url.Parse(normalized)
		if parseErr != nil {
			t.Fatalf("normalized url %q is invalid: %v", normalized, parseErr)
		}
		if got := parsed.Hostname(); got != tc.host {
			t.Errorf("classify(%q) host = %q, want %q", tc.raw, got, tc.host)
		}
	}
}

func TestURLPolicyAppliesRobotsRules(t *testing.T) {
	root, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	policy := newURLPolicy(root, RobotsInfo{Found: true, Disallowed: []string{"/private/"}})

	if _, _, reason := policy.classify("https://example.com/private/secret"); reason == "" {
		t.Error("expected a robots.txt disallow reason for /private/secret")
	}
	if _, _, reason := policy.classify("https://example.com/products/serum"); reason != "" {
		t.Errorf("public product page was refused: %s", reason)
	}
}

func TestHTMLContentTypeDetection(t *testing.T) {
	cases := map[string]bool{
		"":                          true,
		"text/html":                 true,
		"text/html; charset=utf-8":  true,
		"application/xhtml+xml":     true,
		"application/json":          false,
		"image/jpeg":                false,
		"application/xml":           false,
		"text/plain; charset=utf-8": false,
	}

	for contentType, want := range cases {
		if got := isHTMLContentType(contentType); got != want {
			t.Errorf("isHTMLContentType(%q) = %v, want %v", contentType, got, want)
		}
	}
}
