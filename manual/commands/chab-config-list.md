# chab config list

## Usage

```text
chab config list [flags]
```

## Description

List known non-secret CLI configuration values.

The output includes current_profile and known profile fields such as base_url,
api_base_url, locale, default_output, and defaults.project_list_limit. Unknown
YAML keys and stored credentials are never shown.

When config.yml is absent and the built-in local profile has a stored key,
its implicit URL values remain localhost. This list describes config/default
values, not a CHAB_PROFILE override; use auth env for the selected runtime.

JSON output is an object with config_path and a values array. Each value has key,
value, and source, where source is file, derived, or default.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab config get](chab-config-get.md)
- [chab profile show](chab-profile-show.md)

## Examples

```text
  chab config list
  chab config list --plain
  chab config list --jq '.values[].key'
  chab config list --template '{{range .values}}{{.key}}{{"\n"}}{{end}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
