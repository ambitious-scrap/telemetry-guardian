#!/bin/sh

set -eu
. "$(dirname -- "$0")/common.sh"

stop_checkout
compose="$ROOT/foundry/pours/deployment/compose.yaml"
if [ -f "$compose" ]; then
	SIGNOZ_TOKENIZER_JWT_SECRET=teardown-only
	export SIGNOZ_TOKENIZER_JWT_SECRET
	docker compose -f "$compose" -p telemetry-guardian down --volumes --remove-orphans --timeout 20
fi
rm -f "$RUN_DIR"/*
echo "Telemetry Guardian environment removed"
