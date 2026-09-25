package crawler

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestBodyTextKeepsBlocksApart(t *testing.T) {
	src := `<html><head><title>Order desk</title></head><body>
<header><a href="/">Home</a><a href="/pricing">Pricing</a></header>
<main>
  <div><span>All solutions</span><span>OpsERM</span></div>
  <h1>Order desk<br>Every marketplace order.</h1>
  <p>One board for Amazon, Flipkart and Shopify orders.</p><p>Pack, ship and track without switching tabs.</p>
  <ul><li>Stage actions</li><li>Shipment labels</li></ul>
</main>
<footer>© FyBud</footer>
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	page := ExtractPage(doc.Selection, "https://example.com/solutions/order-desk", 1)

	for _, glued := range []string{"All solutionsOpsERM", "Order deskEvery", "orders.Pack", "Stage actionsShipment"} {
		if strings.Contains(page.BodyText, glued) {
			t.Fatalf("text glued together (%q) in:\n%s", glued, page.BodyText)
		}
	}
	if strings.Contains(page.BodyText, "© FyBud") || strings.Contains(page.BodyText, "Pricing") {
		t.Fatalf("site chrome leaked into body:\n%s", page.BodyText)
	}
	if !strings.Contains(page.H1, "Order desk Every marketplace order.") {
		t.Fatalf("h1 = %s", page.H1)
	}
	if got := ReadableExcerpt(page.BodyText, 220); !strings.HasPrefix(got, "One board for Amazon") {
		t.Fatalf("excerpt = %q", got)
	}
}
