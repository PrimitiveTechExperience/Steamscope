package models

import "time"

type User struct {
	UserID       int64     `json:"user_id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	SteamID      *string   `json:"steam_id"`
	IsAdmin      bool      `json:"is_admin"`
	IsBanned     bool      `json:"-"`
	// SubmissionsBlocked users can use the site but can't suggest games.
	SubmissionsBlocked bool `json:"submissions_blocked"`
	CreatedAt    time.Time `json:"created_at"`
}

type Preferences struct {
	Theme                     string   `json:"theme"`
	NotifyPriceDrops          bool     `json:"notify_price_drops"`
	PriceDropThresholdPercent int      `json:"price_drop_threshold_percent"`
	PreferredGenres           []string `json:"preferred_genres"`
}

type WatchedGame struct {
	Game        Game      `json:"game"`
	Pinned      bool      `json:"pinned"`
	TargetPrice *float64  `json:"target_price"`
	WatchedAt   time.Time `json:"watched_at"`
}

type Notification struct {
	NotificationID int64      `json:"notification_id"`
	AppID          *int       `json:"app_id"`
	Kind           string     `json:"kind"`
	Message        string     `json:"message"`
	ReadAt         *time.Time `json:"read_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Submission struct {
	Kind      string    `json:"kind"` // "app" or "bundle"
	ID        int       `json:"id"`
	Name      *string   `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Deal is a game whose current price is meaningfully below its recent
// average, as used by the feed's "below their usual price" section.
type Deal struct {
	Game              Game    `json:"game"`
	AveragePrice      float64 `json:"average_price"`
	PercentBelowUsual int     `json:"percent_below_usual"`
	Watched           bool    `json:"watched"`
}

type Feed struct {
	Watchlist   []WatchedGame `json:"watchlist"`
	Deals       []Deal        `json:"deals"`
	Suggestions []Game        `json:"suggestions"`
}

type SteamProfile struct {
	SteamID          string            `json:"steam_id"`
	PersonaName      string            `json:"persona_name"`
	AvatarURL        string            `json:"avatar_url"`
	ProfileURL       string            `json:"profile_url"`
	Status           string            `json:"status"`
	CurrentlyPlaying string            `json:"currently_playing"`
	RecentlyPlayed   []SteamPlayedGame `json:"recently_played"`
}

type SteamPlayedGame struct {
	AppID            int    `json:"app_id"`
	Name             string `json:"name"`
	IconURL          string `json:"icon_url"`
	PlaytimeTwoWeeks int    `json:"playtime_2weeks"`
	PlaytimeForever  int    `json:"playtime_forever"`
	// TrackStatus is our tracking status for this game ("" = not submitted,
	// else awaiting_approval/pending/tracked/failed/rejected). Filled per
	// request, never cached.
	TrackStatus string `json:"track_status"`
}
