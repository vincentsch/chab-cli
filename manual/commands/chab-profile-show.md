# chab profile show

## Usage

```text
chab profile show [name] [flags]
```

## Description

Show one profile's local, non-secret settings.

When no name is supplied, the selected profile is resolved from --profile,
CHAB_PROFILE, current_profile, or the built-in local default. Missing profiles
show built-in defaults and are marked as not saved. Stored API keys are reported
only as non-secret presence and display metadata; the API key value is never
printed and no API request is made. This stored-auth view is separate from the
effective request credential: CHAB_API_KEY wins for API commands. Run
chab auth status to inspect the effective credential source.

An auth-only legacy installation with no config.yml retains the historical
localhost URL default; inspection shows the same destination as API commands.

All persisted profile names and known profile values are validated before any
result is printed. An invalid persisted profile therefore makes inspection fail
without partial output, even when a different profile was requested.

JSON output is one profile object with name, selected, persisted, base_url,
api_base_url, locale, default_output, project_list_limit, and stored_auth.
stored_auth includes present, display_id, key_name, team_display_id, team_name,
and last_validated_at.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab profile list](chab-profile-list.md)
- [chab config get](chab-config-get.md)

## Examples

```text
  chab profile show
  chab profile show staging
  chab profile show staging --jq .api_base_url
  chab profile show staging --template '{{.name}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
