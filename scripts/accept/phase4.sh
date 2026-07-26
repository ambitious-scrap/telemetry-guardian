#!/bin/sh

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR="$ROOT/.run"
SIGNOZ_URL=${SIGNOZ_URL:-http://127.0.0.1:18080}
GOCACHE=${GOCACHE:-/private/tmp/telemetry-guardian-gocache}
STAMP=$(date -u +%Y%m%d%H%M%S)
cd "$ROOT"
. "$ROOT/scripts/env/common.sh"

required_files='cmd/guardian/main.go
internal/contracts/contracts.go
internal/evidence/evidence.go
internal/verifier/verifier.go
internal/verifier/live_test.go
scripts/accept/phase4.sh'
printf '%s\n' "$required_files" | while IFS= read -r file; do
	test -f "$file" || { echo "missing Phase 4 file: $file" >&2; exit 1; }
done

cleanup() {
	./scripts/env/down.sh >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

timestamp() {
	if date -u -r "$1" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null; then
		return
	fi
	date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'
}

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
			echo "timed out waiting for the deterministic alert bucket" >&2
			exit 1
		fi
		sleep 1
	done
}

assert_states() {
	file=$1
	expected=$2
	jq -e --argjson expected "$expected" '[.checks[].state] == $expected' "$file" >/dev/null
	jq -e 'all(.checks[];
		.state != "" and .requirement_id != "" and .run_id != "" and
		.evidence.retrieval != "" and .evidence.start != "" and .evidence.end != "" and
		(.evidence.sample_count >= 0) and (.evidence.minimum_sample_count > 0) and
		.evidence.summary != "" and .evidence.data_quality != "" and
		(.affected_consumers | length > 0)
	)' "$file" >/dev/null
}

run_verify() {
	label=$1
	expected_code=$2
	expected_states=$3
	i=1
	while [ "$i" -le 3 ]; do
		output="$RUN_DIR/verdict-$label-$i.json"
		set +e
		SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" SIGNOZ_ALERT_ID="$alert_id" \
			GUARDIAN_CONTRACT="$contract_path" GUARDIAN_VERDICT="$output" \
			GUARDIAN_RUN_ID="$run_id" GUARDIAN_START="$start" GUARDIAN_END="$end" \
			GUARDIAN_FAULT_INJECTED_AT="$fault_at" \
			"$RUN_DIR/guardian" verify --poll-interval 100ms
		code=$?
		set -e
		test "$code" = "$expected_code" || {
			jq -c '[.checks[] | {requirement_id, state, summary:.evidence.summary, data_quality:.evidence.data_quality, sample_count:.evidence.sample_count}]' "$output" >&2
			echo "$label verification run $i exited $code, expected $expected_code" >&2
			exit 1
		}
		assert_states "$output" "$expected_states"
		if grep -F "$token" "$output" >/dev/null; then
			echo "credential found in $output" >&2
			exit 1
		fi
		if grep -F "$warmup_run_id" "$output" >/dev/null; then
			echo "schema warmup leaked into candidate evidence in $output" >&2
			exit 1
		fi
		i=$((i + 1))
	done
	cp "$RUN_DIR/verdict-$label-3.json" "$RUN_DIR/verdict-$label.json"
}

mine_contract() {
	run_id=$1
	./scripts/seed/dashboard.sh "$run_id"
	./scripts/seed/alert.sh "$run_id"
	./scripts/seed/verify.sh
	dashboard_id=$(jq -er '.data[]? | select(.data.title == "telemetry-guardian-checkout") | .id' "$RUN_DIR/dashboards.json" | head -1)
	alert_id=$(jq -er '.data.id' "$RUN_DIR/alert-response.json")
	token=$(api_token)
	contract_path="$RUN_DIR/contract-$run_id.yaml"
	SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" \
		SIGNOZ_DASHBOARD_ID="$dashboard_id" SIGNOZ_ALERT_ID="$alert_id" \
		GUARDIAN_SERVICE=telemetry-guardian-checkout GUARDIAN_OUTPUT="$contract_path" \
		"$RUN_DIR/guardian" mine
}

run_live_test() {
	expected=$1
	SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" SIGNOZ_ALERT_ID="$alert_id" \
		GUARDIAN_CONTRACT="$contract_path" GUARDIAN_RUN_ID="$run_id" \
		GUARDIAN_START="$start" GUARDIAN_END="$end" \
		GUARDIAN_FAULT_INJECTED_AT="$fault_at" GUARDIAN_EXPECT="$expected" \
		GOCACHE="$GOCACHE" go test ./internal/verifier -run TestLiveVerifier -count=1
}

