# chab profile

## Usage

```text
chab profile [flags]
```

## Description

Manage local connection profiles for product environments without contacting the API. Profiles hold base URL and locale defaults while credentials are stored separately by chab login. Inspection validates every persisted profile before printing output.

Related commands:
- [chab login](chab-login.md)
- [chab auth status](chab-auth-status.md)
- [chab config](chab-config.md)

## Subcommands

- [chab profile create](chab-profile-create.md) - Create a profile
- [chab profile delete](chab-profile-delete.md) - Delete a profile
- [chab profile list](chab-profile-list.md) - List configured profiles
- [chab profile show](chab-profile-show.md) - Show a profile's settings
- [chab profile use](chab-profile-use.md) - Switch the current profile

## Examples

```text
  chab profile list
  chab profile show staging
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
