package scraper

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/gocolly/colly/v2"
)

// Review Json Example
// {
//   "success": 1,
//   "query_summary": {
//     "num_reviews": 1,
//     "review_score": 8,
//     "review_score_desc": "Very Positive",
//     "total_positive": 1282283,
//     "total_negative": 223086,
//     "total_reviews": 1505369
//   },
//   "reviews": [
//     {
//       "recommendationid": "234860824",
//       "author": {
//         "steamid": "76561199856841008",
//         "personaname": "not_vincent777",
//         "persona_status": "offline",
//         "profile_url": "https://steamcommunity.com/profiles/76561199856841008/",
//         "num_games_owned": 0,
//         "num_reviews": 1,
//         "playtime_forever": 3737,
//         "playtime_last_two_weeks": 674,
//         "playtime_at_review": 3719,
//         "last_played": 1788981084,
//         "avatar": "bf0bd77b9f1d9099fbcbea3b42667be9c744bf92"
//       },
//       "language": "english",
//       "review": "great gambling. would recommend",
//       "timestamp_created": 1788980019,
//       "timestamp_updated": 1788980019,
//       "voted_up": true,
//       "votes_up": 0,
//       "votes_funny": 0,
//       "weighted_vote_score": 0.5,
//       "comment_count": 0,
//       "steam_purchase": true,
//       "received_for_free": false,
//       "refunded": false,
//       "written_during_early_access": false,
//       "primarily_steam_deck": false,
//       "app_release_date": "1345568400",
//       "reactions": []
//     }
//   ],
//   "cursor": "AoJ485fhw6ADeJH2/wY="
// }

type ReviewOption struct {
	Filter string
	MaxReviews int
	Language string
}

type reviewResult struct {
	AppID int
	Reviews []models.Review
	Error error
}

type steamReviewResponse struct {
	Success int `json:"success"`
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

// We use colly approach here as well
func (s *Scraper) newReviewCollector() *colly.Collector {
	c := colly.NewCollector(
		colly.AllowedDomains("store.steampowered.com"),
	)
	c.Async = true
	c.SetCookieJar(s.Jar)
	c.Limit(&colly.LimitRule{
		DomainGlob: "*store.steampowered.com*",
		Parallelism: 4,
		Delay: 250*time.Millisecond,
		RandomDelay: 250*time.Millisecond,
	})
	c.OnRequest(func(r *colly.Request){
		log.Printf("Visiting: %s", r.URL.String())
	})
	c.OnResponse(func(r *colly.Response) {
		log.Printf("Received %d bytes\n", len(r.Body))
	})	
	return c
}


func (s *Scraper) FetchReviews(
	appID int,
	options ReviewOption,
	c *colly.Collector,
) error {
	params := url.Values{}
	params.Set("json", "1")
	params.Set("filter", options.Filter)
	params.Set("language", options.Language)
	params.Set("num_per_page", fmt.Sprintf("%d", options.MaxReviews))
	params.Set("cursor", "*")

	endpoint := fmt.Sprintf("https://store.steampowered.com/appreviews/%d?%s", appID, params.Encode())

	ctx := colly.NewContext()
	ctx.Put("appID", strconv.Itoa(appID))
	return c.Request("GET", endpoint, nil, ctx, nil)
}

func (s *Scraper) FetchReviewsForGames(games []models.Game, options ReviewOption) (map[int][]models.Review, error) {
	c := s.newReviewCollector()
	results := make(chan reviewResult, len(games))
	// Since the collection of reviews is done via JSON, we use onResponse:
	c.OnResponse(func (r *colly.Response)  {
		appIDStr := r.Ctx.Get("appID")

		appID, err := strconv.Atoi(appIDStr)
		if err != nil {
			results <- reviewResult{AppID: appID, Error: fmt.Errorf("Failed to convert appID %s to int: %v", appIDStr, err)}
			return
		}
		// log the json (debug)
		// log.Printf(string(r.Body))
		var data steamReviewResponse
		if err := json.Unmarshal(r.Body, &data); err != nil {
			results <- reviewResult{AppID: appID, Error: fmt.Errorf("Failed to unmarshal JSON for appID %d: %v", appID, err)}
			return
		}
		reviews := make([]models.Review, len(data.Reviews))
		for i, rev := range data.Reviews {
			reviews[i] = models.Review{
				AppID: appID,
				RecommendationID: rev.RecommendationID,
				SteamID: rev.Author.SteamID,
				Language: rev.Language,
				Review: rev.Review,
				VotedUp: rev.VotedUp,
				TimestampCreated: rev.TimestampCreated,
				TimestampUpdated: rev.TimestampUpdated,
				PlaytimeForever: rev.Author.PlaytimeForever,
				PlaytimeAtReview: rev.Author.PlaytimeAtReview,
				HelpfulVotes: rev.VotesUp,
				FunnyVotes: rev.VotesFunny,
			}
		}
		results <- reviewResult{AppID: appID, Reviews: reviews, Error: nil}
	})
	c.OnError(func(r *colly.Response, err error) {
		appIDStr := r.Ctx.Get("appID")
		appID, errConv := strconv.Atoi(appIDStr)
		if errConv != nil {
			results <- reviewResult{AppID: appID, Error: fmt.Errorf("Failed to convert appID %s to int: %v", appIDStr, errConv)}
			return
		}
		results <- reviewResult{AppID: appID, Error: fmt.Errorf("Failed to fetch reviews for appID %d: %v", appID, err)}
	})
	for _, game := range games {
		err := s.FetchReviews(game.AppID, options, c)
		if err != nil {
			results <- reviewResult{AppID: game.AppID, Error: fmt.Errorf("Failed to initiate review fetch for appID %d: %v", game.AppID, err)}
		}
	}
	c.Wait()
	close(results)

	reviewsByGame := make(map[int][]models.Review)
	var fetchErrors []error

	for result := range results {
		if result.Error != nil {
			fetchErrors = append(fetchErrors, result.Error)
		} else {
			reviewsByGame[result.AppID] = result.Reviews
		}
	}
	if len(fetchErrors) > 0 {
		return reviewsByGame, fmt.Errorf("Failed to fetch reviews for some games: %v", fetchErrors)
	}

	return reviewsByGame, nil
}