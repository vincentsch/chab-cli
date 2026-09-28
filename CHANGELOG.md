# Changelog

## 0.1.1 — Full public beta

- Discover the backend's complete 19-operation free catalog dynamically, with scoped file/model helpers and fresh authority checks. Private management and connected-account tools remain unavailable to guests.
- Keep confirmation required before effectful requests, and deny removed scopes, routes, paused starts and malformed compatibility evidence even with valid confirmed input.
- Refresh the standalone skills, command help and installation guidance for the full-beta catalog. Version0.1.0 remains immutable; these changes belong to the0.1.1 candidate, not a claim of deployment or publication.

## 2026-09-25 — Single-origin Chab beta preparation

- Use `https://www.chab.ai` for browser authorization and `https://www.chab.ai/v1` for API/local MCP defaults; keep saved custom destinations and the existing localhost safety behavior.
- Refresh the embedded backend contract, CLI help, setup guides, and agent skill to match the single-origin hosted MCP endpoint at `https://www.chab.ai/mcp` without claiming that it is live yet.
- Align the planned public release repository, installer, and update checks on `vincentsch/chab-cli`. The release has not been published.
- Preserve the previous `app.chab.ai` → `api.chab.ai/v1` derivation for saved legacy profiles so an upgrade never silently moves an existing bearer to another host.
- Keep the publish job inert on the private shared CLI repository; only `vincentsch/chab-cli` may publish Chab release assets.

## 2026-09-24 — Production app/API defaults

- Point a fresh `local` profile at `https://app.chab.ai` for browser authorization and `https://api.chab.ai/v1` for REST/local MCP, while preserving explicit and saved custom destinations. URL-omitting legacy profiles and auth-only state with a stored key retain localhost to avoid redirecting saved credentials; `local` remains a historical profile label. CLI help documents the separate API host; no release installer is claimed.
- Keep no-secret tests on closed loopback, retain production defaults after an empty auth file, and reject browser login when only a custom API URL is set without its matching app origin.
- Retain that browser-login guard after an API-only manual setup persists the implicit app origin; keep test fixtures with omitted app URLs on closed loopback and unsaved stored-key profiles on localhost after another profile is created.

## 2026-09-24 — Uncertain replay safety

- Keep an already-unknown action recoverable after unmarked or `settled` replay errors in typed CLI, raw API, file, billing purchase, and local MCP paths. Only explicit `released` proves a replay denial. Each explicit replay uses one physical request even on an unmarked 429 with `Retry-After` or a connection failure; first-submission retry/denial behavior is unchanged.

## 2026-09-24 — Final backend contract pin

- Pin the CLI's embedded Chab V1 OpenAPI contract to backend commit `75c39c3598fad1e7c3c5ef9882cd1e531f573b8a`, preserving its explicit idempotency-outcome response headers and refreshing checked provenance and the operation map.
- Show the backend's public `free_mode.state` and exact per-family `covered_operation_keys` in `chab health`; declare both in the local MCP `chab_health` output schema so healthy-family status is not mistaken for coverage of every operation.

## 2026-09-24 — Guest trial CLI and local MCP (review candidate)

- Accept browser-issued guest trial credentials in local profiles and show their principal, free-access context and status without inventing a team token.
- Limit guest local MCP discovery and invocation to the live backend compatibility allowlist; preserve REST idempotency and backend lifecycle errors while denying management, billing, connected and paid-only tools locally.
- Bind guest action receipts to `/v1/me` principal identity as well as profile, destination, route and request fingerprint, preventing credential rotation or claim from resuming another principal's action.
- Refresh the pinned OpenAPI fixture and generated manuals, operation map, help snapshots and standalone agent skill for guest-local versus hosted-MCP boundaries.
- Harden guest recovery after review: terminal API denials cannot replay as new work; scope-filtered discovery follows live identity with no tool-list cache; guest promotional balance, remediation, login identity, and local receipt ownership are visible without inventing team credits or tokens.
- Fail closed when a prepared receipt has no persisted submission outcome, even if a denial write failed; keep fresh first submission and persisted unknown-result recovery. Clarify that backend hosted `/mcp` guest OAuth is separate from the CLI's local REST bearer.
- Treat backend-marked idempotency denials as terminal, including 409/429 responses; preserve same-key reconciliation for unmarked admission uncertainty. Suppress automatic `Retry-After` replay of marked 429 responses before the action journal records the denial, and retain safe status/resume helpers during paused free spending.
- Apply that same terminal-denial rule to raw API and file upload/delete action journals in CLI and local MCP; denied file actions no longer advertise resume or resend a released key. Preserve resumability for genuinely unknown 5xx, transport and unmarked admission outcomes.
- Cover malformed marked 4xx error envelopes and reserve every unknown action durably as non-replayable before an effectful retry, so a failed denial write cannot leave a released key replayable. Extend the same rule to billing purchase recovery.
- Treat marked 503 denials as terminal across CLI and local MCP journals; unmarked server failures remain uncertain and recoverable with the original key.
- Synchronize the MCP file-readiness test's request counter and make the file-denial resume mock reach the denied-action journal guard, so recovery regressions are verified reliably.

