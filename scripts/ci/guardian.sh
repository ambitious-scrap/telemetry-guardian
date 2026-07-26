#!/bin/sh

set -u
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
artifact_dir=${GUARDIAN_ARTIFACT_DIR:-"$ROOT/.run/guardian-artifacts"}
guardian_bin=${GUARDIAN_BIN:-"$ROOT/.run/guardian"}
contract=${GUARDIAN_CONTRACT:-"$ROOT/contracts/telemetry.guardian.yaml"}
verdict=${GUARDIAN_VERDICT:-"$artifact_dir/verdict.json"}
report=${GUARDIAN_REPORT:-"$artifact_dir/guardian-report.html"}
summary=${GUARDIAN_SUMMARY:-"$artifact_dir/summary.md"}
mkdir -p "$artifact_dir"

set +e
if [ -n "${GUARDIAN_FIXTURE_VERDICT:-}" ]; then
	cp "$GUARDIAN_FIXTURE_VERDICT" "$verdict"
	code=${GUARDIAN_FIXTURE_EXIT:-0}
else
	"$guardian_bin" verify --contract "$contract" --output "$verdict" >"$artifact_dir/guardian.log" 2>&1
	code=$?
fi
set -e

case "$code" in 0|1|2|3) ;; *) code=2 ;; esac
classification=$($ROOT/scripts/ci/classify.sh "$code")
printf '%s\n' "$code" >"$artifact_dir/exit-code"
printf '%s\n' "$classification" >"$artifact_dir/classification"

if [ ! -s "$verdict" ]; then
	printf '{"classification":"INVALID_GUARDIAN_CONFIGURATION","exit_code":3,"message":"Guardian configuration did not produce a verification verdict"}\n' >"$verdict"
fi

if [ "$code" != 3 ] && [ -s "$contract" ]; then
	set +e
	"$guardian_bin" report --contract "$contract" --verdict "$verdict" --output "$report" --markdown-output "$summary" >>"$artifact_dir/guardian.log" 2>&1
	report_code=$?
	set -e
	if [ "$report_code" != 0 ]; then
		printf '%s\n\nGuardian report could not be rendered from the verification artifact.\n' "$classification" >"$summary"
	fi
else
	printf '%s\n\nGuardian stopped before a valid verification report could be rendered.\n' "$classification" >"$summary"
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	cat "$summary" >>"$GITHUB_STEP_SUMMARY"
fi
exit "$code"
