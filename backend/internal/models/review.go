package models

import "time"

type Review struct {
	// Basic information
	AppID            int `json:"app_id"`
	RecommendationID string `json:"recommendation_id"`
	SteamID          string `json:"steam_id"`
	AuthorName       string `json:"author_name"`
	AuthorAvatar     string `json:"author_avatar"`
	NumGamesOwned    int `json:"num_games_owned"`
	NumReviews       int `json:"num_reviews"`
	Language         string `json:"language"`
	Review           string `json:"review"`
	VotedUp          bool `json:"voted_up"`
	// Unix timestamp of the review
	TimestampCreated time.Time `json:"timestamp_created"`
	TimestampUpdated time.Time `json:"timestamp_updated"`
	// Playtime in minutes
	PlaytimeForever  int `json:"playtime_forever"`
	PlaytimeAtReview int `json:"playtime_at_review"`
	// Vote informations
	HelpfulVotes int `json:"helpful_votes"`
	FunnyVotes   int `json:"funny_votes"`
}