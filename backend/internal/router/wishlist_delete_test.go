package router_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

// A game that was imported and then deleted must stop counting as imported,
// straight away, and be importable again.
func TestDeletedGameIsNoLongerImportedAndCanBeBroughtBack(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	admin := app.NewAdmin()
	app.SeedGame(860001, "Stays", []string{"d"}, []string{"p"})
	app.SeedGame(860002, "Gets Deleted", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 860001, 860002)
	user := wishlistUser(t, app, wishSteamID)

	status := func() map[string]any {
		t.Helper()
		resp := user.Get("/api/me/wishlist/status")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		return resp.JSON()
	}

	if j := user.Post("/api/me/wishlist/import", nil).JSON(); j["state"] != "complete" {
		t.Fatalf("setup: state = %v, want complete", j["state"])
	}
	if got := status()["state"]; got != "complete" {
		t.Fatalf("setup: status = %v, want complete", got)
	}

	// The admin deletes the game (this also removes it from every watchlist).
	if got := admin.Delete("/api/admin/items/app/860002").Code(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d", got)
	}

	t.Run("the status notices straight away, without waiting for a cache to expire", func(t *testing.T) {
		s := status()
		if s["state"] != "incomplete" || s["remaining"] != float64(1) {
			t.Errorf("status = %v, want incomplete with the deleted game remaining", s)
		}
	})

	t.Run("importing again offers the deleted game", func(t *testing.T) {
		j := user.Post("/api/me/wishlist/import", nil).JSON()
		if ids := requestableIDs(j); len(ids) != 1 || ids[0] != 860002 {
			t.Fatalf("requestable = %v, want the deleted game", ids)
		}
		if j["state"] != "incomplete" {
			t.Errorf("state = %v, want incomplete", j["state"])
		}
	})

	t.Run("and requesting it again works", func(t *testing.T) {
		resp := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{860002}, "pinned_app_ids": []int{860002}})
		if resp.Code() != http.StatusAccepted || resp.JSON()["requested"] != float64(1) {
			t.Fatalf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
		if got := status()["state"]; got != "waiting" {
			t.Errorf("state = %v, want waiting", got)
		}
		// Approved and scraped: it is back, watched and pinned, and the wishlist is complete.
		app.SeedGame(860002, "Gets Deleted", []string{"d"}, []string{"p"})
		if n, err := app.DB.ApplyWishlistRequests(context.Background(), 860002); err != nil || n != 1 {
			t.Fatalf("apply: n=%d err=%v", n, err)
		}
		if w, p := pinnedOf(t, app, user.UserID, 860002); !w || !p {
			t.Errorf("watched=%v pinned=%v, want both", w, p)
		}
		if got := status()["state"]; got != "complete" {
			t.Errorf("state = %v, want complete again", got)
		}
	})
}

// Someone deleting a game straight from the database leaves its tracked_games
// row behind; it must not make a missing game look imported.
func TestGameDeletedBehindTheAppsBackIsNotCountedAsImported(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(860101, "Orphaned", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 860101)
	user := wishlistUser(t, app, wishSteamID)
	if j := user.Post("/api/me/wishlist/import", nil).JSON(); j["state"] != "complete" {
		t.Fatalf("setup: %v", j["state"])
	}

	if _, err := app.Pool.Exec(context.Background(), "DELETE FROM games WHERE app_id = 860101"); err != nil {
		t.Fatal(err)
	}

	s := user.Get("/api/me/wishlist/status").JSON()
	if s["state"] == "complete" {
		t.Fatalf("status = %v: a game with no data counted as imported", s)
	}

	// The leftover row says "tracked", but there is nothing to watch, so the
	// import must be able to ask for it again rather than waiting forever.
	j := user.Post("/api/me/wishlist/import", nil).JSON()
	if ids := requestableIDs(j); len(ids) != 1 || ids[0] != 860101 {
		t.Errorf("requestable = %v, want the orphaned game to be requestable again", ids)
	}
	resp := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{860101}})
	if resp.Code() != http.StatusAccepted || resp.JSON()["requested"] != float64(1) {
		t.Errorf("request: status = %d body = %s", resp.Code(), resp.Body.String())
	}
}

