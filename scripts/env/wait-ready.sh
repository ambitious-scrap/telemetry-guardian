#!/bin/sh

set -eu
. "$(dirname -- "$0")/common.sh"

deadline=$(( $(date +%s) + 240 ))
ready_count=0
while [ "$(date +%s)" -lt "$deadline" ]; do
	if curl --silent --fail --max-time 3 "$SIGNOZ_URL/api/v1/health" >/dev/null 2>&1 &&
		docker inspect --format '{{.State.Health.Status}}' telemetry-guardian-signoz-0 2>/dev/null | grep -qx healthy &&
		curl --silent --fail --max-time 3 "$COLLECTOR_HEALTH_URL/" >/dev/null 2>&1 &&
		curl --silent --fail --max-time 3 -H 'Content-Type: application/json' \
			--data '{"resourceSpans":[]}' "$OTLP_URL/v1/traces" >/dev/null 2>&1; then
		ready_count=$((ready_count + 1))
		if [ "$ready_count" -ge 3 ]; then
			echo "SigNoz and OTLP ingester ready"
			exit 0
		fi
	else
		ready_count=0
	fi
	sleep 2
done

echo "environment readiness timed out after 240s" >&2
exit 1
