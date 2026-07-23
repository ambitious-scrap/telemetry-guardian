#!/bin/sh

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR="$ROOT/.run"
SIGNOZ_URL=${SIGNOZ_URL:-http://127.0.0.1:18080}
GOCACHE=${GOCACHE:-/private/tmp/telemetry-guardian-gocache}
RUN_ID=${RUN_ID:-phase2-live-$(date -u +%Y%m%d%H%M%S)}
cd "$ROOT"
. "$ROOT/scripts/env/common.sh"

required_files='internal/signoz/API.md
internal/signoz/client.go
internal/signoz/fake.go
internal/signoz/client_test.go
internal/signoz/live_test.go
internal/signoz/testdata/dashboard-success.json
internal/signoz/testdata/alert-success.json
internal/signoz/testdata/query-success.json
internal/signoz/testdata/query-empty.json
internal/signoz/testdata/history-empty.json'
printf '%s\n' "$required_files" | while IFS= read -r file; do
	test -f "$file" || { echo "missing Phase 2 file: $file" >&2; exit 1; }
done

test -z "$(gofmt -l internal/signoz)"
GOCACHE="$GOCACHE" go test ./internal/signoz -count=1

if grep -RniE 'Bearer[[:space:]]+[A-Za-z0-9._-]{20,}|accessToken|signoz-token' internal/signoz/testdata; then
	echo "secret-like value found in fixture data" >&2
	exit 1
fi

if ! curl --silent --show-error --fail --max-time 5 "$SIGNOZ_URL/api/v1/health" >/dev/null 2>&1; then
	echo "Phase 1 SigNoz is not ready at $SIGNOZ_URL; start or reuse the isolated environment before live acceptance" >&2
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
	go test ./internal/signoz -run '^TestLiveSigNozAdapter$' -count=1 -v

git diff --check
echo "Phase 2 acceptance passed"
