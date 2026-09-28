#!/bin/sh
# Live read-only release smoke (user-invoked, opt-in only). Setting ordinary
# CHAB_API_KEY/CHAB_BASE_URL can never trigger a live call.
#   Required: CHAB_LIVE_SMOKE=1, CHAB_LIVE_BASE_URL, CHAB_LIVE_API_KEY
#   Optional: CHAB_LIVE_PROJECT_ID (enables project show), CHAB_BIN (default chab)
set -eu

if [ "${CHAB_LIVE_SMOKE:-}" != "1" ]; then
	echo "live smoke is opt-in: set CHAB_LIVE_SMOKE=1, CHAB_LIVE_BASE_URL, and CHAB_LIVE_API_KEY" >&2
	exit 1
fi
: "${CHAB_LIVE_BASE_URL:?set CHAB_LIVE_BASE_URL to the product base URL}"
: "${CHAB_LIVE_API_KEY:?set CHAB_LIVE_API_KEY to a team API key from the web UI}"
CHAB_BIN="${CHAB_BIN:-chab}"

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/chab-live-smoke.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM
CHAB_CONFIG="$tmpdir/config.yml"
CHAB_AUTH_FILE="$tmpdir/auth.json"
export CHAB_CONFIG CHAB_AUTH_FILE
unset CHAB_API_BASE_URL

# Never enable shell tracing: traced lines would echo the key into scrollback
# and CI logs. Map CHAB_LIVE_* onto ordinary CHAB_* per invocation, not globally.
run_chab() {
	CHAB_BASE_URL="$CHAB_LIVE_BASE_URL" CHAB_API_KEY="$CHAB_LIVE_API_KEY" \
		"$CHAB_BIN" "$@"
}

run_chab auth status --json
run_chab whoami --json
run_chab doctor --json
run_chab project list --json
if [ -n "${CHAB_LIVE_PROJECT_ID:-}" ]; then
	run_chab project show "$CHAB_LIVE_PROJECT_ID" --json
fi
run_chab credits balance --json
run_chab credits transactions --json
run_chab api get /me --json

echo "live read-only release smoke passed" >&2
