package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/PrimitiveTechExperience/Steamscope/internal/models"
)

type steamReviewResponse struct {
	Success bool `json:"success"`
	QuerySummary struct {
		NumReviews int `json:"num_reviews"`
		ReviewScore int `json:"review_score"`
		ReviewScoreDesc string `json:"review_score_desc"`
		TotalPositive int `json:"total_positive"`
		TotalNegative int `json:"total_negative"`
		TotalReviews int `json:"total_reviews"`
	} `json:"query_summary"`
	Reviews []steamReview `json:"reviews"`
	Cursor string `json:"cursor"`
}

type steamReview struct {
	RecommendationID string `json:"recommendationid"`
	Author struct {
		SteamID string `json:"steamid"`
		NumGamesOwned int `json:"num_games_owned"`
		NumReviews int `json:"num_reviews"`
		PlaytimeForever int `json:"playtime_forever"`
		PlaytimeAtReview int `json:"playtime_at_review"`
	} `json:"author"`

	Language string `json:"language"`
	Review string `json:"review"`
	VotedUp bool `json:"voted_up"`
	TimestampCreated int64 `json:"timestamp_created"`
	TimestampUpdated int64 `json:"timestamp_updated"`
	VotesUp int `json:"votes_up"`
	VotesFunny int `json:"votes_funny"`
}

func (s *Scraper) FetchReviews(
	appID int,
) ([]models.Review, error) {
	params := url.Values{}
	params.Set("json", "1")
	params.Set("filter", "recent")
	params.Set("language", "all")
	params.Set("num_per_page", "100")
	params.Set("cursor", "*")

	endpoint := fmt.Sprintf("https://store.steampowered.com/appreviews/%d?%s", appID, params.Encode())

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch reviews: status code %d", resp.StatusCode)
	}

	var data steamReviewResponse

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode review response: %w", err)
	}

	reviews := make([]models.Review, len(data.Reviews))
	for i, r := range data.Reviews {
		reviews[i] = models.Review{
			RecommendationID: r.RecommendationID,
			SteamID:          r.Author.SteamID,
			Language:         r.Language,
			Review:           r.Review,
			VotedUp:          r.VotedUp,
			TimestampCreated: r.TimestampCreated,
			TimestampUpdated: r.TimestampUpdated,
			PlaytimeForever:  r.Author.PlaytimeForever,
			PlaytimeAtReview: r.Author.PlaytimeAtReview,
			HelpfulVotes:     r.VotesUp,
			FunnyVotes:       r.VotesFunny,
		}
	}
	return reviews, nil
}