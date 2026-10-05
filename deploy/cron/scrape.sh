#!/usr/bin/env bash
# Runs the Steamscope scraper once: scrapes every tracked game and bundle,
# backfills thin price histories and prunes old history. Meant for cron or a
# systemd timer on the machine that hosts the API (no Docker involved).
#
# Usage:  scrape.sh            (uses the defaults below)
# Config via environment (set in the crontab, the systemd unit, or /etc/default/steamscope):
#   STEAMSCOPE_DIR   directory with the .env file and the scraper binary  (default /opt/steamscope)
#   SCRAPER_BIN      path to the scraper binary                            (default $STEAMSCOPE_DIR/scraper)
#   LOG_DIR          where logs go                                          (default $STEAMSCOPE_DIR/logs)
#   KEEP_LOGS        number of daily logs to keep                           (default 14)
#   PING_URL         optional heartbeat URL (healthchecks.io, Uptime Kuma) called on success;
#                    "$PING_URL/fail" is called on failure
set -Eeuo pipefail

STEAMSCOPE_DIR="${STEAMSCOPE_DIR:-/opt/steamscope}"
SCRAPER_BIN="${SCRAPER_BIN:-$STEAMSCOPE_DIR/scraper}"
LOG_DIR="${LOG_DIR:-$STEAMSCOPE_DIR/logs}"
KEEP_LOGS="${KEEP_LOGS:-14}"
LOCK_FILE="${LOCK_FILE:-/tmp/steamscope-scrape.lock}"

mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/scrape-$(date +%F).log"

log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$LOG_FILE"; }

ping() { [[ -n "${PING_URL:-}" ]] && curl -fsS -m 10 --retry 3 -o /dev/null "$PING_URL$1" || true; }

# Never run two scrapes at once (a slow run must not overlap the next tick).
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  log "another scrape is still running; skipping this run"
  exit 0
fi

if [[ ! -x "$SCRAPER_BIN" ]]; then
  log "scraper binary not found or not executable: $SCRAPER_BIN"
  ping /fail
  exit 1
fi

# The scraper reads .env (database URL, ITAD key, ...) from its working directory.
cd "$STEAMSCOPE_DIR"
log "scrape starting"
start=$(date +%s)

if "$SCRAPER_BIN" >>"$LOG_FILE" 2>&1; then
  log "scrape finished OK in $(( $(date +%s) - start ))s"
  ping ""
else
  code=$?
  log "scrape FAILED (exit $code) after $(( $(date +%s) - start ))s; see $LOG_FILE"
  ping /fail
  exit "$code"
fi

# Rotate: keep the newest $KEEP_LOGS daily logs.
ls -1t "$LOG_DIR"/scrape-*.log 2>/dev/null | tail -n +"$((KEEP_LOGS + 1))" | xargs -r rm -f --
