# chab whoami

## Usage

```text
chab whoami [flags]
```

## Description

Show the authenticated team token or guest trial principal for the active profile.

Authentication uses CHAB_API_KEY when set, otherwise the stored login for the
selected profile. Missing credentials exit 3 with stdout empty.

Team API keys are created and revoked in the product web app.

JSON output is the flat GET /v1/me context. Team tokens include team_id,
token_public_id, scopes, and token_controls. Guest credentials include
principal_id, guest_id, scopes, credential and free_access; team_id is null.

Use --plain for copy-safe detail rows. Use --jq or --template to reshape the
same stable JSON bytes that --json emits.

Related commands:
- [chab login](chab-login.md)
- [chab auth status](chab-auth-status.md)
- [chab doctor](chab-doctor.md)

## Examples

```text
  chab whoami
  chab whoami --json
  chab whoami --plain
  chab whoami --jq '.team | .name'
  chab whoami --template '{{.team.name}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

api-key-setup
