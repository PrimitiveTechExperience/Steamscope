package scraper

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/PrimitiveTechExperience/Steamscope/internal/models"
)

type reviewOption struct {
	Filter string
	MaxReviews int
	Language string
}

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
	options reviewOption,
) ([]models.Review, error) {
	params := url.Values{}
	params.Set("json", "1")
	params.Set("filter", options.Filter)
	params.Set("language", options.Language)
	params.Set("num_per_page", fmt.Sprintf("%d", options.MaxReviews))
	params.Set("cursor", "*")

	endpoint := fmt.Sprintf("https://store.steampowered.com/appreviews/%d?%s", appID, params.Encode())

	resp, err := s.Client.Get(endpoint)
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

func (s *Scraper) FetchReviewsForGames(appIDs []int, options reviewOption) (map[int][]models.Review, error) {
	type reviewResult struct {
		AppID   int
		Reviews []models.Review
		Error   error
	}
	jobs := make(chan int)
	results := make(chan reviewResult)

	var wg sync.WaitGroup
	workers := 5 // Number of concurrent workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for appID := range jobs {
				reviews, err := s.FetchReviews(appID, options)
				results <- reviewResult{AppID: appID, Reviews: reviews, Error: err}
			}
		}()
	}

	for _, appID := range appIDs {
		jobs <- appID
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	reviewsMap := make(map[int][]models.Review)
	for result := range results {
		if result.Error != nil {
			log.Printf("Error fetching reviews for AppID %d: %v", result.AppID, result.Error)
			continue
		}
		reviewsMap[result.AppID] = result.Reviews
	}
	return reviewsMap, nil
}