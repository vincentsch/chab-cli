# chab auth login

## Usage

```text
chab auth login [flags]
```

## Description

Authorize the CLI with browser device login when available, or validate a copied team API key or guest trial credential with GET /v1/me and store it for future commands.

Team API keys are created and revoked in the product web app. Guest trial credentials are issued by the guest trial flow. Manual API-key
entry reads the key from a secret prompt or from non-terminal stdin; it is
never accepted from a flag or CHAB_API_KEY. --api-key only selects manual
entry and does not accept the key value.

Interactive input prompts for the product base URL only when the built-in
default is still selected. Values supplied by --base-url, CHAB_BASE_URL, or the
selected profile are used as-is.

For browser login against a custom API host, also set the matching app origin
with --base-url or CHAB_BASE_URL. An API-only override cannot use an implicit
browser issuer.

With no selector, interactive login checks CLI compatibility, prints the
verification URL and user code to stderr, tries to open the browser, polls until
approval or terminal failure, validates the issued API key with GET /v1/me, then
stores it. Use --scope to request explicit space-separated or repeated OAuth
scopes; the default minimal scope is api:projects:read.

Use --web to force browser login. With --web --no-prompt, login prints the
verification URL and user code without opening a browser. Use --api-key to
force manual entry and skip discovery. Non-terminal default login also uses
manual entry without discovery, preserving piped-key automation. With
--no-prompt and terminal stdin, manual login fails with guidance to pipe the
key.

The write target is the resolved profile from --profile, CHAB_PROFILE,
current_profile, or the built-in local profile. When that profile already has a
stored key, interactive login asks before reading the replacement key. --yes
accepts that overwrite confirmation, but it does not provide missing input.
With --no-prompt, an overwrite without --yes fails before stdin is consumed,
the browser is opened, or an authenticated API request is made. Non-terminal
manual stdin without --no-prompt keeps the compatible piped-key path and
overwrites after successful API validation.

Output is human-readable even when --json is inherited from the root. Use
--plain for copy-safe login result rows. jq and template output are not
available because login has no stable JSON shape.

If CHAB_API_KEY is set, login still validates and stores the manual or
browser-issued key, then warns after successful output that the environment
credential takes precedence for ordinary authenticated commands.

Related commands:
- [chab whoami](chab-whoami.md)
- [chab auth status](chab-auth-status.md)
- [chab doctor](chab-doctor.md)

## Examples

```text
  chab auth login
  chab auth login --web
  chab auth login --api-key
  chab auth login --profile staging --base-url https://example.test
  chab auth login --plain --api-key --no-prompt --yes < api-key.txt
```

## Flags

- `--api-key` - use manual API-key entry instead of browser login
- `--device-name` - label this browser authorization device
- `--scope` - request an OAuth scope; may be repeated or space-separated
- `--web` - use browser device login

## Output modes

human, plain

## Authentication

Runs without requiring a credential.

## Help topics

api-key-setup, confirmation