## 2026-09-13 — Paired Chab setup guidance

- Corrected file, conversion, Mail and Drive command help to match current device permissions and approval requirements; conversion examples use the actual `fil_` file identifier.
- Updated the introduction to reflect the complete command and local MCP catalog, and removed obsolete template repository links and inherited hosted-acceptance claims.
- Documented the expanded device permission ceiling, explicit spending/project defaults, expiry, and the three management permissions that require a separately configured key.


## 2026-09-13

- Redact registered credentials from successful project read and local MCP
  operation result payloads, verify metadata checksums for local MCP
  stored-file and artifact downloads, and correct browser/device login scope
  guidance in the agent docs and skills.
- Align opt-in live release smoke scripts with Chab naming and the `/v1/me`
  identity endpoint used by the current public API.
- Expose registry-backed local MCP operation tools with exact path/query/body
  schemas, explicit local confirmation for billable or state-changing calls,
  safe upload/download path controls, recoverable action receipt tools and
  operation-ID tool names.
- Harden file and connected-data recovery: upload resume now checks the
  recorded credential/destination context, malformed upload receipts stay
  recoverable, pending file deletions resume through file lifecycle polling,
  action-ID resume continues accepted stored-file actions through the file
  resource lifecycle, accepted upload records honor `--wait` without
  re-uploading bytes, unavailable file downloads report lifecycle state and
  request id, Mail thread detail handles the object response shape, Mail send
  keeps its retained exact-retry window, signed redirect targets are redacted
  from transport failures, and connected preview operations are labelled in
  help/catalog metadata.
- Add stored-file commands for upload, list/show, scanner readiness polling,
  checksum-verified downloads, deletion receipts and upload recovery.
- Add safe artifact downloads from operation artifact metadata, with explicit
  local output paths, overwrite refusal, checksum verification and bounded
  direct or redirected byte transfers.
- Add connected Mail and Drive command families for Chab-supported reads,
  dry-run-capable mutations, revision-bound sends and accepted Drive operations,
  while leaving unsupported provider-native behavior out of the active surface.
- Regenerate command manuals, root help, agent command reference and the
  operation map for the file and connected-data surface, including local MCP
  bindings for contract-backed operation tools.
- Add first-class management commands for projects, usage, billing context,
  credit packages, purchase lifecycle, auto-recharge settings, team API tokens,
  management approvals, webhook endpoints, deliveries and replay windows.
- Harden secret-producing workflows for token creation, approval proof retrieval
  and webhook signing-secret creation/rotation: each reserves a private output
  path before the request, disables automatic retries for one-time values and
  prints only safe receipts.
- Port project commands to Chab response wrappers, cursor-only list pagination,
  opaque project IDs, explicit nullable description/URL fields and idempotent
  create/update/delete mutations.
- Regenerate the operation map, manual command reference, agent command
  reference and help snapshots. High-risk billing, token and webhook secret
  operations are documented as CLI-only rather than local MCP tools.

## 2026-09-12

- Add durable operation workflows: embedded contract schemas, operation
  estimate/start/wait/result/artifact/cancel/resume commands, local action
  inspection and first-class native commands for search, SEO, business,
  contacts, scraping, screenshots, conversion, translation, LLM and research
  operations.
- Add a private action journal for recoverable operation submissions. The CLI
  records identity, destination, request hash, encoder version and idempotency
  key before live submission, serializes journal mutations with platform file
  locks, persists terminal `start --wait` outcomes and keeps bearer tokens and
  request bodies out of public action projections.
- Add the generated operation map and drift check, refresh command docs and
  help goldens, and route catalogued raw API operation submissions through the
  same acknowledgement, idempotency and recovery record path while preserving
  exact raw request bytes.
