# chab api

## Usage

```text
chab api [flags]
```

## Description

Call public API endpoints directly when a first-class command is not available. Paths are confined below the resolved API base, and command leaves reuse the active profile, credential, locale, retry, error, and output behavior. Raw POST refuses documented v1 routes that return one-time plaintext secrets; use the dedicated private-output workflow for those operations.

Related commands:
- [chab login](chab-login.md)
- [chab health](chab-health.md)

## Subcommands

- [chab api delete](chab-api-delete.md) - Send a DELETE request to an API path
- [chab api get](chab-api-get.md) - Send a GET request to an API path
- [chab api patch](chab-api-patch.md) - Send a PATCH request to an API path
- [chab api post](chab-api-post.md) - Send a POST request to an API path

## Examples

```text
  chab api get /me
  chab api get /credits
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
