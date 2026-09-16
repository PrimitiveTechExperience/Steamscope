package models

import "time"

type Review struct {
	// Basic information
	AppID            int
	RecommendationID string
	SteamID          string
	Language         string
	Review           string
	VotedUp          bool
	// Unix timestamp of the review
	TimestampCreated time.Time
	TimestampUpdated time.Time
	// Playtime in minutes
	PlaytimeForever  int
	PlaytimeAtReview int
	// Vote informations
	HelpfulVotes int
	FunnyVotes   int
}