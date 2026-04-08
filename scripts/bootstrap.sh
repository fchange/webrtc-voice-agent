#!/usr/bin/env bash
set -euo pipefail

echo "Required tools:"
echo "  - Go 1.23+"
echo "  - Node.js 20+"
echo "  - pnpm 10+"
echo
echo "Suggested first steps:"
echo "  1. cp .env.example .env"
echo "  2. make test-go"
echo "  3. make run-signal"
echo "  4. make run-bot"
echo "  5. pnpm install && make run-web"

