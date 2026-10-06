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

// Item markup copied from a real bundle page (store.steampowered.com/bundle/232).
const itemsWithPrices = `<html><body><h2 class="pageheader">Valve Complete Pack</h2>
<div class="bundle_package_item complete_the_set">
  <div class="tab_item" data-ds-appid="240" data-ds-itemkey="App_240">
    <div class="discount_block tab_item_discount" data-price-final="199" data-bundlediscount="0" data-discount="80" role="link" aria-label="80% off. $9.99 normally, discounted to $1.99"><div class="discount_pct">-80%</div></div>
    <div class="tab_item_content"><div class="tab_item_name">Counter-Strike: Source</div></div>
  </div>
</div>
<div class="bundle_package_item complete_the_set">
  <div class="tab_item" data-ds-packageid="7" data-ds-appid="10,80" data-ds-itemkey="Sub_7">
    <div class="discount_block tab_item_discount" data-price-final="199" data-discount="80" aria-label="80% off. $9.99 normally, discounted to $1.99"></div>
    <div class="tab_item_content"><div class="tab_item_name">Counter-Strike: Condition Zero</div></div>
  </div>
</div>
<div class="bundle_package_item">
  <div class="tab_item" data-ds-appid="220" data-ds-itemkey="App_220">
    <div class="discount_block tab_item_discount" data-price-final="999" data-discount="0" aria-label="$9.99"></div>
    <div class="tab_item_content"><div class="tab_item_name">Half-Life 2</div></div>
  </div>
</div>
<div class="bundle_package_item">
  <div class="tab_item" data-ds-appid="9999"><div class="tab_item_content"><div class="tab_item_name">No price shown</div></div></div>
</div>
<div class="bundle_final_package_price">$22.00</div><div class="bundle_discount">10%</div><div class="bundle_final_price_with_discount">$19.80</div>
</body></html>`

func TestParseBundleReadsEachGamesPrices(t *testing.T) {
	b, err := parseBundlePage(bundleDoc(t, itemsWithPrices), 232, "u")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Games) != 4 {
		t.Fatalf("got %d games %+v, want 4 (a multi-app package counts as one item)", len(b.Games), b.Games)
	}
	byID := map[int]struct{ price, regular float64 }{}
	for _, g := range b.Games {
		byID[g.AppID] = struct{ price, regular float64 }{g.Price, g.RegularPrice}
	}
	for id, want := range map[int]struct{ price, regular float64 }{
		240:  {1.99, 9.99}, // discounted: regular comes from the "normally" phrase
		10:   {1.99, 9.99}, // package "10,80" is represented by its first app
		220:  {9.99, 9.99}, // not discounted: regular equals the price
		9999: {0, 0},       // no price block at all: unknown
	} {
		if got := byID[id]; got != want {
			t.Errorf("app %d: price/regular = %v, want %v", id, got, want)
		}
	}
	if b.Games[1].Name != "Counter-Strike: Condition Zero" || b.Games[1].AppID != 10 {
		t.Errorf("the package should be kept under its first app and its own name: %+v", b.Games[1])
	}
}

func TestParseBundleItemPrices(t *testing.T) {
	item := func(block string) *goquery.Selection {
		return bundleDoc(t, `<html><body><div class="tab_item">`+block+`</div></body></html>`).Find(".tab_item")
	}
	for _, tc := range []struct {
		name, block        string
		wantPrice, wantReg float64
	}{
		{"discount with the normally phrase", `<div class="discount_block" data-price-final="2499" data-discount="50" aria-label="50% off. $49.99 normally, discounted to $24.99"></div>`, 24.99, 49.99},
		{"thousands separator", `<div class="discount_block" data-price-final="100000" data-discount="20" aria-label="20% off. $1,250.00 normally, discounted to $1,000.00"></div>`, 1000, 1250},
		{"discount but no phrase: the discount is undone", `<div class="discount_block" data-price-final="750" data-discount="25"></div>`, 7.5, 10},
		{"no discount", `<div class="discount_block" data-price-final="1999" data-discount="0"></div>`, 19.99, 19.99},
		{"no discount attribute at all", `<div class="discount_block" data-price-final="500"></div>`, 5, 5},
		{"free item", `<div class="discount_block" data-price-final="0"></div>`, 0, 0},
		{"no price block", ``, 0, 0},
		{"garbage price", `<div class="discount_block" data-price-final="abc" data-discount="x"></div>`, 0, 0},
	} {
		p, r := parseBundleItemPrices(item(tc.block))
		if p != tc.wantPrice || r != tc.wantReg {
			t.Errorf("%s: got price %v regular %v, want %v / %v", tc.name, p, r, tc.wantPrice, tc.wantReg)
		}
	}
}
