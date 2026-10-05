package sanitize

import "testing"

func TestText(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"  hello   world  ", 50, "hello world"},
		{"a\x00b\x07c", 50, "abc"},
		{"ab\u200bcd", 50, "abcd"},
		{"x\u202eevil", 50, "xevil"},
		{"abcdef", 3, "abc"},
		{"a\n\tb", 50, "a b"},
		{"bad\xffbyte", 50, "badbyte"},
	}
	for _, c := range cases {
		if got := Text(c.in, c.max); got != c.want {
			t.Errorf("Text(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestIdentifier(t *testing.T) {
	if got, ok := Identifier("  me@x.com ", 254); !ok || got != "me@x.com" {
		t.Errorf("got %q, %v", got, ok)
	}
	for _, bad := range []string{"a b", "a\u200bb", "a\x00b"} {
		if _, ok := Identifier(bad, 254); ok {
			t.Errorf("Identifier(%q) should be rejected", bad)
		}
	}
}

func TestValidPassword(t *testing.T) {
	if !ValidPassword("correct horse battery staple!") || !ValidPassword("pässwörd 密码") {
		t.Error("ordinary passwords rejected")
	}
	if ValidPassword("a\x00b") || ValidPassword("a\xffb") {
		t.Error("control/invalid passwords accepted")
	}
}

func TestPattern(t *testing.T) {
	if got, ok := Pattern("  ^shady  games$ ", 50); !ok || got != "^shady  games$" {
		t.Errorf("got %q, %v; inner whitespace must be preserved exactly", got, ok)
	}
	bad := map[string]string{
		"empty":         "   ",
		"too long":      "aaaaaa",
		"control char":  "a\x00b",
		"zero width":    "a\u200bb",
		"invalid utf-8": "a\xffb",
	}
	for name, in := range bad {
		if _, ok := Pattern(in, 5); ok {
			t.Errorf("%s: %q should be rejected", name, in)
		}
	}
}
