# chab auth

## Usage

```text
chab auth [flags]
```

## Description

Inspect and manage API authentication for chab.

Team API keys are created and revoked in the product web app. This family
stores an API key after browser authorization or manual entry validates it
with GET /v1/me, reports the effective credential and token state,
removes local stored credentials, and shows the effective non-secret runtime
values used by CI invocations.

chab never prints an API key. Only non-secret runtime, token, and team metadata
can appear in output.

Related commands:
- [chab login](chab-login.md)
- [chab whoami](chab-whoami.md)
- [chab profile show](chab-profile-show.md)
- [chab doctor](chab-doctor.md)

## Subcommands

- [chab auth env](chab-auth-env.md) - Report the effective authentication environment
- [chab auth login](chab-auth-login.md) - Authorize and store an API or guest trial key
- [chab auth logout](chab-auth-logout.md) - Remove the stored credential for the active profile
- [chab auth status](chab-auth-status.md) - Report credential and key state

## Examples

```text
  chab auth login
  chab auth status
  chab auth env --json
  chab auth logout
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.

## Help topics

api-key-setup
