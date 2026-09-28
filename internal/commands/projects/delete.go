package projects

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/redact"
)

// deleteFlags holds the local flags for project delete.
type deleteFlags struct {
	IdempotencyKey string
}

// deleteResult is the stable CLI-owned shape for project delete output. It
// mirrors only the two public fields, never request, rate-limit, retry, or
// replay metadata.
type deleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// UnmarshalJSON rejects incomplete success data instead of allowing missing or
// null fields to become valid-looking string and Boolean zero values.
func (r *deleteResult) UnmarshalJSON(data []byte) error {
	// Pointers keep a valid empty string or false distinct from a missing or null
	// field, both of which violate the successful response contract.
	var payload struct {
		ID      json.RawMessage `json:"id"`
		Deleted *bool           `json:"deleted"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	id, err := deleteOpaqueID(payload.ID)
	if err != nil {
		return fmt.Errorf("delete response field %q must be a string or number", "id")
	}
	if payload.Deleted == nil {
		return fmt.Errorf("delete response field %q must be a Boolean", "deleted")
	}
	r.ID = id
	r.Deleted = *payload.Deleted
	return nil
}

func deleteOpaqueID(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", fmt.Errorf("empty id")
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text, nil
	}
	var number json.Number
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&number); err == nil {
		if _, err := strconv.ParseFloat(number.String(), 64); err == nil {
			return number.String(), nil
		}
	}
	return "", fmt.Errorf("unsupported id")
}

// NewDeleteCommand builds chab project delete <project-id>.
func NewDeleteCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &deleteFlags{}
	cmd := &cobra.Command{
		Use:   "delete <project-id>",
		Short: "Delete a project",
		Long:  deleteLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, f, flags, args[0])
		},
	}
	cmd.Example = deleteExample
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key (1-255 visible ASCII bytes); generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

// runDelete enforces the destructive-delete ordering. The sequence is part of
// the command contract: local validation, then dry-run, then confirmation, then
// credentials and HTTP. Confirmation precedes credential lookup so a refused
// destructive action stays a local abort and never reaches auth resolution.
func runDelete(cmd *cobra.Command, f *cmdutil.Factory, flags *deleteFlags, id string) (runErr error) {
	// Project ids are opaque. Only empty input is rejected locally; the API owns
	// malformed, missing, deleted, cross-team, and out-of-scope ids.
	if id == "" {
		return &usageError{detail: "project id must not be empty"}
	}

	explicitKey := cmd.Flags().Changed("idempotency-key")
	if explicitKey {
		// Validate explicit keys before confirmation, credentials, dry-run
		// rendering, or HTTP, so an invalid key is local and never previewed.
		if err := api.ValidateIdempotencyKey(flags.IdempotencyKey); err != nil {
			return err
		}
		// Once valid, treat the key as sensitive in every later error and value.
		f.RegisterSecret(flags.IdempotencyKey)
		defer func() {
			runErr = f.RedactError(runErr)
		}()
	}

	requestPath := api.Path("projects", id)

	if dryRunEnabled(cmd) {
		// Nothing that confirms, resolves credentials, or builds an HTTP client
		// may move above this branch; dry-run is an offline preview.
		preview := output.DryRunPreview{
			Method:      "DELETE",
			Path:        f.RedactValue("/" + requestPath),
			Idempotency: dryRunIdempotency(explicitKey),
		}
		return f.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: func(w io.Writer) { preview.Render(w) },
			Plain:  preview.RenderPlain,
		})
	}

	// Use a display-only copy so redaction and quoting cannot change the path
	// sent to the API. Quoting also keeps control bytes from reaching a terminal.
	displayID := f.RedactValue(redact.String(id))
	question := fmt.Sprintf("Delete project %q? If it exists, this removes it from the team.", displayID)
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), question); err != nil {
		return err
	}

	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	cred, findings, err := f.Credential(rt)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return fmt.Errorf("%w; run \"chab login\" or set CHAB_API_KEY", err)
	}
	client, err := f.APIClientConfigured(rt, cred, cmd, func(opts *api.Options) {
		// Delete ids are privacy-sensitive request targets. Keep the original id
		// on the wire while removing its raw and escaped forms from debug and
		// error diagnostics produced by the API runtime.
		opts.SecretValues = append(opts.SecretValues, id)
	})
	if err != nil {
		return err
	}

	idem := api.AutoIdempotency()
	if explicitKey {
		idem = api.ExplicitIdempotency(flags.IdempotencyKey)
	}

	var wrapped struct {
		Project *deleteResult `json:"project"`
	}
	meta, err := client.Delete(cmd.Context(), requestPath, nil, nil, idem, &wrapped)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	if wrapped.Project == nil {
		return &api.ProtocolError{Detail: "project delete response missing data.project", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	result := *wrapped.Project
	result.ID = f.RedactValue(result.ID)

	summary := output.MutationSummary{
		Action:   "Deleted",
		Resource: "project",
		Name:     result.ID,
		Fields:   []output.Node{output.Field("deleted", strconv.FormatBool(result.Deleted))},
	}
	return f.WriteResultWithMeta(cmd, result, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) { summary.Render(w) },
		Plain:  summary.RenderPlain,
	})
}

const deleteLong = `Delete a project visible to the active team API key.

Identify the project by its opaque id (sent to the API as-is, only checked
non-empty locally), so malformed, missing, deleted, cross-team, and out-of-scope
ids all return the API's not_found response. A successful response is
the deleted id and a Boolean deleted flag.

Deleting a project is destructive. In an interactive terminal the command asks
for confirmation before sending the request; the prompt uses a quoted, redacted
copy of the id and makes no claim about whether the project exists. Pass --yes
to confirm without a prompt. In non-interactive or --no-prompt mode a real
delete without --yes fails locally before any credential lookup or request.

Every real delete sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, and idempotency behavior without
confirming, resolving credentials, or contacting the API.

JSON output is exactly one object with string id and Boolean deleted fields.
Without --include-meta, transport and replay context is not shown.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human summary, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab project list
  chab credits`

const deleteExample = `  chab project delete <project-id>
  chab project delete <project-id> --yes
  chab project delete <project-id> --dry-run
  chab project delete <project-id> --idempotency-key <key>
  chab project delete <project-id> --json
  chab project delete <project-id> --json --include-meta
  chab project delete <project-id> --jq .deleted
  chab project delete <project-id> --template '{{.id}}'`
