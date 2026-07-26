#!/bin/sh

# Phase 6 acceptance: run the protected demo through its non-interactive smoke
# path and re-verify every protected outcome from the preserved artifacts.

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR="$ROOT/.run"
DEMO_DIR="$RUN_DIR/demo"
cd "$ROOT"

required_files='scripts/demo.sh
scripts/accept/phase6.sh
scripts/env/up.sh
scripts/env/down.sh
scripts/load/wait-alert.sh
scripts/load/assert-alert-miss.sh
cmd/guardian/main.go
README.md'
printf '%s\n' "$required_files" | while IFS= read -r file; do
	test -f "$file" || { echo "missing Phase 6 file: $file" >&2; exit 1; }
done

sh -n scripts/demo.sh scripts/accept/phase6.sh scripts/env/*.sh scripts/load/*.sh scripts/seed/*.sh

./scripts/demo.sh --smoke

log="$DEMO_DIR/demo.log"
summary="$DEMO_DIR/summary.md"
graph="$DEMO_DIR/blast-graph-broken.html"

artifacts='contract-healthy.yaml
contract-broken.yaml
contract-repaired.yaml
verdict-healthy.json
verdict-broken.json
verdict-repaired.json
blast-graph-broken.html
blast-graph-broken.md
alert-healthy.jsonl
alert-repaired.jsonl
functional-healthy.json
functional-broken.json
functional-repaired.json
functional-tests-healthy.log
functional-tests-broken.log
functional-tests-repaired.log
demo.log
summary.md'
printf '%s\n' "$artifacts" | while IFS= read -r artifact; do
	test -s "$DEMO_DIR/$artifact" || { echo "missing demo artifact: $artifact" >&2; exit 1; }
done
test -f "$DEMO_DIR/alert-broken.jsonl" || { echo "missing demo artifact: alert-broken.jsonl" >&2; exit 1; }

# Every canonical stage transition was announced and executed in order.
stage=1
while [ "$stage" -le 22 ]; do
	marker=$(printf '== STAGE %02d/22 ' "$stage")
	grep -F "$marker" "$log" >/dev/null || { echo "demo log is missing stage $stage" >&2; exit 1; }
	stage=$((stage + 1))
done

# Exact healthy verdict.
jq -e '.overall_state == "PASS" and (.checks | length) == 4 and
	([.checks[] | select(.state == "PASS")] | length) == 4' "$DEMO_DIR/verdict-healthy.json" >/dev/null ||
	{ echo "healthy verdict is not PASS x 4" >&2; exit 1; }

# Exact broken verdict.
jq -e '.overall_state == "FAIL" and
	([.checks[] | select(.state == "FAIL")] | length) == 3 and
	([.checks[] | select(.state == "FAIL") | .requirement_id] | sort) ==
		["alert-must-fire-payment-timeout","required-field-cart-value","required-field-error-type"] and
	([.checks[] | select(.requirement_id == "required-operation-payment-authorize") | .state] == ["PASS"])
	' "$DEMO_DIR/verdict-broken.json" >/dev/null ||
	{ echo "broken verdict is not the exact expected failure set" >&2; exit 1; }

# Exact repaired verdict.
jq -e '.overall_state == "PASS" and (.checks | length) == 4 and
	([.checks[] | select(.state == "PASS")] | length) == 4' "$DEMO_DIR/verdict-repaired.json" >/dev/null ||
	{ echo "repaired verdict is not PASS x 4" >&2; exit 1; }

# Functional equivalence across all three releases.
cmp "$DEMO_DIR/functional-healthy.json" "$DEMO_DIR/functional-broken.json"
cmp "$DEMO_DIR/functional-healthy.json" "$DEMO_DIR/functional-repaired.json"
cmp "$DEMO_DIR/fault-healthy.json" "$DEMO_DIR/fault-broken.json"
cmp "$DEMO_DIR/fault-healthy.json" "$DEMO_DIR/fault-repaired.json"

# Healthy alert fired, broken alert missed, repaired alert fired.
jq -s -e '[.[] | select(.status == "firing")] | length > 0' "$DEMO_DIR/alert-healthy.jsonl" >/dev/null ||
	{ echo "healthy alert did not fire" >&2; exit 1; }
if [ -s "$DEMO_DIR/alert-broken.jsonl" ] &&
	jq -s -e '[.[] | select(.status == "firing")] | length > 0' "$DEMO_DIR/alert-broken.jsonl" >/dev/null 2>&1; then
	echo "broken alert fired but must have missed" >&2
	exit 1
fi
jq -s -e '[.[] | select(.status == "firing")] | length > 0' "$DEMO_DIR/alert-repaired.jsonl" >/dev/null ||
	{ echo "repaired alert did not fire" >&2; exit 1; }

# Blast graph names the affected consumers.
grep -F 'data-state="FAIL"' "$graph" >/dev/null
grep -F 'telemetry-guardian-payment-timeout' "$graph" >/dev/null
grep -F 'Checkout cart value' "$graph" >/dev/null
grep -F 'BREAKS' "$graph" >/dev/null

# Unique run IDs and stale-state isolation.
healthy_run=$(jq -er '.run_id' "$DEMO_DIR/verdict-healthy.json")
broken_run=$(jq -er '.run_id' "$DEMO_DIR/verdict-broken.json")
repaired_run=$(jq -er '.run_id' "$DEMO_DIR/verdict-repaired.json")
test "$healthy_run" != "$broken_run"
test "$healthy_run" != "$repaired_run"
test "$broken_run" != "$repaired_run"
for verdict in healthy broken repaired; do
	run=$(jq -er '.run_id' "$DEMO_DIR/verdict-$verdict.json")
	jq -e --arg run "$run" 'all(.checks[]; .run_id == $run)' "$DEMO_DIR/verdict-$verdict.json" >/dev/null
	if grep -F -- '-warmup-' "$DEMO_DIR/verdict-$verdict.json" >/dev/null; then
		echo "schema warmup leaked into the $verdict verdict" >&2
		exit 1
	fi
done
for verdict in broken repaired; do
	if grep -F "$healthy_run" "$DEMO_DIR/verdict-$verdict.json" >/dev/null; then
		echo "healthy run ID leaked into the $verdict verdict" >&2
		exit 1
	fi
done
if grep -F "$broken_run" "$DEMO_DIR/verdict-repaired.json" >/dev/null; then
	echo "broken run ID leaked into the repaired verdict" >&2
	exit 1
fi

# Final success summary.
grep -F 'TELEMETRY GUARDIAN DEMO PASSED' "$log" >/dev/null
grep -F 'PASS x 4' "$summary" >/dev/null

# Targeted cleanup: no Telemetry Guardian runtime resource survived the demo.
test -z "$(docker ps -aq --filter name=telemetry-guardian)"
test -z "$(docker volume ls -q --filter name=telemetry-guardian)"
if docker network inspect telemetry-guardian-network >/dev/null 2>&1; then
	echo "Telemetry Guardian network survived the demo" >&2
	exit 1
fi

# Secret scan over the preserved artifacts. The Phase 6 scripts are excluded
# because they carry these patterns as scan literals themselves.
if grep -R -E -n 'Bearer[[:space:]]+[A-Za-z0-9._~-]{20,}|accessToken|api[_-]?key[[:space:]]*=' \
	"$DEMO_DIR" >/dev/null 2>&1; then
	echo "secret-like value found in the preserved demo artifacts" >&2
	exit 1
fi

git diff --check
echo 'Phase 6 acceptance passed'
