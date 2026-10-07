package handlers_test

import (
	"context"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

// These cover what happens to wishlist requests when the worker finishes with
// a submitted game, the step an admin approval leads to.

func requestRow(t *testing.T, app *testutil.App, userID int64, appID int) (exists, pinned bool) {
	t.Helper()
	err := app.Pool.QueryRow(context.Background(), "SELECT true, pinned FROM wishlist_requests WHERE user_id = $1 AND app_id = $2", userID, appID).Scan(&exists, &pinned)
	if err != nil {
		return false, false
	}
	return exists, pinned
}

func watching(t *testing.T, app *testutil.App, userID int64, appID int) (watched, pinned bool) {
	t.Helper()
	err := app.Pool.QueryRow(context.Background(), "SELECT true, pinned FROM watched_games WHERE user_id = $1 AND app_id = $2", userID, appID).Scan(&watched, &pinned)
	if err != nil {
		return false, false
	}
	return watched, pinned
}

func TestApprovedGameBecomesWatchedAndPinnedForTheUsersWhoAskedForIt(t *testing.T) {
	app := testutil.NewApp(t)
	ctx := context.Background()
	pinner, plain, bystander := app.NewUser("pinner"), app.NewUser("plain"), app.NewUser("bystander")
	const id = 870001
	if _, err := app.Pool.Exec(ctx, "INSERT INTO tracked_games (app_id, status, submitted_by) VALUES ($1, 'pending', $2)", id, pinner.UserID); err != nil {
		t.Fatal(err)
	}
	app.DB.RecordWishlistRequests(ctx, pinner.UserID, []database.WishlistRequest{{AppID: id, Pinned: true}})
	app.DB.RecordWishlistRequests(ctx, plain.UserID, []database.WishlistRequest{{AppID: id}})

	scrape := func(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, ids []int) error {
		app.SeedGame(ids[0], "Approved Game", []string{"d"}, []string{"p"}) // the scrape finds the game
		return nil
	}
	handlers.ProcessAppSubmissionForTest(ctx, app.DB, &config.Config{}, id, pinner.UserID, scrape)

	if w, p := watching(t, app, pinner.UserID, id); !w || !p {
		t.Errorf("the user who pinned it: watched=%v pinned=%v, want both", w, p)
	}
	if w, p := watching(t, app, plain.UserID, id); !w || p {
		t.Errorf("the user who did not pin it: watched=%v pinned=%v, want watched and not pinned", w, p)
	}
	if w, _ := watching(t, app, bystander.UserID, id); w {
		t.Error("a user who never asked for the game now watches it")
	}
	for _, u := range []int64{pinner.UserID, plain.UserID} {
		if ok, _ := requestRow(t, app, u, id); ok {
			t.Errorf("user %d still has a wishlist request for a game that was added", u)
		}
	}
	// The submitter is told, as before.
	var n int
	app.Pool.QueryRow(ctx, "SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'submission_tracked'", pinner.UserID).Scan(&n)
	if n != 1 {
		t.Errorf("submitter got %d tracked notifications, want 1", n)
	}
}

func TestGameThatCannotBeFoundDropsItsWishlistRequests(t *testing.T) {
	app := testutil.NewApp(t)
	ctx := context.Background()
	user := app.NewUser("lost")
	const id = 870101
	app.Pool.Exec(ctx, "INSERT INTO tracked_games (app_id, status, submitted_by) VALUES ($1, 'pending', $2)", id, user.UserID)
	app.DB.RecordWishlistRequests(ctx, user.UserID, []database.WishlistRequest{{AppID: id, Pinned: true}})

	noGame := func(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, ids []int) error {
		return nil
	}
	handlers.ProcessAppSubmissionForTest(ctx, app.DB, &config.Config{}, id, user.UserID, noGame)

	if ok, _ := requestRow(t, app, user.UserID, id); ok {
		t.Error("the request was kept for a game that does not exist")
	}
	if w, _ := watching(t, app, user.UserID, id); w {
		t.Error("a missing game was added to the watchlist")
	}
	var status string
	app.Pool.QueryRow(ctx, "SELECT status FROM tracked_games WHERE app_id = $1", id).Scan(&status)
	if status != "failed" {
		t.Errorf("status = %q, want failed", status)
	}
}

func TestBlockedGameDropsItsWishlistRequests(t *testing.T) {
	app := testutil.NewApp(t)
	ctx := context.Background()
	admin := app.NewAdmin()
	user := app.NewUser("blocked")
	const id = 870201
	if _, err := app.DB.AddBlacklistRule(ctx, "name", "Forbidden", "test", admin.UserID); err != nil {
		t.Fatal(err)
	}
	app.Pool.Exec(ctx, "INSERT INTO tracked_games (app_id, status, submitted_by) VALUES ($1, 'pending', $2)", id, user.UserID)
	app.DB.RecordWishlistRequests(ctx, user.UserID, []database.WishlistRequest{{AppID: id}})

	scrape := func(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, ids []int) error {
		app.SeedGame(ids[0], "Forbidden Game", []string{"d"}, []string{"p"})
		return nil
	}
	handlers.ProcessAppSubmissionForTest(ctx, app.DB, &config.Config{}, id, user.UserID, scrape)

	if ok, _ := requestRow(t, app, user.UserID, id); ok {
		t.Error("the request was kept for a game the blacklist blocked")
	}
	if w, _ := watching(t, app, user.UserID, id); w {
		t.Error("a blocked game was added to the watchlist")
	}
	var games int
	app.Pool.QueryRow(ctx, "SELECT count(*) FROM games WHERE app_id = $1", id).Scan(&games)
	if games != 0 {
		t.Error("the blocked game was kept")
	}
}
