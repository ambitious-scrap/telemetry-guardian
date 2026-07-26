#!/bin/sh

# Telemetry Guardian protected demo.
#
# Executes the judging narrative live against an isolated SigNoz environment:
# a healthy release passes, a functionally identical broken release fails with
# named consumers and a missed alert, and the repaired release passes again.
#
# Fault injection precedes each verification because the alert_must_fire check
# can only observe an alert that already had a fault to react to.

set -eu

MODE=full
case "${1:-}" in
	''|--full) ;;
	--smoke) MODE=smoke ;;
	*) echo "usage: $0 [--full|--smoke]" >&2; exit 2 ;;
esac

. "$(dirname -- "$0")/env/common.sh"
# common.sh derives ROOT for scripts two levels below the repository root.
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUN_DIR="$ROOT/.run"
mkdir -p "$RUN_DIR"
chmod 700 "$RUN_DIR"
cd "$ROOT"

DEMO_DIR="$RUN_DIR/demo"
PREV_DIR="$RUN_DIR/demo.prev"
STAMP=$(date -u +%Y%m%d%H%M%S)
QUERY_STEP_SECONDS=5
GOCACHE=${GOCACHE:-/private/tmp/telemetry-guardian-gocache}
export GOCACHE

# Preserve the previous run's evidence instead of mixing it into this one.
if [ -d "$DEMO_DIR" ]; then
	rm -rf "$PREV_DIR"
	mv "$DEMO_DIR" "$PREV_DIR"
fi
mkdir -p "$DEMO_DIR"
chmod 700 "$DEMO_DIR"

LOG="$DEMO_DIR/demo.log"
SUMMARY="$DEMO_DIR/summary.md"
STEP_OUT="$DEMO_DIR/.step-output"
SECRETS="$RUN_DIR/demo-tokens"
GUARDIAN="$RUN_DIR/guardian"
: >"$LOG"
: >"$SECRETS"
chmod 600 "$SECRETS"

log() { printf '%s\n' "$*" >>"$LOG"; }
say() { printf '%s\n' "$*"; log "$*"; }
note() { printf -- '- %s\n' "$*" >>"$SUMMARY"; log "  $*"; }

stage() {
	stage_no=$1
	shift
	say ""
	say "== STAGE $stage_no/22 · $*"
}

fail() {
	say ""
	say "DEMO FAILED at stage ${stage_no:-00}: $*"
	say "evidence preserved in $DEMO_DIR"
	exit 1
}

run_step() {
	set +e
	"$@" >"$STEP_OUT" 2>&1
	step_code=$?
	set -e
	cat "$STEP_OUT" >>"$LOG"
	if [ "$MODE" = full ] || [ "$step_code" != 0 ]; then
		cat "$STEP_OUT"
	fi
	return "$step_code"
}

cleanup() {
	exit_code=$?
	trap - EXIT INT TERM HUP
	rm -f "$STEP_OUT"
	./scripts/env/down.sh >>"$LOG" 2>&1 || true
	exit "$exit_code"
}
trap cleanup EXIT INT TERM HUP

timestamp() {
	if date -u -r "$1" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null; then
		return
	fi
	date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'
}

align_query_start() {
	epoch=$1
	printf '%s\n' "$((epoch - (epoch % QUERY_STEP_SECONDS)))"
}

assert_query_bucket_alignment() {
	test_epoch=1700000000
	offset=0
	while [ "$offset" -lt "$QUERY_STEP_SECONDS" ]; do
		candidate_epoch=$((test_epoch + offset))
		aligned_epoch=$(align_query_start "$candidate_epoch")
		expected_epoch=$((candidate_epoch - (candidate_epoch % QUERY_STEP_SECONDS)))
		[ "$aligned_epoch" -eq "$expected_epoch" ] || return 1
		[ "$aligned_epoch" -le "$candidate_epoch" ] || return 1
		adjustment=$((candidate_epoch - aligned_epoch))
		[ "$adjustment" -ge 0 ] && [ "$adjustment" -lt "$QUERY_STEP_SECONDS" ] || return 1
		offset=$((offset + 1))
	done
}

assert_query_bucket_alignment

