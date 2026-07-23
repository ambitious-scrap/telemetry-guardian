#!/bin/sh

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR="$ROOT/.run"
CHECKOUT_URL=${CHECKOUT_URL:-http://127.0.0.1:19090}
cd "$ROOT"

cleanup() {
	./scripts/env/down.sh >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

go test ./demo/checkout

attempt=1
while [ "$attempt" -le 3 ]; do
	echo "phase1 scenario $attempt/3"
	./scripts/env/down.sh
	./scripts/env/up.sh

	stamp=$(date +%s)
	healthy_run="phase1-$attempt-$stamp-healthy"
	broken_run="phase1-$attempt-$stamp-broken"

	./scripts/env/deploy.sh healthy "$healthy_run"
	./scripts/seed/dashboard.sh "$healthy_run"
	./scripts/seed/alert.sh "$healthy_run"
	./scripts/seed/verify.sh
	healthy_status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/healthy-response.json" -w '%{http_code}' \
		-H 'Content-Type: application/json' --data '{"cart_value":42}' "$CHECKOUT_URL/checkout")
	[ "$healthy_status" = 200 ]
	./scripts/load/generate.sh 5
	./scripts/load/fault.sh "$RUN_DIR/healthy-fault.json"
	./scripts/load/assert-telemetry.sh healthy "$healthy_run"
	./scripts/load/wait-alert.sh firing 120

	./scripts/env/deploy.sh broken "$broken_run"
	./scripts/seed/dashboard.sh "$broken_run"
	./scripts/seed/alert.sh "$broken_run"
	./scripts/seed/verify.sh
	baseline=$(jq -s '[.[] | select(.status == "firing")] | length' "$RUN_DIR/alert-events.jsonl")
	broken_status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/broken-response.json" -w '%{http_code}' \
		-H 'Content-Type: application/json' --data '{"cart_value":42}' "$CHECKOUT_URL/checkout")
	[ "$broken_status" = 200 ]
	./scripts/load/generate.sh 5
	./scripts/load/fault.sh "$RUN_DIR/broken-fault.json"
	cmp "$RUN_DIR/healthy-response.json" "$RUN_DIR/broken-response.json"
	cmp "$RUN_DIR/healthy-fault.json" "$RUN_DIR/broken-fault.json"
	./scripts/load/assert-telemetry.sh broken "$broken_run"
	./scripts/load/assert-alert-miss.sh 45 "$baseline"

	./scripts/env/down.sh
	[ -z "$(docker ps -aq --filter name=telemetry-guardian)" ]
	[ -z "$(docker volume ls -q --filter name=telemetry-guardian)" ]
	if docker network inspect telemetry-guardian-network >/dev/null 2>&1; then
		echo "Telemetry Guardian network survived teardown" >&2
		exit 1
	fi
	echo "phase1 scenario $attempt/3 passed"
	attempt=$((attempt + 1))
done

trap - EXIT HUP INT TERM
echo "Phase 1 acceptance passed three consecutive scenarios"
