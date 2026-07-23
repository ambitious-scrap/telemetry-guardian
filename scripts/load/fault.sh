#!/bin/sh

set -eu
. "$(dirname -- "$0")/../env/common.sh"

output=${1:-$RUN_DIR/fault-response.json}
status=$(curl --silent --show-error --max-time 10 -o "$output" -w '%{http_code}' \
	-H 'Content-Type: application/json' --data '{"cart_value":42,"fault":"payment-timeout"}' "$CHECKOUT_URL/checkout")
[ "$status" = 504 ] || { echo "fault injection returned HTTP $status" >&2; exit 1; }
jq -e '.error == "payment timeout"' "$output" >/dev/null
echo "payment-timeout fault injected"