wait_for_alert_bucket() {
	deadline=$(($(date +%s) + 65))
	while :; do
		now=$(date +%s)
		second=$((now % 60))
		# Live SigNoz trace-alert queries expose a fault after the current minute bucket closes.
		if [ "$second" -ge 45 ] && [ "$second" -le 50 ]; then
			return
		fi
		if [ "$now" -ge "$deadline" ]; then
			fail "timed out waiting for the deterministic alert bucket"
		fi
		sleep 1
	done
}

functional_tests() {
	label=$1
	run_step env GOCACHE="$GOCACHE" go test ./demo/checkout -count=1 ||
		fail "checkout functional tests failed for the $label release"
	cp "$STEP_OUT" "$DEMO_DIR/functional-tests-$label.log"
	status=$(curl --silent --show-error --max-time 10 -o "$DEMO_DIR/functional-$label.json" -w '%{http_code}' \
		-H 'Content-Type: application/json' --data '{"cart_value":42}' "$CHECKOUT_URL/checkout")
	[ "$status" = 200 ] || fail "functional checkout probe for $label returned HTTP $status"
	printf '%s\n' "$status" >"$DEMO_DIR/functional-$label.status"
}

seed_and_mine() {
	label=$1
	run_step ./scripts/seed/dashboard.sh "$run_id" || fail "dashboard seed failed for $label"
	run_step ./scripts/seed/alert.sh "$run_id" || fail "alert seed failed for $label"
	run_step ./scripts/seed/verify.sh || fail "dashboard and alert are not both present for $label"
	dashboard_id=$(jq -er '.data[]? | select(.data.title == "telemetry-guardian-checkout") | .id' "$RUN_DIR/dashboards.json" | head -1)
	alert_id=$(jq -er '.data.id' "$RUN_DIR/alert-response.json")
	token=$(api_token)
	printf '%s\n' "$token" >>"$SECRETS"
	chmod 600 "$SECRETS"
	contract_path="$DEMO_DIR/contract-$label.yaml"
	run_step env SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" \
		SIGNOZ_DASHBOARD_ID="$dashboard_id" SIGNOZ_ALERT_ID="$alert_id" \
		GUARDIAN_SERVICE=telemetry-guardian-checkout GUARDIAN_OUTPUT="$contract_path" \
		"$GUARDIAN" mine || fail "contract mining failed for $label"
	[ -s "$contract_path" ] || fail "mined contract is empty for $label"
}

reset_alert_events() {
	label=$1
	if [ -s "$RUN_DIR/alert-events.jsonl" ]; then
		cp "$RUN_DIR/alert-events.jsonl" "$DEMO_DIR/alert-events-before-$label.jsonl"
	fi
	: >"$RUN_DIR/alert-events.jsonl"
	chmod 600 "$RUN_DIR/alert-events.jsonl"
}

inject_fault() {
	label=$1
	telemetry_variant=$2
	reset_alert_events "$label"
	now_epoch=$(date +%s)
	start_epoch=$((now_epoch - (now_epoch % QUERY_STEP_SECONDS)))
	start=$(timestamp "$start_epoch")
	run_step ./scripts/load/generate.sh 5 || fail "workload generation failed for $label"
	wait_for_alert_bucket
	fault_epoch=$(date +%s)
	fault_second=$((fault_epoch % 60))
	if [ "$fault_second" -lt 45 ] || [ "$fault_second" -gt 52 ]; then
		fail "fault injection missed the deterministic minute bucket"
	fi
	fault_at=$(timestamp "$fault_epoch")
	run_step ./scripts/load/fault.sh "$DEMO_DIR/fault-$label.json" || fail "fault injection failed for $label"
	run_step ./scripts/load/assert-telemetry.sh "$telemetry_variant" "$run_id" ||
		fail "expected $telemetry_variant telemetry was not observed for $label"
}