- Distinguish offline previews from authenticated server dry runs: request-body
  `dry_run: true` follows contract metadata, skips confirmation, journals and
  idempotency keys, and fails locally when the operation does not support server
  dry runs.
- Keep operation recovery durable across retries, waits and resumptions:
  long-poll waits keep transport headroom, accepted IDs are emitted before
  waiting, explicit-key reuse cannot fork incompatible records, resume
  preserves recorded routes and no-body cancellation semantics, synchronous
  receipts do not replay, raw acknowledgement follows billable metadata, action
  output preserves safe response metadata and offline schemas include referenced
  definitions.

- Local planning: created the Chab product worktree from CLI framework main
  `d23ae24ffc884625b1714b90bf02b68293fa25fd`, with independent Vroni planning,
  retained framework notes, explicit model routing and disabled automatic sync.
  Companion backend: `/home/vincent/vilt-saas-chab-ai`. Product implementation
  and acceptance evidence will be recorded as the tickets complete. No remote
  changes, provider calls, publication or deployment.

### Chab authentication and API runtime

- Port the executable, module, config/auth paths, environment variables,
  generated command docs, examples, installer/release surfaces and local MCP
  tools to Chab identity. Active commands now use the `/v1` Chab API base,
  root OAuth device endpoints, `CHAB_*` runtime settings and `chab_*` MCP names.
- Replace inherited auth/runtime assumptions with Chab compatibility checks,
  flat `/me` token context, required `retryable` error envelopes, flat cursor
  metadata, public `health` and `errors` commands, Chab credits reads and
  checked mock examples.
- Harden review findings across the runtime: raw API one-time-secret routes are
  refused, raw success output redacts request secrets, operation envelopes are
  accepted, login validates identity before persistence, OAuth throttling honors
  `Retry-After`, compatibility enforces minimum versions and cursor traversal
  rejects incomplete or looping pages.
- Tighten the final Chab runtime contracts: raw management-approval reads are
  private-output-only, operation envelopes reject null/conflicting variants,
  cursor metadata accepts omitted `prev_cursor` where the API contract permits
  it, OAuth browser login accepts narrowed consent while rejecting unrequested
  grants, and MCP whoami/error outputs now match the Chab API contract.
- Remove inherited Mail registrations and generated Mail docs from the active
  Chab surface. Active help goldens, pinned contract drift tests, checked
  examples and product-isolation tests now cover the retained Chab behavior.

### Connection recovery and private setup handoffs

- Add explicit uncertainty acknowledgement using the exact receipt's
  reconciliation revision, retained key and confirmation. It preserves partial
  evidence and never retries or certifies success. Live native/MCP checks pass.
- Add `mail connection handoff --action` for five private setup/recovery paths;
  passwords and provider consent remain in the signed-in browser. Strict output
  filtering rejects provider URLs, credentials and false completion. Local MCP
  now exposes 200 tools / 194 Mail; all five handoff live pairs pass.

### Reviewed provider connection commands

- Add six `mail connection` actions and exact receipt inspection, with seven
  matching local MCP tools (198 total / 192 Mail). Explicit confirmation, exact
  management revision and a retained idempotency key are required.
- Fixed typed receipts preserve partial effects without claiming provider sync
  completion. Full uncached Go race/vet passes; live provider QA is outstanding.

### Stored Mail diagnostics and reviewed safety rules (final review pending)

- Add `mail administration doctor` plus eight `mail safety` reads/writes and
  matching local MCP tools (191 total / 185 Mail). Diagnostics report only stored
  facts for separately approved manageable mailboxes, never a live-provider probe.
- Team sender allow/block and personal remote-content trust require their own
  capability and approval. Creation of personal trust derives the target from an
  approved stored message; it cannot accept a different user or arbitrary target.
- Strict typed input, explicit confirmation, revision-bound writes, secret-free
  offline previews, pagination and defensive output projections have native/MCP
  wire tests. Full uncached Go race and vet pass; live local checks are in progress.

### Private webhook signing-key handoff (automated QA passes; final review pending)

- Add `mail webhook create` and `rotate-secret`, plus matching local MCP tools:
  182total/176Mail. Authored endpoint configuration stays in a private0600 file;
  signing keys go only to a new private0600 JSONL journal, never model output.
