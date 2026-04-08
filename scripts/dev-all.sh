#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

load_env_file() {
  local env_file=$1
  [[ -f "$env_file" ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ -z "$line" ]] && continue
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ "$line" == *=* ]] || continue

    local key=${line%%=*}
    local value=${line#*=}

    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"

    if [[ "$value" =~ ^\".*\"$ ]] || [[ "$value" =~ ^\'.*\'$ ]]; then
      value=${value:1:${#value}-2}
    fi

    export "$key=$value"
  done < "$env_file"
}

load_env_file "$ROOT_DIR/.env.local"

export GOCACHE="${GOCACHE:-/tmp/webrtc-voice-bot-gocache}"
export SIGNAL_ADDR="${SIGNAL_ADDR:-:8080}"
export SIGNAL_PUBLIC_WS_URL="${SIGNAL_PUBLIC_WS_URL:-ws://localhost:8080/ws}"
export SIGNAL_DEV_TOKEN="${SIGNAL_DEV_TOKEN:-dev-token}"
export SIGNAL_BOT_BASE_URL="${SIGNAL_BOT_BASE_URL:-http://localhost:8081}"

export BOT_ADDR="${BOT_ADDR:-:8081}"
export BOT_SIGNAL_WS_URL="${BOT_SIGNAL_WS_URL:-$SIGNAL_PUBLIC_WS_URL}"
export BOT_SIGNAL_TOKEN="${BOT_SIGNAL_TOKEN:-$SIGNAL_DEV_TOKEN}"

signal_http_base="${SIGNAL_PUBLIC_WS_URL%/ws}"
signal_http_base="${signal_http_base/#ws:\/\//http://}"
signal_http_base="${signal_http_base/#wss:\/\//https://}"
export VITE_SIGNAL_HTTP_URL="${VITE_SIGNAL_HTTP_URL:-$signal_http_base}"
export VITE_SIGNAL_WS_URL="${VITE_SIGNAL_WS_URL:-$SIGNAL_PUBLIC_WS_URL}"
export VITE_DEV_TOKEN="${VITE_DEV_TOKEN:-$SIGNAL_DEV_TOKEN}"
export WEB_SIGNAL_HTTP_URL="${WEB_SIGNAL_HTTP_URL:-$VITE_SIGNAL_HTTP_URL}"
export WEB_SIGNAL_WS_URL="${WEB_SIGNAL_WS_URL:-$VITE_SIGNAL_WS_URL}"
export WEB_DEV_TOKEN="${WEB_DEV_TOKEN:-$VITE_DEV_TOKEN}"

declare -a CHILD_PIDS=()

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  for pid in "${CHILD_PIDS[@]:-}"; do
    if kill -0 "$pid" >/dev/null 2>&1; then
      kill "$pid" >/dev/null 2>&1 || true
    fi
  done
  wait >/dev/null 2>&1 || true
  exit "$exit_code"
}

trap cleanup EXIT INT TERM

start_service() {
  local name=$1
  shift

  (
    "$@" 2>&1 | while IFS= read -r line; do
      printf '[%s] %s\n' "$name" "$line"
    done
  ) &
  CHILD_PIDS+=("$!")
}

wait_for_http() {
  local name=$1
  local url=$2
  local attempts=${3:-50}
  local delay=${4:-0.2}

  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      printf '%s is ready: %s\n' "$name" "$url"
      return 0
    fi
    sleep "$delay"
  done

  printf '%s failed readiness check: %s\n' "$name" "$url" >&2
  return 1
}

printf 'Starting WebRTC Voice Bot dev stack\n'
printf '  signal: %s\n' "$SIGNAL_ADDR"
printf '  bot:    %s\n' "$BOT_ADDR"
printf '  web:    %s\n' "${WEB_DEV_URL:-http://localhost:5173}"

start_service signal ./scripts/dev-signal.sh
wait_for_http signal "$signal_http_base/healthz"

start_service bot ./scripts/dev-bot.sh
wait_for_http bot "http://127.0.0.1${BOT_ADDR}/healthz"

start_service web ./scripts/dev-web.sh

wait
