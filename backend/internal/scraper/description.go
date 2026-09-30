package scraper

import (
	"html"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"
)

// Steam store descriptions are author-written HTML (headings, bold/italic,
// lists, images, autoplaying clips). Since the authors are third parties, the
// markup is rebuilt from an allowlist rather than trusting it: unknown tags
// are unwrapped, dangerous ones dropped with their content, all attributes
// are discarded except a few re-validated ones, and images are limited to
// Steam's own CDNs.

// tag -> output tag. Anything not listed (and not in dropTags) is unwrapped.
var descriptionTags = map[string]string{
	"p": "p", "br": "br", "hr": "hr", "strong": "strong", "b": "strong",
	"i": "em", "em": "em", "u": "u", "s": "s", "strike": "s", "del": "s",
	"h1": "h2", "h2": "h2", "h3": "h3", "h4": "h4",
	"ul": "ul", "ol": "ol", "li": "li", "blockquote": "blockquote",
	"pre": "pre", "code": "code", "a": "a", "img": "img",
	"table": "table", "thead": "thead", "tbody": "tbody", "tr": "tr", "th": "th", "td": "td",
}

var voidTags = map[string]bool{"br": true, "hr": true, "img": true}

// Removed along with everything inside them.
var dropTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"form": true, "input": true, "button": true, "svg": true, "noscript": true,
	"template": true, "textarea": true, "select": true, "audio": true,
}

var allowedImageHostSuffixes = []string{
	".steamstatic.com", ".steampowered.com", ".steamusercontent.com", ".akamaihd.net",
}

// sanitizeDescriptionHTML returns safe HTML for the "About This Game" block,
// without Steam's leading "About This Game" heading.
func sanitizeDescriptionHTML(sel *goquery.Selection) string {
	if sel.Length() == 0 {
		return ""
	}
	var b strings.Builder
	first := true
	for n := sel.Nodes[0].FirstChild; n != nil; n = n.NextSibling {
		if first && n.Type == nethtml.ElementNode && n.Data == "h2" &&
			strings.EqualFold(strings.TrimSpace(nodeText(n)), "About This Game") {
			first = false
			continue
		}
		if n.Type != nethtml.TextNode || strings.TrimSpace(n.Data) != "" {
			first = false
		}
		writeSafeNode(&b, n)
	}
	return strings.TrimSpace(b.String())
}

func writeSafeNode(b *strings.Builder, n *nethtml.Node) {
	switch n.Type {
	case nethtml.TextNode:
		b.WriteString(html.EscapeString(n.Data))
	case nethtml.ElementNode:
		writeSafeElement(b, n)
	}
}

func writeSafeElement(b *strings.Builder, n *nethtml.Node) {
	tag := strings.ToLower(n.Data)
	if dropTags[tag] {
		return
	}
	if tag == "video" {
		// Autoplaying clips: show the poster frame as a still image.
		if src := safeImageURL(attr(n, "poster")); src != "" {
			b.WriteString(`<img src="` + html.EscapeString(src) + `" alt="" loading="lazy" />`)
		}
		return
	}

	out, allowed := descriptionTags[tag]
	if !allowed {
		writeSafeChildren(b, n)
		return
	}

	switch tag {
	case "img":
		src := safeImageURL(attr(n, "src"))
		if src == "" {
			return
		}
		b.WriteString(`<img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(attr(n, "alt")) + `" loading="lazy"`)
		if w := positiveInt(attr(n, "width")); w > 0 {
			b.WriteString(` width="` + strconv.Itoa(w) + `"`)
		}
		if h := positiveInt(attr(n, "height")); h > 0 {
			b.WriteString(` height="` + strconv.Itoa(h) + `"`)
		}
		b.WriteString(` />`)
	case "a":
		href := safeLinkURL(attr(n, "href"))
		if href == "" {
			writeSafeChildren(b, n)
			return
		}
		b.WriteString(`<a href="` + html.EscapeString(href) + `" target="_blank" rel="noopener noreferrer nofollow">`)
		writeSafeChildren(b, n)
		b.WriteString(`</a>`)
	default:
		b.WriteString("<" + out + ">")
		if !voidTags[out] {
			writeSafeChildren(b, n)
			b.WriteString("</" + out + ">")
		}
	}
}

func writeSafeChildren(b *strings.Builder, n *nethtml.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeSafeNode(b, c)
	}
}

func attr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func nodeText(n *nethtml.Node) string {
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func positiveInt(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v <= 0 || v > 5000 {
		return 0
	}
	return v
}

// safeImageURL returns raw if it's an https URL on one of Steam's CDNs.
func safeImageURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range allowedImageHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return u.String()
		}
	}
	return ""
}

// safeLinkURL returns raw if it's an absolute http(s) URL.
func safeLinkURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}
