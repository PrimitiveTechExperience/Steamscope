package database

import (
	"context"
	"fmt"
	"time"
)

type GameCount struct {
	AppID int    `json:"app_id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type UserCount struct {
	Username string `json:"username"`
	Count    int    `json:"count"`
}

type DayActivity struct {
	Date        time.Time `json:"date"`
	Signups     int       `json:"signups"`
	WatchAdds   int       `json:"watch_adds"`
	Submissions int       `json:"submissions"`
}

type AdminStats struct {
	Users             int `json:"users"`
	NewUsers7d        int `json:"new_users_7d"`
	BannedUsers       int `json:"banned_users"`
	TrackedGames      int `json:"tracked_games"`
	TrackedBundles    int `json:"tracked_bundles"`
	AwaitingApproval  int `json:"awaiting_approval"`
	WatchlistEntries  int `json:"watchlist_entries"`
	PinnedEntries     int `json:"pinned_entries"`
	BlacklistRules    int `json:"blacklist_rules"`
	UsersWithWatchlst int `json:"users_with_watchlist"`

	MostWatched   []GameCount   `json:"most_watched"`
	MostPinned    []GameCount   `json:"most_pinned"`
	TopSubmitters []UserCount   `json:"top_submitters"`
	TopWatchers   []UserCount   `json:"top_watchers"`
	Activity      []DayActivity `json:"activity"`
	RecentUsers   []AdminUser   `json:"recent_users"`
	RecentItems   []AdminItem   `json:"recent_items"`
}

const statsTopN = 8

func (db *DB) GetAdminStats(ctx context.Context) (*AdminStats, error) {
	var s AdminStats
	err := db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM users WHERE created_at > now() - interval '7 days'),
			(SELECT count(*) FROM users WHERE is_banned),
			(SELECT count(*) FROM tracked_games WHERE status = 'tracked'),
			(SELECT count(*) FROM bundles WHERE status = 'tracked'),
			(SELECT count(*) FROM tracked_games WHERE status = 'awaiting_approval')
				+ (SELECT count(*) FROM bundles WHERE status = 'awaiting_approval'),
			(SELECT count(*) FROM watched_games),
			(SELECT count(*) FROM watched_games WHERE pinned),
			(SELECT count(*) FROM blacklist_rules),
			(SELECT count(DISTINCT user_id) FROM watched_games)`,
	).Scan(&s.Users, &s.NewUsers7d, &s.BannedUsers, &s.TrackedGames, &s.TrackedBundles,
		&s.AwaitingApproval, &s.WatchlistEntries, &s.PinnedEntries, &s.BlacklistRules, &s.UsersWithWatchlst)
	if err != nil {
		return nil, fmt.Errorf("failed to load stats: %w", err)
	}

	if s.MostWatched, err = db.topGames(ctx, false); err != nil {
		return nil, err
	}
	if s.MostPinned, err = db.topGames(ctx, true); err != nil {
		return nil, err
	}
	if s.TopSubmitters, err = db.topUsers(ctx, `
		SELECT u.username, count(*) FROM (
			SELECT submitted_by FROM tracked_games WHERE submitted_by IS NOT NULL
			UNION ALL
			SELECT submitted_by FROM bundles WHERE submitted_by IS NOT NULL
		) s JOIN users u ON u.user_id = s.submitted_by
		GROUP BY u.username ORDER BY count(*) DESC, u.username LIMIT $1`); err != nil {
		return nil, err
	}
	if s.TopWatchers, err = db.topUsers(ctx, `
		SELECT u.username, count(*) FROM watched_games w JOIN users u USING (user_id)
		GROUP BY u.username ORDER BY count(*) DESC, u.username LIMIT $1`); err != nil {
		return nil, err
	}
	if s.Activity, err = db.activity(ctx); err != nil {
		return nil, err
	}
	if s.RecentUsers, err = db.ListUsers(ctx, 6); err != nil {
		return nil, err
	}
	if s.RecentItems, err = db.ListItems(ctx, 8); err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) topGames(ctx context.Context, pinnedOnly bool) ([]GameCount, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT g.app_id, g.name, count(*)
		FROM watched_games w JOIN games g USING (app_id)
		WHERE (NOT $1 OR w.pinned)
		GROUP BY g.app_id, g.name
		ORDER BY count(*) DESC, g.name
		LIMIT $2`, pinnedOnly, statsTopN)
	if err != nil {
		return nil, fmt.Errorf("failed to load top games: %w", err)
	}
	defer rows.Close()
	out := []GameCount{}
	for rows.Next() {
		var c GameCount
		if err := rows.Scan(&c.AppID, &c.Name, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (db *DB) topUsers(ctx context.Context, query string) ([]UserCount, error) {
	rows, err := db.Pool.Query(ctx, query, statsTopN)
	if err != nil {
		return nil, fmt.Errorf("failed to load top users: %w", err)
	}
	defer rows.Close()
	out := []UserCount{}
	for rows.Next() {
		var c UserCount
		if err := rows.Scan(&c.Username, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// activity returns per-day signups, watchlist additions and submissions for
// the last 14 days (including days with none).
func (db *DB) activity(ctx context.Context) ([]DayActivity, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT d::date,
			(SELECT count(*) FROM users WHERE created_at::date = d::date),
			(SELECT count(*) FROM watched_games WHERE created_at::date = d::date),
			(SELECT count(*) FROM (
				SELECT created_at FROM tracked_games UNION ALL SELECT created_at FROM bundles
			) s WHERE s.created_at::date = d::date)
		FROM generate_series(current_date - 13, current_date, interval '1 day') d
		ORDER BY d`)
	if err != nil {
		return nil, fmt.Errorf("failed to load activity: %w", err)
	}
	defer rows.Close()
	out := []DayActivity{}
	for rows.Next() {
		var a DayActivity
		if err := rows.Scan(&a.Date, &a.Signups, &a.WatchAdds, &a.Submissions); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
