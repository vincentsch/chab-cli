# chab auth logout

## Usage

```text
chab auth logout [flags]
```

## Description

Remove the stored credential for the selected profile without changing non-secret configuration.

Logout is local and idempotent. It removes only the selected profile's stored
auth record and cached metadata, preserving other profiles, config, and
current_profile. It does not revoke the API key on the server; revoke keys in
the product web app. If CHAB_API_KEY is set, that environment credential still
takes precedence until it is unset.

Team API keys are created and revoked in the product web app. chab never prints
the stored API key; logout shows only non-secret key metadata.

Output is human-readable even when --json is inherited from the root. Use
--plain for copy-safe local removal rows. jq and template output are not
available because logout has no stable JSON shape.

Related commands:
- [chab login](chab-login.md)
- [chab auth status](chab-auth-status.md)

## Examples

```text
  chab auth logout
  chab auth logout --plain --profile staging
```

## Output modes

human, plain

## Authentication

Runs without requiring a credential.

## Help topics

api-key-setup
