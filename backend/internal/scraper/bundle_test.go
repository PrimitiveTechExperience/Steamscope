package scraper

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Markup mirrors store.steampowered.com/bundle/12958 (Facepunch Complete Bundle).
const bundlePageFixture = `<html><body>
<h2 class="pageheader">Facepunch Complete Bundle</h2>
<div id="package_header_container">
  <img class="package_header" src="https://shared.fastly.steamstatic.com/store_item_assets/steam/bundles/12958/header.jpg?t=1" alt="x">
</div>
<div class="bundle_package_item complete_the_set">
  <div class="tab_item" data-ds-appid="4000"><div class="tab_item_content"><div class="tab_item_name">Garry's Mod</div></div></div>
</div>
<div class="bundle_package_item">
  <div class="tab_item" data-ds-appid="252490"><div class="tab_item_content"><div class="tab_item_name">Rust</div></div></div>
</div>
<div class="bundle_package_item">
  <div class="tab_item" data-ds-appid="4000"><div class="tab_item_content"><div class="tab_item_name">Garry's Mod (duplicate)</div></div></div>
</div>
<div class="package_totals_area" data-ds-bundleid="12958">
  <div class="package_totals_row"><div class="price bundle_final_package_price">$112.92</div><span>Price of individual products:</span></div>
  <div class="package_totals_row"><div class="price bundle_discount">10%</div> Bundle discount: </div>
  <div class="package_totals_row highlight"><div class="price bundle_final_price_with_discount">$101.62</div> Your cost: </div>
</div>
</body></html>`

func bundleDoc(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc.Selection
}

func TestParseBundlePage(t *testing.T) {
	b, err := parseBundlePage(bundleDoc(t, bundlePageFixture), 12958, "https://store.steampowered.com/bundle/12958")
	if err != nil {
		t.Fatalf("parseBundlePage: %v", err)
	}
	if b.Name != "Facepunch Complete Bundle" {
		t.Errorf("name = %q", b.Name)
	}
	if b.Price != 101.62 || b.OriginalPrice != 112.92 || b.DiscountPercentage != 10 {
		t.Errorf("price=%v original=%v discount=%d, want 101.62/112.92/10", b.Price, b.OriginalPrice, b.DiscountPercentage)
	}
	if !strings.Contains(b.HeaderImage, "bundles/12958/header.jpg") {
		t.Errorf("header image = %q", b.HeaderImage)
	}
	if len(b.Games) != 2 || b.Games[0].AppID != 4000 || b.Games[0].Name != "Garry's Mod" || b.Games[1].AppID != 252490 {
		t.Errorf("games = %+v, want deduped [4000 252490]", b.Games)
	}
}

func TestParseBundlePageRejectsBrokenPages(t *testing.T) {
	for name, html := range map[string]string{
		"no name":  `<html><body><div class="bundle_final_package_price">$10</div></body></html>`,
		"no price": `<html><body><h2 class="pageheader">X</h2><div class="bundle_package_item"><div data-ds-appid="1"></div></div></body></html>`,
		"no games": `<html><body><h2 class="pageheader">X</h2><div class="bundle_final_package_price">$10</div></body></html>`,
	} {
		if _, err := parseBundlePage(bundleDoc(t, html), 1, "u"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseBundlePageDerivesPriceFromDiscount(t *testing.T) {
	html := `<html><body><h2 class="pageheader">X</h2>
		<div class="bundle_package_item"><div data-ds-appid="1"></div></div>
		<div class="bundle_final_package_price">$100.00</div><div class="bundle_discount">25%</div></body></html>`
	b, err := parseBundlePage(bundleDoc(t, html), 1, "u")
	if err != nil || b.Price != 75 {
		t.Errorf("got price=%v err=%v, want 75", b.Price, err)
	}
}

func TestDiscoverBundleIDs(t *testing.T) {
	doc := bundleDoc(t, `<html><body>
		<a href="https://store.steampowered.com/bundle/12958/Facepunch_Complete_Bundle/">bundle</a>
		<a href="/bundle/232/">again</a>
		<a href="https://store.steampowered.com/bundle/232/">dup</a>
		<div data-ds-bundleid="12958"></div>
		<div data-ds-bundleid="69433"></div>
		<a href="https://store.steampowered.com/app/4000">not a bundle</a>
	</body></html>`)
	got := discoverBundleIDs(doc)
	want := []int{12958, 232, 69433}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
