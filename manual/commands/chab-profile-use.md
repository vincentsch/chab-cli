# chab profile use

## Usage

```text
chab profile use <name> [flags]
```

## Description

Switch the current profile stored in local config.yml.

The target profile must already exist in config.yml. This command may inspect
which profiles have a stored key to preserve legacy URL defaults; it never
displays the key, validates API access, or contacts the API.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
- [chab profile list](chab-profile-list.md)
- [chab login](chab-login.md)

## Examples

```text
  chab profile use staging
  chab profile use staging --plain
```

## Output modes

human, plain

## Authentication

Runs without requiring a credential.
