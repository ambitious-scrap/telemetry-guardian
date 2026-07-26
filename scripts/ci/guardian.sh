#!/bin/sh

set -u
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
artifact_dir=${GUARDIAN_ARTIFACT_DIR:-"$ROOT/.run/guardian-artifacts"}
guardian_bin=${GUARDIAN_BIN:-"$ROOT/.run/guardian"}
contract=${GUARDIAN_CONTRACT:-"$ROOT/contracts/telemetry.guardian.yaml"}
verdict=${GUARDIAN_VERDICT:-"$artifact_dir/verdict.json"}
report=${GUARDIAN_REPORT:-"$artifact_dir/guardian-report.html"}
summary=${GUARDIAN_SUMMARY:-"$artifact_dir/summary.md"}

if ! mkdir -p "$artifact_dir"; then
	echo "Guardian artifact directory is unavailable" >&2
	exit 3
fi

tmp_dir=$(mktemp -d "$artifact_dir/.guardian.XXXXXX") || {
	echo "Guardian artifact staging directory is unavailable" >&2
	exit 3
}
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

# Remove only the explicitly published outputs so a failed run cannot reuse a
# previous verdict, report, classification, or summary.
rm -f "$verdict" "$report" "$summary" \
	"$artifact_dir/exit-code" "$artifact_dir/classification" "$artifact_dir/guardian.log"
log="$artifact_dir/guardian.log"
: >"$log"

code=3
diagnostic="Guardian did not produce a valid current verification artifact"
source_verdict="$tmp_dir/verdict.json"

if [ -n "${GUARDIAN_FIXTURE_VERDICT:-}" ]; then
	if cp "$GUARDIAN_FIXTURE_VERDICT" "$source_verdict" 2>/dev/null; then
		code=${GUARDIAN_FIXTURE_EXIT:-0}
		case "$code" in
			0|1|2|3) ;;
			*)
				code=2
				diagnostic="Guardian fixture requested an unsupported exit classification"
				;;
		esac
	else
		code=3
		diagnostic="Guardian fixture verdict could not be copied"
		printf '%s\n' "$diagnostic" >"$log"
	fi
else
	"$guardian_bin" verify --contract "$contract" --output "$source_verdict" >"$log" 2>&1
	code=$?
	case "$code" in
		0|1|2|3) ;;
		*)
			code=2
			diagnostic="Guardian verification returned an unsupported exit classification"
			;;
	esac
fi

valid_verdict=false
if [ "$code" != 3 ] && [ -s "$source_verdict" ] && jq -e '
	(.overall_state == "PASS" or .overall_state == "FAIL" or .overall_state == "INCONCLUSIVE") and
	(.checks | type == "array" and length > 0) and
	(all(.checks[]; .state == "PASS" or .state == "FAIL" or .state == "INCONCLUSIVE"))
' "$source_verdict" >/dev/null 2>&1; then
	valid_verdict=true
fi

publish_file() {
	source=$1
	destination=$2
	parent=$(dirname -- "$destination")
	if [ "$parent" != "." ] && ! mkdir -p "$parent"; then
		return 1
	fi
	mv "$source" "$destination"
}

write_diagnostic() {
	message=$1
	printf '{"classification":"INVALID_GUARDIAN_CONFIGURATION","exit_code":3,"message":"%s"}\n' "$message" >"$tmp_dir/verdict.json"
	printf 'INVALID_GUARDIAN_CONFIGURATION\n\nGuardian could not produce a valid verification artifact.\n' >"$tmp_dir/summary.md"
}

if [ "$valid_verdict" = true ]; then
	case "$code" in
		0) expected_state=PASS; expected_classification=PASS ;;
		1) expected_state=FAIL; expected_classification=TELEMETRY_CONTRACT_VIOLATION ;;
		2) expected_state=INCONCLUSIVE; expected_classification=VERIFICATION_INCONCLUSIVE ;;
		*) expected_state=; expected_classification= ;;
	esac
	"$guardian_bin" report --contract "$contract" --verdict "$source_verdict" \
		--output "$tmp_dir/guardian-report.html" --markdown-output "$tmp_dir/summary.md" >>"$log" 2>&1
	report_code=$?
	if [ "$report_code" -eq 0 ] &&
		[ -s "$tmp_dir/guardian-report.html" ] &&
		[ -s "$tmp_dir/summary.md" ] &&
		grep -F "data-state=\"$expected_state\"" "$tmp_dir/guardian-report.html" >/dev/null 2>&1 &&
		grep -F "$expected_classification" "$tmp_dir/summary.md" >/dev/null 2>&1; then
		if ! publish_file "$source_verdict" "$verdict" || \
			! publish_file "$tmp_dir/guardian-report.html" "$report" || \
			! publish_file "$tmp_dir/summary.md" "$summary"; then
			code=3
			diagnostic="Guardian artifacts could not be published atomically"
		fi
	else
		if [ "$report_code" -eq 3 ]; then
			code=3
			diagnostic="Guardian report rejected the verification artifact"
		else
			code=2
			diagnostic="Guardian report could not be rendered"
		fi
		# Preserve the current verdict for a failed verification/report boundary,
		# but never leave a stale report or claim a healthy run.
		if ! publish_file "$source_verdict" "$verdict"; then
			code=3
			diagnostic="Guardian verdict could not be published"
		fi
		printf '%s\n\n%s.\n' "$diagnostic" "$diagnostic" >"$tmp_dir/summary.md"
		publish_file "$tmp_dir/summary.md" "$summary" || code=3
	fi
else
	code=3
	write_diagnostic "$diagnostic"
	publish_file "$tmp_dir/verdict.json" "$verdict" || true
	publish_file "$tmp_dir/summary.md" "$summary" || true
fi

classification=$($ROOT/scripts/ci/classify.sh "$code") || {
	code=3
	classification=INVALID_GUARDIAN_CONFIGURATION
}
printf '%s\n' "$code" >"$tmp_dir/exit-code"
printf '%s\n' "$classification" >"$tmp_dir/classification"
publish_file "$tmp_dir/exit-code" "$artifact_dir/exit-code" || code=3
publish_file "$tmp_dir/classification" "$artifact_dir/classification" || code=3

if [ -n "${GITHUB_STEP_SUMMARY:-}" ] && [ -s "$summary" ]; then
	cat "$summary" >>"$GITHUB_STEP_SUMMARY"
fi

exit "$code"
