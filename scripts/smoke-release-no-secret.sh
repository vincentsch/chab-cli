#!/bin/sh
# No-secret release smoke: validate a built or installed chab binary offline.
# Runs the artifact-verification subset with isolated state and no credentials.
#
# Usage:
#   scripts/smoke-release-no-secret.sh --bin <path-to-chab>
#   scripts/smoke-release-no-secret.sh --list-checks
set -eu

usage() {
	echo "Usage: scripts/smoke-release-no-secret.sh --bin <path-to-chab> | --list-checks"
}

# The fixed no-auth release-verification subset, one canonical token per line.
# Multi-token commands are single lines that contain spaces. Keep this list in sync
# with the catalog assertions in internal/releasecheck.
list_checks() {
	printf '%s\n' 'root' 'help' 'version' 'config' 'credits' 'doctor' 'health' 'errors' 'mcp' 'completion' 'config path' 'auth env'
}

BIN=""
mode="run"
while [ "$#" -gt 0 ]; do
	case "$1" in
		--bin)
			shift
			[ "$#" -gt 0 ] || { echo "error: --bin requires a path" >&2; usage >&2; exit 2; }
			BIN=$1
			;;
		--list-checks) mode="list" ;;
		--help) usage; exit 0 ;;
		*) echo "error: unknown argument: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

if [ "$mode" = "list" ]; then
	list_checks
	exit 0
fi

[ -n "$BIN" ] || { echo "error: --bin <path-to-chab> is required" >&2; usage >&2; exit 2; }
[ -x "$BIN" ] || { echo "error: not an executable binary: $BIN" >&2; exit 1; }

# Drop inherited auth/config so host state cannot change offline output.
# clear-chab-env (keep in sync with internal/testutil.ViltEnvNames):
for name in CHAB_API_KEY CHAB_PROFILE CHAB_CONFIG CHAB_AUTH_FILE CHAB_BASE_URL CHAB_API_BASE_URL CHAB_LOCALE CHAB_PAGER; do
	unset "$name"
done

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/chab-nosecret-smoke.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM
HOME="$tmpdir/home"
CHAB_CONFIG="$tmpdir/config.yml"
CHAB_AUTH_FILE="$tmpdir/auth.json"
# Doctor checks compatibility. Pin its destination to a closed loopback port
# so this no-secret smoke never reaches the production API after defaults move.
CHAB_BASE_URL="http://127.0.0.1:9"
CHAB_API_BASE_URL="http://127.0.0.1:9/v1"
export HOME CHAB_CONFIG CHAB_AUTH_FILE CHAB_BASE_URL CHAB_API_BASE_URL
mkdir -p "$HOME"

# Assert an exact exit code. Output is discarded for implemented offline
# commands: this subset has no secret and we only validate behavior, not
# content.
expect_exit() {
	want=$1; shift
	got=0
	"$@" >/dev/null 2>&1 || got=$?
	if [ "$got" -ne "$want" ]; then
		echo "FAIL: [$*] exit $got, want $want" >&2
		exit 1
	fi
}

expect_exit 0 "$BIN" --help
expect_exit 0 "$BIN" help
expect_exit 0 "$BIN" version
expect_exit 0 "$BIN" version --help
expect_exit 0 "$BIN" version --json
expect_exit 0 "$BIN" config
expect_exit 0 "$BIN" credits
expect_exit 0 "$BIN" doctor
expect_exit 0 "$BIN" health --help
expect_exit 0 "$BIN" errors --help
expect_exit 0 "$BIN" mcp
expect_exit 0 "$BIN" mcp serve --help
for shell in bash zsh fish powershell; do
	expect_exit 0 "$BIN" completion "$shell"
done
expect_exit 0 "$BIN" config path
expect_exit 0 "$BIN" auth env

if [ -e "$CHAB_CONFIG" ] || [ -e "$CHAB_AUTH_FILE" ]; then
	echo "FAIL: no-secret smoke created config/auth state" >&2
	exit 1
fi

echo "no-secret release smoke passed for $BIN" >&2
