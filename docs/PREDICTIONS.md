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

ITAD's history endpoint returns only about the last three months unless it is given a `since` date, so every call
asks for the whole log (`since=2000-01-01`). Forgetting this silently truncates histories; a test guards it.

Recorded days always win over ITAD's for the dates they cover. If ITAD is unreachable, the forecast is built from
recorded history alone. Days before the first known price are never invented. A $0 price is treated as a free
promotion (a free weekend, say) and is carried over with the last paid price, so it is neither a sale nor a record
low. A game free for 30 days up to today, or that never cost anything, is free to play: it gets the verdict "Free"
and no forecast, because nothing can be cheaper than free.

## The forecast model

It is a statistical model, not machine learning. It reads the history as **sale episodes** and simulates the future.

1. **Episodes.** A day is a sale day when its price is at least 5% below the regular price. Consecutive sale days form
   an episode; two episodes separated by a single full-price day are merged. When no regular price is recorded, the
   highest price of the trailing year is used.
2. **When sales start.** The gaps between episode starts are fitted with a Weibull distribution, weighting recent
   gaps more (see recency below). Regular spacing gives a high shape, so a sale becomes likelier the longer it has
   been since the last one; erratic spacing gives a shape near 1, a memoryless process. With one or two sales it falls
   back to a constant-rate (Poisson) model, and with none to a very low rate. A silence much longer than the usual gap
   (over twice the mean) is read as a change in behaviour, not as a sale being "overdue": the process becomes
   memoryless with an expected wait that grows with the silence.
3. **Seasonality.** With enough history, the days of the year on which past sales began raise or lower the chance of a
   sale starting on each date (a smoothed, shrunk-toward-neutral estimate). With under a year of history or fewer
   than four sales, it uses the approximate Steam seasonal-sale calendar instead.
4. **Simulation.** 1,500 futures are simulated day by day for 730 days from today's state (on sale or not, days since the
   last sale started). Sale depths and durations are drawn from the game's own past sales. The regular price is assumed
   to stay constant.
   **Recency:** a sale counts half as much for every two years since it ended, in the fitted gaps and in the sampled
   depths and durations. Stores change how they discount over the years, so a 75%-off sale from 2014 still makes a
   deeper sale possible, but far less likely than one from last year. The lowest price on record comes from ITAD's whole
   log and is not weighted.
5. **Outputs** are read from the simulated futures, so they are mutually consistent:
   - a curve (about weekly) of the expected price, the chance a sale is running, and the chance that, by that date, the
     price has been at least 5% below today's at some point;
   - the same for 30, 90, 180, 365 and 730 days, plus the expected lowest price in each window;
   - when the next sale is likely to start (quartiles), the game's typical sale, a **score** (0-100: the chance of a lower
     price within 180 days) and a **confidence** (0-1: how much evidence there is, kept to four decimals so the interface
     can show an exact percentage).

A "lower price" means one at least 5% below today's. A game already at its lowest price therefore has a 0% chance of
a lower one unless history shows deeper sales. These 0% results are expected, and correct, for games currently at or
near their record low.

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
verdict is **Not enough data**, except that a met target price still gives Buy now. A free game gets **Free**.

**Confidence bands.** Both the forecast confidence and the advice confidence are shown as a word and an exact
percentage (for example "High-medium (54.37%)"). The bands are Very low (under 15%), Low (15-30%), Low-medium (30-45%),
High-medium (45-60%), High (60-80%) and Very high (80% and over). The two middle bands are separate so that a
strong-but-uncertain call is not shown as the same "medium" as a weak one. The advice confidence combines how far the
score is from a toss-up with the forecast's own confidence, so a 90/100 call on a thin forecast and a 21/100 call on
the same forecast now read differently. A "Wait" verdict includes the date
the next sale is expected.

Signed-in users who watch the game get personalized advice (`personalized: true`); everyone else gets the general
call with the default patience.

### Record lows

A game is flagged `at_record_low` when it is on sale and its price is at or below the lowest price on record (the
lower of the stored history and the full ITAD history). Games that have never changed price are not flagged, since
there is no sale to speak of. The flag appears in the forecast and the advice, adds a `record_low` reason (+30) to the
score, and is shown as a banner on the game page and a badge beside the price.

## Bundle value

Bundle pages (`GET /api/bundles/{id}`) include a `value` object judging whether a bundle is worth buying. It compares
the bundle price against:

- the sum of the games' regular prices,
- the sum of what the games cost today if bought separately,
- the bundle's own recorded history (record low, and how close the price is to it),
- each game's proportional share of the bundle, flagging games that are cheaper alone.

The result is a score and a verdict (`great_deal` 75+, `good_deal` 60+, `fair` 40+, otherwise `poor_value`), with
signed reasons. Games with no known price lower the `completeness` and are listed as unknown; with nothing priced the
verdict is `not_enough_data`. Bundle history only starts when the bundle was first tracked, so a bundle's record low
means the lowest price since tracking began. Per-game prices are collected by the scraper, so a bundle shows
`not_enough_data` until it has been scraped once after this was introduced.

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

A cached forecast is served only while the game's price history is unchanged **and** it was made by the same version of the
model (`prediction.ModelVersion`, bumped whenever a change can alter the numbers), so a model change never serves old results
from the shared cache, including during a rolling deploy. It is keyed by the row count, last
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

`go run ./backend/cmd/backtest -audit` is a second check: for each game it compares the model's 90-day chance of a
lower price with how often ITAD's own record shows a price that low (over every 90-day window of the last six years
and last two years), and flags disagreements. Games at their record low should, and do, show 0%.

The scoring code is unit tested, including that a forecast never sees the future and that free games are skipped.

Results on 17 tracked games with up to six years of ITAD-extended history (856 and 824 scored forecasts; samples
overlap and games share sale calendars, so treat the figures as indicative):

| Window | Brier (model) | Brier (base rate) | Skill |
| --- | --- | --- | --- |
| 30 days | 0.136 | 0.246 | +0.45 |
| 90 days | 0.072 | 0.220 | +0.67 |

Where the model says 0-20%, a lower price appeared 6-9% of the time (predicted about 5%), so near-zero
predictions are trustworthy. The middle of the range is under-confident: in the 30-day run, forecasts of about 48% saw
a lower price 63% of the time. No constant has been tuned to this; re-run the backtest as history grows before doing so.

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
