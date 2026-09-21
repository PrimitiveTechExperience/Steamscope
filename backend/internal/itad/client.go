// Package itad talks to the IsThereAnyDeal (https://docs.isthereanydeal.com/)
// API, used to back-fill historical Steam price data that Steam itself does
// not expose.
package itad

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.isthereanydeal.com"

// steamShopID is IsThereAnyDeal's internal id for the Steam shop.
const steamShopID = 61

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type lookupResponse struct {
	Found bool `json:"found"`
	Game  struct {
		ID string `json:"id"`
	} `json:"game"`
}

// LookupGameID resolves a Steam app id to ITAD's internal game id.
func (c *Client) LookupGameID(appID int) (string, error) {
	q := url.Values{}
	q.Set("key", c.apiKey)
	q.Set("appid", fmt.Sprintf("%d", appID))

	resp, err := c.httpClient.Get(baseURL + "/games/lookup/v1?" + q.Encode())
	if err != nil {
		return "", fmt.Errorf("failed to look up ITAD game id for appID %d: %w", appID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ITAD lookup for appID %d returned status %d", appID, resp.StatusCode)
	}

	var result lookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode ITAD lookup response for appID %d: %w", appID, err)
	}
	if !result.Found || result.Game.ID == "" {
		return "", fmt.Errorf("no ITAD game found for appID %d", appID)
	}
	return result.Game.ID, nil
}

// HistoryEvent is a single price-change event as reported by ITAD.
type HistoryEvent struct {
	Timestamp time.Time
	Price     float64
	Regular   float64
	Cut       int
}

type historyEntry struct {
	Timestamp string `json:"timestamp"`
	Deal      struct {
		Price struct {
			Amount float64 `json:"amount"`
		} `json:"price"`
		Regular struct {
			Amount float64 `json:"amount"`
		} `json:"regular"`
		Cut int `json:"cut"`
	} `json:"deal"`
}

// GetHistory fetches the full Steam price-change log for an ITAD game id.
func (c *Client) GetHistory(itadID string) ([]HistoryEvent, error) {
	q := url.Values{}
	q.Set("key", c.apiKey)
	q.Set("id", itadID)
	q.Set("shops", fmt.Sprintf("%d", steamShopID))

	resp, err := c.httpClient.Get(baseURL + "/games/history/v2?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ITAD history for game %s: %w", itadID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ITAD history for game %s returned status %d", itadID, resp.StatusCode)
	}

	var entries []historyEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode ITAD history response for game %s: %w", itadID, err)
	}

	events := make([]HistoryEvent, 0, len(entries))
	for _, entry := range entries {
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			continue
		}
		events = append(events, HistoryEvent{
			Timestamp: ts,
			Price:     entry.Deal.Price.Amount,
			Regular:   entry.Deal.Regular.Amount,
			Cut:       entry.Deal.Cut,
		})
	}
	return events, nil
}
