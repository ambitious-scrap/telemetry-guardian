SHELL := /bin/sh

.PHONY: fmt-check lint test accept-phase0

fmt-check:
	git diff --check
	git diff --cached --check

lint:
	sh -n scripts/accept/phase0.sh

test: lint
	@echo "Phase 0 has no product code; structural checks are the test suite."

accept-phase0: fmt-check test
	./scripts/accept/phase0.sh
