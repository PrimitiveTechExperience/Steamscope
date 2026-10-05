# CI/CD

How Steamscope is built, tested and deployed. Sections 1-3 cover continuous integration (CI) and work on
any fork. Sections 4-8 cover optional deployment and the scheduled scrape.

| What | File |
| --- | --- |
| Tests, lint and build on every push and pull request | `.github/workflows/ci.yml` |
| Build, and optionally deploy, after CI passes on `main` | `.github/workflows/deploy.yml` |
| Weekly dependency-update pull requests | `.github/dependabot.yml` |
| Container images (optional) | `backend/Dockerfile`, `frontend/Dockerfile`, `docker-compose.yml` |
| Daily scrape (cron, systemd or Windows) | `deploy/` |
| Line endings kept as LF | `.gitattributes` |

---

## 1. Continuous integration

The `CI` workflow runs on every push to `main` and on every pull request. A newer push to the same branch
cancels the older run.

| Job | What it does | Fails when |
| --- | --- | --- |
| **Backend (Go)** | `gofmt` check, `go vet`, `go build`, `go test -race` with coverage, against a Postgres 16 service container | formatting differs, a vet warning, or any test fails |
| **Frontend (Angular)** | installs the pinned npm, `npm ci`, `ng test` (in a non-UTC timezone), `ng build` | a test fails, a type or build error, or the bundle exceeds the 1 MB hard budget |
| **Docker images build** | builds both images without pushing | a Dockerfile is broken |
| **Dependency audit** | `govulncheck` and `npm audit` | never blocks (advisory only) |

The backend job writes a coverage summary to the run page and uploads `coverage.html` as an artifact.

### Reproducing CI locally

```bash
# Backend (integration tests need a Postgres; see the note below)
export TEST_DATABASE_URL="postgres://user:pass@host:5432/dbname"
gofmt -l backend && go vet ./... && go test -race ./backend/...

# Frontend
cd frontend
npm install -g npm@11.6.2        # the version pinned by "packageManager" in package.json
npm ci
TZ=America/Los_Angeles npx ng test --watch=false     # PowerShell: $env:TZ='America/Los_Angeles'; npx ng test --watch=false
npx ng build
```

**`TEST_DATABASE_URL`.** The integration tests create a uniquely named schema (`t_<random>`), build every table
into it from `backend/postgres/setup.sql`, and drop it afterwards. They never read or write the `public`
schema. A dedicated database (a local Postgres or a separate hosted project) is still preferable to a
production one, and keeps clear of connection limits such as a pooler's cap. When the variable is unset, the
integration tests are skipped, not failed. Unit tests (parsers, sanitizers, moderation, observability) run
without a database.

`-race` needs a C compiler and a 64-bit platform. CI runs it on Linux; omit the flag locally if it is unsupported.

### Common failures

