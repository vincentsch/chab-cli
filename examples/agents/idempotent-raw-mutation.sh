#!/bin/sh
set -eu

: "${CHAB_EXAMPLE_IDEMPOTENCY_KEY:?checker idempotency key is required}"

preview=$("${CHAB_BIN:-chab}" api post /projects \
  --field name=AgentDemo \
  --dry-run \
  --json \
  --no-prompt)
case "$preview" in
  *'"method": "POST"'*) ;;
  *) echo "raw mutation preview was unexpected" >&2; exit 1 ;;
esac

first=$("${CHAB_BIN:-chab}" api post /projects \
  --field name=AgentDemo \
  --idempotency-key "$CHAB_EXAMPLE_IDEMPOTENCY_KEY" \
  --include-meta \
  --no-prompt \
  --jq '.meta.idempotent_replayed')
second=$("${CHAB_BIN:-chab}" api post /projects \
  --field name=AgentDemo \
  --idempotency-key "$CHAB_EXAMPLE_IDEMPOTENCY_KEY" \
  --no-prompt \
  --json \
  --jq '.local_recovery.known_remote')

[ "$first" = false ] || {
  echo "first mutation was unexpectedly marked as replayed" >&2
  exit 1
}
[ "$second" = true ] || {
  echo "second mutation did not recover the known remote action" >&2
  exit 1
}

printf '%s\n' "$preview"
printf '%s\n' "$first"
printf '%s\n' "$second"
