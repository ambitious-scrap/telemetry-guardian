SHELL := /bin/sh
RUN_ID ?= phase1-local

.PHONY: fmt-check lint test accept-phase0 accept-phase1 env-up env-ready env-down deploy-healthy deploy-broken seed load fault

fmt-check:
	git diff --check
	git diff --cached --check
	@test -z "$$(gofmt -l demo 2>/dev/null)"

lint:
	sh -n $$(find scripts -name '*.sh' -type f -print)
	go vet ./...

test: lint
	go test ./...

accept-phase0: fmt-check test
	./scripts/accept/phase0.sh

accept-phase1: fmt-check test
	./scripts/accept/phase1.sh

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
