package api

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
)

var previewClient = &http.Client{Timeout: 12 * time.Second}

func hostKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "www.")
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func allowedPreviewURL(project db.Project, raw string) (*url.URL, bool) {
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" {
		return nil, false
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, false
	}
	want := hostKey(project.Website)
	got := hostKey(target.Hostname())
	return target, want != "" && want == got
}

func rewritePreviewHTML(html string, pageURL *url.URL, animate bool) string {
	base := pageURL.Scheme + "://" + pageURL.Host + "/"
	injection := `<base href="` + base + `"><meta name="referrer" content="no-referrer">`
	if animate {
		injection += `<script>(function(){function run(){try{var h=Math.max(document.body?document.body.scrollHeight:0,document.documentElement?document.documentElement.scrollHeight:0);var max=Math.max(0,h-(window.innerHeight||600));if(max<60)return;var t0=performance.now(),dur=4200;function step(now){var t=Math.min(1,(now-t0)/dur);var e=t<0.5?2*t*t:-1+(4-2*t)*t;window.scrollTo(0,max*0.58*e);if(t<1)requestAnimationFrame(step);}requestAnimationFrame(step);}catch(e){}}if(document.readyState==="complete")setTimeout(run,400);else window.addEventListener("load",function(){setTimeout(run,400);});})();</script>`
	}
	lower := strings.ToLower(html)
	if i := strings.Index(lower, "<head"); i >= 0 {
		if j := strings.Index(html[i:], ">"); j >= 0 {
			at := i + j + 1
			html = html[:at] + injection + html[at:]
		}
	} else if i := strings.Index(lower, "<html"); i >= 0 {
		if j := strings.Index(html[i:], ">"); j >= 0 {
			at := i + j + 1
			html = html[:at] + "<head>" + injection + "</head>" + html[at:]
		}
	} else {
		html = "<head>" + injection + "</head>" + html
	}

	// Drop frame-busting CSP so the proxied page can render in our iframe.
	html = stripMetaCSP(html)
	return html
}

func stripMetaCSP(html string) string {
	lower := strings.ToLower(html)
	for {
		i := strings.Index(lower, `http-equiv="content-security-policy"`)
		if i < 0 {
			i = strings.Index(lower, `http-equiv='content-security-policy'`)
		}
		if i < 0 {
			break
		}
		start := strings.LastIndex(html[:i], "<")
		end := strings.Index(html[i:], ">")
		if start < 0 || end < 0 {
			break
		}
		html = html[:start] + html[i+end+1:]
		lower = strings.ToLower(html)
	}
	return html
}

// PreviewPage fetches the customer's own page and serves it without
// X-Frame-Options so the scan UI can render it.
func PreviewPage(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("url"))
	if raw == "" {
		c.String(http.StatusBadRequest, "url is required")
		return
	}

	var project db.Project
	if stored, ok := c.Get("project"); ok {
		if p, ok := stored.(*db.Project); ok {
			project = *p
		}
	}
	if project.ID == 0 {
		if err := db.DB.First(&project, c.Param("id")).Error; err != nil {
			c.String(http.StatusNotFound, "project not found")
			return
		}
	}

	target, ok := allowedPreviewURL(project, raw)
	if !ok {
		c.String(http.StatusForbidden, "url is not on this project")
		return
	}

	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid url")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; VisoraPreview/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := previewClient.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "could not fetch page")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		c.String(http.StatusBadGateway, "could not read page")
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(ct), "html") && !strings.Contains(strings.ToLower(string(body[:min(200, len(body))])), "<html") {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
			`<html><body style="font-family:sans-serif;padding:24px">This page is not HTML.</body></html>`,
		))
		return
	}

	html := rewritePreviewHTML(string(body), target, c.Query("scroll") == "1")
	c.Header("Cache-Control", "private, max-age=60")
	c.Header("Content-Security-Policy", "frame-ancestors 'self'")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
