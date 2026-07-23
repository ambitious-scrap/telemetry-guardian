#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

variant=${1:-}
run_id=${2:-}
case "$variant" in healthy|broken) ;; *) echo "usage: $0 healthy|broken RUN_ID" >&2; exit 2 ;; esac
[ -n "$run_id" ] || { echo "RUN_ID is required" >&2; exit 2; }
token=$(api_token)

query_count() {
	signal=$1
	filter=$2
	end=$(( $(date +%s) * 1000 ))
	start=$(( end - 300000 ))
	jq -n --arg signal "$signal" --arg filter "$filter" --argjson start "$start" --argjson end "$end" '{
		schemaVersion:"v1",start:$start,end:$end,requestType:"time_series",
		compositeQuery:{queries:[{type:"builder_query",spec:{name:"A",signal:$signal,stepInterval:5,disabled:false,filter:{expression:$filter},aggregations:[{expression:"count()"}]}}]},
		formatOptions:{formatTableResultForUI:false,fillGaps:false}
	}' >"$RUN_DIR/query.json"
	status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/query-response.json" -w '%{http_code}' \
		-H "Authorization: Bearer $token" -H 'Content-Type: application/json' --data @"$RUN_DIR/query.json" "$SIGNOZ_URL/api/v5/query_range")
	if [ "$status" = 400 ] && jq -e '.error.errors | length > 0 and all(.message | test("^key .* not found$"))' "$RUN_DIR/query-response.json" >/dev/null; then
		echo 0
		return 0
	fi
	[ "$status" = 200 ] || { echo "SigNoz $signal query failed: HTTP $status" >&2; return 1; }
	jq '[.. | objects | .value? | numbers | select(. > 0)] | length' "$RUN_DIR/query-response.json"
}

wait_positive() {
	signal=$1
	filter=$2
	label=$3
	deadline=$(( $(date +%s) + 60 ))
	while [ "$(date +%s)" -lt "$deadline" ]; do
		if count=$(query_count "$signal" "$filter") && [ "$count" -gt 0 ]; then
			echo "$label present"
			return 0
		fi
		sleep 2
	done
	echo "$label not observed within 60s" >&2
	return 1
}

common="service.name = 'telemetry-guardian-checkout' AND run.id = '$run_id'"
wait_positive traces "$common AND name = 'payment.authorize'" "payment.authorize trace"
if [ "$variant" = healthy ]; then
	wait_positive traces "$common AND cart.value = 42" "cart.value"
	wait_positive traces "$common AND error.type = 'payment_timeout'" "healthy error.type"
	wait_positive logs "$common AND error.type = 'payment_timeout'" "correlated healthy error log"
	forbidden_trace="$common AND (cart.amount = 42 OR error.kind = 'timeout')"
else
	wait_positive traces "$common AND cart.amount = 42" "cart.amount"
	wait_positive traces "$common AND error.kind = 'timeout'" "broken error.kind"
	wait_positive logs "$common AND error.kind = 'timeout'" "correlated broken error log"
	forbidden_trace="$common AND (cart.value = 42 OR error.type = 'payment_timeout')"
fi
[ "$(query_count traces "$forbidden_trace")" -eq 0 ] || { echo "unexpected telemetry field observed for $variant" >&2; exit 1; }

deadline=$(( $(date +%s) + 60 ))
while [ "$(date +%s)" -lt "$deadline" ]; do
	curl --silent --show-error --fail --max-time 10 --get -H "Authorization: Bearer $token" \
		--data-urlencode 'searchText=checkout.duration' --data-urlencode 'limit=10' "$SIGNOZ_URL/api/v2/metrics" >"$RUN_DIR/metrics.json"
	if jq -e '.. | strings | select(. == "checkout.duration")' "$RUN_DIR/metrics.json" >/dev/null; then
		echo "checkout.duration metric present"
		exit 0
	fi
	sleep 2
done
echo "checkout.duration metric not observed within 60s" >&2
exit 1
