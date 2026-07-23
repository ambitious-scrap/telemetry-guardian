SHELL := /bin/sh
RUN_ID ?= phase1-local
GO_CACHE ?= /private/tmp/telemetry-guardian-gocache

.PHONY: fmt-check lint test signoz-test signoz-integration miner-test integration-test accept-phase0 accept-phase1 accept-phase2 accept-phase3 mine env-up env-ready env-down deploy-healthy deploy-broken seed load fault

fmt-check:
	git diff --check
	git diff --cached --check
	@test -z "$$(gofmt -l cmd demo internal/contracts internal/miner internal/signoz 2>/dev/null)"

lint:
	sh -n $$(find scripts -name '*.sh' -type f -print)
	GOCACHE=$(GO_CACHE) go vet ./...

test: lint
	GOCACHE=$(GO_CACHE) go test ./...

signoz-test:
	GOCACHE=$(GO_CACHE) go test ./internal/signoz -count=1

miner-test:
	GOCACHE=$(GO_CACHE) go test ./internal/contracts ./internal/miner -count=1

signoz-integration:
	./scripts/accept/phase2.sh

integration-test: signoz-integration

accept-phase0: fmt-check test
	./scripts/accept/phase0.sh

accept-phase1: fmt-check test
	./scripts/accept/phase1.sh

accept-phase2: fmt-check test
	./scripts/accept/phase2.sh

accept-phase3: fmt-check test
	./scripts/accept/phase3.sh

mine:
	GOCACHE=$(GO_CACHE) go run ./cmd/guardian mine

env-up:
	./scripts/env/up.sh

env-ready:
	./scripts/env/wait-ready.sh

env-down:
	./scripts/env/down.sh

deploy-healthy:
	./scripts/env/deploy.sh healthy "$(RUN_ID)"

deploy-broken:
	./scripts/env/deploy.sh broken "$(RUN_ID)"

seed:
	./scripts/seed/dashboard.sh "$(RUN_ID)"
	./scripts/seed/alert.sh "$(RUN_ID)"

load:
	./scripts/load/generate.sh 5

fault:
	./scripts/load/fault.sh
