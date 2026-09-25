package crawler

import (
	"net/http"
	"net/url"
	"strings"
)

// RobotsInfo holds the parts of robots.txt that govern our crawl: the advertised
// sitemaps plus the allow/disallow rules that apply to VisoraBot.
type RobotsInfo struct {
	Sitemaps   []string `json:"sitemaps"`
	Disallowed []string `json:"disallowed,omitempty"`
	Allowed    []string `json:"allowed,omitempty"`
	Found      bool     `json:"found"`
}

// FetchRobots downloads robots.txt and extracts both the sitemap hints and the
// rules that apply to us. Crawl rules are enforced by the URL policy before a URL
// is ever queued, in addition to Colly's own robots handling.
func FetchRobots(client *http.Client, origin string) RobotsInfo {
	info := RobotsInfo{Sitemaps: []string{}}

	root, ok := rootURL(origin)
	if !ok {
		return info
	}

	body, err := fetchBytes(client, root.JoinPath("/robots.txt").String())
	if err != nil {
		return info
	}
	info.Found = true

	seenSitemap := map[string]bool{}
	lastWasAgent := false
	groupApplies := false

	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		separator := strings.Index(line, ":")
		if separator < 0 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(line[:separator]))
		value := strings.TrimSpace(line[separator+1:])

		switch key {
		case "user-agent":
			// Consecutive User-agent lines form a single group.
			if !lastWasAgent {
				groupApplies = false
			}
			if robotsAgentApplies(value) {
				groupApplies = true
			}
			lastWasAgent = true
		case "disallow":
			lastWasAgent = false
			if groupApplies && value != "" && !containsString(info.Disallowed, value) {
				info.Disallowed = append(info.Disallowed, value)
			}
		case "allow":
			lastWasAgent = false
			if groupApplies && value != "" && !containsString(info.Allowed, value) {
				info.Allowed = append(info.Allowed, value)
			}
		case "sitemap":
			lastWasAgent = false
			if value != "" && !seenSitemap[value] {
				seenSitemap[value] = true
				info.Sitemaps = append(info.Sitemaps, value)
			}
		default:
			lastWasAgent = false
		}
	}

	return info
}

// robotsAgentApplies reports whether a User-agent line in robots.txt targets us.
func robotsAgentApplies(agent string) bool {
	agent = strings.ToLower(strings.TrimSpace(agent))
	if agent == "" {
		return false
	}
	return agent == "*" || strings.Contains(agent, "visora")
}

// Blocks reports whether a path may not be crawled, applying the standard
// precedence: the longest matching rule wins and Allow beats Disallow.
func (r RobotsInfo) Blocks(path string) bool {
	if !r.Found || path == "" {
		return false
	}

	bestLength := -1
	blocked := false

	for _, pattern := range r.Disallowed {
		if !robotsPatternMatches(pattern, path) {
			continue
		}
		if length := len(pattern); length > bestLength {
			bestLength = length
			blocked = true
		}
	}
	for _, pattern := range r.Allowed {
		if !robotsPatternMatches(pattern, path) {
			continue
		}
		if length := len(pattern); length >= bestLength {
			bestLength = length
			blocked = false
		}
	}
	return blocked
}

// robotsPatternMatches implements the subset of the robots.txt path syntax that
// real sites use: a literal prefix, a trailing switch to "contains", and the "$"
// end anchor.
func robotsPatternMatches(pattern, path string) bool {
	if pattern == "" {
		return false
	}

	anchored := strings.HasSuffix(pattern, "$")
	if anchored {
		pattern = strings.TrimSuffix(pattern, "$")
	}

	if wildcard := strings.Index(pattern, "*"); wildcard >= 0 {
		prefix := pattern[:wildcard]
		suffix := pattern[wildcard+1:]
		if !strings.HasPrefix(path, prefix) {
			return false
		}
		if suffix == "" {
			return true
		}
		return strings.Contains(path[len(prefix):], suffix)
	}

	if anchored {
		return path == pattern
	}
	return strings.HasPrefix(path, pattern)
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// urlPolicy is the single gate every candidate URL must pass before it can enter
// the crawl frontier: canonical host, robots.txt rules, then the content rules in
// ClassifyURL. Filtering here (instead of after dispatching) means junk URLs never
// consume the page budget.
type urlPolicy struct {
	host   string // canonical host, lowercased and without a leading www.
	robots RobotsInfo
}

// newURLPolicy builds the policy for an origin.
func newURLPolicy(root *url.URL, robots RobotsInfo) urlPolicy {
	host := strings.ToLower(root.Hostname())
	return urlPolicy{
		host:   strings.TrimPrefix(host, "www."),
		robots: robots,
	}
}

// classify returns the crawl priority for a URL. A non-empty reason means the URL
// must not be crawled, and the returned URL is the normalized form to fetch.
func (p urlPolicy) classify(raw string) (priority int, normalized string, reason string) {
	normalized, ok := NormalizeURL(raw)
	if !ok {
		return 0, "", "unusable url"
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return 0, "", "unparseable url"
	}

	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	if host != p.host {
		return 0, "", "off-site host " + host
	}

	if p.robots.Blocks(parsed.Path) {
		return 0, "", "robots.txt disallow " + parsed.Path
	}

	priority, reason = ClassifyURL(normalized)
	if reason != "" {
		return 0, "", reason
	}
	return priority, normalized, ""
}
