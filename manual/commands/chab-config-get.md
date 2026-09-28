# chab config get

## Usage

```text
chab config get <key> [flags]
```

## Description

Print one known non-secret CLI configuration value.

Supported keys are current_profile and profiles.<name>.base_url,
profiles.<name>.api_base_url, profiles.<name>.locale,
profiles.<name>.default_output, and
profiles.<name>.defaults.project_list_limit. Profile names may contain dots.
Stored credential keys and unknown YAML keys are never shown.

All persisted profile names and known profile values are validated before any
result is printed. An invalid persisted profile therefore makes the lookup fail
without partial output, even when a different profile key was requested.

JSON output is an object with key, value, and source, where source is file,
derived, or default.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab config list](chab-config-list.md)
- [chab config set](chab-config-set.md)

## Examples

```text
  chab config get current_profile
  chab config get profiles.staging.base_url
  chab config get profiles.production.eu.api_base_url --jq .value
  chab config get current_profile --template '{{.value}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