func TestStatusFollowsWatchlistChangesImmediately(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(860201, "Watch Me", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 860201)
	user := wishlistUser(t, app, wishSteamID)
	state := func() any { return user.Get("/api/me/wishlist/status").JSON()["state"] }

	if state() != "incomplete" {
		t.Fatalf("state = %v, want incomplete before anything is watched", state())
	}
	// The user watches it from the game page instead of importing.
	if got := user.Put("/api/me/watchlist/860201", map[string]any{}).Code(); got != http.StatusNoContent {
		t.Fatalf("watch status = %d", got)
	}
	if state() != "complete" {
		t.Errorf("state = %v, want complete as soon as the game is watched", state())
	}
	if got := user.Delete("/api/me/watchlist/860201").Code(); got != http.StatusNoContent {
		t.Fatalf("unwatch status = %d", got)
	}
	if state() != "incomplete" {
		t.Errorf("state = %v, want incomplete again after unwatching", state())
	}
}

func TestStatusDoesNotAskSteamEveryTime(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(860301, "One", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 860301)
	user := wishlistUser(t, app, wishSteamID)
	for i := 0; i < 10; i++ {
		user.Get("/api/me/wishlist/status")
	}
	if steam.Calls != 1 {
		t.Errorf("Steam was asked %d times for 10 checks, want 1", steam.Calls)
	}
	// An import is a deliberate action and looks at Steam afresh.
	user.Post("/api/me/wishlist/import", nil)
	if steam.Calls != 2 {
		t.Errorf("Steam was asked %d times after an import, want 2", steam.Calls)
	}
}

func TestADeletedGameCanBeRequestedAgainThroughImportAndSubmission(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	// Left behind when a game is deleted from the database by hand: the row says tracked, the game is gone.
	if _, err := app.Pool.Exec(context.Background(), "INSERT INTO tracked_games (app_id, status) VALUES (860401, 'tracked')"); err != nil {
		t.Fatal(err)
	}
	steam.SetWishlist(wishSteamID, 860401)
	user := wishlistUser(t, app, wishSteamID)

	if j := user.Post("/api/me/wishlist/import", nil).JSON(); len(requestableIDs(j)) != 1 {
		t.Fatalf("import: %v, want the orphaned game offered", j)
	}
	resp := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{860401}})
	if resp.Code() != http.StatusAccepted || resp.JSON()["requested"] != float64(1) {
		t.Fatalf("request: status = %d body = %s", resp.Code(), resp.Body.String())
	}
	var status string
	app.Pool.QueryRow(context.Background(), "SELECT status FROM tracked_games WHERE app_id = 860401").Scan(&status)
	if status != "awaiting_approval" {
		t.Errorf("status = %q, want it back in the approval queue", status)
	}

	// The ordinary submission path handles a left-behind row the same way.
	if _, err := app.Pool.Exec(context.Background(), "INSERT INTO tracked_games (app_id, status) VALUES (860402, 'tracked')"); err != nil {
		t.Fatal(err)
	}
	sub := app.NewUser("sub").Post("/api/submissions", map[string]any{"url": "https://store.steampowered.com/app/860402"})
	if sub.Code() != http.StatusAccepted || sub.JSON()["status"] != "awaiting_approval" {
		t.Errorf("submission: status = %d body = %s", sub.Code(), sub.Body.String())
	}
}

func TestATrackedGameThatStillExistsIsNotRequestedAgain(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(860501, "Real", []string{"d"}, []string{"p"})
	sub := app.NewUser("dup").Post("/api/submissions", map[string]any{"url": "https://store.steampowered.com/app/860501"})
	if sub.Code() != http.StatusOK || sub.JSON()["status"] != "tracked" {
		t.Errorf("status = %d body = %s, want the existing game reported as already tracked", sub.Code(), sub.Body.String())
	}
}
