# chab profile create

## Usage

```text
chab profile create <name> [flags]
```

## Description

Create a saved profile in local config.yml.

Pass the global --base-url flag to set the product base URL for the new profile.
Optional global --api-base-url and --locale flags set that profile's API base URL
and locale. The API base URL is derived from --base-url when --api-base-url is
omitted. Use --project-list-limit to set the profile's default project list
limit. Environment variables such as CHAB_BASE_URL are not used as create input.

This command writes only non-secret config. Stored credentials are managed
separately with chab login and are never created or printed here.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
- [chab profile use](chab-profile-use.md)
- [chab login](chab-login.md)

## Examples

```text
  chab profile create staging --base-url https://staging.example.test
  chab profile create staging --base-url https://staging.example.test --locale en --project-list-limit 50
  chab profile create staging --base-url https://staging.example.test --plain
```

## Flags

- `--project-list-limit` - default project list limit for the profile

## Output modes

human, plain

## Authentication

Runs without requiring a credential.
