# Chab Operation Map

Generated from the pinned public API fixture at backend revision `9f26a3b73961aebdf37055c723c63a8e2bccc87c`.

The Local MCP column shows registered tools for a signed-in API key. With a browser-issued guest trial credential, discovery and calls are constrained by the live `/v1/cli/compatibility` guest-local-MCP list: currently `chab_search_web` and `chab_contacts_email_verify`, plus identity, credits, current-operation status/result/artifact/download/cancel and local action helpers. Guest credentials do not authorize management, connected-account, billing, paid-only or hosted MCP tools.

| Operation | Method | Path | Owner | CLI | Local MCP | Availability | Behavior | Output | Scope | Idempotency |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `auth.me` | `GET` | `/v1/me` | runtime foundation | `chab whoami` | `chab_auth_me` | stable | sync | inline | `-` | no |
| `billing.auto_recharge.get` | `GET` | `/v1/billing/auto-recharge` | management workflows | `chab billing auto-recharge show` | `chab_billing_auto_recharge_get` | preview | sync | inline | `api:billing:read` | no |
| `billing.auto_recharge.update` | `PATCH` | `/v1/billing/auto-recharge` | management workflows | `chab billing auto-recharge update` | excluded: billing setting mutation requires deliberate human approval and confirmation | preview | sync | inline | `api:billing:write` | yes |
| `billing.get` | `GET` | `/v1/billing` | management workflows | `chab billing show` | `chab_billing_get` | preview | sync | inline | `api:billing:read` | no |
| `billing.packages.list` | `GET` | `/v1/billing/packages` | management workflows | `chab billing packages` | `chab_billing_packages_list` | preview | sync | inline | `api:billing:read` | no |
| `billing.purchases.create` | `POST` | `/v1/billing/purchases` | management workflows | `chab billing purchases create` | excluded: purchase creation can trigger billing effects and requires deliberate human approval | preview | sync | inline | `api:billing:write` | yes |
| `billing.purchases.get` | `GET` | `/v1/billing/purchases/{purchase_id}` | management workflows | `chab billing purchases show` | `chab_billing_purchases_get` | preview | sync | inline | `api:billing:read` | no |
| `billing.reconciliation.get` | `GET` | `/v1/billing/reconciliation` | management workflows | `chab billing reconciliation` | `chab_billing_reconciliation_get` | preview | sync | inline | `api:billing:read` | no |
| `business.details` | `POST` | `/v1/business/details` | operation workflows | `chab business details` | `chab_business_details` | preview | async | inline | `api:business:read` | yes |
| `business.search` | `POST` | `/v1/business/search` | operation workflows | `chab business search` | `chab_business_search` | preview | async | inline | `api:business:read` | yes |
| `cli.compatibility` | `GET` | `/v1/cli/compatibility` | runtime foundation | `chab auth login` | excluded: browser/device setup is a deliberate CLI flow | stable | sync | inline | `-` | no |
| `contacts.domain_search` | `POST` | `/v1/contacts/domain-search` | operation workflows | `chab contacts domain-search` | `chab_contacts_domain_search` | preview | async | inline | `api:contact:write` | yes |
| `contacts.email_finder` | `POST` | `/v1/contacts/email-finder` | operation workflows | `chab contacts email-finder` | `chab_contacts_email_finder` | preview | async | inline | `api:contact:write` | yes |
| `contacts.email_verify` | `POST` | `/v1/contacts/email-verify` | operation workflows | `chab contacts email-verify` | `chab_contacts_email_verify` | preview | async | inline | `api:contact:write` | yes |
| `convert.file` | `POST` | `/v1/convert/file` | operation workflows | `chab convert file` | `chab_convert_file` | preview | async | artifact | `api:convert:write` | yes |
| `credits.get` | `GET` | `/v1/credits` | runtime foundation | `chab credits balance` | `chab_credits_get` | preview | sync | inline | `api:credits:read` | no |
| `credits.transactions.list` | `GET` | `/v1/credits/transactions` | runtime foundation | `chab credits transactions` | `chab_credits_transactions_list` | preview | sync | inline | `api:credits:read` | no |
| `drive.connections.list` | `GET` | `/v1/drive/connections` | file and connected-data workflows | `chab drive connections` | `chab_drive_connections_list` | preview | sync | inline | `api:drive:read` | no |
| `drive.folders.create` | `POST` | `/v1/drive/folders` | file and connected-data workflows | `chab drive folders create` | `chab_drive_folders_create` | preview | async | inline | `api:drive:write` | yes |
| `drive.items.download` | `GET` | `/v1/drive/items/{item_id}/download` | file and connected-data workflows | `chab drive items download` | `chab_drive_items_download` | preview | sync | file | `api:drive:read` | no |
| `drive.items.export` | `POST` | `/v1/drive/items/{item_id}/export` | file and connected-data workflows | `chab drive items export` | `chab_drive_items_export` | preview | async | artifact | `api:drive:read` | yes |
| `drive.items.list` | `GET` | `/v1/drive/items` | file and connected-data workflows | `chab drive items list` | `chab_drive_items_list` | preview | sync | inline | `api:drive:read` | no |
| `drive.items.move` | `POST` | `/v1/drive/items/{item_id}/move` | file and connected-data workflows | `chab drive items move` | `chab_drive_items_move` | preview | async | inline | `api:drive:write` | yes |
| `drive.items.read` | `GET` | `/v1/drive/items/{item_id}` | file and connected-data workflows | `chab drive items show` | `chab_drive_items_read` | preview | sync | inline | `api:drive:read` | no |
| `drive.items.trash` | `POST` | `/v1/drive/items/{item_id}/trash` | file and connected-data workflows | `chab drive items trash` | `chab_drive_items_trash` | preview | async | inline | `api:drive:write` | yes |
| `drive.items.update` | `PATCH` | `/v1/drive/items/{item_id}` | file and connected-data workflows | `chab drive items update` | `chab_drive_items_update` | preview | async | inline | `api:drive:write` | yes |
| `drive.items.upload` | `POST` | `/v1/drive/items` | file and connected-data workflows | `chab drive items upload` | `chab_drive_items_upload` | preview | async | inline | `api:drive:write` | yes |
| `drive.permissions.update` | `POST` | `/v1/drive/items/{item_id}/permissions` | file and connected-data workflows | `chab drive permissions update` | `chab_drive_permissions_update` | preview | async | inline | `api:drive:write` | yes |
| `drive.search` | `POST` | `/v1/drive/search` | file and connected-data workflows | `chab drive search` | `chab_drive_search` | preview | sync | inline | `api:drive:read` | no |
| `errors.list` | `GET` | `/v1/errors` | runtime foundation | `chab errors` | `chab_errors` | stable | sync | inline | `-` | no |
| `examples.echo` | `POST` | `/v1/examples/echo` | operation workflows | `chab examples echo` | `chab_examples_echo` | stable | sync | inline | `api:examples:write` | yes |
| `files.create` | `POST` | `/v1/files` | file and connected-data workflows | `chab files upload` | `chab_files_create` | preview | sync | inline | `api:files:write` | yes |
| `files.delete` | `DELETE` | `/v1/files/{file_id}` | file and connected-data workflows | `chab files delete` | `chab_files_delete` | preview | sync | inline | `api:files:write` | yes |
| `files.download` | `GET` | `/v1/files/{file_id}/download` | file and connected-data workflows | `chab files download` | `chab_files_download` | preview | sync | file | `api:files:read` | no |
| `files.get` | `GET` | `/v1/files/{file_id}` | file and connected-data workflows | `chab files show` | `chab_files_get` | preview | sync | inline | `api:files:read` | no |
| `files.list` | `GET` | `/v1/files` | file and connected-data workflows | `chab files list` | `chab_files_list` | preview | sync | inline | `api:files:read` | no |
| `llm.embeddings` | `POST` | `/v1/llm/embeddings` | operation workflows | `chab llm embeddings` | `chab_llm_embeddings` | preview | async | inline | `api:llm:write` | yes |
| `llm.generate` | `POST` | `/v1/llm/generate` | operation workflows | `chab llm generate` | `chab_llm_generate` | preview | async | inline | `api:llm:write` | yes |
| `llm.models` | `GET` | `/v1/llm/models` | operation workflows | `chab llm models` | `chab_llm_models` | preview | sync | inline | `api:llm:read` | no |
| `mail.attachments.download` | `GET` | `/v1/mail/messages/{message_id}/attachments/{attachment_id}` | file and connected-data workflows | `chab mail attachments download` | `chab_mail_attachments_download` | preview | sync | file | `api:mail:read` | no |
| `mail.connections.list` | `GET` | `/v1/mail/connections` | file and connected-data workflows | `chab mail connections` | `chab_mail_connections_list` | preview | sync | inline | `api:mail:read` | no |
| `mail.draft_attachments.add` | `POST` | `/v1/mail/drafts/{draft_id}/attachments` | file and connected-data workflows | `chab mail drafts attachments add` | `chab_mail_draft_attachments_add` | preview | sync | inline | `api:mail:write` | yes |
| `mail.draft_attachments.remove` | `POST` | `/v1/mail/drafts/{draft_id}/attachments/{attachment_id}/remove` | file and connected-data workflows | `chab mail drafts attachments remove` | `chab_mail_draft_attachments_remove` | preview | sync | inline | `api:mail:write` | yes |
| `mail.drafts.create` | `POST` | `/v1/mail/drafts` | file and connected-data workflows | `chab mail drafts create` | `chab_mail_drafts_create` | preview | sync | inline | `api:mail:write` | yes |
| `mail.drafts.discard` | `POST` | `/v1/mail/drafts/{draft_id}/discard` | file and connected-data workflows | `chab mail drafts discard` | `chab_mail_drafts_discard` | preview | sync | inline | `api:mail:write` | yes |
| `mail.drafts.read` | `GET` | `/v1/mail/drafts/{draft_id}` | file and connected-data workflows | `chab mail drafts show` | `chab_mail_drafts_read` | preview | sync | inline | `api:mail:write` | no |
| `mail.drafts.update` | `PATCH` | `/v1/mail/drafts/{draft_id}` | file and connected-data workflows | `chab mail drafts update` | `chab_mail_drafts_update` | preview | sync | inline | `api:mail:write` | yes |
| `mail.folders.list` | `GET` | `/v1/mail/folders` | file and connected-data workflows | `chab mail folders` | `chab_mail_folders_list` | preview | sync | inline | `api:mail:read` | no |
| `mail.messages.body.read` | `GET` | `/v1/mail/messages/{message_id}/body` | file and connected-data workflows | `chab mail messages body` | `chab_mail_messages_body_read` | preview | sync | inline | `api:mail:read` | no |
| `mail.messages.send` | `POST` | `/v1/mail/drafts/{draft_id}/send` | file and connected-data workflows | `chab mail drafts send` | `chab_mail_messages_send` | preview | sync | inline | `api:mail:send` | yes |
| `mail.messages.state.update` | `POST` | `/v1/mail/messages/{message_id}/state` | file and connected-data workflows | `chab mail messages state` | `chab_mail_messages_state_update` | preview | sync | inline | `api:mail:write` | yes |
| `mail.personas.list` | `GET` | `/v1/mail/personas` | file and connected-data workflows | `chab mail personas` | `chab_mail_personas_list` | preview | sync | inline | `api:mail:read` | no |
| `mail.search` | `POST` | `/v1/mail/search` | file and connected-data workflows | `chab mail search` | `chab_mail_search` | preview | sync | inline | `api:mail:read` | no |
| `mail.threads.list` | `GET` | `/v1/mail/threads` | file and connected-data workflows | `chab mail threads list` | `chab_mail_threads_list` | preview | sync | inline | `api:mail:read` | no |
| `mail.threads.read` | `GET` | `/v1/mail/threads/{thread_id}` | file and connected-data workflows | `chab mail threads show` | `chab_mail_threads_read` | preview | sync | inline | `api:mail:read` | no |
| `operations.artifact` | `GET` | `/v1/operations/{id}/artifacts/{artifact_id}` | operation workflows | `chab operations artifact` | `chab_operations_artifact` | stable | sync | inline | `-` | no |
| `operations.artifact_download` | `GET` | `/v1/operations/{id}/artifacts/{artifact_id}/download` | operation workflows | `chab operations artifact download` | `chab_operations_artifact_download` | stable | sync | inline | `-` | no |
| `operations.bulk_cancel` | `POST` | `/v1/operations/cancel` | operation workflows | `chab operations bulk-cancel` | `chab_operations_bulk_cancel` | stable | sync | inline | `-` | yes |
| `operations.cancel` | `POST` | `/v1/operations/{id}/cancel` | operation workflows | `chab operations cancel` | `chab_operations_cancel` | stable | sync | inline | `-` | yes |
| `operations.estimate` | `POST` | `/v1/operations/estimate` | operation workflows | `chab operations estimate` | `chab_operations_estimate` | preview | sync | inline | `-` | no |
| `operations.get` | `GET` | `/v1/operations/{id}` | operation workflows | `chab operations show` | `chab_operations_get` | stable | sync | inline | `-` | no |
| `operations.list` | `GET` | `/v1/operations` | operation workflows | `chab operations list` | `chab_operations_list` | stable | sync | inline | `-` | no |
| `operations.result` | `GET` | `/v1/operations/{id}/result` | operation workflows | `chab operations result` | `chab_operations_result` | stable | sync | inline | `-` | no |
| `projects.create` | `POST` | `/v1/projects` | management workflows | `chab project create` | `chab_projects_create` | preview | sync | inline | `api:projects:create` | yes |
| `projects.delete` | `DELETE` | `/v1/projects/{project_id}` | management workflows | `chab project delete` | `chab_projects_delete` | preview | sync | inline | `api:projects:delete` | yes |
| `projects.get` | `GET` | `/v1/projects/{project_id}` | management workflows | `chab project show` | `chab_projects_get` | preview | sync | inline | `api:projects:read` | no |
| `projects.list` | `GET` | `/v1/projects` | management workflows | `chab project list` | `chab_projects_list` | preview | sync | inline | `api:projects:read` | no |
| `projects.update` | `PATCH` | `/v1/projects/{project_id}` | management workflows | `chab project update` | `chab_projects_update` | preview | sync | inline | `api:projects:update` | yes |
| `research.deep` | `POST` | `/v1/research/deep` | operation workflows | `chab research deep` | `chab_research_deep` | preview | async | artifact | `api:research:write` | yes |
| `scrape.dom` | `POST` | `/v1/scrape/dom` | operation workflows | `chab scrape dom` | `chab_scrape_dom` | preview | async | inline | `api:scrape:read` | yes |
| `scrape.markdown` | `POST` | `/v1/scrape/markdown` | operation workflows | `chab scrape markdown` | `chab_scrape_markdown` | preview | async | inline | `api:scrape:read` | yes |
| `screenshots.url` | `POST` | `/v1/screenshots/url` | operation workflows | `chab screenshots url` | `chab_screenshots_url` | preview | async | artifact | `api:screenshot:write` | yes |
| `search.serp` | `POST` | `/v1/search/serp` | operation workflows | `chab search serp` | `chab_search_serp` | preview | async | inline | `api:search:read` | yes |
| `search.web` | `POST` | `/v1/search/web` | operation workflows | `chab search web` | `chab_search_web` | preview | async | inline | `api:search:read` | yes |
| `seo.domains.backlinks` | `POST` | `/v1/seo/domains/backlinks` | operation workflows | `chab seo domains backlinks` | `chab_seo_domains_backlinks` | preview | async | inline | `api:seo:read` | yes |
| `seo.domains.overview` | `POST` | `/v1/seo/domains/overview` | operation workflows | `chab seo domains overview` | `chab_seo_domains_overview` | preview | async | inline | `api:seo:read` | yes |
| `seo.keywords.ideas` | `POST` | `/v1/seo/keywords/ideas` | operation workflows | `chab seo keywords ideas` | `chab_seo_keywords_ideas` | preview | async | inline | `api:seo:read` | yes |
| `seo.keywords.metrics` | `POST` | `/v1/seo/keywords/metrics` | operation workflows | `chab seo keywords metrics` | `chab_seo_keywords_metrics` | preview | async | inline | `api:seo:read` | yes |
| `system.health` | `GET` | `/v1/health` | runtime foundation | `chab health` | `chab_health` | stable | sync | inline | `-` | no |
| `tokens.create` | `POST` | `/v1/tokens` | management workflows | `chab tokens create` | excluded: returns one-time plaintext API token secret | preview | sync | inline | `api:tokens:write` | yes |
| `tokens.get` | `GET` | `/v1/tokens/{token_public_id}` | management workflows | `chab tokens show` | `chab_tokens_get` | preview | sync | inline | `api:tokens:read` | no |
| `tokens.list` | `GET` | `/v1/tokens` | management workflows | `chab tokens list` | `chab_tokens_list` | preview | sync | inline | `api:tokens:read` | no |
| `tokens.management_approvals.create` | `POST` | `/v1/management-approvals` | management workflows | `chab tokens approvals create` | `chab_tokens_management_approvals_create` | preview | sync | inline | `-` | yes |
| `tokens.management_approvals.get` | `GET` | `/v1/management-approvals/{approval_id}` | management workflows | `chab tokens approvals wait` | excluded: returns one-time management approval proof | preview | sync | inline | `-` | no |
| `tokens.revoke` | `POST` | `/v1/tokens/{token_public_id}/revoke` | management workflows | `chab tokens revoke` | `chab_tokens_revoke` | preview | sync | inline | `api:tokens:write` | yes |
| `tokens.update` | `PATCH` | `/v1/tokens/{token_public_id}` | management workflows | `chab tokens update` | `chab_tokens_update` | preview | sync | inline | `api:tokens:write` | yes |
| `translate.text_or_document` | `POST` | `/v1/translate` | operation workflows | `chab translate text-or-document` | `chab_translate_text_or_document` | preview | async | inline | `api:translate:write` | yes |
| `usage.get` | `GET` | `/v1/usage` | management workflows | `chab usage` | `chab_usage_get` | preview | sync | inline | `api:usage:read` | no |
| `webhooks.deliveries.get` | `GET` | `/v1/webhooks/deliveries/{delivery_id}` | management workflows | `chab webhooks deliveries show` | `chab_webhooks_deliveries_get` | preview | sync | inline | `api:webhooks:read` | no |
| `webhooks.deliveries.list` | `GET` | `/v1/webhooks/deliveries` | management workflows | `chab webhooks deliveries list` | `chab_webhooks_deliveries_list` | preview | sync | inline | `api:webhooks:read` | no |
| `webhooks.deliveries.replay` | `POST` | `/v1/webhooks/deliveries/{delivery_id}/replay` | management workflows | `chab webhooks deliveries replay` | `chab_webhooks_deliveries_replay` | preview | sync | inline | `api:webhooks:write` | yes |
| `webhooks.endpoints.create` | `POST` | `/v1/webhooks/endpoints` | management workflows | `chab webhooks endpoints create` | excluded: returns one-time webhook signing secret | preview | sync | inline | `api:webhooks:write` | yes |
| `webhooks.endpoints.delete` | `DELETE` | `/v1/webhooks/endpoints/{endpoint_id}` | management workflows | `chab webhooks endpoints delete` | `chab_webhooks_endpoints_delete` | preview | sync | inline | `api:webhooks:write` | yes |
| `webhooks.endpoints.get` | `GET` | `/v1/webhooks/endpoints/{endpoint_id}` | management workflows | `chab webhooks endpoints show` | `chab_webhooks_endpoints_get` | preview | sync | inline | `api:webhooks:read` | no |
| `webhooks.endpoints.list` | `GET` | `/v1/webhooks/endpoints` | management workflows | `chab webhooks endpoints list` | `chab_webhooks_endpoints_list` | preview | sync | inline | `api:webhooks:read` | no |
| `webhooks.endpoints.rotate_secret` | `POST` | `/v1/webhooks/endpoints/{endpoint_id}/rotate-secret` | management workflows | `chab webhooks endpoints rotate-secret` | excluded: returns one-time webhook signing secret | preview | sync | inline | `api:webhooks:write` | yes |
| `webhooks.endpoints.update` | `PATCH` | `/v1/webhooks/endpoints/{endpoint_id}` | management workflows | `chab webhooks endpoints update` | `chab_webhooks_endpoints_update` | preview | sync | inline | `api:webhooks:write` | yes |
| `webhooks.replays.create` | `POST` | `/v1/webhooks/replays` | management workflows | `chab webhooks replays create` | `chab_webhooks_replays_create` | preview | sync | inline | `api:webhooks:write` | yes |
