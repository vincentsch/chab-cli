#!/bin/sh
set -eu

# Keep the original report as an output artifact. Compact only the comparison
# so this check accepts both the real indented JSON and the compact test fake.
environment_json=$("${CHAB_BIN:-chab}" auth env --json)
environment_compact=$(printf '%s' "$environment_json" | tr -d '[:space:]')
case "$environment_compact" in
  "{\"profile\":\"local\",\"base_url\":\"$CHAB_BASE_URL\",\"api_base_url\":\"$CHAB_BASE_URL/v1\",\"locale\":\"\",\"secret_variable\":\"CHAB_API_KEY\"}") ;;
  *) echo "authentication environment report was unexpected" >&2; exit 1 ;;
esac

credits_json=$("${CHAB_BIN:-chab}" api get /credits --json)
[ -n "$credits_json" ] || {
  echo "credits JSON output was empty" >&2
  exit 1
}
case "$credits_json" in
  *'"spendable_balance": 131'*) ;;
  *) echo "credits JSON output did not contain the expected balance" >&2; exit 1 ;;
esac

balance=$("${CHAB_BIN:-chab}" api get /credits --jq '.spendable_balance')
[ -n "$balance" ] || {
  echo "credits jq output was empty" >&2
  exit 1
}
case "$balance" in
  131) ;;
  *) echo "credits jq output did not contain the expected balance" >&2; exit 1 ;;
esac

status=0
"${CHAB_BIN:-chab}" api get /missing --json || status=$?
case "$status" in
  5) ;;
  0) echo "missing route unexpectedly succeeded" >&2; exit 1 ;;
  *) echo "missing route returned an unexpected status" >&2; exit 1 ;;
esac

if [ -n "${CHAB_CONFIG:-}" ]; then
  [ ! -e "$CHAB_CONFIG" ] || {
    echo "environment-auth example created a config file" >&2
    exit 1
  }
fi
if [ -n "${CHAB_AUTH_FILE:-}" ]; then
  [ ! -e "$CHAB_AUTH_FILE" ] || {
    echo "environment-auth example created an auth file" >&2
    exit 1
  }
fi

printf '%s\n' "$credits_json"
printf '%s\n' "$balance"
printf '%s\n' "$environment_json"
echo "raw API not-found exit verified" >&2
