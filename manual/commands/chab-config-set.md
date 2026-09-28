# chab config set

## Usage

```text
chab config set <key> <value> [flags]
```

## Description

Set one supported non-secret CLI configuration value.

Supported writable keys are current_profile, profiles.<name>.base_url,
profiles.<name>.api_base_url, profiles.<name>.locale, and
profiles.<name>.defaults.project_list_limit. The target profile must already
exist; this command never creates profiles. Use an empty value for locale to
persist an unset Accept-Language preference.

Stored credentials are not part of config.yml. Secret-bearing keys are refused
locally; manage credentials with chab login and chab logout.

profiles.<name>.default_output is readable with config get and config list, but
is not writable in this config contract.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
- [chab config get](chab-config-get.md)
- [chab profile create](chab-profile-create.md)

## Examples

```text
  chab config set current_profile staging
  chab config set profiles.staging.base_url https://staging.example.test
  chab config set profiles.staging.locale ""
  chab config set profiles.staging.defaults.project_list_limit 50 --plain
```

## Output modes

human, plain

## Authentication

Runs without requiring a credential.
