package router_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func pinnedOf(t *testing.T, app *testutil.App, userID int64, appID int) (watched, pinned bool) {
	t.Helper()
	err := app.Pool.QueryRow(context.Background(), "SELECT true, pinned FROM watched_games WHERE user_id = $1 AND app_id = $2", userID, appID).Scan(&watched, &pinned)
	if err != nil {
		return false, false
	}
	return watched, pinned
}

func requestRows(t *testing.T, app *testutil.App, userID int64) map[int]bool {
	t.Helper()
	rows, err := app.Pool.Query(context.Background(), "SELECT app_id, pinned FROM wishlist_requests WHERE user_id = $1", userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		var pinned bool
		rows.Scan(&id, &pinned)
		out[id] = pinned
	}
	return out
}

func TestWishlistStatusTellsWhetherEverythingIsImported(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(850001, "Have One", []string{"d"}, []string{"p"})
	app.SeedGame(850002, "Have Two", []string{"d"}, []string{"p"})
	user := wishlistUser(t, app, wishSteamID)

	status := func() map[string]any {
		t.Helper()
		resp := user.Get("/api/me/wishlist/status")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		return resp.JSON()
	}

	t.Run("an empty or private wishlist", func(t *testing.T) {
		if got := status()["state"]; got != "empty" {
			t.Errorf("state = %v, want empty", got)
		}
	})

	t.Run("games we have but the user does not watch yet are not imported", func(t *testing.T) {
		steam.SetWishlist(wishSteamID, 850001, 850002)
		app.Mini.Del(fmt.Sprintf("wishlist:ids:%d", user.UserID))
		s := status()
		if s["state"] != "incomplete" || s["remaining"] != float64(2) || s["wishlist_size"] != float64(2) {
			t.Errorf("status = %v, want incomplete with 2 remaining", s)
		}
		if got := watchedCount(t, app, user.UserID); got != 0 {
			t.Errorf("checking the status must not watch anything, watching %d", got)
		}
	})

	t.Run("it is complete once they are all imported, and the page is told straight away", func(t *testing.T) {
		if resp := user.Post("/api/me/wishlist/import", nil); resp.JSON()["state"] != "complete" {
			t.Fatalf("import state = %v, want complete", resp.JSON()["state"])
		}
		if s := status(); s["state"] != "complete" || s["remaining"] != float64(0) {
			t.Errorf("status after import = %v (a stale cached answer?)", s)
		}
	})

	t.Run("a game added to the wishlist later makes it incomplete again", func(t *testing.T) {
		app.SeedGame(850003, "New Wish", []string{"d"}, []string{"p"})
		steam.SetWishlist(wishSteamID, 850001, 850002, 850003)
		app.Mini.Del(fmt.Sprintf("wishlist:ids:%d", user.UserID)) // the cached answer lasts a few minutes
		if got := status()["state"]; got != "incomplete" {
			t.Errorf("state = %v, want incomplete", got)
		}
	})

	t.Run("the answer is cached, so opening the feed repeatedly does not hit Steam", func(t *testing.T) {
		before := steam.Calls
		for i := 0; i < 5; i++ {
			status()
		}
		if steam.Calls-before > 1 {
			t.Errorf("Steam was called %d times for 5 status checks, want at most 1", steam.Calls-before)
		}
	})

	t.Run("needs a linked account and a signed-in user", func(t *testing.T) {
		if got := app.NewUser("nolink").Get("/api/me/wishlist/status").Code(); got != http.StatusBadRequest {
			t.Errorf("unlinked: status = %d, want 400", got)
		}
		if got := app.NewClient().Get("/api/me/wishlist/status").Code(); got != http.StatusUnauthorized {
			t.Errorf("signed out: status = %d, want 401", got)
		}
	})
}

func TestUnavailableGamesDoNotKeepTheWishlistIncomplete(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(850101, "Fine", []string{"d"}, []string{"p"})
	if _, err := app.Pool.Exec(context.Background(), "INSERT INTO tracked_games (app_id, status) VALUES (850102, 'rejected')"); err != nil {
		t.Fatal(err)
	}
	steam.SetWishlist(wishSteamID, 850101, 850102)
	user := wishlistUser(t, app, wishSteamID)

	j := user.Post("/api/me/wishlist/import", nil).JSON()
	if j["state"] != "complete" || j["unavailable"] != float64(1) {
		t.Errorf("state = %v unavailable = %v, want complete with 1 game that cannot be added", j["state"], j["unavailable"])
	}
}

