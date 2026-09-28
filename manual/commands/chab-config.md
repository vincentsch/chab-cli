# chab config

## Usage

```text
chab config [flags]
```

## Description

Inspect and edit local non-secret CLI configuration such as profile defaults and product URL settings without contacting the API. Config files must contain one YAML document with unique mapping keys. Inspection validates every persisted profile before printing output. Stored API keys are never shown or set here.

Related commands:
- [chab profile](chab-profile.md)
- [chab auth status](chab-auth-status.md)

## Subcommands

- [chab config get](chab-config-get.md) - Print one configuration value
- [chab config list](chab-config-list.md) - List configuration values
- [chab config path](chab-config-path.md) - Print the config and auth file paths
- [chab config set](chab-config-set.md) - Set one configuration value

## Examples

```text
  chab config list
  chab config get current_profile
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
