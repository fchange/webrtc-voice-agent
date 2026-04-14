#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/webrtc-voice-agent-gocache}"

go run ./cmd/bot
