#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

window=${1:-30}
baseline=${2:-}
case "$window" in ''|*[!0-9]*) echo "window must be an integer" >&2; exit 2 ;; esac
case "$baseline" in ''|*[!0-9]*) baseline=$(jq -s '[.[] | select(.status == "firing")] | length' "$RUN_DIR/alert-events.jsonl" 2>/dev/null || echo 0) ;; esac
deadline=$(( $(date +%s) + window ))
while [ "$(date +%s)" -lt "$deadline" ]; do
	after=$(jq -s '[.[] | select(.status == "firing")] | length' "$RUN_DIR/alert-events.jsonl" 2>/dev/null || echo 0)
	[ "$after" -eq "$baseline" ] || { echo "broken telemetry unexpectedly fired alert" >&2; exit 1; }
	sleep 2
done
echo "no firing notification during ${window}s observation window"