test -z "$(gofmt -l cmd internal/contracts internal/evidence internal/miner internal/signoz internal/verifier)"
GOCACHE="$GOCACHE" go test ./... -count=1
GOCACHE="$GOCACHE" go test ./internal/verifier -run TestCanonicalHealthyBrokenAndNoLoad -count=3
GOCACHE="$GOCACHE" go build -o "$RUN_DIR/guardian" ./cmd/guardian

./scripts/env/up.sh
./scripts/env/wait-ready.sh

# Warm SigNoz trace-field discovery outside every candidate run before creating its alert.
warmup_run_id="phase4-schema-warmup-$STAMP"
./scripts/env/deploy.sh healthy "$warmup_run_id"
./scripts/load/fault.sh "$RUN_DIR/fault-schema-warmup.json"
./scripts/load/assert-telemetry.sh healthy "$warmup_run_id"

run_id="phase4-healthy-$STAMP"
mine_contract "$run_id"
start=$(timestamp "$(date +%s)")
./scripts/env/deploy.sh healthy "$run_id"
./scripts/load/generate.sh 5
wait_for_alert_bucket
fault_epoch=$(date +%s)
fault_second=$((fault_epoch % 60))
test "$fault_second" -ge 45 && test "$fault_second" -le 52
fault_at=$(timestamp "$fault_epoch")
./scripts/load/fault.sh "$RUN_DIR/fault-healthy.json"
./scripts/load/assert-telemetry.sh healthy "$run_id"
end=$(timestamp $((fault_epoch + 60)))
run_verify healthy 0 '["PASS","PASS","PASS","PASS"]'
run_live_test healthy

run_id="phase4-broken-$STAMP"
mine_contract "$run_id"
start=$(timestamp "$(date +%s)")
./scripts/env/deploy.sh broken "$run_id"
./scripts/load/generate.sh 5
wait_for_alert_bucket
fault_epoch=$(date +%s)
fault_second=$((fault_epoch % 60))
test "$fault_second" -ge 45 && test "$fault_second" -le 52
fault_at=$(timestamp "$fault_epoch")
./scripts/load/fault.sh "$RUN_DIR/fault-broken.json"
./scripts/load/assert-telemetry.sh broken "$run_id"
deadline=$((fault_epoch + 61))
while [ "$(date +%s)" -lt "$deadline" ]; do
	sleep 2
done
end=$(timestamp "$(date +%s)")
run_verify broken 1 '["FAIL","FAIL","PASS","FAIL"]'
run_live_test broken

run_id="phase4-no-load-$STAMP"
mine_contract "$run_id"
now=$(date +%s)
start=$(timestamp $((now - 2)))
fault_at=$(timestamp $((now - 1)))
end=$(timestamp "$now")
run_verify no-load 2 '["INCONCLUSIVE","INCONCLUSIVE","INCONCLUSIVE","INCONCLUSIVE"]'
run_live_test no-load

printf 'apiVersion: invalid\n' >"$RUN_DIR/invalid-contract.yaml"
set +e
SIGNOZ_URL="$SIGNOZ_URL" SIGNOZ_TOKEN="$token" SIGNOZ_ALERT_ID="$alert_id" \
	GUARDIAN_CONTRACT="$RUN_DIR/invalid-contract.yaml" GUARDIAN_VERDICT="$RUN_DIR/invalid-verdict.json" \
	GUARDIAN_RUN_ID="$run_id" GUARDIAN_START="$start" GUARDIAN_END="$end" \
	GUARDIAN_FAULT_INJECTED_AT="$fault_at" "$RUN_DIR/guardian" verify
invalid_code=$?
set -e
test "$invalid_code" = 3 || { echo "invalid contract exited $invalid_code, expected 3" >&2; exit 1; }

test ! -e "$RUN_DIR/invalid-verdict.json"
if grep -RniE 'Bearer[[:space:]]+[A-Za-z0-9._-]{20,}' \
	internal/evidence internal/verifier scripts/accept/phase4.sh >/dev/null; then
	echo "secret-like value found in Phase 4 files" >&2
	exit 1
fi
git diff --check
echo "Phase 4 acceptance passed"
