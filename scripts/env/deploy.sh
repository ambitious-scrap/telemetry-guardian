#!/bin/sh

set -eu
. "$(dirname -- "$0")/common.sh"

variant=${1:-}
run_id=${2:-}
case "$variant" in
	healthy|broken) ;;
	*) echo "usage: $0 healthy|broken RUN_ID" >&2; exit 2 ;;
esac
[ -n "$run_id" ] || { echo "RUN_ID is required" >&2; exit 2; }

stop_checkout
case "$(docker info --format '{{.Architecture}}')" in
	x86_64|amd64) goarch=amd64 ;;
	aarch64|arm64) goarch=arm64 ;;
	*) echo "unsupported Docker architecture" >&2; exit 1 ;;
esac
CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -o "$RUN_DIR/checkout-linux" ./demo/checkout
touch "$RUN_DIR/alert-events.jsonl"
chmod 600 "$RUN_DIR/alert-events.jsonl"

docker run --detach --name telemetry-guardian-checkout \
	--publish 127.0.0.1:19090:19090 \
	--volume "$RUN_DIR/checkout-linux:/checkout:ro" \
	--volume "$RUN_DIR:/run" \
	--env "RELEASE_VARIANT=$variant" \
	--env "RUN_ID=$run_id" \
	--env 'ALERT_EVENTS_FILE=/run/alert-events.jsonl' \
	--env 'OTEL_EXPORTER_OTLP_ENDPOINT=http://host.docker.internal:14318' \
	--env 'LISTEN_ADDR=0.0.0.0:19090' \
	--entrypoint /checkout \
	signoz/signoz:latest >/dev/null

deadline=$(( $(date +%s) + 30 ))
while [ "$(date +%s)" -lt "$deadline" ]; do
	if curl --silent --fail --max-time 2 "$CHECKOUT_URL/healthz" >/dev/null 2>&1; then
		echo "checkout $variant ready run=$run_id"
		exit 0
	fi
	if [ "$(docker inspect --format '{{.State.Running}}' telemetry-guardian-checkout 2>/dev/null || true)" != true ]; then
		echo "checkout exited before readiness" >&2
		docker logs --tail 40 telemetry-guardian-checkout >&2 || true
		exit 1
	fi
	sleep 1
done

echo "checkout readiness timed out after 30s" >&2
exit 1
