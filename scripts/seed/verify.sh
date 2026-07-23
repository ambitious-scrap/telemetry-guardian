#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

token=$(api_token)
curl --silent --show-error --fail --max-time 10 -H "Authorization: Bearer $token" "$SIGNOZ_URL/api/v1/dashboards" >"$RUN_DIR/dashboards.json"
curl --silent --show-error --fail --max-time 10 -H "Authorization: Bearer $token" "$SIGNOZ_URL/api/v2/rules" >"$RUN_DIR/rules.json"
jq -e '.data[]? | select(.data.title == "telemetry-guardian-checkout")' "$RUN_DIR/dashboards.json" >/dev/null
jq -e '.data[]? | select(.alert == "telemetry-guardian-payment-timeout")' "$RUN_DIR/rules.json" >/dev/null
echo "dashboard and alert exist"
