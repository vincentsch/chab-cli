# chab config path

## Usage

```text
chab config path [flags]
```

## Description

Print the local config and auth file paths selected for this invocation.

This command resolves --config, --auth-file, CHAB_CONFIG, CHAB_AUTH_FILE, and the
built-in defaults without loading or parsing either file. It still works when
config.yml is malformed.

JSON output is an object with config_path and auth_path.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab config list](chab-config-list.md)
- [chab profile list](chab-profile-list.md)

## Examples

```text
  chab config path
  chab config path --plain
  chab config path --jq .config_path
  chab config path --template '{{.config_path}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
