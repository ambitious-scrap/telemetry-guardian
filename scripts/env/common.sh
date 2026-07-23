#!/bin/sh

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR="$ROOT/.run"
SIGNOZ_URL=${SIGNOZ_URL:-http://127.0.0.1:18080}
CHECKOUT_URL=${CHECKOUT_URL:-http://127.0.0.1:19090}
OTLP_URL=${OTLP_URL:-http://127.0.0.1:14318}
COLLECTOR_HEALTH_URL=${COLLECTOR_HEALTH_URL:-http://127.0.0.1:13134}

mkdir -p "$RUN_DIR"
chmod 700 "$RUN_DIR"

stop_checkout() {
	if docker container inspect telemetry-guardian-checkout >/dev/null 2>&1; then
		docker rm --force telemetry-guardian-checkout >/dev/null
	fi
}

api_token() {
	"$ROOT/scripts/seed/auth.sh" >/dev/null
	sed -n '1p' "$RUN_DIR/signoz-token"
}
