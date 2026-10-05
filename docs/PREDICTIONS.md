# Price forecasts and buy-now-or-wait advice

Each game page can answer "should I buy now, or wait for a lower price?". The answer combines a forecast of the
game's future prices with a small, transparent scoring rule that turns the forecast into advice.

The code is in `backend/internal/prediction/`, the endpoints in `backend/internal/handlers/predictions.go`,
and the page section in `frontend/src/app/components/price-prediction/`.

## Inputs

| Source | Used for |
| --- | --- |
| `price_history` (up to two years of daily prices) | the primary record of the game's sales |
| IsThereAnyDeal (ITAD) history, when `ITAD_API_KEY` is set | extends the record back up to six years, so there are more sales to learn from and the true lowest price is known |
| The user's watchlist entry | target price, how long the game has been watched, and its price on the day watching began |

Recorded days always win over ITAD's for the dates they cover. If ITAD is unreachable, the forecast is built from
recorded history alone. Days before the first known price are never invented.

## The forecast model

It is a statistical model, not machine learning. It reads the history as **sale episodes** and simulates the future.

1. **Episodes.** A day is a sale day when its price is at least 5% below the regular price. Consecutive sale days form
   an episode; two episodes separated by a single full-price day are merged. When no regular price is recorded, the
   highest price of the trailing year is used.
2. **When sales start.** The gaps between episode starts are fitted with a Weibull distribution. Regular spacing gives
   a high shape, so a sale becomes likelier the longer it has been since the last one; erratic spacing gives a shape
   near 1, a memoryless process. With one or two sales it falls back to a constant-rate (Poisson) model, and with
   none to a very low rate.
3. **Seasonality.** With enough history, the days of the year on which past sales began raise or lower the chance of a
   sale starting on each date (a smoothed, shrunk-toward-neutral estimate). With under a year of history or fewer
   than four sales, it uses the approximate Steam seasonal-sale calendar instead.
4. **Simulation.** 1,500 futures are simulated day by day for 730 days from today's state (on sale or not, days since the
   last sale started). Sale depths and durations are drawn from the game's own past sales. The regular price is assumed
   to stay constant.
5. **Outputs** are read from the simulated futures, so they are mutually consistent:
   - a curve (about weekly) of the expected price, the chance a sale is running, and the chance that, by that date, the
     price has been at least 5% below today's at some point;
   - the same for 30, 90, 180, 365 and 730 days, plus the expected lowest price in each window;
   - when the next sale is likely to start (quartiles), the game's typical sale, a **score** (0-100: the chance of a lower
     price within 180 days) and a **confidence** (0-1: how much evidence there is).

The simulation is seeded from the data, so the same history always yields the same forecast.

A forecast needs at least 60 days of history with 30 recorded days; otherwise the model is `insufficient` and no curve
is produced.

## Buy-now-or-wait advice

The advice starts from a score of 50 (0 = wait, 100 = buy now) and applies the rules below. Every rule that fires is
returned with its point impact so the interface can show why. Rules based on the forecast are scaled by
`0.5 + 0.5 x forecast confidence`, so a weak forecast moves the score less. The user's own target price is not scaled.

| Rule | Effect |
| --- | --- |
| Price at or below the user's target | +40, and the verdict is forced to buy now |
| Price above the user's target | up to -15, in proportion to the gap |
| Within 2% of the lowest price on record | +30 |
| Within 10% of the lowest price on record | +15 |
| 50% or more above the lowest price on record | -10 |
| Discount at least the game's usual sale depth | +15 |
| Discount under half the usual depth | -10 |
| Not on sale (and the game does go on sale) | -15 |
| 50% or more chance of a lower price within the patience window | up to -40, by chance and expected saving |
| Under 25% chance of a lower price within the window | up to +20 |
| Watched for 30 days or more | +2 per 30 days, up to +12 |
| Cheaper than when watching began (by 10%) / pricier (by 5%) | +10 / -5 |

The **patience window** is how far ahead the advice looks: 90 days, shrinking to 60, 45 and 30 days after 60, 120 and
240 days of watching, because someone who has already waited a long time has shown less patience.

Verdict: score 60 or more is **Buy now**, 40 or less is **Wait**, in between is **Toss-up**. Without enough history the
verdict is **Not enough data**, except that a met target price still gives Buy now. A "Wait" verdict includes the date
the next sale is expected.

Signed-in users who watch the game get personalized advice (`personalized: true`); everyone else gets the general
call with the default patience.

## API

| Endpoint | Auth | Result |
| --- | --- | --- |
| `GET /api/games/{id}/prediction` | none | the forecast (`model`, `score`, `confidence`, `curve`, `horizons`, `next_sale`, `typical_sale`, ...) |
| `GET /api/games/{id}/advice` | optional | the verdict, score, confidence, reasons, patience, expected saving and `wait_until` |

Both return `400` for an invalid ID, `404` for an unknown game and `429` beyond 30 requests a minute per address
(a forecast can call ITAD, whose key is rate limited). A game without enough history returns `200` with
`model: "insufficient"`.

## Caching

The most recent forecasts are cached in Redis, **at most five games at a time**: each game's predicted prices
and score. Storing a sixth evicts the one stored longest ago, atomically (one Lua script), so concurrent requests cannot
exceed the limit. Entries also expire after 12 hours.

A cached forecast is served only while the game's price history is unchanged. It is keyed by the row count, last
recorded date and last price, so the daily scrape invalidates it automatically. Requests for the same game that arrive
together share one computation. Forecasts with too little history are not cached.

Because only five fit, cycling through more than five games recomputes each one (about 0.2 to 1 second including
the ITAD call). Raise `prediction.DefaultMaxEntries` if that matters.

Metrics: `steamscope_prediction_requests_total{result="hit|miss|insufficient"}` and
`steamscope_prediction_compute_seconds` (see [OPERATIONS.md](OPERATIONS.md)).

## Validating the model

`go run ./backend/cmd/backtest [-days 90] [-min 180] [-step 30] [-itad]` replays history: standing at past dates it
forecasts the next window using only earlier data, then checks what happened. It prints the Brier score against a
constant "base rate" guess and a calibration table (when the model says 30%, does it happen about 30% of the time?).

The scoring code is unit tested, including that a forecast never sees the future. **Real-data validation needs history.**
With only about three months of history per game, a backtest has too few independent samples to say anything. A small
early run suggested the model is under-confident on very short histories (it predicted 35-60% for sales that did
happen). Re-run the backtest as history accumulates before tuning any constant.

## Limitations

- It forecasts from a game's own past. It cannot know about announced sales, a price change by the publisher, a new
  edition or a bundle.
- Short histories give wide, low-confidence forecasts. The confidence value, and the "Low / Medium / High" label in the
  interface, reflect that.
- The regular price is assumed constant, and a sale already running is assumed not to deepen.
- All games are modelled independently; nothing is learned from other games.

## Where machine learning could help

A learned model would pay off once there is much more data (many games with years of history), mainly by learning
across games: which genres, publishers and ages of game get discounted, and how sales cluster around events. A
global survival or gradient-boosted model trained offline, with its parameters loaded by the API, would be the natural
step. It should be judged with the backtest above and only adopted if it beats this model's Brier score.

A language model is not a good fit for the numbers (it cannot be calibrated or reproduced) but could help with the
text: turning the reasons into a friendlier sentence, or reading announced seasonal-sale dates from news pages to
feed the calendar prior.
