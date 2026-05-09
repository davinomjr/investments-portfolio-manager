#!/usr/bin/env bash
# Run the local B3 worker (which can bypass Cloudflare from a residential IP)
# and POST the resulting holdings to the Railway-hosted backend's push
# endpoint. Useful when scheduling sync from a developer machine — Railway
# itself can't run the browser due to Cloudflare.
#
# Required env vars:
#   PUSH_TOKEN   — bearer token matching backend's PUSH_TOKEN env var
#   PUSH_URL     — full URL to the push endpoint
#                  (e.g. https://davinomjr.com/investments/api/portfolio/import-push)
#
# Optional:
#   WORKER_DIR   — path to the worker (defaults to ../worker relative to repo root)
#   WORKER_VENV  — path to the worker's venv (defaults to $WORKER_DIR/.venv)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKER_DIR="${WORKER_DIR:-$REPO_ROOT/worker}"
WORKER_VENV="${WORKER_VENV:-$WORKER_DIR/.venv}"

ENV_FILE="$REPO_ROOT/scripts/.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

: "${PUSH_TOKEN:?PUSH_TOKEN must be set}"
: "${PUSH_URL:?PUSH_URL must be set}"

if [[ ! -x "$WORKER_VENV/bin/python" ]]; then
  echo "worker venv not found at $WORKER_VENV — run setup first" >&2
  exit 1
fi

cd "$WORKER_DIR"
# shellcheck disable=SC1091
source "$WORKER_VENV/bin/activate"

# The worker prints a Python-repr-ish dict to stdout instead of clean JSON
# when called without --json. With --json it writes valid JSON we can pipe
# straight to curl.
TMP_PAYLOAD="$(mktemp -t push-portfolio.XXXXXX.json)"
trap 'rm -f "$TMP_PAYLOAD"' EXIT

# B3 occasionally fails on the modal/download step — a transient timeout
# leaves the worker producing no holdings, which the backend rejects with
# 400. Retry up to 2 more times before giving up so a single bad morning
# doesn't lose the day's sync.
HTTP_STATUS=""
for attempt in 1 2 3; do
  if [[ "$attempt" -gt 1 ]]; then
    echo "[push] retrying worker (attempt $attempt) after 15s…" >&2
    sleep 15
  fi

  echo "[push] running worker (this opens a browser; B3 portal scrape)…" >&2
  : > "$TMP_PAYLOAD"
  if ! python -m app.main import --json > "$TMP_PAYLOAD"; then
    echo "[push] worker exited non-zero on attempt $attempt" >&2
    continue
  fi

  if [[ ! -s "$TMP_PAYLOAD" ]]; then
    echo "[push] worker produced empty output on attempt $attempt" >&2
    continue
  fi

  echo "[push] uploading to $PUSH_URL …" >&2
  HTTP_STATUS="$(curl --silent --show-error --output /tmp/push-portfolio-response.json \
    --write-out '%{http_code}' \
    -X POST "$PUSH_URL" \
    -H "Authorization: Bearer $PUSH_TOKEN" \
    -H "Content-Type: application/json" \
    --data-binary @"$TMP_PAYLOAD")"

  echo "[push] HTTP $HTTP_STATUS (attempt $attempt)" >&2
  cat /tmp/push-portfolio-response.json
  echo

  if [[ "$HTTP_STATUS" == "200" ]]; then
    break
  fi
done

if [[ "$HTTP_STATUS" != "200" ]]; then
  echo "[push] B3 push failed after retries — skipping IBKR" >&2
  exit 1
fi

# IBKR uses Flex Query (plain HTTPS), so Railway can fetch it directly —
# trigger the backend to run its own IBKR sync alongside the B3 push.
if [[ -n "${IBKR_TRIGGER_URL:-}" ]]; then
  echo "[push] triggering IBKR sync at $IBKR_TRIGGER_URL …" >&2
  IBKR_STATUS="$(curl --silent --show-error --output /tmp/push-ibkr-response.json \
    --write-out '%{http_code}' \
    -X POST "$IBKR_TRIGGER_URL" \
    -H "Authorization: Bearer $PUSH_TOKEN")"
  echo "[push] IBKR HTTP $IBKR_STATUS" >&2
  cat /tmp/push-ibkr-response.json
  echo
  if [[ "$IBKR_STATUS" != "202" && "$IBKR_STATUS" != "200" ]]; then
    exit 1
  fi
fi