- Reserve and sync the journal before exactly one non-replayable request. Refuse
  existing files and symlinks; retain uncertain journals instead of discarding
  a potentially issued key. Rotation binds the exact reviewed revision.
- Explicit confirmation, offline dry-run, no debug response leakage and typed
  safe failure receipts are covered by shared/native/MCP wire tests. Full uncached
  Go race and vet pass; documentation and agent reference regenerated.

### Reviewed webhook configuration (live native QA complete; final review pending)

- Add `mail webhook update`, `enable`, `disable` and `delete`, with matching
  local MCP tools (180 total / 174 Mail). Exact configuration revisions and
  explicit confirmation protect destination-bound changes and historical replay.
- Fix the advertised `--revision` flag for webhook commands, including retry.
  Metadata updates reject credential fields; all results use fixed receipts.
- All eight live read/write pairs and four read-only/revoked denial pairs pass.
  New native/MCP wire checks and full uncached race/vet pass. Generated command
  docs and agent reference are current; integrated Fable review remains pending.

### Webhook diagnostics and confirmed retry (live QA complete; final review pending)

- Add `mail webhook list`, `show`, `deliveries`, `delivery` and `retry`, with
  matching local MCP tools: 176 total / 170 Mail. Uses existing team webhook
  authority, not personal mailbox consent. No duplicate webhook infrastructure.
- Subscription lists preserve page-number pagination; delivery lists use cursors.
  Diagnostic projections omit endpoint paths, secrets, payloads and receiver text.
- Retry requires exact revision, explicit confirmation and an idempotency key.
  Fixed receipts verify the target and distinguish queued/waiting from delivered.
  Offline and CLI/MCP wire regressions and full uncached race/vet pass.
  All six live native/MCP pairs pass, including cached retry, read-only replay
  denial and revoked-key denial. The disposable fixture is removed.

### Cancellation test synchronization

- Wait for a complete child PID before canceling the fake build, preventing a
  race with shell output-file creation. The test still verifies descendant
  termination and now always cancels if its setup deadline is reached.

### Mailbox administration operations (live QA complete; final review pending)

- Nine `mail administration` commands and matching MCP tools bring the catalog
  to 171 tools / 165 Mail. Complete grants, signatures, eligible alias activation
  and revision-bound default styles use separate mailbox-settings consent.
- All twelve live native/MCP pairs pass, as do their withdrawn-consent and
  revoked-key denial checks. Offline tests reject malformed IDs, nested/oversized
  grants, identity overrides and missing style revisions without network access.
- Focused tests and the prior full uncached race pass; integrated review is pending.

### Personal notification preference operations (live QA complete; final review pending)

- Added five `mail preferences` commands and matching local MCP tools (162 total,
  156 Mail). Separate personal consent, typed offline validation, exact revisions
  and explicit confirmation apply. Timezone changes warn that they affect the
  entire account; enabling digests warns about future notification email.
- Full uncached race checks pass after refreshing explicit inventories and
  generated documentation. All five live native/MCP operations and all cached
  withdrawal/revocation checks pass. Disposable member/settings and key cleaned up.

### Inbox-group operations (live QA complete; final review pending)

- Added eleven `mail collection` and matching local MCP operations, taking the
  catalog to 157 tools / 151 Mail tools. Typed offline input, explicit approval,
  exact row/order receipts and approved-subset membership semantics are shared.
- All eleven live native/MCP pairs pass. Withdrawal and revocation deny reads and
  cached metadata/membership receipts. Disposable groups and key cleaned up;
  original group and mailboxes remain intact. Integrated review remains pending.

- Follow-up sequence and run lists now show state, next/scheduled time and
  blocking reason instead of an unhelpful missing label. Full tests/race/vet,
  generated docs/examples and uncached packaging checks pass. Govulncheck reports
  zero reachable vulnerabilities; three unused module findings remain reported.

## Unreleased

- Added 12 saved view/search commands and MCP tools, including exact lifecycle,
  complete ordering and open pagination/coverage. Writes use shared local schemas,
  private offline dry-run and explicit approval. Workspace permission never grants
  email access. Catalog now has 146 tools, 140 Mail; live QA is in progress.

- Added field values/history/set/clear commands and MCP tools, with typed
  private values, exact target/field revisions and independently required
  CRM/content/mailbox-administration access. Clear retains history. Full
  uncached Go race and all 16 live native/MCP pairs pass. Added explicit
  contact/company conversations commands with approved-only counts and up to
  20 opaque links. Current catalog: 134 tools total, 128 Mail.

