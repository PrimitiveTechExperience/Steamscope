# Operations: health, metrics, request IDs and tests

## Endpoints

| Endpoint | Purpose | Auth | Success | Failure |
| --- | --- | --- | --- | --- |
| `GET /api/health` | **Liveness**: is the process up? Cheap, never touches dependencies. | none | `200 {"status":"ok","version":"<sha>","uptime_seconds":N}` | - |
| `GET /api/ready` | **Readiness**: can it serve traffic? Pings Postgres and Redis (2 s timeout). | none | `200 {"status":"ok","checks":{"database":"ok","redis":"ok"}}` | `503` naming the failed check |
| `GET /metrics` | Prometheus exposition format. | `Authorization: Bearer $METRICS_TOKEN` when `METRICS_TOKEN` is set | `200` | `401` |

Use `/api/health` for "restart the container if this fails" and `/api/ready` for "stop routing traffic" or
"fail the deploy". Keeping them separate prevents a database outage from restart-looping a healthy process.

```bash
curl -s localhost:8080/api/health
curl -s localhost:8080/api/ready
curl -s -H "Authorization: Bearer $METRICS_TOKEN" localhost:8080/metrics | head
```

## Request IDs

Every response carries an `X-Request-ID` header, and every access-log line carries the same ID.

* A well-formed incoming `X-Request-ID` (8-64 characters from `A-Z a-z 0-9 . _ -`) is reused, so a trace ID
  from a proxy or the browser flows through. Anything else (too short, containing spaces or newlines) is
  replaced with a generated 16-character hex ID, which stops a client from injecting text into logs.
* The header is exposed to browser JavaScript through CORS, so an error screen can display it.

Finding everything that happened for one request:

```bash
curl -si localhost:8080/api/games/999999 | grep -i x-request-id      # x-request-id: 3f9c1e0a7b2d4c11
journalctl -u steamscope-api | grep 3f9c1e0a7b2d4c11
```

## Access log

By default only server errors (status 500 and up) are logged, plus recovered panics. One line per request would bury
real problems, since a single page makes dozens of calls. Set `ACCESS_LOG=true` to log every request; `/metrics`
scrapes are never logged. Request IDs and metrics do not depend on this setting.

Requests the client abandons (the browser navigates away or cancels) are recorded as status 499 rather than 500, so
they are not logged as errors and do not appear as server failures in the metrics. A request that exceeds its own
deadline is still a real error and is logged.

### Server errors

Every 500 is logged as a single structured line at error level, with the cause on the same line, so one search for
the request ID explains it:

```json
{"level":"ERROR","msg":"request failed","request_id":"a8f41ca8efafde2a","method":"GET","route":"GET /api/bundles/{bundleID}",
 "path":"/api/bundles/233","status":500,"duration_ms":12.4,"op":"get bundle","bundle_id":233,
 "error":"failed to get bundle: ...","error_type":"*fmt.wrapError","error_chain":["*pgconn.PgError: ..."],
 "pg_code":"42703","pg_table":"bundles","pg_constraint":""}
```

- `op` names what was being attempted, `error` is the full message, and `error_chain` lists each wrapped cause, which
  usually points at the real problem. Database errors add the Postgres code, table and constraint, and `user_id` is
  included for signed-in requests.
- The client gets only a short message and the same `request_id` (also in the `X-Request-ID` header). The site shows
  it as "(reference ...)" so a report can be matched to its log line. Internals are never sent to the client.
- Recovered panics are logged with the request ID, path and stack trace.
- A 5xx with no recorded cause is logged with `op":"unknown"`; that means a handler wrote the status without going
  through `serverError`, and is worth fixing.

In handler code, report a failure with `serverError(w, r, "operation", err, "message for the user", "key", value...)`.

A logged line looks like this:

```json
{"time":"2026-10-05T04:15:02Z","level":"INFO","msg":"request","request_id":"3f9c1e0a7b2d4c11",
 "method":"GET","route":"GET /api/games/{appID}","path":"/api/games/730","status":200,
 "duration_ms":4.21,"bytes":2210,"remote":"203.0.113.9:51234"}
```

A panic inside a handler is recovered into a JSON `500`, logged with its stack trace and request ID, and
counted in `steamscope_http_panics_total`.

```bash
# slowest requests
journalctl -u steamscope-api -o cat | jq -c 'select(.msg=="request") | {route,duration_ms,status}' | sort -t: -k3 -n | tail
# all 5xx responses
journalctl -u steamscope-api -o cat | jq -c 'select(.msg=="request" and .status>=500)'
```

## How the scraper reads prices

A store page can list several purchase blocks: the game itself, then DLC, upgrades, editions and bundle upsells. The
scraper uses the first block that is the game's own, in page order:

- A **"Free To Play"** block means the game is free (price 0), even if paid upgrades are listed after it. Counter-Strike
  2's Prime upgrade and Team Fortress 2's items were once stored as the game's price this way.
- Otherwise the first purchasable package (an `add_to_cart` block) is used. Demo blocks and bundle dropdowns are skipped.
- The discount is read from the block's `data-discount` and `data-price-final` attributes, then from its "-50%" text.
  If neither is present it is worked out from the price and the undiscounted price, so a discounted game always has a
  percentage.
- A page with **no price at all** (a delisted or unreleased game) is not treated as free: the last known price is kept
  and no price-history row is written for that day. The log line `No price found on the page for ...` marks these.

