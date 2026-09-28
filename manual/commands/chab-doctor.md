# chab doctor

## Usage

```text
chab doctor [flags]
```

## Description

Check local setup and API readiness for common CLI workflows.

Doctor resolves local config and auth paths without creating files. When a
credential exists, it makes at most one GET /v1/me request. Missing
credentials, unsafe auth-file permissions, unreachable API, unusable keys, and
blocked plan API access are warnings. Malformed local config/auth files and
invalid persisted profile values are failing findings.

Stable finding ids are config.path, config.runtime, profile, api.base_url,
locale, auth.path, auth.file, auth.permissions, credential.source,
compatibility, api.connectivity, granted_scopes, token_controls,
spending_allowance, and project_access.

Exit model: 0 means pass or warnings; 1 means one or more failing findings.
The readiness report still renders when doctor exits 1 because of failing
findings. Invocation errors such as invalid flag or environment values keep
stdout empty.

JSON output has exactly the top-level fields status and findings. Each finding
has id, severity, summary, and optional detail and request_id.
Use --plain for a copy-safe TSV table. Use --jq or --template to reshape the
same stable JSON bytes that --json emits.

Related commands:
- [chab auth status](chab-auth-status.md)
- [chab login](chab-login.md)
- [chab version](chab-version.md)

## Examples

```text
  chab doctor
  chab doctor --json
  chab doctor --plain
  chab doctor --jq .status
  chab doctor --template '{{.status}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
