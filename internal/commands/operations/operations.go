package operationscmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

func NewOperationsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operations",
		Short: "Inspect and recover Chab operations",
		Long: `Inspect and recover Chab operations.

Operation starts create a private local action record before sending the first
effectful request. The action record stores recovery metadata and an
idempotency key, but never stores a bearer token or request body.
Journal directories are created private where the platform supports file
permissions. Record writes sync the data file before atomic publication;
directory metadata sync is best effort for portability.

Schemas are read from the pinned Chab API contract embedded in the binary, so
schema inspection works offline.

Related commands:
  chab search web
  chab credits balance`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(
		newOperationListCommand(f),
		newOperationShowCommand(f),
		newOperationEstimateCommand(f),
		newOperationStartCommand(f),
		newOperationWaitCommand(f),
		newOperationResultCommand(f),
		newOperationArtifactCommand(f),
		newOperationCancelCommand(f),
		newOperationBulkCancelCommand(f),
		newOperationSchemaCommand(f),
		newOperationResumeCommand(f),
		newActionCommand(f),
	)
	return cmd
}

func newOperationListCommand(f *cmdutil.Factory) *cobra.Command {
	var status, family, operationKey, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List remote operations",
		Long: `List remote operations for the authenticated team.

Filters are passed directly to the Chab operation history endpoint. Cursor
metadata can be exposed with --include-meta in machine-output modes.
With --json, --jq, or --template, --include-meta wraps the operation list
under data and safe response metadata under meta. Use --plain for tabular
operation output.

Related commands:
  chab operations show
  chab operations actions list`,
		Args: cobra.NoArgs,
		Example: `  chab operations list
  chab operations list --status running --operation-key search.web --json
  chab operations list --json --include-meta
  chab operations list --jq '.[].id'
  chab operations list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'
  chab operations list --plain`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			q := url.Values{}
			if status != "" {
				q.Set("status", status)
			}
			if family != "" {
				q.Set("family", family)
			}
			if operationKey != "" {
				q.Set("operation_key", operationKey)
			}
			if cursor != "" {
				q.Set("cursor", cursor)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, "operations", q, nil, api.IdempotencyNone)
			if err != nil {
				return err
			}
			return writeRawJSONValue(cmd, f, result.Data, result.Meta)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by operation status")
	cmd.Flags().StringVar(&family, "family", "", "filter by operation family")
	cmd.Flags().StringVar(&operationKey, "operation-key", "", "filter by operation key")
	cmd.Flags().StringVar(&cursor, "cursor", "", "cursor for the next page")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size requested from the API")
	return cmd
}

func newOperationShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <operation-id>",
		Short: "Show operation status",
		Long: `Show operation status without treating a failed remote operation as a local command failure.

With --json, --jq, or --template, --include-meta wraps the operation status
under data and safe response metadata under meta. Use --plain to print the raw
status value.

Related commands:
  chab operations wait
  chab operations result`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations show op_123
  chab operations show op_123 --json
  chab operations show op_123 --json --include-meta
  chab operations show op_123 --jq .status
  chab operations show op_123 --template '{{.status}}'
  chab operations show op_123 --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			raw, payload, meta, err := operations.Status(cmd.Context(), authn.Client, args[0], 0)
			if err != nil {
				return err
			}
			if err := operations.ValidateStatus(payload.Status, args[0]); err != nil {
				return err
			}
			return writeRawJSONValue(cmd, f, raw, meta)
		},
	}
}

func newOperationEstimateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &requestFlags{}
	cmd := &cobra.Command{
		Use:   "estimate <operation-key>",
		Short: "Estimate an operation request",
		Long: `Estimate an operation request.

Estimates are advisory. They validate and price the supplied input where the
backend supports generic estimates, but they are not atomic spending
guarantees for a later live request.
JSON output is the estimate data returned by the API. Use --plain for compact
script output.

Related commands:
  chab operations start
  chab operations schema`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations estimate search.web --input @request.json
  chab operations estimate search.web --input @request.json --json
  chab operations estimate search.web --input @request.json --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if err := ensureEstimateSupported(key); err != nil {
				return err
			}
			request, err := resolveRequest(cmd, f, flags, key)
			if err != nil {
				return err
			}
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			value, meta, err := operations.Estimate(cmd.Context(), authn.Client, key, request)
			if err != nil {
				return err
			}
			return writeRawJSONValue(cmd, f, value, meta)
		},
	}
	registerRequestFlags(cmd, flags, nil, false)
	return cmd
}

func newOperationStartCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &requestFlags{}
	cmd := &cobra.Command{
		Use:   "start <operation-key>",
		Short: "Start a supported operation by key",
		Long: `Start a supported operation by operation key using a JSON request body.

The start command is the generic entrypoint for supported operation starts. It
creates or reuses one local action record and one idempotency key for the
logical action before sending the effectful request.
Paid live submissions require confirmation through --yes or an interactive
prompt. Use --dry-run for a local preview that does not resolve credentials or
contact the API. For operations whose schema supports dry_run, an input body
with "dry_run": true performs authenticated server validation without creating
a local action record or sending an idempotency key. JSON output is one stable
object with action, remote operation and recovery fields. Use --plain to print
the accepted operation id or action id. In --no-prompt or non-interactive mode,
use --yes to acknowledge paid live work. Pass --idempotency-key to supply a
caller-owned replay key.

Related commands:
  chab operations resume
  chab operations wait
  chab operations schema`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations start search.web --input @request.json --yes
  chab operations start search.web --input @request.json --yes --wait --json
  chab operations start search.web --input @request.json --dry-run
  chab operations start examples.echo --input @request.json --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			op, err := operationByKey(args[0])
			if err != nil {
				return err
			}
			if err := ensureStartable(op); err != nil {
				return err
			}
			request, err := resolveRequest(cmd, f, flags, op.ID)
			if err != nil {
				return err
			}
			out, _, err := startOperation(cmd, f, op, request, flags)
			if flags.OfflinePreview {
				return err
			}
			if err != nil && !hasCommandOutput(out) {
				return err
			}
			if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
				return writeErr
			}
			return err
		},
	}
	registerRequestFlags(cmd, flags, nil, true)
	return cmd
}

func newOperationWaitCommand(f *cmdutil.Factory) *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "wait <operation-id>",
		Short: "Wait for an operation to finish",
		Long: `Wait for an operation to reach a terminal state.

The command uses the server long-poll query with waits capped at thirty
seconds. Interrupting this command stops only local waiting; it does not cancel
remote work.
With --json, timeout or interruption writes a recovery object before returning
nonzero. With --json, --jq, or --template, --include-meta wraps successful
terminal operation status under data and safe response metadata under meta. Use
--plain for raw terminal status output.

Related commands:
  chab operations cancel
  chab operations result`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations wait op_123
  chab operations wait op_123 --timeout 2m --json
  chab operations wait op_123 --json --include-meta
  chab operations wait op_123 --jq .status
  chab operations wait op_123 --template '{{.status}}'
  chab operations wait op_123 --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			raw, payload, meta, err := waitUntilTerminal(cmd, f, authn.Client, args[0], timeout)
			if err != nil {
				if reason, ok := waitRecoveryReason(err); ok {
					if writeErr := writeCommandOutput(cmd, f, waitRecoveryOutput(args[0], raw, payload, meta, reason)); writeErr != nil {
						return writeErr
					}
					return waitRecoveryErrorForReason(args[0], reason, err)
				}
				return err
			}
			if writeErr := writeRawJSONValue(cmd, f, raw, meta); writeErr != nil {
				return writeErr
			}
			if payload.Status != "succeeded" && payload.Status != "" {
				return &usageError{detail: fmt.Sprintf("operation %s finished with status %s", args[0], payload.Status)}
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum local wait duration")
	return cmd
}

func newOperationResultCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "result <operation-id>",
		Short: "Fetch operation result metadata",
		Long: `Fetch the retained result for a completed operation.

If the operation succeeded with result_available=false, Chab may return a
not-found or not-ready API error. Artifact bytes are handled by a later file
transfer workflow; this command returns result metadata and inline results.
With --json, --jq, or --template, --include-meta wraps the operation result
under data and safe response metadata under meta. Use --plain for raw result
output.

Related commands:
  chab operations wait
  chab operations artifact`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations result op_123
  chab operations result op_123 --json
  chab operations result op_123 --json --include-meta
  chab operations result op_123 --jq .result
  chab operations result op_123 --template '{{.result}}'
  chab operations result op_123 --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			raw, meta, err := operations.Result(cmd.Context(), authn.Client, args[0])
			if err != nil {
				return err
			}
			return writeRawJSONValue(cmd, f, raw, meta)
		},
	}
}

func newOperationArtifactCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact <operation-id> <artifact-id>",
		Short: "Fetch operation artifact metadata",
		Long: `Fetch metadata for a customer-visible operation artifact.

Use the download subcommand to write artifact bytes to an explicit new local
path. Metadata and byte downloads both resolve server-provided resource links
below the configured API base.
With --json, --jq, or --template, --include-meta wraps artifact metadata under
data and safe response metadata under meta. Use --plain for raw metadata
output.

Related commands:
  chab operations result
  chab operations artifact download`,
		Args: cobra.ExactArgs(2),
		Example: `  chab operations artifact op_123 art_123
  chab operations artifact op_123 art_123 --json
  chab operations artifact op_123 art_123 --json --include-meta
  chab operations artifact op_123 art_123 --jq .id
  chab operations artifact op_123 art_123 --template '{{.id}}'
  chab operations artifact op_123 art_123 --plain
  chab operations artifact download op_123 art_123 --output ./result.md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			raw, meta, err := operations.Artifact(cmd.Context(), authn.Client, args[0], args[1])
			if err != nil {
				return err
			}
			return writeRawJSONValue(cmd, f, raw, meta)
		},
	}
	cmd.AddCommand(newOperationArtifactDownloadCommand(f))
	return cmd
}

func newOperationArtifactDownloadCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &downloadFlags{MaxBytes: defaultTransferMaxBytes}
	cmd := &cobra.Command{
		Use:   "download <operation-id> <artifact-id>",
		Short: "Download operation artifact bytes",
		Long: `Download operation artifact bytes to an explicit new local path.

The command first reads trusted artifact metadata, resolves the documented
download_url below the configured API base, and then downloads bytes from the
canonical artifact download endpoint. Existing output files are refused. Safe
external HTTPS redirects are followed with Chab authorization and cookies
stripped from the redirected request. When a sha256:<hex> checksum is available
or supplied through --checksum, the file is published only after the completed
transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
  chab operations artifact
  chab operations result`,
		Args: cobra.ExactArgs(2),
		Example: `  chab operations artifact download op_123 art_123 --output ./result.md
  chab operations artifact download op_123 art_123 -o ./result.md --checksum sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --json
  chab operations artifact download op_123 art_123 -o ./result.md --jq .bytes
  chab operations artifact download op_123 art_123 -o ./result.md --template '{{.path}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAPIClient(cmd, f, false)
			if err != nil {
				return err
			}
			raw, meta, err := operations.Artifact(cmd.Context(), authn.Client, args[0], args[1])
			if err != nil {
				return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
			}
			link, checksum, err := downloadMetadata(raw, meta)
			if err != nil {
				return err
			}
			err = downloadToLocalPath(cmd, f, authn.Client, func(w io.Writer) (int64, api.ResponseMeta, error) {
				return authn.Client.DownloadAPILink(cmd.Context(), link, w, api.DownloadOptions{MaxBytes: flags.MaxBytes, FollowHTTPSRedirect: true})
			}, *flags, checksum)
			if err != nil {
				return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
			}
			return nil
		},
	}
	registerDownloadFlags(cmd, flags)
	return cmd
}

func newOperationCancelCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &requestFlags{}
	cmd := &cobra.Command{
		Use:   "cancel <operation-id>",
		Short: "Cancel an operation",
		Long: `Cancel an operation cooperatively.

Cancellation changes remote operation state and requires explicit
acknowledgement through --yes or an interactive confirmation. The local action
record is created before the cancel request and uses one stable idempotency key.
Use --plain for the accepted operation id or action id, and --json for the
stable action and recovery object. Pass --idempotency-key to supply a
caller-owned replay key. In --no-prompt or non-interactive mode, use --yes to
acknowledge cancellation.

Related commands:
  chab operations wait
  chab operations bulk-cancel`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations cancel op_123 --yes
  chab operations cancel op_123 --yes --json
  chab operations cancel op_123 --yes --plain
  chab operations cancel op_123 --idempotency-key <key> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			op, err := operationByKey("operations.cancel")
			if err != nil {
				return err
			}
			op.Path = "/v1/" + api.Path("operations", args[0], "cancel")
			out, _, err := startOperation(cmd, f, op, nil, flags)
			if err != nil && !hasCommandOutput(out) {
				return err
			}
			if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
				return writeErr
			}
			return err
		},
	}
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	return cmd
}

func newOperationBulkCancelCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &requestFlags{}
	cmd := &cobra.Command{
		Use:   "bulk-cancel",
		Short: "Cancel operations by filter",
		Long: `Cancel operations by request body filters.

Pass the documented operations.bulk_cancel request body with --input. This is
a remote state-changing action and requires --yes or an interactive
confirmation.
Use --plain for the accepted operation id or action id, and --json for the
stable action and recovery object. Pass --idempotency-key to supply a
caller-owned replay key. In --no-prompt or non-interactive mode, use --yes to
acknowledge cancellation.

Related commands:
  chab operations cancel
  chab operations list`,
		Args: cobra.NoArgs,
		Example: `  chab operations bulk-cancel --input @cancel.json --yes
  chab operations bulk-cancel --input @cancel.json --yes --json
  chab operations bulk-cancel --input @cancel.json --yes --plain`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			op, err := operationByKey("operations.bulk_cancel")
			if err != nil {
				return err
			}
			request, err := resolveRequest(cmd, f, flags, op.ID)
			if err != nil {
				return err
			}
			out, _, err := startOperation(cmd, f, op, request, flags)
			if err != nil && !hasCommandOutput(out) {
				return err
			}
			if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
				return writeErr
			}
			return err
		},
	}
	registerRequestFlags(cmd, flags, nil, false)
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	return cmd
}

func newOperationSchemaCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "schema <operation-key>",
		Short: "Show an embedded operation schema",
		Long: `Show the embedded request and result schema for an operation.

Schema inspection reads the pinned contract built into the binary. It does not
resolve a profile, read credentials, or contact the API.
Use --json to print the embedded schema object or --plain to print the
operation key.

Related commands:
  chab operations estimate
  chab operations start`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations schema search.web
  chab operations schema search.web --json
  chab operations schema search.web --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := chabcontract.MustLoad()
			doc, ok := registry.SchemaDocument(args[0])
			if !ok {
				return &usageError{detail: "unknown operation key " + args[0]}
			}
			return f.WriteResult(cmd, doc, cmdutil.HumanOutput{
				Render: func(w io.Writer) {
					fmt.Fprintf(w, "Operation: %s\nMethod: %s\nPath: %s\nAvailability: %s\nBehavior: %s\nOutput: %s\n", doc.OperationID, doc.Method, doc.Path, doc.Availability, doc.Behavior, doc.OutputKind)
					if len(doc.RequestSchema) > 0 {
						fmt.Fprintf(w, "Request schema: %s\n", string(doc.RequestSchema))
					}
					if doc.ResultSchemaRef != "" {
						fmt.Fprintf(w, "Result schema: %s\n", doc.ResultSchemaRef)
					}
				},
				Plain: func(data, prose io.Writer) { fmt.Fprintln(data, doc.OperationID) },
			})
		},
	}
}

func newOperationResumeCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &requestFlags{}
	cmd := &cobra.Command{
		Use:   "resume <action-id>",
		Short: "Resume a local action",
		Long: `Resume a local action from the private journal.

Unknown outcomes that have no accepted remote ID require the original request
input so the CLI can verify the same deterministic request bytes before it
replays with the stored idempotency key. Actions with an accepted operation ID
or a completed local receipt are reported without resubmission.
An existing prepared receipt has no persisted response and cannot be replayed:
the earlier request may have been denied while local persistence failed.
Stored-file upload and deletion actions continue through the stored-file
resource lifecycle and may return a file receipt rather than an operation
receipt.
Use --json for the stable action and recovery object and --plain for the
resumed operation id or action id.

Related commands:
  chab operations actions show
  chab operations wait`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations resume act_123 --input @request.json
  chab operations resume act_123 --json
  chab operations resume act_123 --input @request.json --json
  chab operations resume act_123 --input @request.json --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
			if err != nil {
				return err
			}
			store := operations.StoreForRuntime(rt, f.Clock())
			record, err := store.Load(args[0])
			if err != nil {
				return err
			}
			needsRequest := resumeNeedsRequest(record)
			requestProvided := resumeRequestProvided(cmd, flags)
			var request []byte
			if needsRequest || requestProvided {
				request, err = resolveRecordedRequest(cmd, f, flags, record)
				if err != nil {
					return err
				}
				if err := store.VerifyInput(record, request); err != nil {
					return err
				}
			}
			if err := store.Replayable(record); err != nil {
				return err
			}
			if record.State == operations.ActionCompleted && record.AcceptedOperationID == "" {
				return writeCommandOutput(cmd, f, outputFromRecord(record))
			}
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			if err := ensureRecordedActionContext(authn, record); err != nil {
				return err
			}
			if isFileLifecycleAction(record) && record.AcceptedOperationID != "" {
				return resumeFileLifecycleAction(cmd, f, store, record, authn)
			}
			if isPurchaseAction(record) {
				return resumePurchaseAction(cmd, f, store, record, request, authn)
			}
			if record.AcceptedOperationID != "" {
				raw, payload, meta, err := operations.Status(cmd.Context(), authn.Client, record.AcceptedOperationID, 0)
				if err != nil {
					return err
				}
				out := outputFromRecord(record)
				out.OperationID = record.AcceptedOperationID
				out.Server = raw
				out.RequestID = meta.RequestID
				out.Meta = operationResponseMeta(meta)
				statusErr := operations.ValidateStatus(payload.Status, record.AcceptedOperationID)
				var persistErr error
				if statusErr == nil {
					var updated operations.ActionRecord
					if operations.ActiveStatus(payload.Status) {
						updated, persistErr = store.MarkAccepted(record.ID, payload, meta)
					} else {
						updated, persistErr = store.MarkCompleted(record.ID, payload, meta)
					}
					if persistErr != nil {
						out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
					} else {
						projection := operations.Projection(updated)
						out.Action = &projection
						out.LocalPersistence = &operations.PersistenceState{State: "persisted"}
						out.LocalRecovery.State = string(updated.State)
					}
				}
				if operations.ActiveStatus(payload.Status) {
					out.LocalRecovery.CanResume = true
					out.LocalRecovery.KnownRemote = true
					out.LocalRecovery.ResumeHint = "chab operations wait " + record.AcceptedOperationID
				} else {
					out.LocalRecovery.CanResume = false
					out.LocalRecovery.KnownRemote = true
					out.LocalRecovery.ResumeHint = ""
				}
				if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
					return writeErr
				}
				if persistErr != nil {
					return fmt.Errorf("operation %s returned status %s but failed to persist local action state: %w", record.AcceptedOperationID, payload.Status, persistErr)
				}
				return statusErr
			}
			op, err := operationByKey(record.OperationKey)
			if err != nil {
				return err
			}
			op.Method = record.Method
			op.Path = record.Path
			op.RequiredScope = record.RequiredScope
			out, err := operations.Start(cmd.Context(), authn.Client, operations.StartInput{
				Operation:      op,
				RuntimeProfile: authn.Runtime.Profile,
				Destination:    authn.Runtime.APIBaseURL,
				TokenPublicID:  authn.Identity.TokenPublicID,
				PrincipalID:    authn.Identity.PrincipalID,
				EncoderVersion: record.EncoderVersion,
				RequestBytes:   request,
				IdempotencyKey: record.IdempotencyKey,
				Journal:        store,
			})
			if err != nil && !hasCommandOutput(out) {
				return err
			}
			if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
				return writeErr
			}
			return err
		},
	}
	registerRequestFlags(cmd, flags, nil, false)
	return cmd
}

