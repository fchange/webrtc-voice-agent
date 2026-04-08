.PHONY: help test-go run-signal run-bot run-web check bootstrap fmt-go

help:
	@echo "Available targets:"
	@echo "  make bootstrap   # show local setup hints"
	@echo "  make test-go     # run Go unit tests"
	@echo "  make run-signal  # start signaling stub"
	@echo "  make run-bot     # start bot stub"
	@echo "  make run-web     # start web app"
	@echo "  make check       # run lightweight checks"

bootstrap:
	@./scripts/bootstrap.sh

test-go:
	@./scripts/test.sh

run-signal:
	@./scripts/dev-signal.sh

run-bot:
	@./scripts/dev-bot.sh

run-web:
	@./scripts/dev-web.sh

check:
	@./scripts/check.sh

fmt-go:
	@gofmt -w cmd internal pkg

