#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

required_files='Telemetry_Guardian_PRODUCT_SPEC.md
Telemetry_Guardian_BUILD_PLAN.md
docs/ARCHITECTURE.md
docs/STATUS.md
AGENTS.md
CLAUDE.md
Makefile
scripts/accept/phase0.sh'

printf '%s\n' "$required_files" | while IFS= read -r file; do
	test -f "$file" || {
		echo "missing required file: $file" >&2
		exit 1
	}
done

test ! -e docs/PRODUCT_SPEC.md
test ! -e docs/PROJECT_SPEC.md
test ! -e docs/BUILD_PLAN.md

git diff --quiet main -- \
	Telemetry_Guardian_PRODUCT_SPEC.md \
	Telemetry_Guardian_BUILD_PLAN.md || {
	echo "authoritative documents changed" >&2
	exit 1
}

if grep -En 'docs/(PRODUCT_SPEC|PROJECT_SPEC|BUILD_PLAN)\.md' \
	AGENTS.md CLAUDE.md docs/ARCHITECTURE.md docs/STATUS.md; then
	echo "stale authority path found" >&2
	exit 1
fi

for heading in \
	'## Protected MVP' \
	'## Explicit non-goals' \
	'## Package and directory boundaries' \
	'## Minimum domain model' \
	'## External boundaries' \
	'## Interfaces requiring empirical validation' \
	'## Fixture-backed and live-integration boundary' \
	'## Result semantics' \
	'## Evidence requirements' \
	'## Timeout and stale-data isolation' \
	'## Future worktree ownership' \
	'## Critical assumptions' \
	'## Highest-risk demo failure modes' \
	'## Deferred decisions'; do
	grep -Fqx "$heading" docs/ARCHITECTURE.md || {
		echo "missing architecture section: $heading" >&2
		exit 1
	}
done

awk -F '|' '
/^\| [A-Z][A-Z]-[0-9][0-9] / {
	count++
	owner=$4
	gate=$5
	gsub(/^[[:space:]]+|[[:space:]]+$/, "", owner)
	gsub(/^[[:space:]]+|[[:space:]]+$/, "", gate)
	if (owner == "" || gate == "") bad=1
}
END { exit(count == 0 || bad) }
' docs/STATUS.md || {
	echo "critical unknown without owner or resolution gate" >&2
	exit 1
}

git diff --check
git diff --cached --check
echo "phase0 acceptance: PASS"