type purchaseActionPayload struct {
	ID          string          `json:"id"`
	PackageID   string          `json:"package_id"`
	State       string          `json:"state"`
	NextCheckAt *string         `json:"next_check_at"`
	CreatedAt   string          `json:"created_at"`
	Raw         json.RawMessage `json:"-"`
}

func isPurchaseAction(record operations.ActionRecord) bool {
	return record.OperationKey == "billing.purchases.create" &&
		record.Method == http.MethodPost &&
		record.Path == "/v1/billing/purchases"
}

func resumePurchaseAction(cmd *cobra.Command, f *cmdutil.Factory, store operations.Store, record operations.ActionRecord, request []byte, authn authContext) error {
	var result api.RawResult
	var err error
	if record.AcceptedOperationID != "" {
		result, err = authn.Client.DoRaw(cmd.Context(), http.MethodGet, api.Path("billing", "purchases", record.AcceptedOperationID), nil, nil, api.IdempotencyNone)
	} else {
		var requestBody any
		if request != nil {
			requestBody = api.ExactJSONBody(request)
		}
		if err := store.BeginReplay(record); err != nil {
			return err
		}
		result, err = authn.Client.DoRaw(cmd.Context(), record.Method, operationEndpointPath(record.Path), nil, requestBody, api.JournaledIdempotency(record.IdempotencyKey, true))
	}
	if err != nil {
		if record.AcceptedOperationID == "" {
			if operations.DefinitiveAdmissionDenialForSubmission(err, true) {
				if markErr := store.MarkDenied(record.ID, err); markErr != nil {
					return fmt.Errorf("purchase denied and local denial could not be persisted: %w (persistence: %v)", err, markErr)
				}
			} else if markErr := store.MarkUnknown(record.ID, err); markErr != nil {
				return fmt.Errorf("purchase outcome uncertain and local recovery could not be persisted: %w (persistence: %v)", err, markErr)
			}
		}
		return err
	}
	payload, err := decodePurchaseActionPayload(result.Data, result.Meta)
	if err != nil {
		if record.AcceptedOperationID == "" {
			_ = store.MarkUnknown(record.ID, err)
		}
		return err
	}
	operationPayload := operations.OperationPayload{ID: payload.ID, OperationKey: record.OperationKey, Family: "billing", Status: payload.State}
	var updated operations.ActionRecord
	if purchaseActionTerminal(payload.State) {
		updated, err = store.MarkCompleted(record.ID, operationPayload, result.Meta)
	} else {
		updated, err = store.MarkAccepted(record.ID, operationPayload, result.Meta)
	}
	persisted := updated
	if err != nil {
		persisted = record
	}
	out := purchaseActionOutput(persisted, payload, result.Data, result.Meta)
	if err != nil {
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
	} else {
		out.LocalPersistence = &operations.PersistenceState{State: "persisted"}
	}
	if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return fmt.Errorf("purchase %s returned state %s but failed to persist local action state: %w", payload.ID, payload.State, err)
	}
	return nil
}

