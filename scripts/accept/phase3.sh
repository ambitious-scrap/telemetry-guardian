#!/bin/sh

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR="$ROOT/.run"
SIGNOZ_URL=${SIGNOZ_URL:-http://127.0.0.1:18080}
GOCACHE=${GOCACHE:-/private/tmp/telemetry-guardian-gocache}
RUN_ID=${RUN_ID:-phase3-live-$(date -u +%Y%m%d%H%M%S)}
OUTPUT="$RUN_DIR/telemetry.guardian.yaml"
cd "$ROOT"
. "$ROOT/scripts/env/common.sh"

required_files='cmd/guardian/main.go
internal/contracts/contracts.go
internal/miner/miner.go
internal/miner/testdata/canonical-contract.yaml
contracts/telemetry.guardian.yaml'
printf '%s\n' "$required_files" | while IFS= read -r file; do
	test -f "$file" || { echo "missing Phase 3 file: $file" >&2; exit 1; }
done

test -z "$(gofmt -l cmd internal/contracts internal/miner internal/signoz)"
GOCACHE="$GOCACHE" go test ./... -count=1

if grep -RniE 'Bearer[[:space:]]+[A-Za-z0-9._-]{20,}|accessToken|signoz-token' internal/signoz/testdata internal/contracts internal/miner contracts; then
	echo "secret-like value found in Phase 3 data" >&2
	exit 1
fi

./scripts/seed/dashboard.sh "$RUN_ID"
./scripts/seed/alert.sh "$RUN_ID"
./scripts/seed/verify.sh

dashboard_id=$(jq -er '.data[]? | select(.data.title == "telemetry-guardian-checkout") | .id' "$RUN_DIR/dashboards.json" | head -1)
alert_id=$(jq -er '.data.id' "$RUN_DIR/alert-response.json")
token=$(api_token)

GOCACHE="$GOCACHE" SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" \
	SIGNOZ_DASHBOARD_ID="$dashboard_id" SIGNOZ_ALERT_ID="$alert_id" \
	GUARDIAN_OUTPUT="$OUTPUT" \
	go run ./cmd/guardian mine

test -s "$OUTPUT"
check_count=$(awk '
 /^checks:$/ { in_checks=1; next }
 in_checks && /^  - id:/ { count++ }
 END { print count + 0 }
' "$OUTPUT")
test "$check_count" = 4 || { echo "expected 4 checks, got $check_count" >&2; exit 1; }
grep -F 'field: cart.value' "$OUTPUT" >/dev/null
grep -F 'field: error.type' "$OUTPUT" >/dev/null
grep -F 'operation: payment.authorize' "$OUTPUT" >/dev/null
grep -F 'alert_id: payment-timeout' "$OUTPUT" >/dev/null
grep -F 'source_path:' "$OUTPUT" >/dev/null

git diff --check
echo "Phase 3 acceptance passed"