## Metrics

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `steamscope_http_requests_total` | counter | method, route, status | requests handled. `route` is the route **pattern** (`GET /api/games/{appID}`), never the raw path, which keeps cardinality bounded; unknown paths are labelled `unmatched` |
| `steamscope_http_request_duration_seconds` | histogram | method, route | request latency |
| `steamscope_http_requests_in_flight` | gauge | - | requests currently being served |
| `steamscope_http_panics_total` | counter | - | recovered handler panics |
| `steamscope_login_attempts_total` | counter | outcome = success / bad_credentials / banned / rate_limited | authentication health and brute-force signal |
| `steamscope_submissions_total` | counter | kind, outcome | game and bundle suggestions |
| `steamscope_prediction_requests_total` | counter | result = hit / miss / insufficient | price-forecast lookups: served from the cache, computed, or too little history |
| `steamscope_prediction_compute_seconds` | histogram | - | time to compute a forecast, including any ITAD calls |
| `steamscope_scrape_runs_total` | counter | result = success / error | scrape runs by the in-process scheduler |
| `steamscope_scrape_last_duration_seconds`, `steamscope_scrape_last_games` | gauge | - | duration and size of the last run |
| `steamscope_scrape_last_success_timestamp_seconds` | gauge | - | time of the last successful scrape |
| `steamscope_build_info` | gauge | version | which build is running |
| `go_*`, `process_*` | - | - | goroutines, GC, memory, CPU, file descriptors |

### Prometheus scrape configuration

```yaml
scrape_configs:
  - job_name: steamscope-api
    metrics_path: /metrics
    scheme: https
    authorization:
      type: Bearer
      credentials: <METRICS_TOKEN>
    static_configs:
      - targets: ["api.example.com:443"]
```

### Useful queries (PromQL)

```promql
# requests per second, by route
sum by (route) (rate(steamscope_http_requests_total[5m]))

# 95th percentile latency, by route
histogram_quantile(0.95, sum by (le, route) (rate(steamscope_http_request_duration_seconds_bucket[5m])))

# error ratio (5xx)
sum(rate(steamscope_http_requests_total{status=~"5.."}[5m])) / sum(rate(steamscope_http_requests_total[5m]))

# availability over 30 days (share of non-5xx responses)
1 - (sum(increase(steamscope_http_requests_total{status=~"5.."}[30d])) / sum(increase(steamscope_http_requests_total[30d])))

# failed logins per minute (credential stuffing shows up here)
sum(rate(steamscope_login_attempts_total{outcome=~"bad_credentials|rate_limited"}[5m])) * 60

# hours since the last successful scrape
(time() - steamscope_scrape_last_success_timestamp_seconds) / 3600
```

### Example alert rules

```yaml
groups:
  - name: steamscope
    rules:
      - alert: HighErrorRate
        expr: sum(rate(steamscope_http_requests_total{status=~"5.."}[5m])) / sum(rate(steamscope_http_requests_total[5m])) > 0.02
        for: 10m
      - alert: SlowAPI
        expr: histogram_quantile(0.95, sum by (le) (rate(steamscope_http_request_duration_seconds_bucket[5m]))) > 1
        for: 10m
      - alert: ScrapeStale
        expr: time() - steamscope_scrape_last_success_timestamp_seconds > 36 * 3600
      - alert: APIDown
        expr: up{job="steamscope-api"} == 0
        for: 2m
```

## Measuring quality

Commands for producing the project's quality figures.

| Measure | How to obtain it |
| --- | --- |
| Number of automated tests | `go test -json ./backend/... \| grep -c '"Action":"pass"'`, plus the count printed by `ng test` |
| Backend statement coverage | `go test -coverpkg=./backend/internal/... -coverprofile=c.out ./backend/... && go tool cover -func=c.out \| tail -1` (CI uploads `coverage.html`) |
| Latency under load | run a load test (`hey -z 30s -c 50 http://localhost:8080/api/games`, or k6), then read the p95 query above |
| Availability | the 30-day availability query above, once deployed |
| CI duration and deploy frequency | the Actions tab (run durations and the Deploy workflow history) |
| Deploy safety | the deploy workflow's smoke-test step, which polls `/api/ready` |
| Data volume | `select count(*) from price_history;` and the `steamscope_scrape_last_games` gauge |

Coverage from `go test -coverprofile` counts only the package under test unless `-coverpkg` is given, which is
why the command above sets it: handler code is exercised by tests that live in the router package.

## Test suite

* **Backend integration tests** (`backend/internal/router/*_test.go`) drive the real router, middleware and
  handlers against a real Postgres schema and an in-memory Redis. They assert HTTP status codes and JSON
  shapes for success and failure on: registration, login, logout and sessions; CSRF and CORS; rate limits;
  admin access control (401 for anonymous, 403 for regular users, on every admin route); user moderation; the
  approval queue; blacklist rules; submissions; preferences; the watchlist; notifications; Steam sign-in
  redirects; bundles; game search; and health, readiness and metrics. They need `TEST_DATABASE_URL`
  (see [CI-CD.md](CI-CD.md)) and are skipped without it.
* **Backend unit tests** cover the scraper and parsers, the description sanitizer, the price-series builder,
  the input sanitizer, username moderation, blacklist matching and the observability middleware.
* **Frontend component tests** (`*.spec.ts`) render components with test doubles for HTTP and authentication
  and assert the rendered HTML: prices and discount badges; the bundle cover collage and its fallbacks; the
  sanitized description rendering as real elements, with hostile markup stripped; status labels; admin tables
  and their controls; empty, loading and error states; route guards; and navigation per role. Dates are tested
  in a non-UTC timezone (CI sets `TZ=America/Los_Angeles`) because calendar days must not shift for viewers west
  of UTC.
