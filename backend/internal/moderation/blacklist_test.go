package moderation

import "testing"

func TestCompileRuleValidation(t *testing.T) {
	bad := []struct{ field, pattern string }{
		{"app_id", "abc"},
		{"app_id", "-5"},
		{"name", "("},
		{"name", ""},
		{"nope", "x"},
	}
	for _, c := range bad {
		if _, err := CompileRule(c.field, c.pattern); err == nil {
			t.Errorf("CompileRule(%q, %q) should fail", c.field, c.pattern)
		}
	}
}

func TestRuleMatches(t *testing.T) {
	game := GameMeta{
		AppID:      730,
		Name:       "Counter-Strike 2",
		Developers: []string{"Valve"},
		Publishers: []string{"Valve Corporation", "Other Co"},
	}
	cases := []struct {
		field, pattern string
		want           bool
	}{
		{"app_id", "730", true},
		{"app_id", "731", false},
		{"name", "^counter-strike", true},
		{"name", "dota", false},
		{"developer", "^valve$", true},
		{"developer", "^corp", false},
		{"publisher", "other", true},
		{"publisher", "^valve$", false},
	}
	for _, c := range cases {
		r, err := CompileRule(c.field, c.pattern)
		if err != nil {
			t.Fatalf("CompileRule(%q, %q): %v", c.field, c.pattern, err)
		}
		if got := r.Matches(game); got != c.want {
			t.Errorf("%s %q matches = %v, want %v", c.field, c.pattern, got, c.want)
		}
	}
}
