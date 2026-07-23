#!/bin/sh

set -eu
. "$(dirname -- "$0")/common.sh"

command -v foundryctl >/dev/null 2>&1 || {
	echo "foundryctl is required" >&2
	exit 1
}
command -v docker >/dev/null 2>&1 || {
	echo "docker is required" >&2
	exit 1
}

if [ ! -s "$RUN_DIR/tokenizer-secret" ]; then
	openssl rand -hex 32 >"$RUN_DIR/tokenizer-secret"
	chmod 600 "$RUN_DIR/tokenizer-secret"
fi
SIGNOZ_TOKENIZER_JWT_SECRET=$(sed -n '1p' "$RUN_DIR/tokenizer-secret")
export SIGNOZ_TOKENIZER_JWT_SECRET

cd "$ROOT/foundry"
foundryctl --no-ledger --no-updater -f casting.yaml -p pours cast
"$ROOT/scripts/env/wait-ready.sh"
