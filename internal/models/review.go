package models

type Review struct {
	// Basic information
	RecommendationID string
	SteamID          string
	Language         string
	Review           string
	VotedUp          bool
	// Unix timestamp of the review
	TimestampCreated int64
	TimestampUpdated int64
	// Playtime in minutes
	PlaytimeForever  int
	PlaytimeAtReview int
	// Vote informations
	HelpfulVotes int
	FunnyVotes   int
}