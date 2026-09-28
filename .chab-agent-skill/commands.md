# Chab Command Reference

This section is generated from the active command catalog.

| Command | Summary | Output |
| --- | --- | --- |
| `chab api` | Call public API endpoints directly | none (command family) |
| `chab api delete` | Send a DELETE request to an API path | human, JSON, plain, jq, template |
| `chab api get` | Send a GET request to an API path | human, JSON, plain, jq, template |
| `chab api patch` | Send a PATCH request to an API path | human, JSON, plain, jq, template |
| `chab api post` | Send a POST request to an API path | human, JSON, plain, jq, template |
| `chab auth` | Inspect and manage API authentication | none (command family) |
| `chab auth env` | Report the effective authentication environment | human, JSON, plain, jq, template |
| `chab auth login` | Authorize and store an API or guest trial key | human, plain |
| `chab auth logout` | Remove the stored credential for the active profile | human, plain |
| `chab auth status` | Report credential and key state | human, JSON, plain, jq, template |
| `chab billing` | Manage billing settings | none (command family) |
| `chab billing auto-recharge` | Manage auto-recharge | none (command family) |
| `chab billing auto-recharge show` | Show auto-recharge | human, JSON, plain, jq, template |
| `chab billing auto-recharge update` | Update auto-recharge | human, JSON, plain, jq, template |
| `chab billing packages` | List billing packages | human, JSON, plain, jq, template |
| `chab billing purchases` | Manage billing purchases | none (command family) |
| `chab billing purchases create` | Create a credit purchase | human, JSON, plain, jq, template |
| `chab billing purchases show` | Show a credit purchase | human, JSON, plain, jq, template |
| `chab billing purchases wait` | Wait for a credit purchase | human, JSON, plain, jq, template |
| `chab billing reconciliation` | Show billing reconciliation | human, JSON, plain, jq, template |
| `chab billing show` | Show billing context | human, JSON, plain, jq, template |
| `chab business` | Run business-data operations | none (command family) |
| `chab business details` | Start a business details operation (preview) | human, JSON, plain |
| `chab business search` | Start a business search operation (preview) | human, JSON, plain |
| `chab completion` | Generate the autocompletion script for the specified shell | shell script |
| `chab config` | Inspect and edit non-secret CLI configuration | none (command family) |
| `chab config get` | Print one configuration value | human, JSON, plain, jq, template |
| `chab config list` | List configuration values | human, JSON, plain, jq, template |
| `chab config path` | Print the config and auth file paths | human, JSON, plain, jq, template |
| `chab config set` | Set one configuration value | human, plain |
| `chab contacts` | Run contact discovery operations | none (command family) |
| `chab contacts domain-search` | Start a domain contact search (preview) | human, JSON, plain |
| `chab contacts email-finder` | Start an email finder operation (preview) | human, JSON, plain |
| `chab contacts email-verify` | Start an email verification operation (preview) | human, JSON, plain |
| `chab convert` | Run conversion operations | none (command family) |
| `chab convert file` | Start a file conversion operation (preview) | human, JSON, plain |
| `chab credits` | Inspect team credits | none (command family) |
| `chab credits balance` | Show the available credit balance | human, JSON, plain, jq, template |
| `chab credits transactions` | List credit transactions | human, JSON, plain, jq, template |
| `chab doctor` | Check local setup and API readiness | human, JSON, plain, jq, template |
| `chab drive` | Work with connected drive files | none (command family) |
| `chab drive connections` | List drive connections (preview) | human, JSON, plain, jq, template |
| `chab drive folders` | Work with drive folders | none (command family) |
| `chab drive folders create` | Create a drive folder (preview) | human, JSON, plain |
| `chab drive items` | Work with drive items | none (command family) |
| `chab drive items download` | Download drive item bytes (preview) | human, JSON, plain, jq, template |
| `chab drive items export` | Export a drive-native document (preview) | human, JSON, plain |
| `chab drive items list` | List drive items (preview) | human, JSON, plain, jq, template |
| `chab drive items move` | Move or rename a drive item (preview) | human, JSON, plain |
| `chab drive items show` | Show drive item metadata (preview) | human, JSON, plain, jq, template |
| `chab drive items trash` | Move a drive item to trash (preview) | human, JSON, plain |
| `chab drive items update` | Replace drive item bytes (preview) | human, JSON, plain |
| `chab drive items upload` | Upload a stored source to drive (preview) | human, JSON, plain |
| `chab drive permissions` | Work with drive permissions | none (command family) |
| `chab drive permissions update` | Update drive item permissions (preview) | human, JSON, plain |
| `chab drive search` | Search drive item names (preview) | human, JSON, plain, jq, template |
| `chab errors` | List public API error codes | human, JSON, plain, jq, template |
| `chab examples` | Run example operations | none (command family) |
| `chab examples echo` | Echo a setup payload | human, JSON, plain |
| `chab files` | Work with stored files | none (command family) |
| `chab files delete` | Delete a stored file (preview) | human, JSON, plain |
| `chab files download` | Download stored file bytes (preview) | human, JSON, plain, jq, template |
| `chab files list` | List stored files (preview) | human, JSON, plain, jq, template |
| `chab files resume` | Resume a stored-file upload (preview) | human, JSON, plain |
| `chab files show` | Show stored file metadata (preview) | human, JSON, plain, jq, template |
| `chab files upload` | Upload a stored file (preview) | human, JSON, plain |
| `chab files wait` | Wait for stored file readiness (preview) | human, JSON, plain, jq, template |
| `chab health` | Show public API health | human, JSON, plain, jq, template |
| `chab help` | Help about any command | help |
| `chab llm` | Run model operations | none (command family) |
| `chab llm embeddings` | Start an embeddings operation (preview) | human, JSON, plain |
| `chab llm generate` | Start an LLM generation operation (preview) | human, JSON, plain |
| `chab llm models` | List available LLM models (preview) | human, JSON, plain |
| `chab login` | Authorize and store an API or guest trial key | human, plain |
| `chab logout` | Remove the stored credential for the active profile | human, plain |
| `chab mail` | Work with connected mail | none (command family) |
| `chab mail attachments` | Work with message attachments | none (command family) |
| `chab mail attachments download` | Download a mail attachment (preview) | human, JSON, plain, jq, template |
| `chab mail connections` | List mail connections (preview) | human, JSON, plain, jq, template |
| `chab mail drafts` | Work with API-owned mail drafts | none (command family) |
| `chab mail drafts attachments` | Work with draft attachments | none (command family) |
| `chab mail drafts attachments add` | Add a draft attachment (preview) | human, JSON, plain |
| `chab mail drafts attachments remove` | Remove a draft attachment (preview) | human, JSON, plain |
| `chab mail drafts create` | Create a mail draft (preview) | human, JSON, plain |
| `chab mail drafts discard` | Discard a mail draft (preview) | human, JSON, plain |
| `chab mail drafts send` | Send a mail draft (preview) | human, JSON, plain |
| `chab mail drafts show` | Show a mail draft (preview) | human, JSON, plain, jq, template |
| `chab mail drafts update` | Update a mail draft (preview) | human, JSON, plain |
| `chab mail folders` | List mail folders (preview) | human, JSON, plain, jq, template |
| `chab mail messages` | Work with mail messages | none (command family) |
| `chab mail messages body` | Read a mail message body (preview) | human, JSON, plain, jq, template |
| `chab mail messages state` | Update mail message state (preview) | human, JSON, plain |
| `chab mail personas` | List mail personas (preview) | human, JSON, plain, jq, template |
| `chab mail search` | Search connected mail (preview) | human, JSON, plain, jq, template |
| `chab mail threads` | Work with mail threads | none (command family) |
| `chab mail threads list` | List mail threads (preview) | human, JSON, plain, jq, template |
| `chab mail threads show` | Show a mail thread (preview) | human, JSON, plain, jq, template |
| `chab mcp` | Run local MCP integrations | none (command family) |
| `chab mcp serve` | Serve Chab tools over MCP stdio | MCP stdio |
| `chab operations` | Inspect and recover Chab operations | none (command family) |
| `chab operations actions` | Inspect local action records | none (command family) |
| `chab operations actions list` | List local action records | human, JSON, plain |
| `chab operations actions show` | Show a local action record | human, JSON, plain |
| `chab operations artifact` | Fetch operation artifact metadata | human, JSON, plain, jq, template |
| `chab operations artifact download` | Download operation artifact bytes | human, JSON, plain, jq, template |
| `chab operations bulk-cancel` | Cancel operations by filter | human, JSON, plain |
| `chab operations cancel` | Cancel an operation | human, JSON, plain |
| `chab operations estimate` | Estimate an operation request | human, JSON, plain |
| `chab operations list` | List remote operations | human, JSON, plain, jq, template |
| `chab operations result` | Fetch operation result metadata | human, JSON, plain, jq, template |
| `chab operations resume` | Resume a local action | human, JSON, plain |
| `chab operations schema` | Show an embedded operation schema | human, JSON, plain |
| `chab operations show` | Show operation status | human, JSON, plain, jq, template |
| `chab operations start` | Start a supported operation by key | human, JSON, plain |
| `chab operations wait` | Wait for an operation to finish | human, JSON, plain, jq, template |
| `chab profile` | Manage connection profiles | none (command family) |
| `chab profile create` | Create a profile | human, plain |
| `chab profile delete` | Delete a profile | human, plain |
| `chab profile list` | List configured profiles | human, JSON, plain, jq, template |
| `chab profile show` | Show a profile's settings | human, JSON, plain, jq, template |
| `chab profile use` | Switch the current profile | human, plain |
| `chab project` | Manage projects | none (command family) |
| `chab project archive` | Archive a project | human, JSON, plain, jq, template |
| `chab project create` | Create a project | human, JSON, plain, jq, template |
| `chab project delete` | Delete a project | human, JSON, plain, jq, template |
| `chab project list` | List projects | human, JSON, plain, jq, template |
| `chab project pause` | Pause a project | human, JSON, plain, jq, template |
| `chab project resume` | Resume a project | human, JSON, plain, jq, template |
| `chab project show` | Show a project | human, JSON, plain, jq, template |
| `chab project update` | Update a project | human, JSON, plain, jq, template |
| `chab research` | Run research operations | none (command family) |
| `chab research deep` | Start a deep research operation (preview) | human, JSON, plain |
| `chab scrape` | Run scrape operations | none (command family) |
| `chab scrape dom` | Start a DOM scrape operation (preview) | human, JSON, plain |
| `chab scrape markdown` | Start a markdown scrape operation (preview) | human, JSON, plain |
| `chab screenshots` | Run screenshot operations | none (command family) |
| `chab screenshots url` | Start a URL screenshot operation (preview) | human, JSON, plain |
| `chab search` | Run search operations | none (command family) |
| `chab search serp` | Start a SERP search operation (preview) | human, JSON, plain |
| `chab search web` | Start a web search operation (preview) | human, JSON, plain |
| `chab seo` | Run SEO operations | none (command family) |
| `chab seo domains` | Run domain SEO operations | none (command family) |
| `chab seo domains backlinks` | Start a backlinks operation (preview) | human, JSON, plain |
| `chab seo domains overview` | Start a domain overview operation (preview) | human, JSON, plain |
| `chab seo keywords` | Run keyword operations | none (command family) |
| `chab seo keywords ideas` | Start a keyword ideas operation (preview) | human, JSON, plain |
| `chab seo keywords metrics` | Start a keyword metrics operation (preview) | human, JSON, plain |
| `chab setup` | Set up a verified API or guest trial profile | human, plain |
| `chab tokens` | Manage team API tokens | none (command family) |
| `chab tokens approvals` | Manage approval challenges | none (command family) |
| `chab tokens approvals create` | Create an approval challenge | human, JSON, plain, jq, template |
| `chab tokens approvals wait` | Wait for an approval proof | human, JSON, plain, jq, template |
| `chab tokens create` | Create an API token | human, JSON, plain, jq, template |
| `chab tokens list` | List API tokens | human, JSON, plain, jq, template |
| `chab tokens revoke` | Revoke an API token | human, JSON, plain, jq, template |
| `chab tokens show` | Show API token metadata | human, JSON, plain, jq, template |
| `chab tokens update` | Update an API token | human, JSON, plain, jq, template |
| `chab translate` | Run translation operations | none (command family) |
| `chab translate text-or-document` | Start a text or document translation (preview) | human, JSON, plain |
| `chab usage` | Show team API usage | human, JSON, plain, jq, template |
| `chab version` | Print build metadata | human, JSON, plain, jq, template |
| `chab webhooks` | Manage webhook endpoints and replays | none (command family) |
| `chab webhooks deliveries` | Manage webhook deliveries | none (command family) |
| `chab webhooks deliveries list` | List webhook deliveries | human, JSON, plain, jq, template |
| `chab webhooks deliveries replay` | Replay a webhook delivery | human, JSON, plain, jq, template |
| `chab webhooks deliveries show` | Show a webhook delivery | human, JSON, plain, jq, template |
| `chab webhooks endpoints` | Manage webhook endpoints | none (command family) |
| `chab webhooks endpoints create` | Create a webhook endpoint | human, JSON, plain, jq, template |
| `chab webhooks endpoints delete` | Delete a webhook endpoint | human, JSON, plain, jq, template |
| `chab webhooks endpoints list` | List webhook endpoints | human, JSON, plain, jq, template |
| `chab webhooks endpoints rotate-secret` | Rotate a webhook secret | human, JSON, plain, jq, template |
| `chab webhooks endpoints show` | Show a webhook endpoint | human, JSON, plain, jq, template |
| `chab webhooks endpoints update` | Update a webhook endpoint | human, JSON, plain, jq, template |
| `chab webhooks replays` | Manage webhook replay windows | none (command family) |
| `chab webhooks replays create` | Create a webhook replay window | human, JSON, plain, jq, template |
| `chab whoami` | Show the authenticated team or guest principal | human, JSON, plain, jq, template |
