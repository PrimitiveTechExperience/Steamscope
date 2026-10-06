package itad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// History fetches the full Steam price-change log for a Steam app, honouring
// ctx's deadline. It does the id lookup and the history call in one go, and
// is meant for request-time use, where the calls must not outlive the request.
//
// ITAD's log reaches back further than the two years kept in the database, so
// it gives the price forecast more sales to learn from and the true lowest price.
func (c *Client) History(ctx context.Context, appID int) ([]HistoryEvent, error) {
	var lookup lookupResponse
	q := url.Values{"key": {c.apiKey}, "appid": {fmt.Sprint(appID)}}
	if err := c.getJSON(ctx, "/games/lookup/v1?"+q.Encode(), &lookup); err != nil {
		return nil, fmt.Errorf("ITAD lookup for appID %d: %w", appID, err)
	}
	if !lookup.Found || lookup.Game.ID == "" {
		return nil, fmt.Errorf("no ITAD game found for appID %d", appID)
	}

	events, err := c.historyByID(ctx, lookup.Game.ID)
	if err != nil {
		return nil, fmt.Errorf("ITAD history for appID %d: %w", appID, err)
	}
	return events, nil
}

// historyByID fetches the whole Steam price-change log of an ITAD item (a game
// or a bundle), by ITAD's own id.
func (c *Client) historyByID(ctx context.Context, itadID string) ([]HistoryEvent, error) {
	q := url.Values{"key": {c.apiKey}, "id": {itadID}, "shops": {fmt.Sprint(steamShopID)}, "country": {"US"}, "since": {allHistorySince}}
	var entries []historyEntry
	if err := c.getJSON(ctx, "/games/history/v2?"+q.Encode(), &entries); err != nil {
		return nil, err
	}

	events := make([]HistoryEvent, 0, len(entries))
	for _, entry := range entries {
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			continue
		}
		events = append(events, HistoryEvent{
			Timestamp: ts, Price: entry.Deal.Price.Amount, Regular: entry.Deal.Regular.Amount, Cut: entry.Deal.Cut,
		})
	}
	return events, nil
}

func (c *Client) getJSON(ctx context.Context, pathAndQuery string, out any) error {
	return c.doJSON(ctx, http.MethodGet, pathAndQuery, nil, out)
}

func (c *Client) doJSON(ctx context.Context, method, pathAndQuery string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+pathAndQuery, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
