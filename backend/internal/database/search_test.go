package database

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func TestSearchTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"half life", []string{"half", "life"}},
		{"  Half   LIFE  ", []string{"half", "life"}},
		{"Half-Life 2", []string{"half", "life", "2"}},
		{"Counter-Strike: Source", []string{"counter", "strike", "source"}},
		{"Garry's Mod", []string{"garry", "s", "mod"}},
		{"Pokémon Légendes", []string{"pokémon", "légendes"}},
		{"日本語 ゲーム", []string{"日本語", "ゲーム"}},
		{"left4dead", []string{"left4dead"}},
		{"", nil},
		{"   ", nil},
		{"+++ &&& ||| !!!", nil},
		{"a & b | c", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		if got := SearchTerms(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SearchTerms(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSearchTermsCannotCarryQuerySyntax(t *testing.T) {
	// Every character that means something to a full-text query must be gone.
	for _, in := range []string{
		"a & b", "a | b", "!a", "a:*", "a <-> b", "(a)", "'a'", `a\b`, "a'); DROP TABLE games;--", "a\x00b", "a\nb",
	} {
		for _, term := range SearchTerms(in) {
			if strings.ContainsAny(term, "&|!:*<>()'\\\x00 \n;-") {
				t.Errorf("SearchTerms(%q) kept query syntax in %q", in, term)
			}
		}
	}
}

func TestSearchTermsAreBounded(t *testing.T) {
	many := strings.Repeat("word ", 50)
	if got := SearchTerms(many); len(got) != maxSearchTerms {
		t.Errorf("%d terms kept, want %d", len(got), maxSearchTerms)
	}
	long := SearchTerms(strings.Repeat("x", 500))
	if len(long) != 1 || len([]rune(long[0])) != maxSearchTermLen {
		t.Errorf("a long word was kept as %d characters, want %d", len([]rune(long[0])), maxSearchTermLen)
	}
	multibyte := SearchTerms(strings.Repeat("é", 100))
	if len(multibyte) != 1 || len([]rune(multibyte[0])) != maxSearchTermLen {
		t.Errorf("multibyte word cut to %d characters", len([]rune(multibyte[0])))
	}
}

// collector stands in for the query builder's argument list.
type collector struct{ args []any }

func (c *collector) add(v any) string {
	c.args = append(c.args, v)
	return fmt.Sprintf("$%d", len(c.args))
}

func TestPrefixQueryUsesPlaceholdersOnly(t *testing.T) {
	c := &collector{}
	q := prefixQuery([]string{"half", "life"}, c.add)
	if strings.Contains(q, "half") || strings.Contains(q, "life") {
		t.Errorf("the words are in the SQL text itself: %s", q)
	}
	if want := "(to_tsquery('simple', $1) && to_tsquery('simple', $2))"; q != want {
		t.Errorf("query = %s, want %s", q, want)
	}
	if !reflect.DeepEqual(c.args, []any{"half:*", "life:*"}) {
		t.Errorf("args = %v, want each word as a prefix", c.args)
	}
}

func TestFullQueryMatchesNamesAsPrefixesAndDescriptionsAsEnglish(t *testing.T) {
	c := &collector{}
	q := fullQuery([]string{"run", "zombie"}, c.add)
	want := "((to_tsquery('simple', $1) || plainto_tsquery('english', $2)) && (to_tsquery('simple', $3) || plainto_tsquery('english', $4)))"
	if q != want {
		t.Errorf("query = %s\nwant    %s", q, want)
	}
	if !reflect.DeepEqual(c.args, []any{"run:*", "run", "zombie:*", "zombie"}) {
		t.Errorf("args = %v", c.args)
	}
}

func TestGameQueryUsesTheSearchVector(t *testing.T) {
	sql, args := buildGameQuery(models.GameFilters{Search: "half life", Limit: 10}, false)
	if !strings.Contains(sql, "g.search_vector @@") {
		t.Errorf("a text search does not use the search vector:\n%s", sql)
	}
	if strings.Contains(sql, "g.name ILIKE") || strings.Contains(sql, "g.description ILIKE") {
		t.Errorf("a text search still scans with ILIKE:\n%s", sql)
	}
	order := sql[strings.Index(sql, "ORDER BY"):]
	namedFirst := strings.Index(order, "to_tsvector('simple', g.name) @@")
	byRank := strings.Index(order, "ts_rank_cd(g.search_vector,")
	if namedFirst < 0 || byRank < 0 || namedFirst > byRank || !strings.Contains(order, "DESC, g.name") {
		t.Errorf("results should be ordered: name matches, then relevance, then name:\n%s", order)
	}
	for _, a := range args {
		if s, ok := a.(string); ok && strings.Contains(s, "%") {
			t.Errorf("a LIKE pattern is still being passed: %q", s)
		}
	}
	if strings.Contains(sql, "half") || strings.Contains(sql, "life") {
		t.Errorf("the user's words are in the SQL text:\n%s", sql)
	}
}

func TestGameQueryWithoutASearchIsOrderedByName(t *testing.T) {
	sql, _ := buildGameQuery(models.GameFilters{Limit: 10}, false)
	if strings.Contains(sql, "search_vector") || strings.Contains(sql, "ts_rank") {
		t.Errorf("full-text search used with no search text:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY g.name") {
		t.Errorf("not ordered by name:\n%s", sql)
	}
}

func TestBlankSearchIsNoSearch(t *testing.T) {
	for _, s := range []string{"", "   ", "\t\n"} {
		sql, _ := buildGameQuery(models.GameFilters{Search: s}, false)
		if strings.Contains(sql, "search_vector") || strings.Contains(sql, "ILIKE") {
			t.Errorf("search %q filtered the results:\n%s", s, sql)
		}
	}
}

func TestPunctuationOnlySearchMatchesTheTextAsTyped(t *testing.T) {
	sql, args := buildGameQuery(models.GameFilters{Search: " ++ "}, false)
	if strings.Contains(sql, "search_vector") {
		t.Errorf("no words to search for, yet the vector was used:\n%s", sql)
	}
	if !strings.Contains(sql, "g.name ILIKE $1") {
		t.Errorf("did not fall back to the name:\n%s", sql)
	}
	if len(args) != 1 || args[0] != "%++%" {
		t.Errorf("args = %v", args)
	}

	// LIKE wildcards in what the user typed are text, not wildcards.
	_, args = buildGameQuery(models.GameFilters{Search: `%_\`}, false)
	if len(args) != 1 || args[0] != `%\%\_\\%` {
		t.Errorf("wildcards were not escaped: %v", args)
	}

	// Text that does have words searches by them, whatever else is in it.
	sql, _ = buildGameQuery(models.GameFilters{Search: "C++"}, false)
	if !strings.Contains(sql, "search_vector @@") {
		t.Errorf("C++ has the word c, which should be searched:\n%s", sql)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_off\`); got != `50\%\_off\\` {
		t.Errorf("escapeLike = %q", got)
	}
}

func TestCountQueryHasNoRanking(t *testing.T) {
	sql, _ := buildGameQuery(models.GameFilters{Search: "half life"}, true)
	if strings.Contains(sql, "ORDER BY") || strings.Contains(sql, "ts_rank") {
		t.Errorf("a count does not need ordering:\n%s", sql)
	}
	if !strings.Contains(sql, "search_vector @@") {
		t.Errorf("the count ignores the search:\n%s", sql)
	}
}

func TestSearchFiltersCombineWithOtherFilters(t *testing.T) {
	sql, args := buildGameQuery(models.GameFilters{Search: "dragon", MinDiscount: 25, MaxPrice: 20, Genres: []string{"RPG"}}, false)
	for _, want := range []string{"g.search_vector @@", "g.discount_percentage >=", "g.price <=", "ge.genre ILIKE ANY"} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
	if len(args) < 5 {
		t.Errorf("args = %v", args)
	}
}
