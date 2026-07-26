#!/bin/sh

set -eu
umask 077
. "$(dirname -- "$0")/../env/common.sh"

email=telemetry-guardian@localhost.invalid
version_file="$RUN_DIR/version.json"
: >"$version_file"
chmod 600 "$version_file"
curl --silent --show-error --fail --max-time 10 "$SIGNOZ_URL/api/v1/version" >"$version_file"

if [ "$(jq -r '.setupCompleted' "$version_file")" = false ]; then
	password="$(openssl rand -hex 18)Aa1!"
	: >"$RUN_DIR/admin-password"
	chmod 600 "$RUN_DIR/admin-password"
	printf '%s\n' "$password" >"$RUN_DIR/admin-password"
	chmod 600 "$RUN_DIR/admin-password"
	: >"$RUN_DIR/register-request.json"
	chmod 600 "$RUN_DIR/register-request.json"
	jq -n --arg email "$email" --arg password "$password" '{name:"telemetry-guardian",orgId:"",orgName:"telemetry-guardian",email:$email,password:$password}' >"$RUN_DIR/register-request.json"
	chmod 600 "$RUN_DIR/register-request.json"
	: >"$RUN_DIR/register-response.json"
	chmod 600 "$RUN_DIR/register-response.json"
	status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/register-response.json" -w '%{http_code}' -H 'Content-Type: application/json' --data @"$RUN_DIR/register-request.json" "$SIGNOZ_URL/api/v1/register")
	chmod 600 "$RUN_DIR/register-response.json"
	[ "$status" = 200 ] || { echo "SigNoz registration failed: HTTP $status" >&2; exit 1; }
elif [ ! -s "$RUN_DIR/admin-password" ]; then
	echo "SigNoz is configured but Phase 1 runtime credentials are absent; run make env-down for a clean environment" >&2
	exit 1
fi

password=$(sed -n '1p' "$RUN_DIR/admin-password")
: >"$RUN_DIR/session-context.json"
chmod 600 "$RUN_DIR/session-context.json"
curl --silent --show-error --fail --max-time 10 --get \
	--data-urlencode "email=$email" --data-urlencode "ref=$SIGNOZ_URL" \
	"$SIGNOZ_URL/api/v2/sessions/context" >"$RUN_DIR/session-context.json"
chmod 600 "$RUN_DIR/session-context.json"
org_id=$(jq -er '.data.orgs[0].id' "$RUN_DIR/session-context.json")
: >"$RUN_DIR/login-request.json"
chmod 600 "$RUN_DIR/login-request.json"
jq -n --arg email "$email" --arg password "$password" --arg orgId "$org_id" \
	'{email:$email,password:$password,orgId:$orgId}' >"$RUN_DIR/login-request.json"
: >"$RUN_DIR/login-response.json"
chmod 600 "$RUN_DIR/login-response.json"
status=$(curl --silent --show-error --max-time 10 -o "$RUN_DIR/login-response.json" -w '%{http_code}' -H 'Content-Type: application/json' --data @"$RUN_DIR/login-request.json" "$SIGNOZ_URL/api/v2/sessions/email_password")
chmod 600 "$RUN_DIR/login-request.json" "$RUN_DIR/login-response.json"
[ "$status" = 200 ] || { echo "SigNoz login failed: HTTP $status" >&2; exit 1; }
: >"$RUN_DIR/signoz-token"
chmod 600 "$RUN_DIR/signoz-token"
jq -er '.data.accessToken' "$RUN_DIR/login-response.json" >"$RUN_DIR/signoz-token"
chmod 600 "$RUN_DIR/signoz-token"
echo "SigNoz runtime authentication ready"