verify_release() {
	label=$1
	expected_code=$2
	verdict="$DEMO_DIR/verdict-$label.json"
	set +e
	env SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" SIGNOZ_ALERT_ID="$alert_id" \
		GUARDIAN_CONTRACT="$contract_path" GUARDIAN_VERDICT="$verdict" \
		GUARDIAN_RUN_ID="$run_id" GUARDIAN_START="$start" GUARDIAN_END="$end" \
		GUARDIAN_FAULT_INJECTED_AT="$fault_at" \
		"$GUARDIAN" verify --poll-interval 100ms >"$STEP_OUT" 2>&1
	verify_code=$?
	set -e
	cat "$STEP_OUT" >>"$LOG"
	if [ "$MODE" = full ] || [ "$verify_code" != "$expected_code" ]; then
		cat "$STEP_OUT"
	fi
	if [ "$verify_code" != "$expected_code" ]; then
		jq -c '[.checks[] | {requirement_id, state, summary: .evidence.summary}]' "$verdict" >&2 2>/dev/null || true
		fail "$label verification exited $verify_code, expected $expected_code"
	fi
	[ -s "$verdict" ] || fail "$label verification produced no verdict"
	jq -e --arg run "$run_id" '.run_id == $run and all(.checks[]; .run_id == $run)' "$verdict" >/dev/null ||
		fail "$label verdict does not carry its own run ID"
	jq -e 'all(.checks[];
		.state != "" and .requirement_id != "" and
		.evidence.retrieval != "" and .evidence.summary != "" and
		(.evidence.minimum_sample_count > 0) and (.affected_consumers | length > 0))' "$verdict" >/dev/null ||
		fail "$label verdict is missing evidence or affected consumers"
	for stale in $stale_run_ids; do
		if grep -F "$stale" "$verdict" >/dev/null 2>&1; then
			fail "stale run ID $stale leaked into the $label verdict"
		fi
	done
	stale_run_ids="$stale_run_ids $run_id"
}

assert_pass_four() {
	label=$1
	jq -e '.overall_state == "PASS" and (.checks | length) == 4 and
		([.checks[] | select(.state == "PASS")] | length) == 4' "$DEMO_DIR/verdict-$label.json" >/dev/null ||
		fail "$label release did not produce PASS x 4"
}

# ---------------------------------------------------------------------------

printf '# Telemetry Guardian demo %s\n\n' "$STAMP" >"$SUMMARY"
say "Telemetry Guardian protected demo (mode=$MODE, stamp=$STAMP)"
say "artifacts: $DEMO_DIR"

stage 01 "Start the isolated environment and prove it ready"
for tool in docker foundryctl go jq curl openssl; do
	command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
done
run_step ./scripts/env/down.sh || fail "could not clean previous Telemetry Guardian runtime resources"
run_step env GOCACHE="$GOCACHE" go build -o "$GUARDIAN" ./cmd/guardian || fail "guardian build failed"
run_step ./scripts/env/up.sh || fail "isolated environment did not start"
run_step ./scripts/env/wait-ready.sh || fail "isolated environment did not become ready"
# Cold SigNoz discovers trace fields lazily; warm it outside every candidate run.
warmup_run_id="demo-warmup-$STAMP"
stale_run_ids="$warmup_run_id"
run_step ./scripts/env/deploy.sh healthy "$warmup_run_id" || fail "schema warmup deployment failed"
run_step ./scripts/load/fault.sh "$RUN_DIR/fault-warmup.json" || fail "schema warmup fault failed"
run_step ./scripts/load/assert-telemetry.sh healthy "$warmup_run_id" || fail "schema warmup telemetry absent"
note "Environment: isolated SigNoz ready, trace schema warmed"

stage 02 "Deploy the healthy release"
run_id="demo-healthy-$STAMP"
run_step ./scripts/env/deploy.sh healthy "$run_id" || fail "healthy deployment failed"
note "Healthy release deployed as run $run_id"

stage 03 "Run the functional tests against the healthy release"
functional_tests healthy
note "Functional tests: PASS (healthy)"

stage 04 "Mine the consumer contract from the live dashboard and alert"
seed_and_mine healthy
note "Contract mined: $(basename "$contract_path")"

stage 05 "Inject the deterministic payment timeout"
inject_fault healthy healthy
note "Payment timeout injected at $fault_at (healthy)"

stage 06 "Verify the healthy release — expect PASS x 4"
end=$(timestamp $((fault_epoch + 60)))
verify_release healthy 0
assert_pass_four healthy
say "healthy verdict: PASS x 4"
note "Healthy verdict: PASS x 4"

