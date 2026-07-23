#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

run_id=${1:-}
[ -n "$run_id" ] || { echo "usage: $0 RUN_ID" >&2; exit 2; }
token=$(api_token)
channel=telemetry-guardian-webhook
alert=telemetry-guardian-payment-timeout

curl --silent --show-error --fail --max-time 10 -H "Authorization: Bearer $token" \
	"$SIGNOZ_URL/api/v1/channels" >"$RUN_DIR/channels.json"
if ! jq -e --arg name "$channel" '.data[]? | select(.name == $name)' "$RUN_DIR/channels.json" >/dev/null; then
	jq -n --arg name "$channel" '{name:$name,webhook_configs:[{send_resolved:true,url:"http://host.docker.internal:19090/alerts",http_config:{}}]}' >"$RUN_DIR/channel.json"
	status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/channel-response.json" -w '%{http_code}' \
		-H "Authorization: Bearer $token" -H 'Content-Type: application/json' --data @"$RUN_DIR/channel.json" "$SIGNOZ_URL/api/v1/channels")
	case "$status" in 200|201) ;; *) echo "channel seed failed: HTTP $status" >&2; exit 1 ;; esac
fi

curl --silent --show-error --fail --max-time 10 -H "Authorization: Bearer $token" \
	"$SIGNOZ_URL/api/v2/rules" >"$RUN_DIR/rules.json"
rule_id=$(jq -r --arg name "$alert" '.data[]? | select(.alert == $name) | .id' "$RUN_DIR/rules.json" | head -1)
if [ -n "$rule_id" ]; then
	status=$(curl --silent --show-error --max-time 10 -o /dev/null -w '%{http_code}' -X DELETE \
		-H "Authorization: Bearer $token" "$SIGNOZ_URL/api/v2/rules/$rule_id")
	case "$status" in 200|204) ;; *) echo "existing alert removal failed: HTTP $status" >&2; exit 1 ;; esac
fi

sed "s/__RUN_ID__/$run_id/g" "$ROOT/fixtures/alerts/telemetry-guardian-payment-timeout.json" >"$RUN_DIR/alert.json"
status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/alert-response.json" -w '%{http_code}' \
	-H "Authorization: Bearer $token" -H 'Content-Type: application/json' --data @"$RUN_DIR/alert.json" "$SIGNOZ_URL/api/v2/rules")
case "$status" in 200|201) ;; *) echo "alert seed failed: HTTP $status" >&2; exit 1 ;; esac
echo "alert ready: $alert run=$run_id"
