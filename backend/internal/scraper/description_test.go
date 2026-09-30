package scraper

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func sanitize(t *testing.T, inner string) string {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<div id="game_area_description">` + inner + `</div>`))
	if err != nil {
		t.Fatal(err)
	}
	return sanitizeDescriptionHTML(doc.Find("#game_area_description"))
}

func TestSanitizeDescriptionKeepsFormatting(t *testing.T) {
	got := sanitize(t, `
		<h2>About This Game</h2>
		<strong>The most-played game.</strong><br>Every day, <i>millions</i> play.<br><br>
		<h2 class="bb_tag">Features</h2>
		<ul class="bb_ul"><li>One</li><li>Two &amp; three</li></ul>
		<span class="bb_img_ctn"><img class="bb_img" src="https://shared.fastly.steamstatic.com/a.avif?t=1" width=628 height=200 /></span>`)

	for _, want := range []string{
		"<strong>The most-played game.</strong>",
		"<em>millions</em>",
		"<h2>Features</h2>",
		"<ul><li>One</li><li>Two &amp; three</li></ul>",
		`<img src="https://shared.fastly.steamstatic.com/a.avif?t=1" alt="" loading="lazy" width="628" height="200" />`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "About This Game") {
		t.Errorf("leading 'About This Game' heading should be removed:\n%s", got)
	}
	if strings.Contains(got, "class=") || strings.Contains(got, "span") {
		t.Errorf("classes and wrapper spans should be stripped:\n%s", got)
	}
}

func TestSanitizeDescriptionBlocksHostileMarkup(t *testing.T) {
	tests := map[string]string{
		"script tag":           `<p>hi</p><script>alert(1)</script>`,
		"inline handler":       `<p onclick="alert(1)">hi</p>`,
		"javascript link":      `<a href="javascript:alert(1)">click</a>`,
		"data link":            `<a href="data:text/html,<script>alert(1)</script>">x</a>`,
		"iframe":               `<iframe src="https://evil.example"></iframe>`,
		"external image":       `<img src="https://evil.example/pixel.png" />`,
		"http image":           `<img src="http://shared.fastly.steamstatic.com/a.png" />`,
		"image handler":        `<img src="https://a.steamstatic.com/a.png" onerror="alert(1)" />`,
		"style attr":           `<p style="background:url(javascript:alert(1))">x</p>`,
		"lookalike host":       `<img src="https://steamstatic.com.evil.example/a.png" />`,
		"credentials in image": `<img src="https://user:pw@a.steamstatic.com/a.png" />`,
	}
	for name, input := range tests {
		got := sanitize(t, input)
		lower := strings.ToLower(got)
		for _, bad := range []string{"<script", "onclick", "onerror", "javascript:", "data:", "<iframe", "evil.example", "style=", "user:pw"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s: output contains %q:\n%s", name, bad, got)
			}
		}
	}

	// A link keeps its text but gets safe attributes.
	got := sanitize(t, `<a href="https://store.steampowered.com/app/1" onclick="x()">Buy</a>`)
	want := `<a href="https://store.steampowered.com/app/1" target="_blank" rel="noopener noreferrer nofollow">Buy</a>`
	if got != want {
		t.Errorf("link: got %s, want %s", got, want)
	}
}

func TestSanitizeDescriptionVideoBecomesPoster(t *testing.T) {
	got := sanitize(t, `<video class="bb_img" autoplay muted loop poster="https://shared.fastly.steamstatic.com/p.avif?t=1"><source src="https://shared.fastly.steamstatic.com/v.webm" type="video/webm"></video>`)
	if strings.Contains(got, "video") || strings.Contains(got, "source") {
		t.Errorf("video markup should be removed: %s", got)
	}
	if !strings.Contains(got, `src="https://shared.fastly.steamstatic.com/p.avif?t=1"`) {
		t.Errorf("poster image should remain: %s", got)
	}
}