stage 07 "Prove the healthy alert fired"
run_step ./scripts/load/wait-alert.sh firing 120 || fail "the healthy release did not fire the payment-timeout alert"
cp "$RUN_DIR/alert-events.jsonl" "$DEMO_DIR/alert-healthy.jsonl"
note "Healthy alert: FIRED"

stage 08 "Deploy the broken release"
run_id="demo-broken-$STAMP"
run_step ./scripts/env/deploy.sh broken "$run_id" || fail "broken deployment failed"
note "Broken release deployed as run $run_id"

stage 09 "Run the same functional tests against the broken release"
functional_tests broken
cmp "$DEMO_DIR/functional-healthy.json" "$DEMO_DIR/functional-broken.json" ||
	fail "broken release is not functionally equivalent to the healthy release"
note "Functional tests: PASS (broken), responses identical to healthy"

stage 10 "Mine the consumer contract for the broken candidate"
seed_and_mine broken
note "Contract mined: $(basename "$contract_path")"

stage 11 "Inject the same payment timeout"
inject_fault broken broken
cmp "$DEMO_DIR/fault-healthy.json" "$DEMO_DIR/fault-broken.json" ||
	fail "broken fault response differs from the healthy fault response"
note "Payment timeout injected at $fault_at (broken), response identical to healthy"

stage 12 "Verify the broken release — expect FAIL x 3 with payment.authorize intact"
deadline=$((fault_epoch + 61))
while [ "$(date +%s)" -lt "$deadline" ]; do
	sleep 2
done
end=$(timestamp "$(date +%s)")
verify_release broken 1
jq -e '.overall_state == "FAIL" and
	([.checks[] | select(.state == "FAIL")] | length) == 3 and
	([.checks[] | select(.state == "FAIL") | .requirement_id] | sort) ==
		["alert-must-fire-payment-timeout","required-field-cart-value","required-field-error-type"] and
	([.checks[] | select(.requirement_id == "required-operation-payment-authorize") | .state] == ["PASS"])
	' "$DEMO_DIR/verdict-broken.json" >/dev/null ||
	fail "broken release did not produce the exact expected verdict"
say "broken verdict: FAIL cart.value, FAIL error.type, PASS payment.authorize, FAIL payment-timeout"
note "Broken verdict: FAIL cart.value, FAIL error.type, PASS payment.authorize, FAIL payment-timeout"

stage 13 "Produce the consumer blast graph"
graph="$DEMO_DIR/blast-graph-broken.html"
run_step "$GUARDIAN" report --contract "$contract_path" --verdict "$DEMO_DIR/verdict-broken.json" \
	--output "$graph" --markdown-output "$DEMO_DIR/blast-graph-broken.md" || fail "blast graph rendering failed"
note "Blast graph: $(basename "$graph")"

stage 14 "Prove the graph names the affected dashboard panel and alert"
grep -F 'data-state="FAIL"' "$graph" >/dev/null || fail "blast graph does not report a failing state"
grep -F 'telemetry-guardian-payment-timeout' "$graph" >/dev/null || fail "blast graph does not name the affected alert"
grep -F 'Checkout cart value' "$graph" >/dev/null || fail "blast graph does not name the affected dashboard panel"
grep -F 'BREAKS' "$graph" >/dev/null || fail "blast graph does not label the broken dependency"
say "blast graph names: Checkout cart value (dashboard panel), telemetry-guardian-payment-timeout (alert)"
note "Blast graph consumers: Checkout cart value panel, telemetry-guardian-payment-timeout alert"

stage 15 "Prove the existing alert missed the injected fault"
run_step ./scripts/load/assert-alert-miss.sh 45 0 || fail "the broken release unexpectedly fired the alert"
cp "$RUN_DIR/alert-events.jsonl" "$DEMO_DIR/alert-broken.jsonl"
note "Broken alert: MISSED"

stage 16 "Deploy the repaired release"
run_id="demo-repaired-$STAMP"
run_step ./scripts/env/deploy.sh healthy "$run_id" || fail "repaired deployment failed"
note "Repaired release deployed as run $run_id"

