# chab auth env

## Usage

```text
chab auth env [flags]
```

## Description

Report the effective, non-secret authentication environment for this invocation.

The command uses strict runtime resolution, including normal flag, environment,
profile-file, normalization, and API-base derivation rules. This differs from
profile show and config inspection, which report persisted profile-file state
without applying all invocation overrides.

JSON output is one stable object with exactly profile, base_url, api_base_url,
locale, and secret_variable. locale remains present as an empty string when
unset. Use --plain for ordered CHAB_* tab-separated rows; its CHAB_LOCALE row
is omitted when locale is unset. Use --jq or --template to reshape the same
stable JSON value.

This is not an authentication or connectivity check. It may inspect whether
the selected profile has a stored auth-file key but no saved destination, so a
legacy auth-only profile keeps its localhost destination; the key is never
returned. It does not authenticate, construct an API client, or contact the
network. Set CHAB_API_KEY from your CI provider's secret store.
Team API keys are created and revoked in the product web app.

Copying both URL rows freezes the resolved API base URL. A later change to only
CHAB_BASE_URL will not re-derive it while CHAB_API_BASE_URL remains explicitly
set.

Related commands:
- [chab profile show](chab-profile-show.md)
- [chab config list](chab-config-list.md)
- [chab auth status](chab-auth-status.md)

## Examples

```text
  chab auth env
  chab auth env --profile staging --plain
  chab auth env --profile staging --json
  chab auth env --profile staging --jq .api_base_url
  chab auth env --profile staging --template '{{.profile}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.

## Help topics

api-key-setup
