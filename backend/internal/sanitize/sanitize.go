// Package sanitize cleans free-text input from users before it is stored,
// cached or used in a key. SQL injection is already prevented by
// parameterised queries and XSS by Angular's output escaping; this removes
// the characters that are never legitimate in such fields (control
// characters, zero-width and bidi-override characters that enable spoofing)
// and bounds their length.
package sanitize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Text trims s, drops invalid UTF-8, control characters and invisible
// formatting characters (zero-width, bidi overrides), collapses runs of
// whitespace to single spaces, and truncates to max runes.
func Text(s string, max int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	lastSpace := true // also trims leading whitespace
	n := 0
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
				n++
			}
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == unicode.ReplacementChar:
			continue
		}
		if n >= max {
			break
		}
		b.WriteRune(r)
		lastSpace = false
		n++
	}
	return strings.TrimRight(b.String(), " ")
}

// Identifier is Text for single-line identifiers (emails, login names):
// the same cleaning, but any whitespace at all is rejected rather than
// collapsed. ok is false if the input contained whitespace or was altered
// by cleaning beyond outer trimming.
func Identifier(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	clean := Text(s, max)
	return clean, clean == s && !strings.ContainsFunc(clean, unicode.IsSpace)
}

// Password only checks what must hold for any password: valid UTF-8 and no
// NUL or other control characters. It is never trimmed or altered.
func ValidPassword(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}