- Added 18 custom-field definition/option/order commands and MCP tools (128 total,
  122 Mail), with explicit Custom-only consent, confirmation, exact revisions,
  strict offline input validation and private dry-run output. Permanent deletion
  requires the separately reviewed value/history impact revision. Per-operation
  native/MCP transport tests, help/catalog and generated docs pass. All 23 live
  local checks pass. Live QA caught and fixed field-order collection receipts
  incorrectly requiring a record ID; validation now binds the resource type,
  revision and exact ordered IDs, and transport mocks use the real shape.
  Target-value adapters are implemented in the entry above.
- Added contact/company permanent-delete commands and MCP tools (110 total,
  104 Mail). Exact trashed revision and confirmation required; irreversible
  content-free receipts replay without repeating deletion. Company contacts are
  unlinked, not deleted. Full uncached Go race passes; six live Sail/native/MCP
  checks pass with final consent/key revocation and complete fixture cleanup.
  Generated commands and agent reference are synchronized.
- CRM25operation acceptance is now green:full uncached Go race, vet, generated
  docs/examples and all29live native/MCP checks. Same-key replay causes no repeated
  effects; actual consent withdrawal denies cached writes. The temporary key19
  is revoked and all four owned CRM records are soft-deleted. Govulncheck finds
  zero reachable vulnerabilities, with three unused-module findings retained.
- Added twenty-five native CRM commands and matching local MCP tools (108 total,
  102 Mail), with separate explicit team-CRM consent and no implicit mailbox
  content access. Contact/company CRUD, lifecycle, email/domain identities,
  account links and merges use exact reviewed revisions and confirmed writes.
  Merges bind both records; domain auto-link effects are documented. All21CRM
  writes have native/MCP transport confirmation checks; focused tests pass.
  Generated command/reference docs are current. Full race and live Sail
  acceptance are running; internal assistant/custom fields/admin are not claimed
  complete by this CLI addition.
- Added native snippet/writing-style library commands and thirteen matching MCP
  tools (83 total,77Mail). Team-library access requires explicit personal consent
  and no mailbox selection. Writes require confirmation and exact revisions;
  dry-runs perform no IO or private input disclosure. Grant replacement uses PUT
  and a content-free receipt. Offline validation covers policy keys and sort
  bounds. Full Go race tests and docs/vet/examples pass. All thirteen operations
  passed against the local Sail API, including same-key MCP mutation replay.
- Added provider message state/action/commands/command operations to native CLI
  and local MCP (67tools,61Mail), with separate consent, exact revisions,
  conditional placement/label fields, mandatory confirmation and visible
  uncertain receipts. Public HTTP acceptance passed the bounded29-test suite;
  live API/CLI/MCP acceptance passed22checks against an isolated local IMAP
  mailbox, including8actions, replay/conflict/delete replay and key withdrawal/
  revocation. Full Go tests/race/vet/generated docs/executable examples pass.

### Local, 2026-09-06: Mail command and MCP integration (in progress)

- Added seven provider-folder operations: list, show, capabilities, confirmed
  create/rename/delete, and command status. Native CLI and MCP preserve source/
  folder revisions, stable retry keys and unresolved outcomes without automatic
  resubmission. Discovery:63tools/57Mail. Full Go tests/race/vet/generated docs/
  examples pass. Real local Dovecot acceptance passed12checks; withdrawal403 and
  revoked-key401 passed, temporary folder removed, no mail sent.

- Added native follow-up source/list/show/runs/create/update/action and matching
  MCP tools. Arming future email requires explicit confirmation; pause/cancel/
  target removal cannot change sending authority. Strict source/sequence
  revisions, stable retry keys, explicit takeover, validated dates/steps and
  body-free offline previews share one adapter contract. Discovery:56tools/50Mail.
  Full Go tests/race/vet/docs/examples pass. Live isolated acceptance passed19
  checks including withdrawal/takeover; canceled sequence, discarded original,
  both temporary keys revoked, no outbound mail or browser left open.

- Fixed human/plain collaboration list labels: follower names, one-line note
  bodies/deletion markers and reminder due dates no longer render as `<nil>`.
  Eight native output regression cases and full race/vet/docs passed. Reminder
  lists additionally disclose scope/state; timestamp schema and validation now
  reject invalid calendar/timezone or out-of-storage-range inputs locally.
