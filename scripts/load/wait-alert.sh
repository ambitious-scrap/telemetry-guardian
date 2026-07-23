#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

state=${1:-}
timeout=${2:-90}
case "$state" in firing|resolved) ;; *) echo "usage: $0 firing|resolved [TIMEOUT_SECONDS]" >&2; exit 2 ;; esac
case "$timeout" in ''|*[!0-9]*) echo "timeout must be an integer" >&2; exit 2 ;; esac
deadline=$(( $(date +%s) + timeout ))
while [ "$(date +%s)" -lt "$deadline" ]; do
	if [ -s "$RUN_DIR/alert-events.jsonl" ] && jq -s -e --arg state "$state" '.[] | select(.status == $state)' "$RUN_DIR/alert-events.jsonl" >/dev/null 2>&1; then
		echo "alert notification observed: $state"
		exit 0
	fi
	sleep 2
done
echo "alert notification $state not observed within ${timeout}s" >&2
exit 1
