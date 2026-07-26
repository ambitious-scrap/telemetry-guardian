#!/bin/sh

set -eu
code=${1:-}
case "$code" in
	0) echo PASS ;;
	1) echo TELEMETRY_CONTRACT_VIOLATION ;;
	2) echo VERIFICATION_INCONCLUSIVE ;;
	3) echo INVALID_GUARDIAN_CONFIGURATION ;;
	*) echo "unknown Guardian exit code" >&2; exit 2 ;;
esac