- Added reminder list/show and confirmed create/reschedule/dismiss/delete with
  explicit personal/shared scope, revision protection and disclosed snooze/
  notification effects. Discovery:49tools/43Mail. Full race/vet/generated docs
  passed; live personal/shared lifecycle and cleanup passed12checks. Fable accepted
  the bounded reminder adapters with a shared backend date-range fix; targeted
  CLI/MCP/date-validation tests passed after corresponding input polish.
- Added follower list/show and confirmed revision-bound follow/unfollow, with
  separate personal authority visible in whoami. Discovery:44 tools (38 Mail).
  Live CLI/MCP replay and restored unfollow state passed; full race/vet/docs
  checks passed. Added note validation tables and native offline/live command
  tests for create/update/delete/read plus follower update.
- Added note list/show/candidates/create/update/redaction/mention-read commands
  and MCP tools, separate approval, explicit confirmation, revisions and bounded
  mention/note IDs. Discovery is now41 tools (35 Mail); latest checks in progress.
  Corrected workflow read output schemas and added notes to whoami.

- Added workflow state/candidates and confirmed single/bulk commands. Explicit
  per-row revisions and retained retry keys protect concurrent changes; machine
  output preserves partial failures. Live local CLI/MCP acceptance passed and
  the synthetic conversation was restored. Discovery now has 34 tools (28 Mail).
  Full race tests passed after workflow integration. Human whoami now names
  sending, AI and workflow permissions separately from content and drafts.
- Added Mail AI generation, result readback, cancellation and explicit cached
  preview application in native CLI and MCP. Discovery now exposes 30 tools
  (24 Mail). Generation preserves pending status and retry identity; apply also
  requires draft revision and explicit confirmation. A real-provider local
  generation/replay/apply/readback check passed without sending mail.
- Added five native Mail draft mutations and matching MCP tools: create, update,
  discard, attach and detach. Shared validation rejects identity overrides;
  explicit retry keys and revisions prevent silent overwrites. Offline dry runs
  omit private bodies, file uploads are bounded, and destructive operations
  require confirmation. MCP annotations distinguish writes from reads.
- Added native prepared-send, confirmed-submit, outcome and scheduled-cancel
  commands and matching MCP tools. Separate send delegation, warning fingerprints,
  explicit confirmation and retained retry keys preserve the public send boundary.
  Uncertain results remain visibly uncertain; no automatic replacement send.
- At outbound integration, protocol discovery included 26 tools (20 Mail). The full race suite and
  vet passed after outbound integration; help and command references updated.
  Real local CLI schedule/submit/replay and MCP cancellation passed; provider
  inspection confirmed no canceled recipient copy and no remaining outbox item.

- Thirteen native Mail commands cover mailbox/conversation summaries and delegated
  message lists, summaries, sanitized bodies, search and private source/attachment
  downloads, plus draft list/detail and granted persona discovery.
  Eleven read-only Mail MCP tools use
  the same executable operation definitions and public API client.
- Added validated opaque cursor pagination without weakening legacy page
  validation; continuation is explicit and private content is never bulk fetched
  implicitly. Effective Mail consent survives CLI and MCP whoami projections and
  is shown by human whoami output. Search uses read-only POST and reports coverage.
- Downloads are size-bounded, reject redirects and publish private files without
  overwriting any existing target. Atomic publication requires a destination
  filesystem supporting same-directory hard links; unsupported filesystems fail
  safely. No binary bytes or credentials are printed to terminal output.
- Updated command catalog, generated help/manuals and installed agent command
  reference. Mail content requires both an explicit key capability and separate
  bound-user session approval; no command can grant itself access. Sending uses
  the separately approved and confirmed outbound commands described above.
- QA including the draft/persona additions: all-package race tests and vet passed,
  as did generated catalog/docs checks. The MCP whoami schema now includes draft
  access, with actual structured-output JSON-schema validation in its regression.
  Live personally approved draft/persona reads passed in CLI and MCP; withdrawing
  approval returned HTTP 403 and cleared effective whoami draft access.
  govulncheck v1.6.0 reports
  zero reachable vulnerabilities, with three findings in required modules not
  called by this code. Tests include body terminal controls in human/plain/jq/
  template modes and search filter/coverage transport. Mutation/tool parity remains open;
  this is not a full project completion or release claim. No push or deployment.
