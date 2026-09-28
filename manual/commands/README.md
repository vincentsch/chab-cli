# chab command reference

chab is the command-line interface for Chab. It talks to Chab's public
REST API (/v1) using a scoped team API token from browser device login,
manual entry, or CHAB_API_KEY. Browser-issued guest trial credentials can be
used for the backend-advertised free operations without signup.

Quick start:

```text
chab version --json    # print build metadata for scripts
chab setup             # set up and verify a copied team or guest key
chab login             # authorize or replace a stored team API key
chab whoami --json     # print authenticated team and token data
chab doctor            # check local setup and API readiness
chab profile list      # inspect local connection profiles
chab config path       # print the local config and auth paths
chab health            # inspect public API health
chab errors --json     # inspect public API error catalog
chab search web --help # start a recoverable operation
chab files list        # inspect stored files
chab operations schema search.web # inspect embedded operation schema
chab api get /me --json # call a public API path directly
chab mcp serve --help  # configure local MCP tools
```

## Commands

- [chab api](chab-api.md) - Call public API endpoints directly
- [chab api delete](chab-api-delete.md) - Send a DELETE request to an API path
- [chab api get](chab-api-get.md) - Send a GET request to an API path
- [chab api patch](chab-api-patch.md) - Send a PATCH request to an API path
- [chab api post](chab-api-post.md) - Send a POST request to an API path
- [chab auth](chab-auth.md) - Inspect and manage API authentication
- [chab auth env](chab-auth-env.md) - Report the effective authentication environment
- [chab auth login](chab-auth-login.md) - Authorize and store an API or guest trial key
- [chab auth logout](chab-auth-logout.md) - Remove the stored credential for the active profile
- [chab auth status](chab-auth-status.md) - Report credential and key state
- [chab billing](chab-billing.md) - Manage billing settings
- [chab billing auto-recharge](chab-billing-auto-recharge.md) - Manage auto-recharge
- [chab billing auto-recharge show](chab-billing-auto-recharge-show.md) - Show auto-recharge
- [chab billing auto-recharge update](chab-billing-auto-recharge-update.md) - Update auto-recharge
- [chab billing packages](chab-billing-packages.md) - List billing packages
- [chab billing purchases](chab-billing-purchases.md) - Manage billing purchases
- [chab billing purchases create](chab-billing-purchases-create.md) - Create a credit purchase
- [chab billing purchases show](chab-billing-purchases-show.md) - Show a credit purchase
- [chab billing purchases wait](chab-billing-purchases-wait.md) - Wait for a credit purchase
- [chab billing reconciliation](chab-billing-reconciliation.md) - Show billing reconciliation
- [chab billing show](chab-billing-show.md) - Show billing context
- [chab business](chab-business.md) - Run business-data operations
- [chab business details](chab-business-details.md) - Start a business details operation (preview)
- [chab business search](chab-business-search.md) - Start a business search operation (preview)
- [chab completion](chab-completion.md) - Generate the autocompletion script for the specified shell
- [chab config](chab-config.md) - Inspect and edit non-secret CLI configuration
- [chab config get](chab-config-get.md) - Print one configuration value
- [chab config list](chab-config-list.md) - List configuration values
- [chab config path](chab-config-path.md) - Print the config and auth file paths
- [chab config set](chab-config-set.md) - Set one configuration value
- [chab contacts](chab-contacts.md) - Run contact discovery operations
- [chab contacts domain-search](chab-contacts-domain-search.md) - Start a domain contact search (preview)
- [chab contacts email-finder](chab-contacts-email-finder.md) - Start an email finder operation (preview)
- [chab contacts email-verify](chab-contacts-email-verify.md) - Start an email verification operation (preview)
- [chab convert](chab-convert.md) - Run conversion operations
- [chab convert file](chab-convert-file.md) - Start a file conversion operation (preview)
- [chab credits](chab-credits.md) - Inspect team credits
- [chab credits balance](chab-credits-balance.md) - Show the available credit balance
- [chab credits transactions](chab-credits-transactions.md) - List credit transactions
- [chab doctor](chab-doctor.md) - Check local setup and API readiness
- [chab drive](chab-drive.md) - Work with connected drive files
- [chab drive connections](chab-drive-connections.md) - List drive connections (preview)
- [chab drive folders](chab-drive-folders.md) - Work with drive folders
- [chab drive folders create](chab-drive-folders-create.md) - Create a drive folder (preview)
- [chab drive items](chab-drive-items.md) - Work with drive items
- [chab drive items download](chab-drive-items-download.md) - Download drive item bytes (preview)
- [chab drive items export](chab-drive-items-export.md) - Export a drive-native document (preview)
- [chab drive items list](chab-drive-items-list.md) - List drive items (preview)
- [chab drive items move](chab-drive-items-move.md) - Move or rename a drive item (preview)
- [chab drive items show](chab-drive-items-show.md) - Show drive item metadata (preview)
- [chab drive items trash](chab-drive-items-trash.md) - Move a drive item to trash (preview)
- [chab drive items update](chab-drive-items-update.md) - Replace drive item bytes (preview)
- [chab drive items upload](chab-drive-items-upload.md) - Upload a stored source to drive (preview)
- [chab drive permissions](chab-drive-permissions.md) - Work with drive permissions
- [chab drive permissions update](chab-drive-permissions-update.md) - Update drive item permissions (preview)
- [chab drive search](chab-drive-search.md) - Search drive item names (preview)
- [chab errors](chab-errors.md) - List public API error codes
- [chab examples](chab-examples.md) - Run example operations
- [chab examples echo](chab-examples-echo.md) - Echo a setup payload
- [chab files](chab-files.md) - Work with stored files
- [chab files delete](chab-files-delete.md) - Delete a stored file (preview)
- [chab files download](chab-files-download.md) - Download stored file bytes (preview)
- [chab files list](chab-files-list.md) - List stored files (preview)
- [chab files resume](chab-files-resume.md) - Resume a stored-file upload (preview)
- [chab files show](chab-files-show.md) - Show stored file metadata (preview)
- [chab files upload](chab-files-upload.md) - Upload a stored file (preview)
- [chab files wait](chab-files-wait.md) - Wait for stored file readiness (preview)
- [chab health](chab-health.md) - Show public API health
- [chab help](chab-help.md) - Help about any command
- [chab llm](chab-llm.md) - Run model operations
- [chab llm embeddings](chab-llm-embeddings.md) - Start an embeddings operation (preview)
- [chab llm generate](chab-llm-generate.md) - Start an LLM generation operation (preview)
- [chab llm models](chab-llm-models.md) - List available LLM models (preview)
- [chab login](chab-login.md) - Authorize and store an API or guest trial key
- [chab logout](chab-logout.md) - Remove the stored credential for the active profile
- [chab mail](chab-mail.md) - Work with connected mail
- [chab mail attachments](chab-mail-attachments.md) - Work with message attachments
- [chab mail attachments download](chab-mail-attachments-download.md) - Download a mail attachment (preview)
- [chab mail connections](chab-mail-connections.md) - List mail connections (preview)
- [chab mail drafts](chab-mail-drafts.md) - Work with API-owned mail drafts
- [chab mail drafts attachments](chab-mail-drafts-attachments.md) - Work with draft attachments
- [chab mail drafts attachments add](chab-mail-drafts-attachments-add.md) - Add a draft attachment (preview)
- [chab mail drafts attachments remove](chab-mail-drafts-attachments-remove.md) - Remove a draft attachment (preview)
- [chab mail drafts create](chab-mail-drafts-create.md) - Create a mail draft (preview)
- [chab mail drafts discard](chab-mail-drafts-discard.md) - Discard a mail draft (preview)
- [chab mail drafts send](chab-mail-drafts-send.md) - Send a mail draft (preview)
- [chab mail drafts show](chab-mail-drafts-show.md) - Show a mail draft (preview)
- [chab mail drafts update](chab-mail-drafts-update.md) - Update a mail draft (preview)
- [chab mail folders](chab-mail-folders.md) - List mail folders (preview)
- [chab mail messages](chab-mail-messages.md) - Work with mail messages
- [chab mail messages body](chab-mail-messages-body.md) - Read a mail message body (preview)
- [chab mail messages state](chab-mail-messages-state.md) - Update mail message state (preview)
- [chab mail personas](chab-mail-personas.md) - List mail personas (preview)
- [chab mail search](chab-mail-search.md) - Search connected mail (preview)
- [chab mail threads](chab-mail-threads.md) - Work with mail threads
- [chab mail threads list](chab-mail-threads-list.md) - List mail threads (preview)
- [chab mail threads show](chab-mail-threads-show.md) - Show a mail thread (preview)
- [chab mcp](chab-mcp.md) - Run local MCP integrations
- [chab mcp serve](chab-mcp-serve.md) - Serve Chab tools over MCP stdio
- [chab operations](chab-operations.md) - Inspect and recover Chab operations
- [chab operations actions](chab-operations-actions.md) - Inspect local action records
- [chab operations actions list](chab-operations-actions-list.md) - List local action records
- [chab operations actions show](chab-operations-actions-show.md) - Show a local action record
- [chab operations artifact](chab-operations-artifact.md) - Fetch operation artifact metadata
- [chab operations artifact download](chab-operations-artifact-download.md) - Download operation artifact bytes
- [chab operations bulk-cancel](chab-operations-bulk-cancel.md) - Cancel operations by filter
- [chab operations cancel](chab-operations-cancel.md) - Cancel an operation
- [chab operations estimate](chab-operations-estimate.md) - Estimate an operation request
- [chab operations list](chab-operations-list.md) - List remote operations
- [chab operations result](chab-operations-result.md) - Fetch operation result metadata
- [chab operations resume](chab-operations-resume.md) - Resume a local action
- [chab operations schema](chab-operations-schema.md) - Show an embedded operation schema
- [chab operations show](chab-operations-show.md) - Show operation status
- [chab operations start](chab-operations-start.md) - Start a supported operation by key
- [chab operations wait](chab-operations-wait.md) - Wait for an operation to finish
- [chab profile](chab-profile.md) - Manage connection profiles
- [chab profile create](chab-profile-create.md) - Create a profile
- [chab profile delete](chab-profile-delete.md) - Delete a profile
- [chab profile list](chab-profile-list.md) - List configured profiles
- [chab profile show](chab-profile-show.md) - Show a profile's settings
- [chab profile use](chab-profile-use.md) - Switch the current profile
- [chab project](chab-project.md) - Manage projects
- [chab project archive](chab-project-archive.md) - Archive a project
- [chab project create](chab-project-create.md) - Create a project
- [chab project delete](chab-project-delete.md) - Delete a project
- [chab project list](chab-project-list.md) - List projects
- [chab project pause](chab-project-pause.md) - Pause a project
- [chab project resume](chab-project-resume.md) - Resume a project
- [chab project show](chab-project-show.md) - Show a project
- [chab project update](chab-project-update.md) - Update a project
- [chab research](chab-research.md) - Run research operations
- [chab research deep](chab-research-deep.md) - Start a deep research operation (preview)
- [chab scrape](chab-scrape.md) - Run scrape operations
- [chab scrape dom](chab-scrape-dom.md) - Start a DOM scrape operation (preview)
- [chab scrape markdown](chab-scrape-markdown.md) - Start a markdown scrape operation (preview)
- [chab screenshots](chab-screenshots.md) - Run screenshot operations
- [chab screenshots url](chab-screenshots-url.md) - Start a URL screenshot operation (preview)
- [chab search](chab-search.md) - Run search operations
- [chab search serp](chab-search-serp.md) - Start a SERP search operation (preview)
- [chab search web](chab-search-web.md) - Start a web search operation (preview)
- [chab seo](chab-seo.md) - Run SEO operations
- [chab seo domains](chab-seo-domains.md) - Run domain SEO operations
- [chab seo domains backlinks](chab-seo-domains-backlinks.md) - Start a backlinks operation (preview)
- [chab seo domains overview](chab-seo-domains-overview.md) - Start a domain overview operation (preview)
- [chab seo keywords](chab-seo-keywords.md) - Run keyword operations
- [chab seo keywords ideas](chab-seo-keywords-ideas.md) - Start a keyword ideas operation (preview)
- [chab seo keywords metrics](chab-seo-keywords-metrics.md) - Start a keyword metrics operation (preview)
- [chab setup](chab-setup.md) - Set up a verified API or guest trial profile
- [chab tokens](chab-tokens.md) - Manage team API tokens
- [chab tokens approvals](chab-tokens-approvals.md) - Manage approval challenges
- [chab tokens approvals create](chab-tokens-approvals-create.md) - Create an approval challenge
- [chab tokens approvals wait](chab-tokens-approvals-wait.md) - Wait for an approval proof
- [chab tokens create](chab-tokens-create.md) - Create an API token
- [chab tokens list](chab-tokens-list.md) - List API tokens
- [chab tokens revoke](chab-tokens-revoke.md) - Revoke an API token
- [chab tokens show](chab-tokens-show.md) - Show API token metadata
- [chab tokens update](chab-tokens-update.md) - Update an API token
- [chab translate](chab-translate.md) - Run translation operations
- [chab translate text-or-document](chab-translate-text-or-document.md) - Start a text or document translation (preview)
- [chab usage](chab-usage.md) - Show team API usage
- [chab version](chab-version.md) - Print build metadata
- [chab webhooks](chab-webhooks.md) - Manage webhook endpoints and replays
- [chab webhooks deliveries](chab-webhooks-deliveries.md) - Manage webhook deliveries
- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md) - List webhook deliveries
- [chab webhooks deliveries replay](chab-webhooks-deliveries-replay.md) - Replay a webhook delivery
- [chab webhooks deliveries show](chab-webhooks-deliveries-show.md) - Show a webhook delivery
- [chab webhooks endpoints](chab-webhooks-endpoints.md) - Manage webhook endpoints
- [chab webhooks endpoints create](chab-webhooks-endpoints-create.md) - Create a webhook endpoint
- [chab webhooks endpoints delete](chab-webhooks-endpoints-delete.md) - Delete a webhook endpoint
- [chab webhooks endpoints list](chab-webhooks-endpoints-list.md) - List webhook endpoints
- [chab webhooks endpoints rotate-secret](chab-webhooks-endpoints-rotate-secret.md) - Rotate a webhook secret
- [chab webhooks endpoints show](chab-webhooks-endpoints-show.md) - Show a webhook endpoint
- [chab webhooks endpoints update](chab-webhooks-endpoints-update.md) - Update a webhook endpoint
- [chab webhooks replays](chab-webhooks-replays.md) - Manage webhook replay windows
- [chab webhooks replays create](chab-webhooks-replays-create.md) - Create a webhook replay window
- [chab whoami](chab-whoami.md) - Show the authenticated team or guest principal

## Global flags

- `--api-base-url` - API base URL (Chab default uses www.chab.ai/v1; custom base URLs use <base-url>/v1)
- `--auth-file` - path to the CLI auth file
- `--base-url` - product web base URL
- `--config` - path to the CLI config file
- `--debug` - write redacted request details to stderr
- `--include-meta` - include safe response metadata in machine output where supported
- `--jq` - filter supported JSON output with a jq expression
- `--json` - print stable JSON output where supported
- `--locale` - response language sent as Accept-Language (en or de)
- `--no-ansi` - disable ANSI terminal controls and paging where supported
- `--no-color` - disable color in human output where supported
- `--no-pager` - disable pager use for long human output where supported
- `--no-prompt` - fail instead of prompting for interactive input
- `--plain` - print copy-safe plain output where supported
- `--profile` - profile to use for this invocation
- `--template` - render supported JSON output with a Go text/template
- `--yes` - accept supported confirmations for risky or destructive operations; does not supply missing input
