package projects

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewUpdateCommand builds chab project update <project-id>.
func NewUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &writeFlags{}
	cmd := &cobra.Command{
		Use:   "update <project-id>",
		Short: "Update a project",
		Long:  updateLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			// Project ids are opaque. Only blank input is rejected locally; the
			// API owns malformed, missing, and out-of-scope id responses.
			if id == "" {
				return &usageError{detail: "project id must not be empty"}
			}
			// Keep assembly deferred so invalid explicit idempotency keys win
			// over body-file access or stdin reads.
			return runWrite(cmd, f, flags, func() ([]payloadField, bool, error) {
				return assembleWritePayload(cmd, f, flags)
			}, writeOp{
				kind:        kindUpdate,
				method:      httpMethodPatch,
				action:      "Updated",
				requestPath: api.Path("projects", id),
				displayPath: "/" + api.Path("projects", id),
			})
		},
	}
	cmd.Example = updateExample
	registerWriteFlags(cmd, flags)
	return cmd
}

const updateLong = `Update a project visible to the active team API key.

Identify the project by its opaque id. Every non-empty value is sent as one
escaped API path segment without local normalization. Build the change
set from field flags or from a JSON body. Set any of --name, --description,
--description-null, --url, --url-null, --status (active, paused, archived),
--timezone, --language, --limit, and --automate; only the flags you set are
sent, so --description "" clears to an empty value while --description-null
sends JSON null. Field flags cannot be combined with body input. Alternatively
pass the whole change set as JSON with --body '<json>' or --body-file <path>,
and use --body-file - to read it from stdin. At least one field must change: an
update with no fields, including --body '{}', fails locally before any request.

--limit is the project resource's integer limit field sent in the body; it is
not a pagination control here. --language is the project content language sent
in the body and is independent of the global --locale flag, which only sets the
request Accept-Language header.

Every real update sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, body fields, and idempotency behavior
without resolving credentials or contacting the API.

JSON output is the bare project object with fields id, name, description, url,
status, timezone, language, limit, automate, created_at, updated_at. Default
JSON without --include-meta has no request, replay, idempotency, or metadata
wrapper.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human summary, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab project show
  chab project list`

const updateExample = `  chab project update <project-id> --name "Renamed"
  chab project update <project-id> --status paused
  chab project update <project-id> --description ""
  chab project update <project-id> --description-null --url-null
  chab project update <project-id> --body '{"status":"archived"}'
  chab project update <project-id> --body-file project.json
  chab project update <project-id> --name "Renamed" --dry-run
  chab project update <project-id> --idempotency-key <key> --status paused
  chab project update <project-id> --status paused --json
  chab project update <project-id> --status paused --jq .status
  chab project update <project-id> --status paused --template '{{.id}}'
  chab project update <project-id> --status paused --include-meta --template '{{.meta.request_id}}'`
