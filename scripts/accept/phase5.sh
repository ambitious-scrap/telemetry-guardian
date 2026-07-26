#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RUN_DIR=$(mktemp -d "${TMPDIR:-/tmp}/telemetry-guardian-phase5.XXXXXX")
trap 'rm -rf "$RUN_DIR"' EXIT INT TERM
cd "$ROOT"
required_files='.github/workflows/guardian.yml
internal/report/report.go
internal/report/report_test.go
scripts/ci/classify.sh
scripts/ci/guardian.sh
scripts/accept/phase5.sh'
printf '%s\n' "$required_files" | while IFS= read -r file; do
	test -f "$file" || { echo "missing Phase 5 file: $file" >&2; exit 1; }
done
test -z "$(gofmt -l cmd internal/contracts internal/evidence internal/miner internal/report internal/signoz internal/verifier)"
GOCACHE=${GOCACHE:-/private/tmp/telemetry-guardian-gocache} go test ./... -count=1
sh -n scripts/accept/phase5.sh scripts/ci/classify.sh scripts/ci/guardian.sh scripts/env/*.sh scripts/load/*.sh scripts/seed/*.sh
ruby -e 'require "yaml"; YAML.load_file(".github/workflows/guardian.yml")'
test "$(scripts/ci/classify.sh 0)" = PASS
test "$(scripts/ci/classify.sh 1)" = TELEMETRY_CONTRACT_VIOLATION
test "$(scripts/ci/classify.sh 2)" = VERIFICATION_INCONCLUSIVE
test "$(scripts/ci/classify.sh 3)" = INVALID_GUARDIAN_CONFIGURATION
guardian="$RUN_DIR/guardian"
GOCACHE=${GOCACHE:-/private/tmp/telemetry-guardian-gocache} go build -o "$guardian" ./cmd/guardian
contract="$ROOT/contracts/telemetry.guardian.yaml"
run_report() {
	label=$1
	fixture=$2
	out="$RUN_DIR/$label.html"
	md="$RUN_DIR/$label.md"
	"$guardian" report --contract "$contract" --verdict "$fixture" --output "$out" --markdown-output "$md" >/dev/null
	test -s "$out"
	test -s "$md"
	printf '%s\n' "$out"
}
healthy=$(run_report healthy internal/report/testdata/healthy.json)
broken=$(run_report broken internal/report/testdata/broken.json)
inconclusive=$(run_report inconclusive internal/report/testdata/inconclusive.json)
test "$(grep -o 'data-state="PASS"' "$healthy" | wc -l | tr -d ' ')" = 1
grep -F 'contract healthy' "$healthy" >/dev/null
grep -F 'TELEMETRY_CONTRACT_VIOLATION' "$broken" >/dev/null
grep -F 'dashboard-fixture-id' "$broken" >/dev/null
grep -F 'Cart value' "$broken" >/dev/null
grep -F 'telemetry-guardian-payment-timeout' "$broken" >/dev/null
grep -F 'BREAKS' "$broken" >/dev/null
grep -F 'REQUIRED_BY' "$broken" >/dev/null
grep -F 'PART_OF' "$broken" >/dev/null
grep -F 'VERIFICATION_INCONCLUSIVE' "$inconclusive" >/dev/null
if grep -F 'contract healthy' "$inconclusive" >/dev/null; then
	echo 'INCONCLUSIVE was presented as healthy' >&2
	exit 1
fi
if ! grep -F 'UNRESOLVED' "$inconclusive" >/dev/null; then
	echo 'INCONCLUSIVE graph omitted its neutral relationship' >&2
	exit 1
fi
cp "$broken" "$RUN_DIR/broken-again.html"
cmp "$broken" "$RUN_DIR/broken-again.html"
grep -F 'evidence-drawer' "$broken" >/dev/null
grep -F 'aria-controls="evidence-drawer"' "$broken" >/dev/null
grep -F 'data-open-consumer' "$broken" >/dev/null
grep -F 'role="button"' "$broken" >/dev/null
grep -F 'aria-expanded="false"' "$broken" >/dev/null
grep -F 'prefers-reduced-motion' "$broken" >/dev/null
grep -F ':focus-visible' "$broken" >/dev/null
if grep -E 'Math\.random|forceSimulation|Authorization:|Bearer[[:space:]]|/Users/|/home/' "$RUN_DIR"/*.html "$RUN_DIR"/*.md >/dev/null; then
	echo 'unsafe or nondeterministic content found in Phase 5 report' >&2
	exit 1
fi
for code in 0 1 2 3; do
	artifact="$RUN_DIR/artifacts-$code"
	set +e
	if [ "$code" = 3 ]; then
		GUARDIAN_ARTIFACT_DIR="$artifact" GUARDIAN_BIN="$guardian" GUARDIAN_CONTRACT="$RUN_DIR/missing-contract.yaml" GUARDIAN_FIXTURE_EXIT=3 scripts/ci/guardian.sh
	else
		case "$code" in
			0) fixture=healthy ;;
			1) fixture=broken ;;
			2) fixture=inconclusive ;;
		esac
		GUARDIAN_ARTIFACT_DIR="$artifact" GUARDIAN_BIN="$guardian" GUARDIAN_CONTRACT="$contract" GUARDIAN_FIXTURE_VERDICT="$ROOT/internal/report/testdata/$fixture.json" GUARDIAN_FIXTURE_EXIT="$code" scripts/ci/guardian.sh
	fi
	actual=$?
	set -e
	test "$actual" = "$code"
	test -s "$artifact/verdict.json"
	test -s "$artifact/summary.md"
	test "$(cat "$artifact/classification")" = "$(scripts/ci/classify.sh "$code")"
done

stale_artifacts="$RUN_DIR/stale-artifacts"
mkdir -p "$stale_artifacts"
printf '%s\n' stale >"$stale_artifacts/verdict.json"
printf '%s\n' stale >"$stale_artifacts/guardian-report.html"
printf '%s\n' stale >"$stale_artifacts/summary.md"
set +e
GUARDIAN_ARTIFACT_DIR="$stale_artifacts" GUARDIAN_BIN="$guardian" GUARDIAN_CONTRACT="$contract" GUARDIAN_FIXTURE_VERDICT="$ROOT/internal/report/testdata/healthy.json" GUARDIAN_FIXTURE_EXIT=0 scripts/ci/guardian.sh
actual=$?
set -e
test "$actual" = 0
if grep -F stale "$stale_artifacts/verdict.json" "$stale_artifacts/summary.md" "$stale_artifacts/guardian-report.html" >/dev/null 2>&1; then
	echo 'stale Guardian artifacts survived a current run' >&2
	exit 1
fi

missing_fixture_artifacts="$RUN_DIR/missing-fixture"
set +e
GUARDIAN_ARTIFACT_DIR="$missing_fixture_artifacts" GUARDIAN_BIN="$guardian" GUARDIAN_CONTRACT="$contract" GUARDIAN_FIXTURE_VERDICT="$RUN_DIR/does-not-exist.json" GUARDIAN_FIXTURE_EXIT=0 scripts/ci/guardian.sh
actual=$?
set -e
test "$actual" = 3
test "$(cat "$missing_fixture_artifacts/classification")" = INVALID_GUARDIAN_CONFIGURATION
grep -F 'INVALID_GUARDIAN_CONFIGURATION' "$missing_fixture_artifacts/verdict.json" >/dev/null

invalid_fixture="$RUN_DIR/invalid-verdict.json"
printf '%s\n' '{not-json' >"$invalid_fixture"
invalid_artifacts="$RUN_DIR/invalid-verdict"
set +e
GUARDIAN_ARTIFACT_DIR="$invalid_artifacts" GUARDIAN_BIN="$guardian" GUARDIAN_CONTRACT="$contract" GUARDIAN_FIXTURE_VERDICT="$invalid_fixture" GUARDIAN_FIXTURE_EXIT=0 scripts/ci/guardian.sh
actual=$?
set -e
test "$actual" = 3
test "$(cat "$invalid_artifacts/classification")" = INVALID_GUARDIAN_CONFIGURATION
grep -F 'INVALID_GUARDIAN_CONFIGURATION' "$invalid_artifacts/verdict.json" >/dev/null

failing_report_guardian="$RUN_DIR/failing-report-guardian"
printf '%s\n' '#!/bin/sh' 'exit 1' >"$failing_report_guardian"
chmod 700 "$failing_report_guardian"
report_failure_artifacts="$RUN_DIR/report-failure"
mkdir -p "$report_failure_artifacts"
printf '%s\n' stale >"$report_failure_artifacts/guardian-report.html"
set +e
GUARDIAN_ARTIFACT_DIR="$report_failure_artifacts" GUARDIAN_BIN="$failing_report_guardian" GUARDIAN_CONTRACT="$contract" GUARDIAN_FIXTURE_VERDICT="$ROOT/internal/report/testdata/healthy.json" GUARDIAN_FIXTURE_EXIT=0 scripts/ci/guardian.sh
actual=$?
set -e
test "$actual" = 2
test -s "$report_failure_artifacts/verdict.json"
test -s "$report_failure_artifacts/summary.md"
test ! -e "$report_failure_artifacts/guardian-report.html"

grep -F 'umask 077' scripts/seed/auth.sh >/dev/null
grep -F 'chmod 600 "$RUN_DIR/register-request.json"' scripts/seed/auth.sh >/dev/null
grep -F 'chmod 600 "$RUN_DIR/register-response.json"' scripts/seed/auth.sh >/dev/null
grep -F 'chmod 600 "$RUN_DIR/session-context.json"' scripts/seed/auth.sh >/dev/null
grep -F 'chmod 600 "$RUN_DIR/signoz-token"' scripts/seed/auth.sh >/dev/null
if grep -F 'string(message)' demo/checkout/main.go >/dev/null || grep -F 'io.ReadAll(io.LimitReader(response.Body' demo/checkout/main.go >/dev/null; then
	echo 'raw OTLP response body handling remains in checkout exporter' >&2
	exit 1
fi

if grep -RniE 'Bearer[[:space:]]+[A-Za-z0-9._~-]{20,}|accessToken|api[_-]?key[[:space:]]*=' internal/report scripts/ci internal/report/testdata >/dev/null; then
	echo 'secret-like value found in Phase 5 files' >&2
	exit 1
fi
git diff --check
echo 'Phase 5 acceptance passed'
