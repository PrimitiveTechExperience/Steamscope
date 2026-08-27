package models

type Review struct {
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
	// Number of people who found this review helpful
	HelpfulVotes int
	FunnyVotes   int
}