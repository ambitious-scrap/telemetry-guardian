#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

count=${1:-5}
case "$count" in ''|*[!0-9]*) echo "count must be an integer" >&2; exit 2 ;; esac
i=0
while [ "$i" -lt "$count" ]; do
	response=$(curl --silent --show-error --max-time 10 -o /dev/null -w '%{http_code}' \
		-H 'Content-Type: application/json' --data '{"cart_value":42}' "$CHECKOUT_URL/checkout")
	[ "$response" = 200 ] || { echo "workload request failed: HTTP $response" >&2; exit 1; }
	i=$((i + 1))
done
echo "generated $count checkout requests"