func decodePurchaseActionPayload(raw json.RawMessage, meta api.ResponseMeta) (purchaseActionPayload, error) {
	var payload purchaseActionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return purchaseActionPayload{}, &api.ProtocolError{Detail: "purchase response data does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: err, Meta: meta}
	}
	payload.Raw = append(json.RawMessage(nil), raw...)
	if payload.ID == "" {
		return purchaseActionPayload{}, &api.ProtocolError{Detail: "purchase response missing id", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	if payload.State == "" {
		return purchaseActionPayload{}, &api.ProtocolError{Detail: "purchase response missing state", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return payload, nil
}

func purchaseActionOutput(record operations.ActionRecord, payload purchaseActionPayload, raw json.RawMessage, meta api.ResponseMeta) operations.CommandOutput {
	projection := operations.Projection(record)
	canResume := purchaseActionCanResume(payload.State)
	out := operations.CommandOutput{
		Action:      &projection,
		OperationID: payload.ID,
		Server:      raw,
		RequestID:   meta.RequestID,
		Meta:        operationResponseMeta(meta),
		LocalRecovery: operations.RecoveryStatus{
			State:       string(record.State),
			CanResume:   canResume,
			KnownRemote: true,
		},
	}
	if canResume {
		out.LocalRecovery.ResumeHint = "chab billing purchases wait " + payload.ID
	}
	return out
}

func purchaseActionTerminal(state string) bool {
	return state == "fulfilled" || state == "failed"
}

func purchaseActionCanResume(state string) bool {
	return state == "pending" || state == "pending_reconciliation"
}

func resolveRecordedRequest(cmd *cobra.Command, f *cmdutil.Factory, flags *requestFlags, record operations.ActionRecord) ([]byte, error) {
	switch record.EncoderVersion {
	case "", operations.EncoderVersion:
		if flags.Input == "" {
			if len(flags.Set) == 0 && !flagChanged(cmd, "max-credits") && record.RequestSHA256 == operations.RequestSHA256(nil) {
				return nil, nil
			}
			return nil, &usageError{detail: "--input is required to resume an action"}
		}
		return resolveRequest(cmd, f, flags, record.OperationKey)
	case operations.RawEncoderVersion:
		if len(flags.Set) > 0 || flagChanged(cmd, "max-credits") {
			return nil, &usageError{detail: "raw action recovery requires byte-identical --input without --set or convenience flags"}
		}
		if flags.Input == "" {
			return nil, nil
		}
		raw, err := readInput(cmd, f, flags.Input)
		if err != nil {
			return nil, err
		}
		trimmed := bytes.TrimSpace(raw)
		if !json.Valid(trimmed) {
			return nil, &usageError{detail: "raw action input must be valid JSON"}
		}
		return append([]byte(nil), trimmed...), nil
	default:
		return nil, &usageError{detail: fmt.Sprintf("action %s uses unsupported encoder version %q", record.ID, record.EncoderVersion)}
	}
}

func resumeNeedsRequest(record operations.ActionRecord) bool {
	return record.AcceptedOperationID == "" && record.State != operations.ActionCompleted
}

func resumeRequestProvided(cmd *cobra.Command, flags *requestFlags) bool {
	return flags.Input != "" || len(flags.Set) > 0 || flagChanged(cmd, "max-credits")
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

func outputFromRecord(record operations.ActionRecord) operations.CommandOutput {
	projection := operations.Projection(record)
	out := operations.CommandOutput{
		Action:      &projection,
		OperationID: record.AcceptedOperationID,
		RequestID:   record.RequestID,
		LocalRecovery: operations.RecoveryStatus{
			State:       string(record.State),
			CanResume:   record.State != operations.ActionCompleted,
			KnownRemote: record.AcceptedOperationID != "",
		},
		LocalPersistence: &operations.PersistenceState{State: "persisted"},
	}
	if record.AcceptedOperationID != "" && record.State != operations.ActionCompleted {
		out.LocalRecovery.ResumeHint = "chab operations wait " + record.AcceptedOperationID
	}
	return out
}

func newActionCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "Inspect local action records",
		Long: `Inspect local action records.

Action output is a public projection. It omits stored idempotency keys, request
hashes and other private recovery material.
Use --json or --plain on the list and show leaves.

Related commands:
  chab operations resume`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(newActionsListCommand(f), newActionsShowCommand(f))
	return cmd
}

func newActionsListCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local action records",
		Long: `List local action records without exposing private recovery material.

Use --json for structured output and --plain for tab-separated rows.

Related commands:
  chab operations actions show
  chab operations resume`,
		Args: cobra.NoArgs,
		Example: `  chab operations actions list
  chab operations actions list --json
  chab operations actions list --plain`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := f.ResolveRuntime(cmd, config.ResolveForWrite)
			if err != nil {
				return err
			}
			records, err := operations.StoreForRuntime(rt, f.Clock()).List()
			if err != nil {
				return err
			}
			records = visibleActionRecords(cmd, f, rt, records)
			projections := operations.Projections(records)
			return f.WriteResult(cmd, projections, cmdutil.HumanOutput{
				Render: func(w io.Writer) {
					for _, action := range projections {
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", action.ID, action.OperationKey, action.State, action.AcceptedOperationID)
					}
				},
				Plain: func(data, prose io.Writer) {
					for _, action := range projections {
						fmt.Fprintf(data, "%s\t%s\t%s\t%s\n", action.ID, action.OperationKey, action.State, action.AcceptedOperationID)
					}
				},
			})
		},
	}
}

func newActionsShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <action-id>",
		Short: "Show a local action record",
		Long: `Show a public projection of one local action record.

Use --json for structured output and --plain for the action id.

Related commands:
  chab operations resume
  chab operations actions list`,
		Args: cobra.ExactArgs(1),
		Example: `  chab operations actions show act_123
  chab operations actions show act_123 --json
  chab operations actions show act_123 --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := f.ResolveRuntime(cmd, config.ResolveForWrite)
			if err != nil {
				return err
			}
			record, err := operations.StoreForRuntime(rt, f.Clock()).Load(args[0])
			if err != nil {
				return err
			}
			if record.PrincipalID != "" && len(visibleActionRecords(cmd, f, rt, []operations.ActionRecord{record})) == 0 {
				return &usageError{detail: "action is not available to the current credential"}
			}
			projection := operations.Projection(record)
			return f.WriteResult(cmd, projection, cmdutil.HumanOutput{
				Render: func(w io.Writer) {
					fmt.Fprintf(w, "Action ID: %s\nOperation: %s\nState: %s\n", projection.ID, projection.OperationKey, projection.State)
					if projection.AcceptedOperationID != "" {
						fmt.Fprintf(w, "Accepted operation: %s\n", projection.AcceptedOperationID)
					}
				},
				Plain: func(data, prose io.Writer) { fmt.Fprintln(data, projection.ID) },
			})
		},
	}
}

// Guest receipts are local metadata but still belong to a live guest
// principal. A logged-out or rotated local profile must not enumerate them.
func visibleActionRecords(cmd *cobra.Command, f *cmdutil.Factory, rt config.Runtime, records []operations.ActionRecord) []operations.ActionRecord {
	needsGuestIdentity := false
	for _, record := range records {
		if record.PrincipalID != "" {
			needsGuestIdentity = true
			break
		}
	}
	if !needsGuestIdentity {
		return records
	}
	principal := ""
	if cred, _, err := f.Credential(rt); err == nil && cred.PrincipalType == "guest_trial" {
		if client, clientErr := f.APIClient(rt, cred, cmd); clientErr == nil {
			if identity, _, identityErr := client.Whoami(cmd.Context()); identityErr == nil && identity.PrincipalType == "guest_trial" {
				principal = identity.PrincipalID
			}
		}
	}
	visible := make([]operations.ActionRecord, 0, len(records))
	for _, record := range records {
		if record.PrincipalID == "" || (principal != "" && record.PrincipalID == principal && record.Profile == rt.Profile && record.Destination == rt.APIBaseURL) {
			visible = append(visible, record)
		}
	}
	return visible
}