stage 17 "Run the same functional tests against the repaired release"
functional_tests repaired
cmp "$DEMO_DIR/functional-healthy.json" "$DEMO_DIR/functional-repaired.json" ||
	fail "repaired release is not functionally equivalent to the healthy release"
note "Functional tests: PASS (repaired), responses identical to healthy"

stage 18 "Mine the consumer contract for the repaired candidate"
seed_and_mine repaired
note "Contract mined: $(basename "$contract_path")"

stage 19 "Inject the payment timeout again"
inject_fault repaired healthy
note "Payment timeout injected at $fault_at (repaired)"

stage 20 "Verify the repaired release — expect PASS x 4"
end=$(timestamp $((fault_epoch + 60)))
verify_release repaired 0
assert_pass_four repaired
say "repaired verdict: PASS x 4"
note "Repaired verdict: PASS x 4"

stage 21 "Prove the repaired alert fired"
run_step ./scripts/load/wait-alert.sh firing 120 || fail "the repaired release did not fire the payment-timeout alert"
cp "$RUN_DIR/alert-events.jsonl" "$DEMO_DIR/alert-repaired.jsonl"
note "Repaired alert: FIRED"

stage 22 "Preserve artifacts and summarise"
for artifact in contract-healthy.yaml contract-broken.yaml contract-repaired.yaml \
	verdict-healthy.json verdict-broken.json verdict-repaired.json \
	blast-graph-broken.html blast-graph-broken.md \
	alert-healthy.jsonl alert-repaired.jsonl \
	fault-healthy.json fault-broken.json fault-repaired.json \
	functional-healthy.json functional-broken.json functional-repaired.json \
	functional-tests-healthy.log functional-tests-broken.log functional-tests-repaired.log \
	demo.log summary.md; do
	[ -s "$DEMO_DIR/$artifact" ] || fail "expected artifact is missing or empty: $artifact"
done
# The broken alert evidence is legitimately empty: nothing ever fired.
[ -f "$DEMO_DIR/alert-broken.jsonl" ] || fail "expected artifact is missing: alert-broken.jsonl"
healthy_run=$(jq -r '.run_id' "$DEMO_DIR/verdict-healthy.json")
broken_run=$(jq -r '.run_id' "$DEMO_DIR/verdict-broken.json")
repaired_run=$(jq -r '.run_id' "$DEMO_DIR/verdict-repaired.json")
if [ "$healthy_run" = "$broken_run" ] || [ "$healthy_run" = "$repaired_run" ] || [ "$broken_run" = "$repaired_run" ]; then
	fail "candidate verifications did not use unique run IDs"
fi
[ -s "$SECRETS" ] || fail "no SigNoz token was recorded for the artifact secret scan"
if grep -R -F -f "$SECRETS" "$DEMO_DIR" >/dev/null 2>&1; then
	fail "a SigNoz access token leaked into the preserved artifacts"
fi
if grep -R -E -n 'Bearer[[:space:]]+[A-Za-z0-9._~-]{20,}|accessToken' "$DEMO_DIR" >/dev/null 2>&1; then
	fail "a secret-like value leaked into the preserved artifacts"
fi

{
	printf '\n## Result\n\n'
	printf 'Healthy run `%s` PASS x 4, alert fired.\n' "$healthy_run"
	printf 'Broken run `%s` FAIL x 3 (cart.value, error.type, payment-timeout alert) with payment.authorize still passing, alert missed.\n' "$broken_run"
	printf 'Repaired run `%s` PASS x 4, alert fired.\n' "$repaired_run"
	printf '\nArtifacts: `%s`\n' "$DEMO_DIR"
} >>"$SUMMARY"

say ""
say "================================================================"
say "TELEMETRY GUARDIAN DEMO PASSED"
say "  healthy  $healthy_run   PASS x 4   alert FIRED"
say "  broken   $broken_run   FAIL x 3   alert MISSED   payment.authorize PASS"
say "  repaired $repaired_run   PASS x 4   alert FIRED"
say "  blast graph: file://$graph"
say "  artifacts:   $DEMO_DIR"
say "================================================================"