func TestRequestingWithPinsWatchesAndPinsGamesOnceAdded(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	steam.SetWishlist(wishSteamID, 850201, 850202, 850203)
	user := wishlistUser(t, app, wishSteamID)

	resp := user.Post("/api/me/wishlist/request", map[string]any{
		"app_ids":        []int{850201, 850202, 850203},
		"pinned_app_ids": []int{850202, 999999}, // 999999 was not asked for, so it is ignored
	})
	if resp.Code() != http.StatusAccepted || resp.JSON()["requested"] != float64(3) {
		t.Fatalf("status = %d body = %s", resp.Code(), resp.Body.String())
	}
	if resp.JSON()["state"] != "waiting" {
		t.Errorf("state = %v, want waiting for the admin", resp.JSON()["state"])
	}
	if rows := requestRows(t, app, user.UserID); len(rows) != 3 || rows[850201] || !rows[850202] || rows[850203] {
		t.Errorf("remembered requests = %v, want all three with only 850202 pinned", rows)
	}
	if got := watchedCount(t, app, user.UserID); got != 0 {
		t.Errorf("watching %d games before any were added", got)
	}

	// An admin approves two of them and they get scraped: the games now exist.
	for _, id := range []int{850201, 850202} {
		app.SeedGame(id, "Added", []string{"d"}, []string{"p"})
		if n, err := app.DB.ApplyWishlistRequests(context.Background(), id); err != nil || n != 1 {
			t.Fatalf("apply %d: n=%d err=%v", id, n, err)
		}
	}
	if w, p := pinnedOf(t, app, user.UserID, 850201); !w || p {
		t.Errorf("850201 watched=%v pinned=%v, want watched and not pinned", w, p)
	}
	if w, p := pinnedOf(t, app, user.UserID, 850202); !w || !p {
		t.Errorf("850202 watched=%v pinned=%v, want watched and pinned", w, p)
	}
	if rows := requestRows(t, app, user.UserID); len(rows) != 1 || rows[850203] {
		t.Errorf("remaining requests = %v, want only the one not added yet", rows)
	}

	// Applying again changes nothing.
	if n, _ := app.DB.ApplyWishlistRequests(context.Background(), 850201); n != 0 {
		t.Errorf("a second apply touched %d rows", n)
	}
}

func TestPinnedChoiceCanBeChangedByAskingAgain(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("rechoose")
	ctx := context.Background()
	app.DB.RecordWishlistRequests(ctx, user.UserID, []database.WishlistRequest{{AppID: 850701, Pinned: false}})
	app.DB.RecordWishlistRequests(ctx, user.UserID, []database.WishlistRequest{{AppID: 850701, Pinned: true}})
	if rows := requestRows(t, app, user.UserID); !rows[850701] || len(rows) != 1 {
		t.Errorf("requests = %v, want one pinned request", rows)
	}
}

func TestApplyingRequestsKeepsAnExistingPin(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("pin")
	ctx := context.Background()
	app.SeedGame(850301, "Pinned Elsewhere", []string{"d"}, []string{"p"})
	app.DB.UpsertWatchedGame(ctx, user.UserID, 850301, true, nil)
	app.DB.RecordWishlistRequests(ctx, user.UserID, []database.WishlistRequest{{AppID: 850301, Pinned: false}})
	app.DB.ApplyWishlistRequests(ctx, 850301)
	if _, p := pinnedOf(t, app, user.UserID, 850301); !p {
		t.Error("an unpinned request must not unpin a game the user already pinned")
	}
}

func TestImportFollowsGamesSomeoneElseAlreadyRequested(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	if _, err := app.Pool.Exec(context.Background(), "INSERT INTO tracked_games (app_id, status) VALUES (850401, 'awaiting_approval')"); err != nil {
		t.Fatal(err)
	}
	steam.SetWishlist(wishSteamID, 850401)
	user := wishlistUser(t, app, wishSteamID)

	j := user.Post("/api/me/wishlist/import", nil).JSON()
	if j["state"] != "waiting" || j["awaiting_review"] != float64(1) {
		t.Errorf("state = %v awaiting = %v, want waiting on 1", j["state"], j["awaiting_review"])
	}
	// When it is added, this user watches it too, though they never asked an admin.
	app.SeedGame(850401, "Someone Elses Request", []string{"d"}, []string{"p"})
	app.DB.ApplyWishlistRequests(context.Background(), 850401)
	if w, _ := pinnedOf(t, app, user.UserID, 850401); !w {
		t.Error("the game was not added to their watchlist once it became tracked")
	}
}

func TestTurningAGameDownCancelsWishlistRequests(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	admin := app.NewAdmin()
	steam.SetWishlist(wishSteamID, 850501, 850502)
	user := wishlistUser(t, app, wishSteamID)
	user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{850501, 850502}})
	if len(requestRows(t, app, user.UserID)) != 2 {
		t.Fatal("setup: expected 2 remembered requests")
	}

	if got := admin.Post("/api/admin/items/app/850501/reject", nil).Code(); got != http.StatusNoContent {
		t.Fatalf("reject status = %d", got)
	}
	if rows := requestRows(t, app, user.UserID); len(rows) != 1 {
		t.Errorf("after a rejection: %v, want the rejected game request gone", rows)
	}
	if got := admin.Delete("/api/admin/items/app/850502").Code(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d", got)
	}
	if rows := requestRows(t, app, user.UserID); len(rows) != 0 {
		t.Errorf("after a deletion: %v, want none", rows)
	}
}

func TestDeletingAUserRemovesTheirWishlistRequests(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("gone")
	app.DB.RecordWishlistRequests(context.Background(), user.UserID, []database.WishlistRequest{{AppID: 850601}})
	if _, err := app.Pool.Exec(context.Background(), "DELETE FROM users WHERE user_id = $1", user.UserID); err != nil {
		t.Fatal(err)
	}
	var n int
	app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM wishlist_requests WHERE user_id = $1", user.UserID).Scan(&n)
	if n != 0 {
		t.Errorf("%d requests left behind for a deleted user", n)
	}
}
