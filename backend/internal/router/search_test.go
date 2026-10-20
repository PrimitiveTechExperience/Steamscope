package router_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

// seedDescribed stores a tracked game with the given name and description.
func seedDescribed(t *testing.T, app *testutil.App, id int, name, description string) {
	t.Helper()
	g := models.Game{
		AppID: id, Name: name, URL: fmt.Sprintf("https://store.steampowered.com/app/%d", id),
		Description: description, DescriptionHTML: "<p>" + description + "</p>", HeaderImage: "https://example.com/h.jpg",
		ReleaseDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Price: 10, OriginalPrice: 10,
		Developers: []string{"Dev"}, Publishers: []string{"Pub"}, Genres: []string{"Action"}, Tags: []string{"Co-op"},
		SupportedLanguages: []string{"English"}, ReviewScore: "Positive",
	}
	if err := app.DB.InsertGame(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Pool.Exec(context.Background(), `INSERT INTO tracked_games (app_id, status) VALUES ($1, 'tracked') ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatal(err)
	}
}

func search(c *testutil.Client, q string, extra ...string) []string {
	path := "/api/games?limit=100&search=" + url.QueryEscape(q)
	for _, e := range extra {
		path += "&" + e
	}
	resp := c.Get(path)
	var names []string
	if resp.Code() != http.StatusOK {
		return []string{fmt.Sprintf("<status %d: %s>", resp.Code(), resp.Body.String())}
	}
	for _, g := range resp.JSON()["games"].([]any) {
		names = append(names, g.(map[string]any)["name"].(string))
	}
	return names
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestSearchFindsGamesByName(t *testing.T) {
	app := testutil.NewApp(t)
	for i, name := range []string{"Half-Life 2", "Half Moon Bay", "The Forest", "Left 4 Dead 2", "Counter-Strike: Source", "Garry's Mod", "Portal"} {
		seedDescribed(t, app, 900000+i, name, "A game.")
	}
	c := app.NewClient()

	cases := []struct {
		query string
		want  []string
	}{
		{"half-life", []string{"Half-Life 2"}},
		{"Half-Life 2", []string{"Half-Life 2"}},
		{"half life", []string{"Half-Life 2"}},
		{"HALF", []string{"Half Moon Bay", "Half-Life 2"}},
		{"half li", []string{"Half-Life 2"}},              // a half-typed word still finds it
		{"hal", []string{"Half Moon Bay", "Half-Life 2"}}, // so does a short start
		{"the forest", []string{"The Forest"}},
		{"the", []string{"The Forest"}},        // "the" is a stop word in English, but not in a name
		{"forest the", []string{"The Forest"}}, // word order does not matter
		{"left 4", []string{"Left 4 Dead 2"}},
		{"4 dead 2", []string{"Left 4 Dead 2"}},
		{"counter strike", []string{"Counter-Strike: Source"}},
		{"counter-strike: source", []string{"Counter-Strike: Source"}},
		{"garry", []string{"Garry's Mod"}},
		{"garry's mod", []string{"Garry's Mod"}},
		{"  portal  ", []string{"Portal"}},
		{"dragon", nil},
		{"half dragon", nil}, // every word must match
	}
	for _, tc := range cases {
		if got := sorted(search(c, tc.query)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestSearchFindsGamesByDescription(t *testing.T) {
	app := testutil.NewApp(t)
	seedDescribed(t, app, 900100, "Plain Title", "You are running from zombies across a ruined city.")
	seedDescribed(t, app, 900101, "Other Title", "A calm farming game about vegetables.")
	c := app.NewClient()

	cases := []struct {
		query string
		want  []string
	}{
		{"zombies", []string{"Plain Title"}},
		{"zombie", []string{"Plain Title"}}, // singular finds plural: the description is stemmed
		{"run", []string{"Plain Title"}},    // "running" in the text
		{"runs", []string{"Plain Title"}},
		{"ruined city", []string{"Plain Title"}},
		{"vegetable", []string{"Other Title"}},
		{"farming vegetables", []string{"Other Title"}},
		{"zombies farming", nil}, // the words are in different games
	}
	for _, tc := range cases {
		if got := sorted(search(c, tc.query)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestSearchRanksNameMatchesAboveDescriptionMatches(t *testing.T) {
	app := testutil.NewApp(t)
	// Alphabetically first, and its description is full of the word.
	seedDescribed(t, app, 900200, "Aardvark Adventures", "Dragon dragon dragon dragon: a tale of a dragon and another dragon.")
	seedDescribed(t, app, 900201, "Dragon Quest", "A role playing game.")
	seedDescribed(t, app, 900202, "Zebra Zone", "There is one dragon.")
	got := search(app.NewClient(), "dragon")
	if len(got) != 3 || got[0] != "Dragon Quest" {
		t.Errorf("results = %v, want the game named Dragon Quest first", got)
	}
	// Among the description-only matches, more mentions rank higher.
	if got[1] != "Aardvark Adventures" || got[2] != "Zebra Zone" {
		t.Errorf("results = %v, want the description matches by relevance", got)
	}
}

func TestSearchWithoutRelevanceDifferencesIsAlphabetical(t *testing.T) {
	app := testutil.NewApp(t)
	for i, n := range []string{"Zeta Pack", "Alpha Pack", "Mid Pack"} {
		seedDescribed(t, app, 900300+i, n, "x")
	}
	if got := search(app.NewClient(), "pack"); !reflect.DeepEqual(got, []string{"Alpha Pack", "Mid Pack", "Zeta Pack"}) {
		t.Errorf("results = %v", got)
	}
	// And with no search at all, as before.
	if got := search(app.NewClient(), ""); !reflect.DeepEqual(got, []string{"Alpha Pack", "Mid Pack", "Zeta Pack"}) {
		t.Errorf("no search = %v", got)
	}
}

func TestSearchPunctuationAndOperators(t *testing.T) {
	app := testutil.NewApp(t)
	seedDescribed(t, app, 900400, "C++ Basics", "x")
	seedDescribed(t, app, 900401, "Ordinary Game", "x")
	seedDescribed(t, app, 900402, "Half Life", "x")
	c := app.NewClient()

	t.Run("text with no words is matched as typed", func(t *testing.T) {
		if got := search(c, "++"); !reflect.DeepEqual(got, []string{"C++ Basics"}) {
			t.Errorf("++ = %v", got)
		}
	})
	t.Run("LIKE wildcards are plain text", func(t *testing.T) {
		for _, q := range []string{"%", "_", "%%", "\\"} {
			if got := search(c, q); len(got) != 0 {
				t.Errorf("%q matched %v; wildcards must not match everything", q, got)
			}
		}
	})
	t.Run("full-text operators are just separators", func(t *testing.T) {
		// "half | life" must not mean OR, "!half" must not mean NOT.
		if got := search(c, "half | life"); !reflect.DeepEqual(got, []string{"Half Life"}) {
			t.Errorf("half | life = %v", got)
		}
		if got := search(c, "ordinary | zzzz"); len(got) != 0 {
			t.Errorf("ordinary | zzzz = %v: | was treated as OR", got)
		}
		if got := search(c, "!ordinary"); !reflect.DeepEqual(got, []string{"Ordinary Game"}) {
			t.Errorf("!ordinary = %v: ! was treated as NOT", got)
		}
		if got := search(c, "half & life"); !reflect.DeepEqual(got, []string{"Half Life"}) {
			t.Errorf("half & life = %v", got)
		}
	})
	t.Run("nothing a user types can break the query", func(t *testing.T) {
		nasty := []string{
			"'", "\"", "(", ")", "((", "a'", "a:*", ":*", "a <-> b", "<->", "a & ", "& a", "| |", "!", "!!", "a:b", "a:AB",
			"'; DROP TABLE games; --", "\\", "\\'", "a\\", "*", "a*", "**", "\x00", "a\x00b", "\n", "\t half \t",
			strings.Repeat("a", 5000), strings.Repeat("a ", 500), strings.Repeat("(", 400), "日本語", "😀", "ß", "İ",
			"%00", "a%00b", "<script>alert(1)</script>", "../../etc/passwd", "${jndi:ldap://x}",
		}
		for _, q := range nasty {
			resp := c.Get("/api/games?search=" + url.QueryEscape(q))
			if resp.Code() != http.StatusOK && resp.Code() != http.StatusBadRequest {
				t.Errorf("search %q: status %d: %s", q, resp.Code(), resp.Body.String())
			}
		}
		// The table is still there.
		if got := search(c, "ordinary"); len(got) != 1 {
			t.Errorf("after the attempts: %v", got)
		}
	})
	t.Run("only the first eight words count", func(t *testing.T) {
		resp := c.Get("/api/games?search=" + url.QueryEscape("half life one two three four five six seven eight nine ten"))
		if resp.Code() != http.StatusOK {
			t.Errorf("status = %d", resp.Code())
		}
	})
}

func TestSearchCombinesWithTheOtherFilters(t *testing.T) {
	app := testutil.NewApp(t)
	seedDescribed(t, app, 900500, "Pack One", "x")
	seedDescribed(t, app, 900501, "Pack Two", "x")
	seedDescribed(t, app, 900502, "Other Game", "x")
	if _, err := app.Pool.Exec(context.Background(), `UPDATE games SET discount_percentage = 50, price = 5 WHERE app_id = 900500`); err != nil {
		t.Fatal(err)
	}
	c := app.NewClient()

	if got := search(c, "pack", "minDiscount=25"); !reflect.DeepEqual(got, []string{"Pack One"}) {
		t.Errorf("search + discount = %v", got)
	}
	if got := search(c, "pack", "maxPrice=7"); !reflect.DeepEqual(got, []string{"Pack One"}) {
		t.Errorf("search + price = %v", got)
	}
	if got := search(c, "pack", "genres=Action"); len(got) != 2 {
		t.Errorf("search + genre = %v", got)
	}
	if got := search(c, "pack", "genres=Puzzle"); len(got) != 0 {
		t.Errorf("search + a genre they lack = %v", got)
	}
	if got := search(c, "other", "minDiscount=25"); len(got) != 0 {
		t.Errorf("a matching name that fails the discount filter was returned: %v", got)
	}
}

func TestSearchResultsAreCachedPerSearch(t *testing.T) {
	app := testutil.NewApp(t)
	seedDescribed(t, app, 900600, "Alpha Quest", "x")
	seedDescribed(t, app, 900601, "Beta Racer", "x")
	c := app.NewClient()
	first := c.Get("/api/games?search=alpha")
	if first.Header().Get("X-Cache") != "MISS" {
		t.Errorf("first request X-Cache = %q", first.Header().Get("X-Cache"))
	}
	if c.Get("/api/games?search=alpha").Header().Get("X-Cache") != "HIT" {
		t.Error("the repeat was not served from the cache")
	}
	// A different search must not be served alpha's result.
	if got := search(c, "beta"); !reflect.DeepEqual(got, []string{"Beta Racer"}) {
		t.Errorf("beta = %v (served another search's cached result?)", got)
	}
}

// ---- bundles ----

func seedSearchBundle(t *testing.T, app *testutil.App, id int, name string, games ...models.BundleGame) {
	t.Helper()
	app.SeedBundle(id, name, games...)
}

func bundleNames(resp testutil.Response) []string {
	var names []string
	raw, _ := resp.JSON()["bundles"].([]any)
	for _, b := range raw {
		names = append(names, b.(map[string]any)["name"].(string))
	}
	return names
}

func searchBundles(c *testutil.Client, q string, extra ...string) testutil.Response {
	path := "/api/games?includeBundles=true&limit=50"
	if q != "" {
		path += "&search=" + url.QueryEscape(q)
	}
	for _, e := range extra {
		path += "&" + e
	}
	return c.Get(path)
}

func TestBundlesAreOnlyIncludedWhenAskedFor(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 910001, "Valve Pack", models.BundleGame{AppID: 1, Name: "Portal"})
	c := app.NewClient()

	for _, q := range []string{"", "&includeBundles=false", "&includeBundles=0"} {
		resp := c.Get("/api/games?search=valve" + q)
		if _, present := resp.JSON()["bundles"]; present {
			t.Errorf("%q: bundles were returned without being asked for", q)
		}
	}
	if got := bundleNames(searchBundles(c, "valve")); !reflect.DeepEqual(got, []string{"Valve Pack"}) {
		t.Errorf("with includeBundles=true: %v", got)
	}
	if got := bundleNames(c.Get("/api/games?search=valve&includeBundles=1")); !reflect.DeepEqual(got, []string{"Valve Pack"}) {
		t.Errorf("includeBundles=1: %v", got)
	}
	for _, bad := range []string{"yes", "maybe", "2", "TRUE", "<script>"} {
		if got := c.Get("/api/games?includeBundles=" + url.QueryEscape(bad)).Code(); got != http.StatusBadRequest {
			t.Errorf("includeBundles=%q: status %d, want 400", bad, got)
		}
	}
}

func TestBundlesAreFoundByTheirOwnName(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 910101, "Valve Complete Pack", models.BundleGame{AppID: 1, Name: "Portal"})
	seedSearchBundle(t, app, 910102, "Indie Mega Bundle", models.BundleGame{AppID: 2, Name: "Celeste"})
	c := app.NewClient()
	cases := map[string][]string{
		"valve":          {"Valve Complete Pack"},
		"valv":           {"Valve Complete Pack"}, // a half-typed word
		"VALVE PACK":     {"Valve Complete Pack"},
		"complete pack":  {"Valve Complete Pack"},
		"bundle":         {"Indie Mega Bundle"},
		"mega indie":     {"Indie Mega Bundle"},
		"pack bundle":    nil, // the words are in different bundles
		"nothing at all": nil,
	}
	for q, want := range cases {
		if got := bundleNames(searchBundles(c, q)); !reflect.DeepEqual(got, want) {
			t.Errorf("bundles for %q = %v, want %v", q, got, want)
		}
	}
}

func TestBundlesAreFoundByTheGamesInThem(t *testing.T) {
	app := testutil.NewApp(t)
	seedDescribed(t, app, 910201, "Portal 2", "puzzles")
	seedSearchBundle(t, app, 910202, "Valve Complete Pack",
		models.BundleGame{AppID: 910201, Name: "Portal 2"},        // a game we track
		models.BundleGame{AppID: 910299, Name: "Half-Life: Alyx"}, // a game we do not
		models.BundleGame{AppID: 910298, Name: "Team Fortress 2"})
	seedSearchBundle(t, app, 910203, "Puzzle Starter", models.BundleGame{AppID: 910201, Name: "Portal 2"}, models.BundleGame{AppID: 910297, Name: "The Witness"})
	seedSearchBundle(t, app, 910204, "Racing Bundle", models.BundleGame{AppID: 910296, Name: "Forza"})
	c := app.NewClient()

	t.Run("a tracked game", func(t *testing.T) {
		resp := searchBundles(c, "portal")
		got := bundleNames(resp)
		sort.Strings(got)
		if !reflect.DeepEqual(got, []string{"Puzzle Starter", "Valve Complete Pack"}) {
			t.Errorf("bundles containing Portal = %v", got)
		}
		// And the game itself is still in the game results.
		games := resp.JSON()["games"].([]any)
		if len(games) != 1 || games[0].(map[string]any)["name"] != "Portal 2" {
			t.Errorf("games = %v", games)
		}
	})

	t.Run("a game we do not track", func(t *testing.T) {
		if got := bundleNames(searchBundles(c, "alyx")); !reflect.DeepEqual(got, []string{"Valve Complete Pack"}) {
			t.Errorf("bundles containing Half-Life: Alyx = %v", got)
		}
		if got := bundleNames(searchBundles(c, "witness")); !reflect.DeepEqual(got, []string{"Puzzle Starter"}) {
			t.Errorf("bundles containing The Witness = %v", got)
		}
	})

	t.Run("a half-typed game name", func(t *testing.T) {
		if got := bundleNames(searchBundles(c, "team fort")); !reflect.DeepEqual(got, []string{"Valve Complete Pack"}) {
			t.Errorf("bundles = %v", got)
		}
	})

	t.Run("says which games matched", func(t *testing.T) {
		for _, b := range searchBundles(c, "portal").JSON()["bundles"].([]any) {
			m := b.(map[string]any)
			matched, _ := m["matched_games"].([]any)
			if len(matched) != 1 || matched[0] != "Portal 2" {
				t.Errorf("%v: matched_games = %v, want [Portal 2]", m["name"], matched)
			}
		}
	})

	t.Run("a bundle with no matching game is left out", func(t *testing.T) {
		for _, name := range bundleNames(searchBundles(c, "portal")) {
			if name == "Racing Bundle" {
				t.Error("an unrelated bundle was returned")
			}
		}
	})
}

func TestBundlesMatchedByNameComeBeforeBundlesContainingTheGame(t *testing.T) {
	app := testutil.NewApp(t)
	// The containing bundles have the bigger discount; the one named for the search should still lead.
	seedSearchBundle(t, app, 910301, "Portal Collection", models.BundleGame{AppID: 1, Name: "Something"})
	seedSearchBundle(t, app, 910302, "Big Sale Pack", models.BundleGame{AppID: 910390, Name: "Portal 2"})
	if _, err := app.Pool.Exec(context.Background(), `UPDATE bundles SET discount_percentage = 10 WHERE bundle_id = 910301`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Pool.Exec(context.Background(), `UPDATE bundles SET discount_percentage = 90 WHERE bundle_id = 910302`); err != nil {
		t.Fatal(err)
	}
	got := bundleNames(searchBundles(app.NewClient(), "portal"))
	if !reflect.DeepEqual(got, []string{"Portal Collection", "Big Sale Pack"}) {
		t.Errorf("bundles = %v, want the one named Portal first, then the one containing it", got)
	}
	// And a bundle named for the search gets no "contains" note, since that is not why it matched.
	first := searchBundles(app.NewClient(), "portal").JSON()["bundles"].([]any)[0].(map[string]any)
	if m, _ := first["matched_games"].([]any); len(m) != 0 {
		t.Errorf("matched_games = %v for a bundle matched by name only", m)
	}
}

func TestBundlesWithoutASearchAreListedByDiscount(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 910401, "Small Deal")
	seedSearchBundle(t, app, 910402, "Big Deal")
	seedSearchBundle(t, app, 910403, "Medium Deal")
	for id, d := range map[int]int{910401: 10, 910402: 80, 910403: 40} {
		app.Pool.Exec(context.Background(), `UPDATE bundles SET discount_percentage = $2 WHERE bundle_id = $1`, id, d)
	}
	if got := bundleNames(searchBundles(app.NewClient(), "")); !reflect.DeepEqual(got, []string{"Big Deal", "Medium Deal", "Small Deal"}) {
		t.Errorf("bundles = %v", got)
	}
}

func TestOnlyTrackedBundlesAreSearchable(t *testing.T) {
	app := testutil.NewApp(t)
	for id, status := range map[int]string{910501: "tracked", 910502: "pending", 910503: "awaiting_approval", 910504: "failed", 910505: "rejected"} {
		seedSearchBundle(t, app, id, "Hidden Treasure Pack "+status, models.BundleGame{AppID: id, Name: "Treasure Game " + status})
		app.Pool.Exec(context.Background(), `UPDATE bundles SET status = $2 WHERE bundle_id = $1`, id, status)
	}
	got := bundleNames(searchBundles(app.NewClient(), "treasure"))
	if !reflect.DeepEqual(got, []string{"Hidden Treasure Pack tracked"}) {
		t.Errorf("bundles = %v, want only the tracked one", got)
	}
}

func TestBundleSearchHonoursPriceAndDiscountFilters(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 910601, "Cheap Pack")
	seedSearchBundle(t, app, 910602, "Dear Pack")
	app.Pool.Exec(context.Background(), `UPDATE bundles SET price = 5, discount_percentage = 20 WHERE bundle_id = 910601`)
	app.Pool.Exec(context.Background(), `UPDATE bundles SET price = 50, discount_percentage = 70 WHERE bundle_id = 910602`)
	c := app.NewClient()

	cases := []struct {
		filters string
		want    []string
	}{
		{"", []string{"Dear Pack", "Cheap Pack"}},
		{"maxPrice=10", []string{"Cheap Pack"}},
		{"minPrice=10", []string{"Dear Pack"}},
		{"minDiscount=50", []string{"Dear Pack"}},
		{"minDiscount=10", []string{"Dear Pack", "Cheap Pack"}},
		{"minDiscount=50&maxPrice=10", nil},
	}
	for _, tc := range cases {
		if got := bundleNames(searchBundles(c, "pack", tc.filters)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("filters %q: bundles = %v, want %v", tc.filters, got, tc.want)
		}
	}
}

func TestBundlesAreLeftOutWhenAGameOnlyFilterIsSet(t *testing.T) {
	app := testutil.NewApp(t)
	seedDescribed(t, app, 910701, "Pack Game", "x")
	seedSearchBundle(t, app, 910702, "Pack Bundle", models.BundleGame{AppID: 910701, Name: "Pack Game"})
	c := app.NewClient()
	if got := bundleNames(searchBundles(c, "pack")); len(got) != 1 {
		t.Fatalf("setup: bundles = %v", got)
	}
	// A bundle has no genre, tag, developer, publisher or language of its own to judge.
	for _, f := range []string{"genres=Action", "tags=Co-op", "developers=Dev", "publishers=Pub", "languages=English", "genre=Action", "developer=Dev", "publisher=Pub"} {
		resp := searchBundles(c, "pack", f)
		if got := bundleNames(resp); len(got) != 0 {
			t.Errorf("with %s: bundles = %v, want none", f, got)
		}
		if resp.Code() != http.StatusOK || len(resp.JSON()["games"].([]any)) != 1 {
			t.Errorf("with %s: the games should still be searched (%d)", f, resp.Code())
		}
	}
}

func TestBundlesComeWithTheFirstPageOnly(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 910801, "Page Pack")
	c := app.NewClient()
	if got := bundleNames(searchBundles(c, "page")); len(got) != 1 {
		t.Fatalf("first page: %v", got)
	}
	if got := bundleNames(searchBundles(c, "page", "offset=20")); len(got) != 0 {
		t.Errorf("a later page repeated the bundles: %v", got)
	}
}

func TestBundleResultsAreCapped(t *testing.T) {
	app := testutil.NewApp(t)
	for i := 0; i < 20; i++ {
		seedSearchBundle(t, app, 910900+i, fmt.Sprintf("Cap Pack %02d", i))
	}
	c := app.NewClient()
	if got := bundleNames(searchBundles(c, "cap")); len(got) != 12 {
		t.Errorf("%d bundles, want the cap of 12", len(got))
	}
	if got := bundleNames(c.Get("/api/games?includeBundles=true&search=cap&limit=3")); len(got) != 3 {
		t.Errorf("%d bundles with limit=3, want 3", len(got))
	}
}

func TestMatchedGamesAreCapped(t *testing.T) {
	app := testutil.NewApp(t)
	games := []models.BundleGame{}
	for i := 0; i < 6; i++ {
		games = append(games, models.BundleGame{AppID: 920000 + i, Name: fmt.Sprintf("Racer %d", i)})
	}
	seedSearchBundle(t, app, 920100, "Everything Pack", games...)
	b := searchBundles(app.NewClient(), "racer").JSON()["bundles"].([]any)[0].(map[string]any)
	matched := b["matched_games"].([]any)
	if len(matched) != 3 || matched[0] != "Racer 0" || matched[2] != "Racer 2" {
		t.Errorf("matched_games = %v, want the first three alphabetically", matched)
	}
}

func TestBundleSearchIsCachedSeparately(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 920201, "Cache Pack")
	c := app.NewClient()
	with := searchBundles(c, "cache")
	if with.Header().Get("X-Cache") != "MISS" || len(bundleNames(with)) != 1 {
		t.Fatalf("first: X-Cache %q bundles %v", with.Header().Get("X-Cache"), bundleNames(with))
	}
	if searchBundles(c, "cache").Header().Get("X-Cache") != "HIT" {
		t.Error("the repeat was not cached")
	}
	// The same search without bundles must not be served the cached one with them.
	without := c.Get("/api/games?search=cache&limit=50")
	if _, present := without.JSON()["bundles"]; present {
		t.Error("a search without bundles was served the cached search with them")
	}
}

func TestBundleSearchCannotBeBroken(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 920301, "C++ Pack", models.BundleGame{AppID: 1, Name: "Game"})
	c := app.NewClient()
	for _, q := range []string{"'", "(", "a:*", "!", "a | b", "a & ", "\\", "%", "_", "'; DROP TABLE bundles; --", "\x00", strings.Repeat("a ", 400), "😀"} {
		resp := searchBundles(c, q)
		if resp.Code() != http.StatusOK && resp.Code() != http.StatusBadRequest {
			t.Errorf("search %q: status %d: %s", q, resp.Code(), resp.Body.String())
		}
	}
	if got := bundleNames(searchBundles(c, "++")); !reflect.DeepEqual(got, []string{"C++ Pack"}) {
		t.Errorf("++ = %v", got)
	}
	if got := bundleNames(searchBundles(c, "%")); len(got) != 0 {
		t.Errorf("%% matched %v", got)
	}
	if got := bundleNames(searchBundles(c, "")); len(got) != 1 {
		t.Errorf("the bundle is gone after the attempts: %v", got)
	}
}

func TestBundleCardsInSearchCarryWhatTheCardNeeds(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 920401, "Card Pack",
		models.BundleGame{AppID: 1, Name: "One"}, models.BundleGame{AppID: 2, Name: "Two"}, models.BundleGame{AppID: 3, Name: "Three"})
	b := searchBundles(app.NewClient(), "card").JSON()["bundles"].([]any)[0].(map[string]any)
	for _, key := range []string{"bundle_id", "name", "price", "original_price", "discount_percentage", "at_record_low", "game_count", "games", "header_image"} {
		if _, ok := b[key]; !ok {
			t.Errorf("bundle result lacks %q", key)
		}
	}
	if b["game_count"] != float64(3) || len(b["games"].([]any)) != 3 {
		t.Errorf("game_count %v, games %v", b["game_count"], b["games"])
	}
}

func TestRecentSearchesRememberTheBundlesSwitch(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("recent")

	if got := user.Post("/api/me/recent-searches", map[string]any{"search": "portal", "include_bundles": true}).Code(); got != http.StatusNoContent {
		t.Fatalf("status = %d", got)
	}
	saved := user.Get("/api/me/recent-searches").JSONArray()
	if len(saved) != 1 || saved[0].(map[string]any)["search"] != "portal" || saved[0].(map[string]any)["include_bundles"] != true {
		t.Errorf("saved = %v", saved)
	}
	// The switch alone is not a search worth remembering.
	user.Post("/api/me/recent-searches", map[string]any{"include_bundles": true})
	if n := len(user.Get("/api/me/recent-searches").JSONArray()); n != 1 {
		t.Errorf("%d searches saved, want the switch on its own ignored", n)
	}
	// A text-only search is remembered, without the switch.
	user.Post("/api/me/recent-searches", map[string]any{"search": "half life"})
	list := user.Get("/api/me/recent-searches").JSONArray()
	if len(list) != 2 {
		t.Fatalf("%d searches saved, want 2", len(list))
	}
	if _, has := list[0].(map[string]any)["include_bundles"]; has {
		t.Errorf("a search saved without the switch carries it: %v", list[0])
	}
}

func bundlePageNames(t *testing.T, resp testutil.Response) []string {
	t.Helper()
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	var names []string
	for _, b := range resp.JSONArray() {
		names = append(names, b.(map[string]any)["name"].(string))
	}
	return names
}

func TestBundlesPageSearch(t *testing.T) {
	app := testutil.NewApp(t)
	seedSearchBundle(t, app, 930001, "Valve Complete Pack", models.BundleGame{AppID: 930101, Name: "Portal 2"}, models.BundleGame{AppID: 930102, Name: "Half-Life: Alyx"})
	seedSearchBundle(t, app, 930002, "Puzzle Starter", models.BundleGame{AppID: 930101, Name: "Portal 2"})
	seedSearchBundle(t, app, 930003, "Racing Bundle", models.BundleGame{AppID: 930103, Name: "Forza"})
	seedSearchBundle(t, app, 930004, "Hidden Pack", models.BundleGame{AppID: 930104, Name: "Hidden Game"})
	app.Pool.Exec(context.Background(), `UPDATE bundles SET status = 'pending' WHERE bundle_id = 930004`)
	c := app.NewClient()

	t.Run("without a search it lists every tracked bundle, as before", func(t *testing.T) {
		got := sorted(bundlePageNames(t, c.Get("/api/bundles")))
		if !reflect.DeepEqual(got, []string{"Puzzle Starter", "Racing Bundle", "Valve Complete Pack"}) {
			t.Errorf("bundles = %v", got)
		}
		if got := bundlePageNames(t, c.Get("/api/bundles?search=")); len(got) != 3 {
			t.Errorf("an empty search = %v, want them all", got)
		}
	})

	t.Run("finds bundles by name, including a half-typed word", func(t *testing.T) {
		for _, q := range []string{"valve", "VALV", "complete pack"} {
			if got := bundlePageNames(t, c.Get("/api/bundles?search="+url.QueryEscape(q))); !reflect.DeepEqual(got, []string{"Valve Complete Pack"}) {
				t.Errorf("search %q = %v", q, got)
			}
		}
	})

	t.Run("finds bundles by a game in them, and says which", func(t *testing.T) {
		resp := c.Get("/api/bundles?search=portal")
		if got := sorted(bundlePageNames(t, resp)); !reflect.DeepEqual(got, []string{"Puzzle Starter", "Valve Complete Pack"}) {
			t.Errorf("search portal = %v", got)
		}
		for _, b := range resp.JSONArray() {
			m := b.(map[string]any)
			if matched, _ := m["matched_games"].([]any); len(matched) != 1 || matched[0] != "Portal 2" {
				t.Errorf("%v: matched_games = %v", m["name"], m["matched_games"])
			}
		}
		if got := bundlePageNames(t, c.Get("/api/bundles?search=alyx")); !reflect.DeepEqual(got, []string{"Valve Complete Pack"}) {
			t.Errorf("search alyx = %v", got)
		}
	})

	t.Run("finds nothing for a search nothing matches, and never untracked bundles", func(t *testing.T) {
		if got := bundlePageNames(t, c.Get("/api/bundles?search=zzzz")); len(got) != 0 {
			t.Errorf("search zzzz = %v", got)
		}
		if got := bundlePageNames(t, c.Get("/api/bundles?search=hidden")); len(got) != 0 {
			t.Errorf("a pending bundle was found: %v", got)
		}
	})

	t.Run("search and app_id cannot be combined", func(t *testing.T) {
		resp := c.Get("/api/bundles?search=portal&app_id=930101")
		if resp.Code() != http.StatusBadRequest || resp.Error() == "" {
			t.Errorf("status = %d body = %s, want a JSON 400", resp.Code(), resp.Body.String())
		}
	})

	t.Run("the limit still applies", func(t *testing.T) {
		if got := bundlePageNames(t, c.Get("/api/bundles?search=portal&limit=1")); len(got) != 1 {
			t.Errorf("limit=1 returned %d", len(got))
		}
	})

	t.Run("nothing typed can break it", func(t *testing.T) {
		for _, q := range []string{"'", "(", "a:*", "!", "a | b", "\\", "%", "_", "'; DROP TABLE bundles; --", strings.Repeat("a ", 300)} {
			if got := c.Get("/api/bundles?search=" + url.QueryEscape(q)).Code(); got != http.StatusOK {
				t.Errorf("search %q: status %d", q, got)
			}
		}
		if got := bundlePageNames(t, c.Get("/api/bundles?search=%25")); len(got) != 0 {
			t.Errorf("%% matched %v", got)
		}
	})
}

func TestOnlyTheFiveLatestRecentSearchesAreKept(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("five")
	for i := 1; i <= 7; i++ {
		if got := user.Post("/api/me/recent-searches", map[string]any{"search": fmt.Sprintf("search %d", i)}).Code(); got != http.StatusNoContent {
			t.Fatalf("save %d: status %d", i, got)
		}
	}
	var got []string
	for _, s := range user.Get("/api/me/recent-searches").JSONArray() {
		got = append(got, s.(map[string]any)["search"].(string))
	}
	if !reflect.DeepEqual(got, []string{"search 7", "search 6", "search 5", "search 4", "search 3"}) {
		t.Errorf("kept %v, want the five latest, newest first", got)
	}
	// Repeating an older one moves it to the front without growing the list.
	user.Post("/api/me/recent-searches", map[string]any{"search": "search 4"})
	got = nil
	for _, s := range user.Get("/api/me/recent-searches").JSONArray() {
		got = append(got, s.(map[string]any)["search"].(string))
	}
	if !reflect.DeepEqual(got, []string{"search 4", "search 7", "search 6", "search 5", "search 3"}) {
		t.Errorf("after repeating one: %v", got)
	}
}
