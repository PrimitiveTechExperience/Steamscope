package scraper

import (
	"testing"
	"time"
)

func TestParseReleaseDate(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		in   string
		want time.Time
	}{
		{"Sep 2, 2026", d(2026, 9, 2)},
		{"2 Sep, 2026", d(2026, 9, 2)},
		{"September 2, 2026", d(2026, 9, 2)},
		{" 15 Jan, 2013 ", d(2013, 1, 15)},
		{"Sep 2026", d(2026, 9, 1)},
		{"2027", d(2027, 1, 1)},
		{"Coming soon", time.Time{}},
		{"", time.Time{}},
	}
	for _, c := range cases {
		if got := parseReleaseDate(c.in); !got.Equal(c.want) {
			t.Errorf("parseReleaseDate(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
