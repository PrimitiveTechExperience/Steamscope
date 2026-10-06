package itad

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

// ErrBundleUnknown means ITAD has no record of a Steam bundle.
var ErrBundleUnknown = errors.New("ITAD does not know this bundle")

// LookupBundleID resolves a Steam bundle id to ITAD's internal id. ITAD tracks
// Steam bundles as items of their own, with a price log like any game's.
func (c *Client) LookupBundleID(ctx context.Context, bundleID int) (string, error) {
	key := fmt.Sprintf("bundle/%d", bundleID)
	q := url.Values{"key": {c.apiKey}}
	var ids map[string]*string
	if err := c.doJSON(ctx, "POST", fmt.Sprintf("/lookup/id/shop/%d/v1?%s", steamShopID, q.Encode()), []string{key}, &ids); err != nil {
		return "", fmt.Errorf("ITAD lookup for bundle %d: %w", bundleID, err)
	}
	id := ids[key]
	if id == nil || *id == "" {
		return "", fmt.Errorf("bundle %d: %w", bundleID, ErrBundleUnknown)
	}
	return *id, nil
}

// BundleHistory fetches the full Steam price-change log of a bundle, honouring
// ctx's deadline.
func (c *Client) BundleHistory(ctx context.Context, bundleID int) ([]HistoryEvent, error) {
	id, err := c.LookupBundleID(ctx, bundleID)
	if err != nil {
		return nil, err
	}
	events, err := c.historyByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ITAD history for bundle %d: %w", bundleID, err)
	}
	return events, nil
}
