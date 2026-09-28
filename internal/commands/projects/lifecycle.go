package projects

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

const (
	pauseStatus   = "paused"
	resumeStatus  = "active"
	archiveStatus = "archived"
)

// lifecycleCommandOptions keeps the user-facing intent beside each exported
// constructor while the private constructor owns the shared request behavior.
type lifecycleCommandOptions struct {
	use     string
	short   string
	long    string
	action  string
	status  string
	example string
}

// NewPauseCommand builds chab project pause <project-id>.
func NewPauseCommand(f *cmdutil.Factory) *cobra.Command {
	return newLifecycleCommand(f, lifecycleCommandOptions{
		use:     "pause <project-id>",
		short:   "Pause a project",
		long:    lifecycleLong("Pause", pauseStatus),
		action:  "Paused",
		status:  pauseStatus,
		example: pauseExample,
	})
}

// NewResumeCommand builds chab project resume <project-id>.
func NewResumeCommand(f *cmdutil.Factory) *cobra.Command {
	return newLifecycleCommand(f, lifecycleCommandOptions{
		use:     "resume <project-id>",
		short:   "Resume a project",
		long:    lifecycleLong("Resume", resumeStatus),
		action:  "Resumed",
		status:  resumeStatus,
		example: resumeExample,
	})
}

// NewArchiveCommand builds chab project archive <project-id>.
func NewArchiveCommand(f *cmdutil.Factory) *cobra.Command {
	return newLifecycleCommand(f, lifecycleCommandOptions{
		use:     "archive <project-id>",
		short:   "Archive a project",
		long:    lifecycleLong("Archive", archiveStatus),
		action:  "Archived",
		status:  archiveStatus,
		example: archiveExample,
	})
}

// newLifecycleCommand builds a fixed-status Project mutation. It centralizes
// opaque-id handling and shared execution without exposing generic update
// fields that could widen the command's one-field request.
func newLifecycleCommand(f *cmdutil.Factory, options lifecycleCommandOptions) *cobra.Command {
	flags := &writeFlags{}
	cmd := &cobra.Command{
		Use:   options.use,
		Short: options.short,
		Long:  options.long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			// Project ids are opaque. Only blank input is rejected locally; the
			// API owns malformed, missing, and out-of-scope id responses.
			if id == "" {
				return &usageError{detail: "project id must not be empty"}
			}
			// Build the fixed field inside runWrite so an explicit idempotency
			// key is validated and registered before any payload work begins.
			return runWrite(cmd, f, flags, func() ([]payloadField, bool, error) {
				return []payloadField{{
					name:    "status",
					value:   options.status,
					display: options.status,
				}}, false, nil
			}, writeOp{
				kind:           kindLifecycle,
				method:         httpMethodPatch,
				action:         options.action,
				requestPath:    api.Path("projects", id),
				displayPath:    "/" + api.Path("projects", id),
				expectedStatus: options.status,
			})
		},
	}
	cmd.Example = options.example
	// The status is command-owned, so these leaves intentionally omit the
	// generic Project field and body flags.
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key (1-255 visible ASCII bytes); generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

// lifecycleLong keeps the shared safety and output contract identical while
// allowing each command to state its own action and fixed status.
func lifecycleLong(action, status string) string {
	return action + ` a project visible to the active team API key.

Identify the project by its opaque id. Every non-empty value is sent as one
escaped API path segment without local normalization. The command sends exactly
one PATCH body: {"status":"` + status + `"}. The API remains authoritative for
transition validation: the CLI does not read the project first or enforce a
transition graph.

Every real update sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, body fields, and idempotency behavior
without resolving credentials or contacting the API.

JSON output is the bare project object with fields id, name, description, url,
status, timezone, language, limit, automate, created_at, updated_at. Every field
comes from the API response, including status; the requested status is never
substituted into output. Default JSON without --include-meta has no request,
replay, idempotency, or metadata wrapper.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human summary, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

This reversible status update does not ask for confirmation; inherited --yes
does not change execution. Use chab project update when changing status together
with other project fields or when supplying a generic JSON change set.

Related commands:
  chab project show
  chab project update`
}

const pauseExample = `  chab project pause <project-id>
  chab project pause <project-id> --dry-run --json
  chab project pause <project-id> --jq .status
  chab project pause <project-id> --template '{{.id}}'`

const resumeExample = `  chab project resume <project-id>
  chab project resume <project-id> --idempotency-key <key> --json
  chab project resume <project-id> --jq .status
  chab project resume <project-id> --template '{{.id}}'`

const archiveExample = `  chab project archive <project-id>
  chab project archive <project-id> --dry-run --json
  chab project archive <project-id> --jq .status
  chab project archive <project-id> --include-meta --template '{{.meta.request_id}}'`
