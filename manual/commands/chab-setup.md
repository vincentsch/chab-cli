# chab setup

## Usage

```text
chab setup [flags]
```

## Description

Set up and verify the selected profile with a copied team API key or
browser-issued guest trial credential.

Team API keys are created and revoked in the product web app. Guest
trial credentials are issued and revoked in the browser trial UI. Guest
trial credentials support only the advertised free operations and expire;
they are not hosted MCP OAuth tokens. An explicit
missing profile name is created and selected. Setup reuses a stored key when
possible, verifies it with one GET /v1/me request, and does not rewrite
auth state when that check succeeds. It makes only the config repair needed to
reproduce the resolved profile and connection settings later.

Piped stdin:

```text
Stored key state | Piped stdin behavior | Result
--- | --- | ---
No usable stored key | Read the pipe as the copied key | Validate, persist, select the profile, and report ready
Stored key passes /v1/me | Never read the pipe | Reuse the identity, make only any required config repair, and report ready
Stored key returns an authentication-class failure | Never read the pipe | Use the replacement policy below; a non-interactive invocation returns that API error with explicit recovery guidance
```

Only an interactive terminal may replace a stored key rejected for
authentication. --yes accepts that overwrite confirmation inside an
interactive setup. --no-prompt disables replacement even with --yes and even
on a terminal; setup returns the authentication error and the recovery command
instead. Other API, network, protocol, authorization, and rate-limit failures
never offer replacement. Exit status 3 identifies the authentication failure.
For explicit scripted replacement, run:

```text
chab login --api-key --no-prompt --yes < api-key.txt
```

If CHAB_API_KEY is set, setup still validates the stored or copied profile key,
then warns after successful output that the environment credential takes
precedence for ordinary authenticated commands.

Output is human-readable even when --json is inherited from the root. Use
--plain for copy-safe setup result rows. jq, template, response metadata,
and dry-run output are not available.

Related commands:
- [chab login](chab-login.md)
- [chab auth status](chab-auth-status.md)
- [chab doctor](chab-doctor.md)

## Examples

```text
  chab setup
  chab setup --profile staging --base-url https://example.test
  chab setup --no-prompt < api-key.txt
  chab setup --plain
```

## Output modes

human, plain

## Authentication

Runs without requiring a credential.

## Help topics

api-key-setup, confirmation
