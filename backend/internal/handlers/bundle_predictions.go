package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
)

const (
	// A bundle's saved history is trusted once it covers about two months;
	// before that (a bundle only just found) ITAD is asked directly.
	minBundleRows = 60
	// A bundle counts as "on sale" when it is this far below the highest price
	// it has had in the past year, which stands in for the regular price.
	regularWindowDays = 365
)

// GetBundlePrediction is the price forecast for a bundle, the same model and
// response as a game's.
func (h *Handler) GetBundlePrediction(w http.ResponseWriter, r *http.Request) {
	id, ok := h.bundlePredictionRequest(w, r)
	if !ok {
		return
	}
	f, err := h.bundleForecast(r.Context(), id)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		serverError(w, r, "compute bundle forecast", err, "failed to compute the forecast", "bundle_id", id)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// GetBundleAdvice says whether to buy a bundle now or wait. There is no
// watchlist for bundles, so it is always the general answer.
func (h *Handler) GetBundleAdvice(w http.ResponseWriter, r *http.Request) {
	id, ok := h.bundlePredictionRequest(w, r)
	if !ok {
		return
	}
	f, err := h.bundleForecast(r.Context(), id)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		serverError(w, r, "compute bundle advice", err, "failed to compute the advice", "bundle_id", id)
		return
	}
	writeJSON(w, http.StatusOK, adviceResponse{Advice: prediction.Advise(f, prediction.AdviceInput{}), GeneratedAt: f.GeneratedAt, Cached: f.Cached})
}

func (h *Handler) bundlePredictionRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("bundleID"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid bundle id")
		return 0, false
	}
	if !cache.Allow(r.Context(), h.Redis, "ratelimit:predict:"+clientIP(r), predictLimit, predictWindow) {
		writeError(w, http.StatusTooManyRequests, "too many forecast requests, try again in a minute")
		return 0, false
	}
	if _, err := h.DB.GetBundleName(r.Context(), id); errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "bundle not found")
		return 0, false
	} else if err != nil {
		if r.Context().Err() != nil {
			return 0, false
		}
		serverError(w, r, "forecast: load bundle", err, "failed to load the bundle", "bundle_id", id)
		return 0, false
	}
	return id, true
}

func (h *Handler) bundleForecast(ctx context.Context, bundleID int) (prediction.Forecast, error) {
	summary, err := h.DB.GetBundlePriceHistorySummary(ctx, bundleID)
	if err != nil {
		return prediction.Forecast{}, err
	}
	return h.cachedForecast(ctx, forecastSource{
		bundleID: bundleID,
		version:  fmt.Sprintf("m%s|bundle|%d|%s|%.2f|itad=%t", prediction.ModelVersion, summary.Rows, summary.LastDate.Format("2006-01-02"), summary.LastPrice, h.ITAD != nil),
		load: func(ctx context.Context) ([]prediction.Point, float64, bool, error) {
			return h.bundleHistoryPoints(ctx, bundleID)
		},
	})
}

// bundleHistoryPoints loads a bundle's saved daily prices (which include the
// history imported from ITAD). A bundle with little saved history is extended
// from ITAD directly. Either way the "regular" price is rebuilt with
// trailingRegular, because the saved values mix Steam's and ITAD's meanings of it.
func (h *Handler) bundleHistoryPoints(ctx context.Context, bundleID int) (points []prediction.Point, allTimeLow float64, extended bool, err error) {
	recorded, err := h.DB.GetBundlePriceHistory(ctx, bundleID)
	if err != nil {
		return nil, 0, false, err
	}
	points = make([]prediction.Point, 0, len(recorded))
	for _, p := range recorded {
		points = append(points, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
	}

	if len(points) < minBundleRows && h.ITAD != nil {
		ictx, cancel := context.WithTimeout(ctx, itadTimeout)
		defer cancel()
		events, err := h.ITAD.BundleHistory(ictx, bundleID)
		if err != nil {
			log.Printf("prediction: ITAD history for bundle %d unavailable, using recorded history only: %v", bundleID, err)
		} else {
			points = mergeHistory(points, events, time.Now())
			extended = true
		}
	}
	return trailingRegular(points, regularWindowDays), lowestPoint(points), extended, nil
}

// trailingRegular sets every point's regular price to the highest price in the
// preceding window of days (itself included). Bundles have no list price of
// their own - Steam's "original price" is just the games added up, and ITAD's
// is something else again - so what a bundle normally costs is read off its
// own history, and a "sale" is a dip below that. Points must be in date order.
func trailingRegular(points []prediction.Point, windowDays int) []prediction.Point {
	out := make([]prediction.Point, len(points))
	start := 0
	for i, p := range points {
		for points[start].Date.Before(p.Date.AddDate(0, 0, -windowDays+1)) {
			start++
		}
		regular := p.Price
		for _, q := range points[start : i+1] {
			regular = max(regular, q.Price)
		}
		p.Regular = regular
		out[i] = p
	}
	return out
}

// lowestPoint is the lowest paid price in the points, or 0 if there is none.
func lowestPoint(points []prediction.Point) float64 {
	low := 0.0
	for _, p := range points {
		if p.Price > 0 && (low == 0 || p.Price < low) {
			low = p.Price
		}
	}
	return low
}
