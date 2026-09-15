package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func TestReviewScraper_FetchReviews(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			fmt.Fprint(w, `{
				"success": 1,
				"query_summary": {
					"num_reviews": 2,
					"review_score": 8,
					"review_score_desc": "Very Positive",
					"total_positive": 900,
					"total_negative": 100,
					"total_reviews": 1000
				},
				"reviews": [
					{
						"recommendationid": "123456789",
						"author": {
							"steamid": "11111111111111111",
							"num_games_owned": 50,
							"num_reviews": 10,
							"playtime_forever": 12000,
							"playtime_at_review": 10000
						},
						"language": "english",
						"review": "This is a great game!",
						"voted_up": true,
						"timestamp_created": 1700000000,
						"timestamp_updated": 1700000100,
						"votes_up": 25,
						"votes_funny": 2
					},
					{
						"recommendationid": "987654321",
						"author": {
							"steamid": "22222222222222222",
							"num_games_owned": 20,
							"num_reviews": 5,
							"playtime_forever": 5000,
							"playtime_at_review": 4000
						},
						"language": "english",
						"review": "Pretty good game.",
						"voted_up": true,
						"timestamp_created": 1700000200,
						"timestamp_updated": 1700000300,
						"votes_up": 10,
						"votes_funny": 1
					}
				]
			}`)
		}),
	)
	defer server.Close()

	s := New(server.URL)
	c := s.newReviewCollector()
	err := s.FetchReviews(12345, ReviewOption{"recent", 1, "english"}, c)

	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
}

func TestReviewScraper_FetchReviews_TestRequestDataCorrectness(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/appreviews/12345" {
				t.Errorf("Expected path '/appreviews/12345', got '%s'", r.URL.Path)
			}
			if r.URL.Query().Get("filter") != "recent" {
				t.Errorf("Expected filter 'recent', got '%s'", r.URL.Query().Get("filter"))
			}
			if r.URL.Query().Get("language") != "english" {
				t.Errorf("Expected language 'english', got '%s'", r.URL.Query().Get("language"))
			}
			if r.URL.Query().Get("num_per_page") != "1" {
				t.Errorf("Expected num_per_page '1', got '%s'", r.URL.Query().Get("num_per_page"))
			}
			if r.URL.Query().Get("cursor") != "*" {
				t.Errorf("Expected cursor '*', got '%s'", r.URL.Query().Get("cursor"))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"success": 1,
				"query_summary": {
					"num_reviews": 2,
					"review_score": 8,
					"review_score_desc": "Very Positive",
					"total_positive": 900,
					"total_negative": 100,
					"total_reviews": 1000
				},
				"reviews": [
					{
						"recommendationid": "123456789",
						"author": {
							"steamid": "11111111111111111",
							"num_games_owned": 50,
							"num_reviews": 10,
							"playtime_forever": 12000,
							"playtime_at_review": 10000
						},
						"language": "english",
						"review": "This is a great game!",
						"voted_up": true,
						"timestamp_created": 1700000000,
						"timestamp_updated": 1700000100,
						"votes_up": 25,
						"votes_funny": 2
					},
					{
						"recommendationid": "987654321",
						"author": {
							"steamid": "22222222222222222",
							"num_games_owned": 20,
							"num_reviews": 5,
							"playtime_forever": 5000,
							"playtime_at_review": 4000
						},
						"language": "english",
						"review": "Pretty good game.",
						"voted_up": true,
						"timestamp_created": 1700000200,
						"timestamp_updated": 1700000300,
						"votes_up": 10,
						"votes_funny": 1
					}
				]
			}`)
		}),
	)
	defer server.Close()
	
	s := New(server.URL)
	c := s.newReviewCollector()
	err := s.FetchReviews(12345, ReviewOption{"recent", 1, "english"}, c)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
}

func TestReviewScraper_FetchReviewsForGames_BasicCorrectness(t *testing.T) {
	// create a gummy game model
	game := models.Game{
		AppID: 12345,
		Name: "Test Game",
		Reviews: []models.Review{},
	}
	games := []models.Game{game}
	// create a test server
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Simulate a response for each game
			fmt.Fprint(w, `{
				"success": 1,
				"query_summary": {
					"num_reviews": 2,
					"review_score": 8,
					"review_score_desc": "Very Positive",
					"total_positive": 900,
					"total_negative": 100,
					"total_reviews": 1000
				},
				"reviews": [
					{
						"recommendationid": "123456789",
						"author": {
							"steamid": "11111111111111111",
							"num_games_owned": 50,
							"num_reviews": 10,
							"playtime_forever": 12000,
							"playtime_at_review": 10000
						},
						"language": "english",
						"review": "This is a great game!",
						"voted_up": true,
						"timestamp_created": 1700000000,
						"timestamp_updated": 1700000100,
						"votes_up": 25,
						"votes_funny": 2
					},
					{
						"recommendationid": "987654321",
						"author": {
							"steamid": "22222222222222222",
							"num_games_owned": 20,
							"num_reviews": 5,
							"playtime_forever": 5000,
							"playtime_at_review": 4000
						},
						"language": "english",
						"review": "Pretty good game.",
						"voted_up": true,
						"timestamp_created": 1700000200,
						"timestamp_updated": 1700000300,
						"votes_up": 10,
						"votes_funny": 1
					}
				]
			}`)
		}),
	)
	defer server.Close()

	s := New(server.URL)
	reviews, err := s.FetchReviewsForGames(games, ReviewOption{"recent", 1, "english"})
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
	if len(reviews) != 2 {
		t.Errorf("Expected 2 reviews, but got %d", len(reviews))
	}
}