| Symptom | Cause and fix |
| --- | --- |
| `These files need gofmt` | run `gofmt -w backend` and commit |
| Backend tests skipped | `TEST_DATABASE_URL` is unset (expected locally, impossible in CI) |
| `ng build` budget error | the initial bundle exceeded 1 MB; lazy-load a route |
| `npm ci` fails with `Missing: @emnapi/core ... from lock file` | npm version mismatch, see [npm version](#npm-version) |
| `gofmt` flags a file nobody changed | CRLF line endings; run `git add --renormalize .` |

### npm version

`package.json` pins npm 11 through the `packageManager` field. Lockfiles written by Dependabot and by local
installs come from npm 11, but Node 22 bundles npm 10, which rejects them (`Missing: @emnapi/core ... from lock
file`). CI and the frontend Dockerfile therefore install `npm@11.6.2` before `npm ci`. Local installs should use
the same version. The lockfile should not be edited by hand.

## 2. Branch protection

Recommended rules for `main` (Settings -> Rules, or Branches):

* Require a pull request before merging.
* Require status checks to pass: **Backend (Go)**, **Frontend (Angular)**, **Docker images build**. The names
  must match the job names exactly, and they appear in the list after the first run.
* Require branches to be up to date before merging.
* Required approvals: a project with a single maintainer cannot approve its own pull requests, so use `0`
  approvals (the PR and the checks are still required) or add a second reviewer.

A status badge for the README:

```md
![CI](https://github.com/<owner>/<repo>/actions/workflows/ci.yml/badge.svg)
```

### Pull request workflow

```bash
git checkout main && git pull origin main
git checkout -b <branch-name>
# ...edit...
git add <files>
git commit -m "Describe the change"
git push -u origin <branch-name>      # prints a link that opens the pull request
```

Review on GitHub: **Files changed** -> **Review changes** -> Comment, Approve or Request changes. Merge once
the required checks are green.

## 3. Dependabot

`.github/dependabot.yml` opens update pull requests for Go modules, npm packages, GitHub Actions and Docker
base images.

* Angular packages pin each other's exact versions, so they are grouped into a single pull request and must
  move together.
* Major-version updates are not proposed automatically for npm packages, the Node base image or the Go base
  image. Those upgrades (for example a new Angular major, which also needs a newer TypeScript) are done
  deliberately.
* A Dependabot PR created before a fix landed on `main` inherits the old failures. Comment
  `@dependabot rebase` to update it, or `@dependabot recreate` to rebuild it. `@dependabot close` and
  `@dependabot ignore this major version` dismiss one permanently.
* Merge them one at a time: each merge makes the others stale.

## 4. Deployment configuration

`deploy.yml` runs after CI succeeds on `main`, and can also be started by hand (Actions -> Deploy -> Run
workflow). By default it only builds Linux binaries and attaches them to the run. Each further stage is enabled
by a repository variable.

Settings -> Secrets and variables -> Actions:

| Name | Kind | Used by | Meaning |
| --- | --- | --- | --- |
| `DEPLOY_SSH_ENABLED` | variable | SSH deploy | `true` enables the stage |
| `DEPLOY_HOST` | secret | SSH deploy | server hostname or IP |
| `DEPLOY_USER` | secret | SSH deploy | login user (for example `steamscope`) |
| `DEPLOY_SSH_KEY` | secret | SSH deploy | private key (see 5.2) |
| `DEPLOY_SSH_PORT` | variable | SSH deploy | default 22 |
| `DEPLOY_PATH` | variable | SSH deploy | default `/opt/steamscope` |
| `HEALTHCHECK_URL` | variable | SSH deploy | for example `https://api.example.com`; `/api/ready` is polled after the restart |
| `PUBLISH_IMAGES` | variable | GHCR | `true` pushes images to `ghcr.io/<owner>/<repo>-api` and `-web` |
| `API_URL` | variable | GHCR | public API URL baked into the web image, for example `https://api.example.com/api` |

The SSH stage uses a GitHub Environment named `production`. GitHub creates it on first use; add required
reviewers to it for a manual approval before each deploy.

## 5. Deploying the backend to a Linux server (no Docker)

### 5.1 One-time server setup

```bash
# as a sudo user on the server
sudo useradd --system --create-home --home-dir /opt/steamscope --shell /usr/sbin/nologin steamscope
sudo mkdir -p /opt/steamscope/{logs,deploy}
sudo chown -R steamscope:steamscope /opt/steamscope

# configuration (never commit this file)
sudo nano /opt/steamscope/.env          # DATABASE_URL, REDIS_URL, SESSION_*_KEY, ITAD_API_KEY, ...
sudo chmod 600 /opt/steamscope/.env
sudo chown steamscope:steamscope /opt/steamscope/.env

# Steam cookie file the scraper reads from ./internal/config/config.json
sudo -u steamscope mkdir -p /opt/steamscope/internal/config
sudo -u steamscope cp config.json /opt/steamscope/internal/config/config.json   # from the build artifact
```

Production values for `.env`: `COOKIE_SECURE=true`, `FRONTEND_URL=https://app.example.com`,
`BACKEND_URL=https://api.example.com`, `METRICS_TOKEN=<random string>`, and `DISABLE_SCHEDULER=true` when a
timer or cron job runs the scrape (section 6).

Install the API service unit from `deploy/systemd/`:

```bash
sudo cp deploy/systemd/steamscope-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable steamscope-api
```

Allow the deploy user to restart only the API, without a password:

```bash
echo 'steamscope ALL=(root) NOPASSWD: /usr/bin/systemctl restart steamscope-api' | sudo tee /etc/sudoers.d/steamscope
```

Terminate TLS in a reverse proxy in front of the API; for example, Caddy:

```
api.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

### 5.2 Deploy key

```bash
ssh-keygen -t ed25519 -f steamscope_deploy -N "" -C "github-actions-deploy"
ssh-copy-id -i steamscope_deploy.pub steamscope@<server>     # or append it to ~/.ssh/authorized_keys on the server
```

The private key (`steamscope_deploy`) goes into the `DEPLOY_SSH_KEY` secret; delete the local copy afterwards.
Set `DEPLOY_HOST`, `DEPLOY_USER` and `DEPLOY_SSH_ENABLED=true`.

### 5.3 What a deploy does

It builds the binaries, uploads `server`, `scraper`, `backfill-history` and `deploy/` to the deploy path,
restarts the API, and, when `HEALTHCHECK_URL` is set, waits for `/api/ready` to return 200.

## 6. Scheduled scrape

The API process can scrape on a timer itself (the default). A separate scheduled job keeps scraping out of the
web process and is the more robust option. Use exactly one of them, and set `DISABLE_SCHEDULER=true` for the
API when using an external job.

`deploy/cron/scrape.sh` wraps the scraper with a lock (no overlapping runs), a dated log file with 14-day
rotation, a non-zero exit code on failure, and an optional heartbeat URL (`PING_URL`, for example
healthchecks.io or Uptime Kuma) that alerts when the job stops running.

**cron**

```bash
sudo -u steamscope crontab -e
# add the line from deploy/cron/steamscope.crontab:
15 4 * * * /usr/bin/env bash /opt/steamscope/deploy/cron/scrape.sh
```

**systemd timer** (logs in `journalctl`; catches up after downtime)

```bash
sudo cp deploy/systemd/steamscope-scrape.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now steamscope-scrape.timer
systemctl list-timers steamscope-scrape.timer       # next run
sudo systemctl start steamscope-scrape.service      # run now
journalctl -u steamscope-scrape.service -n 50
```

**Windows Task Scheduler**

```powershell
powershell -ExecutionPolicy Bypass -File deploy\windows\register-scrape-task.ps1 -At 04:15
Start-ScheduledTask -TaskName SteamscopeDailyScrape     # run now
Get-Content logs\scrape-*.log -Tail 20
```

Each run scrapes every tracked game and bundle, backfills games with a thin price history from
IsThereAnyDeal, and prunes history older than two years. The exit code is non-zero if the scrape failed. The
metric `steamscope_scrape_last_success_timestamp_seconds` is only updated by the in-process scheduler (see
[OPERATIONS.md](OPERATIONS.md)); for an external job, monitor the heartbeat URL or the exit code.

## 7. Deploying the frontend

The frontend is an Angular server-side-rendered app that runs on Node.

* **Vercel or Netlify:** import the repository, set the root directory to `frontend`, and use `npx ng build`
  as the build command. Set the production API address in `frontend/src/environments/environment.production.ts`
  (or generate it in a pre-build step) before building.
* **Self-hosted:** use the Docker image (`frontend/Dockerfile`), or run `node dist/frontend/server/server.mjs`
  behind the same reverse proxy. Set `NG_ALLOWED_HOSTS=app.example.com` so Angular accepts the hostname.

**Cookies and CORS.** Login uses a `SameSite=Lax` cookie, and the API accepts state-changing requests only when
their `Origin` equals `FRONTEND_URL`. Therefore:

* Serve the site and the API from sibling subdomains of one domain (`app.example.com` and `api.example.com`),
  or from one domain with `/api` proxied. Unrelated domains cannot share a login.
* `FRONTEND_URL` must equal the site's exact origin: scheme and host, with no trailing slash.
* `COOKIE_SECURE=true` whenever HTTPS is used.

## 8. Containers (optional)

```bash
docker build -f backend/Dockerfile -t steamscope-api .
docker build -t steamscope-web --build-arg API_URL=https://api.example.com/api frontend
docker compose up --build                               # both, configured from .env
docker compose --profile tools run --rm scraper         # one-off scrape
```

Both images run as a non-root user and define a `HEALTHCHECK`.

## 9. Releases and monitoring

* Image and binary builds embed the commit SHA, exposed as `steamscope_build_info{version="..."}`, so the
  running build is always identifiable.
* The dependency-audit job is advisory; review it periodically.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| Dependabot PR fails `npm audit` or `npm ci` without touching those files | the branch is older than `main`; comment `@dependabot rebase` |
| `npm install` rewrites `package-lock.json` with only `"peer": true` changes | the local npm is older than the pinned version; update it and restore the file with `git checkout frontend/package-lock.json` |
| Deploy: `Permission denied (publickey)` | key missing from the server's `authorized_keys`, or the secret has extra whitespace |
| Deploy: `sudo: a password is required` | the `/etc/sudoers.d/steamscope` rule is missing |
| Login works locally but not in production | cookie or CORS configuration, see section 7 |
| The SSR page returns `400` for the domain | set `NG_ALLOWED_HOSTS` on the web server |
| The scrape appears to do nothing | check `logs/scrape-*.log`, the cookie file path, and `DATABASE_URL` in `.env` |

## Security notes

* `.env` is ignored by git. `backend/internal/config/config.json` (Steam cookies) is tracked, so anything in it
  is public; keep it free of real session cookies and rotate any that were committed.
* `scraper.exe` and `logs/` are ignored; remove an accidentally committed binary with `git rm --cached`.
