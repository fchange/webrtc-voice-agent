#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/webrtc-voice-bot-gocache}"

go test ./...
