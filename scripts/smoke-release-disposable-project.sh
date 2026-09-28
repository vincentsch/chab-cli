#!/bin/sh
# Disposable-project live smoke (user-invoked, double opt-in). Creates, updates,
# and deletes ONE clearly named throwaway project against a live API.
#   Required: CHAB_LIVE_DISPOSABLE_PROJECT=1 plus CHAB_LIVE_SMOKE=1,
#             CHAB_LIVE_BASE_URL, CHAB_LIVE_API_KEY
#   Optional: CHAB_LIVE_DISPOSABLE_PROJECT_PREFIX (default chab-smoke-disposable),
#             CHAB_BIN (default chab)
set -eu

guard_msg="disposable-project smoke is opt-in: set CHAB_LIVE_DISPOSABLE_PROJECT=1 with CHAB_LIVE_SMOKE=1, CHAB_LIVE_BASE_URL, and CHAB_LIVE_API_KEY"
if [ "${CHAB_LIVE_DISPOSABLE_PROJECT:-}" != "1" ]; then
	echo "$guard_msg" >&2
	exit 1
fi
if [ "${CHAB_LIVE_SMOKE:-}" != "1" ]; then
	echo "$guard_msg" >&2
	exit 1
fi
: "${CHAB_LIVE_BASE_URL:?set CHAB_LIVE_BASE_URL to the product base URL}"
: "${CHAB_LIVE_API_KEY:?set CHAB_LIVE_API_KEY to a team API key from the web UI}"
CHAB_BIN="${CHAB_BIN:-chab}"
prefix="${CHAB_LIVE_DISPOSABLE_PROJECT_PREFIX:-chab-smoke-disposable}"

# Map CHAB_LIVE_* per invocation; never export globally; never enable tracing.
run_chab() {
	CHAB_BASE_URL="$CHAB_LIVE_BASE_URL" CHAB_API_KEY="$CHAB_LIVE_API_KEY" \
		"$CHAB_BIN" "$@"
}

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/chab-disposable-smoke.XXXXXX")
project_id=""
deleted=0
cleanup() {
	# Best-effort cleanup: if the explicit delete did not run, try to remove
	# the project so a stray project is not left behind. Log the outcome without
	# hiding earlier failures.
	if [ -n "$project_id" ] && [ "$deleted" -ne 1 ]; then
		if run_chab project delete "$project_id" --yes --no-prompt >/dev/null 2>&1; then
			echo "cleanup: deleted leftover project $project_id" >&2
		else
			echo "cleanup: WARNING could not delete project $project_id; remove it manually" >&2
		fi
	fi
	rm -rf "$tmpdir"
}
trap cleanup EXIT HUP INT TERM

CHAB_CONFIG="$tmpdir/config.yml"
CHAB_AUTH_FILE="$tmpdir/auth.json"
export CHAB_CONFIG CHAB_AUTH_FILE
unset CHAB_API_BASE_URL

name="$prefix-$(date -u +%Y%m%dT%H%M%SZ)-$$"
echo "creating disposable project: $name" >&2
project_id=$(run_chab project create --name "$name" --template '{{.id}}')
[ -n "$project_id" ] || { echo "create did not return a project id" >&2; exit 1; }
echo "created disposable project id: $project_id" >&2

run_chab project update "$project_id" --status paused --json
run_chab project show "$project_id" --json
run_chab project delete "$project_id" --yes --no-prompt --json
deleted=1
echo "disposable-project smoke passed; project $project_id created, updated, and deleted" >&2
