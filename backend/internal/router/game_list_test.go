package router_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

// countingPool counts the queries sent to the database.
type countingPool struct {
	database.Pool
	queries atomic.Int64
}

func (c *countingPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.queries.Add(1)
	return c.Pool.Query(ctx, sql, args...)
}

func (c *countingPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.queries.Add(1)
	return c.Pool.QueryRow(ctx, sql, args...)
}

func (c *countingPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.queries.Add(1)
	return c.Pool.Exec(ctx, sql, args...)
}

// seedVaried stores a game with its own, distinguishable details.
func seedVaried(t *testing.T, app *testutil.App, id int, devs, pubs, genres, tags, langs []string) {
	t.Helper()
	g := models.Game{
		AppID: id, Name: fmt.Sprintf("Varied %d", id), URL: fmt.Sprintf("https://store.steampowered.com/app/%d", id),
		Description: "d", DescriptionHTML: "<p>d</p>", HeaderImage: "https://example.com/h.jpg",
		Price: 5, OriginalPrice: 10, DiscountPercentage: 50,
		Developers: devs, Publishers: pubs, Genres: genres, Tags: tags, SupportedLanguages: langs, ReviewScore: "Positive",
	}
	if err := app.DB.InsertGame(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	reviews := []models.Review{{
		RecommendationID: fmt.Sprintf("rec-%d", id), AppID: id, SteamID: "1", AuthorName: "A", Language: "english", Review: "great", VotedUp: true,
		TimestampCreated: time.Now(), TimestampUpdated: time.Now(),
	}}
	if err := app.DB.StoreReviews(context.Background(), id, reviews, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Pool.Exec(context.Background(), `INSERT INTO tracked_games (app_id, status) VALUES ($1, 'tracked') ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatal(err)
	}
}

func TestGameListShowsEachGamesOwnDetails(t *testing.T) {
	app := testutil.NewApp(t)
	seedVaried(t, app, 890001, []string{"Zed Studio", "Alpha Studio"}, []string{"Big Pub"}, []string{"Action", "RPG"}, []string{"Co-op", "Open World"}, []string{"English", "French"})
	seedVaried(t, app, 890002, []string{"Solo Dev"}, []string{"Indie Pub"}, []string{"Puzzle"}, []string{"Relaxing"}, []string{"English"})
	seedVaried(t, app, 890003, nil, nil, nil, nil, nil) // no details at all

	resp := app.NewClient().Get("/api/games?limit=100")
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	byID := map[float64]map[string]any{}
	for _, g := range resp.JSON()["games"].([]any) {
		m := g.(map[string]any)
		byID[m["app_id"].(float64)] = m
	}
	list := func(id float64, key string) []string {
		var out []string
		for _, v := range byID[id][key].([]any) {
			out = append(out, v.(string))
		}
		return out
	}
	eq := func(got, want []string) bool { return fmt.Sprint(got) == fmt.Sprint(want) }

	// Each game has only its own details, in a steady (alphabetical) order.
	if got := list(890001, "developers"); !eq(got, []string{"Alpha Studio", "Zed Studio"}) {
		t.Errorf("developers of 890001 = %v", got)
	}
	if got := list(890001, "genres"); !eq(got, []string{"Action", "RPG"}) {
		t.Errorf("genres of 890001 = %v", got)
	}
	if got := list(890001, "tags"); !eq(got, []string{"Co-op", "Open World"}) {
		t.Errorf("tags of 890001 = %v", got)
	}
	if got := list(890001, "supported_languages"); !eq(got, []string{"English", "French"}) {
		t.Errorf("languages of 890001 = %v", got)
	}
	if got := list(890002, "developers"); !eq(got, []string{"Solo Dev"}) {
		t.Errorf("developers of 890002 = %v (another game's leaked in?)", got)
	}
	if got := list(890002, "publishers"); !eq(got, []string{"Indie Pub"}) {
		t.Errorf("publishers of 890002 = %v", got)
	}
	if got := list(890002, "tags"); !eq(got, []string{"Relaxing"}) {
		t.Errorf("tags of 890002 = %v", got)
	}

	// A game with nothing gets empty lists, not null, so the page can always loop over them.
	for _, key := range []string{"developers", "publishers", "genres", "tags", "supported_languages", "reviews"} {
		v, ok := byID[890003][key].([]any)
		if !ok || len(v) != 0 {
			t.Errorf("%s of a game with no details = %v, want an empty list", key, byID[890003][key])
		}
	}
}

func TestGameListLeavesReviewsToTheGamePage(t *testing.T) {
	app := testutil.NewApp(t)
	seedVaried(t, app, 890101, []string{"d"}, []string{"p"}, []string{"g"}, []string{"t"}, []string{"English"})
	c := app.NewClient()

	for _, g := range c.Get("/api/games?limit=10").JSON()["games"].([]any) {
		if m := g.(map[string]any); m["app_id"] == float64(890101) && len(m["reviews"].([]any)) != 0 {
			t.Errorf("the list carries %d reviews per game; only the game page needs them", len(m["reviews"].([]any)))
		}
	}
	detail := c.Get("/api/games/890101").JSON()
	if reviews, _ := detail["reviews"].([]any); len(reviews) != 1 {
		t.Errorf("the game page has %d reviews, want its 1", len(reviews))
	}
}

func TestGameListQueriesDoNotGrowWithThePage(t *testing.T) {
	app := testutil.NewApp(t)
	for i := 0; i < 30; i++ {
		seedVaried(t, app, 891000+i, []string{"Dev"}, []string{"Pub"}, []string{"Action"}, []string{"Co-op"}, []string{"English"})
	}
	counted := &countingPool{Pool: app.DB.Pool}
	db := &database.DB{Pool: counted}

	run := func(limit int) (games int, queries int64) {
		before := counted.queries.Load()
		list, err := db.GetGames(context.Background(), models.GameFilters{Limit: limit})
		if err != nil {
			t.Fatal(err)
		}
		return len(list), counted.queries.Load() - before
	}
	smallN, small := run(2)
	bigN, big := run(30)
	if smallN != 2 || bigN != 30 {
		t.Fatalf("got %d and %d games", smallN, bigN)
	}
	if small != big {
		t.Errorf("%d queries for 2 games but %d for 30: the cost still grows with the page (one round trip each is what made the home page take five seconds)", small, big)
	}
	if big > 6 {
		t.Errorf("%d queries for a page, want 6 or fewer (the games plus five details)", big)
	}
}
