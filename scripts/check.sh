#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/webrtc-voice-agent-gocache}"

echo "==> gofmt"
gofmt -w cmd internal pkg

echo "==> go test"
go test ./...

echo "==> frontend dependency check"
if [ -f apps/web/package.json ]; then
  echo "apps/web exists; run 'pnpm install' before frontend checks"
fi
