# chab auth status

## Usage

```text
chab auth status [flags]
```

## Description

Report credential and key state for the active profile.

The command is read-only. It never creates config or auth files and never
refreshes cached auth metadata. When a credential exists, it makes at most one
GET /v1/me request.

JSON output has exactly these top-level fields: profile, api_base_url,
credential_source, usable, stored, live, and optional request_id. stored is
null for environment or missing credentials. live is null when no live probe
ran. Stored metadata is limited to principal_type, principal_id (for guests),
team_id, token_public_id, token_id, scopes, and last_validated_at.
Use --plain for copy-safe status rows. Use --jq or --template to reshape the
same stable JSON bytes that --json emits.

Exit codes: 0 usable; 3 no credential, invalid, expired, or revoked; 4 plan,
scope, token-control, or authorization denial; 2 API/network/protocol error; 6
rate limited; 1 local usage or config error. Reports render on stdout for exits
0, 3, and 4. Stdout stays empty for exits 1, 2, and 6. Not-found responses from
the /me probe are folded into exit 2.

Team API keys are created and revoked in the product web app.

Related commands:
- [chab login](chab-login.md)
- [chab whoami](chab-whoami.md)
- [chab doctor](chab-doctor.md)

## Examples

```text
  chab auth status
  chab auth status --json
  chab auth status --plain
  chab auth status --jq .usable
  chab auth status --template '{{.profile}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.

## Help topics

api-key-setup
