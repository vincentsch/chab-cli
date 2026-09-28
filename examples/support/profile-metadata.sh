#!/bin/sh
set -eu

created=$("${CHAB_BIN:-chab}" profile create support --base-url "$CHAB_BASE_URL" --plain)
case "$created" in
  *"Created	profile support"*) ;;
  *) echo "profile creation output was unexpected" >&2; exit 1 ;;
esac

profile=$("${CHAB_BIN:-chab}" profile show support --json)
case "$profile" in
  *'"name": "support"'*'"selected": true'*) ;;
  *) echo "the first profile was not selected" >&2; exit 1 ;;
esac

config_value=$("${CHAB_BIN:-chab}" config get profiles.support.base_url --plain)
case "$config_value" in
  *"key	profiles.support.base_url"*) ;;
  *) echo "profile base URL was not stored" >&2; exit 1 ;;
esac

balance=$("${CHAB_BIN:-chab}" credits balance --json)
case "$balance" in
  *'"spendable_balance": 131'*) ;;
  *) echo "credit balance output was unexpected" >&2; exit 1 ;;
esac

# Response metadata is limited to API-backed leaves, requires JSON, jq, or
# template output, and cannot be combined with an active dry-run.
metadata=$("${CHAB_BIN:-chab}" api get /credits/transactions --include-meta --jq \
  '{request_id: .meta.request_id, limit: .meta.cursor.limit}')
metadata_compact=$(printf '%s' "$metadata" | tr -d '[:space:]')
case "$metadata_compact" in
  *'"request_id":"mock-req-000002"'*) ;;
  *) echo "response metadata request id was unexpected" >&2; exit 1 ;;
esac
case "$metadata_compact" in
  *'"limit":25'*) ;;
  *) echo "response metadata limit was unexpected" >&2; exit 1 ;;
esac

printf '%s\n' "$created"
printf '%s\n' "$profile"
printf '%s\n' "$config_value"
printf '%s\n' "$balance"
printf '%s\n' "$metadata_compact"
