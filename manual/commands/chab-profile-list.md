# chab profile list

## Usage

```text
chab profile list [flags]
```

## Description

List configured connection profiles and show which profile is selected for this invocation.

This command reads local config.yml and auth.json only. Stored API keys are
reported as non-secret presence, display id, and key name; the API key value is
never printed and no API request is made. This stored-auth view is separate from
the effective request credential: CHAB_API_KEY wins for API commands. Run
chab auth status to inspect the effective credential source.

An auth-only legacy installation with no config.yml retains the historical
localhost URL default; inspection shows the same destination as API commands.

JSON output is an object with current_profile, config_path, auth_path, and a
profiles array. Each profile includes name, selected, persisted, base_url,
api_base_url, locale, default_output, project_list_limit, and stored_auth.
stored_auth includes present, display_id, key_name, team_display_id, team_name,
and last_validated_at.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab profile show](chab-profile-show.md)
- [chab config list](chab-config-list.md)

## Examples

```text
  chab profile list
  chab profile list --plain
  chab profile list --jq '.profiles[].name'
  chab profile list --template '{{.current_profile}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
