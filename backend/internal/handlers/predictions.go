package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
)

const (
	// Forecasts can call ITAD, whose key is rate limited, so cap how often one
	// address can ask for them.
	predictLimit  = 30
	predictWindow = time.Minute
	// How far back ITAD history is used, and how long the call may take.
	extendedHistoryYears = 6
	itadTimeout          = 6 * time.Second
)

// GetPrediction returns the price forecast for a game: predicted prices and
// the chance of a lower price for up to two years ahead.
func (h *Handler) GetPrediction(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.predictionRequest(w, r)
	if !ok {
		return
	}
	f, err := h.forecast(r.Context(), appID)
	if err != nil {
		log.Printf("prediction for %d: %v", appID, err)
		writeError(w, http.StatusInternalServerError, "failed to compute the forecast")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

type adviceResponse struct {
	prediction.Advice
	GeneratedAt time.Time `json:"generated_at"`
	Cached      bool      `json:"cached"`
}

// GetAdvice says whether to buy now or wait. Anyone gets a general answer; a
// signed-in user who watches the game gets one that also weighs their target
// price and how long (and at what price) they have been watching.
func (h *Handler) GetAdvice(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.predictionRequest(w, r)
	if !ok {
		return
	}
	f, err := h.forecast(r.Context(), appID)
	if err != nil {
		log.Printf("advice for %d: %v", appID, err)
		writeError(w, http.StatusInternalServerError, "failed to compute the advice")
		return
	}

	var in prediction.AdviceInput
	if user := auth.CurrentUser(r.Context()); user != nil {
		watch, err := h.DB.GetWatchInfo(r.Context(), user.UserID, appID)
		switch {
		case err == nil:
			in = prediction.AdviceInput{
				Watching:    true,
				TargetPrice: watch.TargetPrice,
				DaysWatched: max(0, int(time.Since(watch.WatchedAt).Hours()/24)),
			}
			if p, err := h.DB.GetPriceOnOrBefore(r.Context(), appID, watch.WatchedAt); err == nil {
				in.WatchStartPrice = p
			}
		case !errors.Is(err, database.ErrNotFound):
			log.Printf("advice for %d: watch info: %v", appID, err)
		}
	}

	writeJSON(w, http.StatusOK, adviceResponse{Advice: prediction.Advise(f, in), GeneratedAt: f.GeneratedAt, Cached: f.Cached})
}

// predictionRequest validates the request shared by both endpoints.
func (h *Handler) predictionRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	appID, err := strconv.Atoi(r.PathValue("appID"))
	if err != nil || appID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid appID")
		return 0, false
	}
	if !cache.Allow(r.Context(), h.Redis, "ratelimit:predict:"+clientIP(r), predictLimit, predictWindow) {
		writeError(w, http.StatusTooManyRequests, "too many forecast requests, try again in a minute")
		return 0, false
	}
	if _, err := h.DB.GetGameName(r.Context(), appID); errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return 0, false
	} else if err != nil {
		log.Printf("prediction: game lookup %d: %v", appID, err)
		writeError(w, http.StatusInternalServerError, "failed to load the game")
		return 0, false
	}
	return appID, true
}

// forecast returns the cached forecast when the price history hasn't changed
// since it was built, and otherwise computes (and caches) a new one. Requests
// for the same game that arrive together share one computation.
func (h *Handler) forecast(ctx context.Context, appID int) (prediction.Forecast, error) {
	summary, err := h.DB.GetPriceHistorySummary(ctx, appID)
	if err != nil {
		return prediction.Forecast{}, err
	}
	version := fmt.Sprintf("%d|%s|%.2f|itad=%t", summary.Rows, summary.LastDate.Format("2006-01-02"), summary.LastPrice, h.ITAD != nil)

	if f, ok := h.Predictions.Get(ctx, appID, version); ok {
		observability.PredictionRequests.WithLabelValues("hit").Inc()
		return *f, nil
	}

	v, err, _ := h.predictFlight.Do(strconv.Itoa(appID)+"|"+version, func() (any, error) {
		start := time.Now()
		// The shared computation must not die with the first caller's request.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*itadTimeout)
		defer cancel()

		points, extended, err := h.historyPoints(cctx, appID)
		if err != nil {
			return nil, err
		}
		f := prediction.Predict(appID, points, version, prediction.Options{})
		f.UsedExtended = extended
		observability.PredictionCompute.Observe(time.Since(start).Seconds())

		if f.Model == prediction.ModelInsufficient {
			observability.PredictionRequests.WithLabelValues("insufficient").Inc()
			return f, nil // not worth a cache slot: there is nothing to reuse
		}
		observability.PredictionRequests.WithLabelValues("miss").Inc()
		if err := h.Predictions.Put(cctx, f); err != nil {
			log.Printf("prediction: cache put %d: %v", appID, err)
		}
		return f, nil
	})
	if err != nil {
		return prediction.Forecast{}, err
	}
	return v.(prediction.Forecast), nil
}

// historyPoints loads the game's recorded history and, when an ITAD key is
// configured, extends it backwards with ITAD's longer log. Recorded days always
// win over ITAD's for the dates they cover. A failing ITAD call only costs the
// extension, never the forecast.
func (h *Handler) historyPoints(ctx context.Context, appID int) ([]prediction.Point, bool, error) {
	recorded, err := h.DB.GetPriceHistory(ctx, appID)
	if err != nil {
		return nil, false, err
	}
	points := make([]prediction.Point, 0, len(recorded))
	for _, p := range recorded {
		points = append(points, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
	}
	if h.ITAD == nil {
		return points, false, nil
	}

	ictx, cancel := context.WithTimeout(ctx, itadTimeout)
	defer cancel()
	events, err := h.ITAD.History(ictx, appID)
	if err != nil {
		log.Printf("prediction: ITAD history for %d unavailable, using recorded history only: %v", appID, err)
		return points, false, nil
	}
	return mergeHistory(points, events, time.Now()), true, nil
}

// mergeHistory prepends the daily series built from ITAD events to the
// recorded points, covering only the dates before the first recorded day.
func mergeHistory(recorded []prediction.Point, events []itad.HistoryEvent, now time.Time) []prediction.Point {
	if len(events) == 0 {
		return recorded
	}
	cutoff := now.AddDate(0, 0, 1)
	if len(recorded) > 0 {
		cutoff = recorded[0].Date
	}
	series := itad.BuildDailySeries(events, now.AddDate(-extendedHistoryYears, 0, 0), now)
	merged := make([]prediction.Point, 0, len(series)+len(recorded))
	for _, p := range series {
		if p.Date.Before(cutoff) {
			merged = append(merged, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
		}
	}
	return append(merged, recorded...)
}
