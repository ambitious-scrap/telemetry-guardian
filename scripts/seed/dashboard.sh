#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

run_id=${1:-}
[ -n "$run_id" ] || { echo "usage: $0 RUN_ID" >&2; exit 2; }
token=$(api_token)
name=telemetry-guardian-checkout
sed "s/__RUN_ID__/$run_id/g" "$ROOT/fixtures/dashboards/telemetry-guardian-checkout.json" >"$RUN_DIR/dashboard.json"

curl --silent --show-error --fail --max-time 10 -H "Authorization: Bearer $token" \
	"$SIGNOZ_URL/api/v1/dashboards" >"$RUN_DIR/dashboards.json"
dashboard_id=$(jq -r --arg name "$name" '.data[]? | select(.data.title == $name) | .id' "$RUN_DIR/dashboards.json" | head -1)
if [ -n "$dashboard_id" ]; then
	method=PUT
	url="$SIGNOZ_URL/api/v1/dashboards/$dashboard_id"
else
	method=POST
	url="$SIGNOZ_URL/api/v1/dashboards"
fi
status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/dashboard-response.json" -w '%{http_code}' \
	-X "$method" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' --data @"$RUN_DIR/dashboard.json" "$url")
case "$status" in 200|201) ;; *) echo "dashboard seed failed: HTTP $status" >&2; exit 1 ;; esac
echo "dashboard ready: $name"
