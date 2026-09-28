package operationscmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

type fileUploadFlags struct {
	ProjectID      int64
	RetentionHours int64
	Filename       string
	IdempotencyKey string
	Wait           bool
	MaxBytes       int64
}

type fileListFlags struct {
	Visibility    string
	ProjectID     int64
	State         string
	CreatedAfter  string
	CreatedBefore string
	Page          cmdutil.CursorPaginationFlags
}

type fileActionOutput struct {
	Action           *operations.ActionProjection `json:"action,omitempty"`
	FileID           string                       `json:"file_id,omitempty"`
	Server           json.RawMessage              `json:"server,omitempty"`
	LocalRecovery    operations.RecoveryStatus    `json:"local_recovery"`
	LocalPersistence *operations.PersistenceState `json:"local_persistence,omitempty"`
	RequestID        string                       `json:"request_id,omitempty"`
	Meta             *output.MetaView             `json:"meta,omitempty"`
}

// NewFilesCommand builds chab files.
func NewFilesCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("files", "Work with stored files")
	cmd.Long = `Work with stored files.

Stored files are private Chab inputs for conversion and connected-drive
workflows. Uploads are scanned asynchronously; use files wait before using a
new file in another operation when the server reports scan_pending. Request
file read/write and any required operation scopes during browser device login.
Project access and spending require separate approval.

Related commands:
  chab convert file
  chab drive items upload
  chab operations artifact download`
	cmd.AddCommand(
		newFilesListCommand(f),
		newFilesShowCommand(f),
		newFilesUploadCommand(f),
		newFilesWaitCommand(f),
		newFilesDownloadCommand(f),
		newFilesDeleteCommand(f),
		newFilesResumeCommand(f),
	)
	return cmd
}

func newFilesListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &fileListFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: withPreviewLabel("files.list", "List stored files"),
		Long: withPreviewNotice("files.list", `List stored files visible to the active API key.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab files show
  chab files upload`),
		Args: cobra.NoArgs,
		Example: `  chab files list
  chab files list --project-id 42 --state available
  chab files list --all --json --include-meta
  chab files list --jq '.[0].id'
  chab files list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setStringQuery(q, "visibility", flags.Visibility)
			if cmd.Flags().Changed("project-id") {
				q.Set("project_id", strconv.FormatInt(flags.ProjectID, 10))
			}
			setStringQuery(q, "state", flags.State)
			setStringQuery(q, "created_after", flags.CreatedAfter)
			setStringQuery(q, "created_before", flags.CreatedBefore)
			return cursorListRaw(cmd, f, "files", q, &flags.Page)
		},
	}
	cmd.Flags().StringVar(&flags.Visibility, "visibility", "", "visibility filter: creator or project")
	cmd.Flags().Int64Var(&flags.ProjectID, "project-id", 0, "numeric Chab project id")
	cmd.Flags().StringVar(&flags.State, "state", "", "file state filter")
	cmd.Flags().StringVar(&flags.CreatedAfter, "created-after", "", "include files created at or after this API date-time")
	cmd.Flags().StringVar(&flags.CreatedBefore, "created-before", "", "include files created at or before this API date-time")
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newFilesShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <file-id>",
		Short: withPreviewLabel("files.get", "Show stored file metadata"),
		Long: withPreviewNotice("files.get", `Show stored file metadata.

The stored-file show endpoint rejects generic project or provider query
context; visibility is resolved by the active API key.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab files download
  chab files wait`),
		Args: cobra.ExactArgs(1),
		Example: `  chab files show fil_123
  chab files show fil_123 --json --include-meta
  chab files show fil_123 --jq .file.id
  chab files show fil_123 --template '{{.file.id}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOnlyRaw(cmd, f, api.Path("files", args[0]), nil)
		},
	}
}

func newFilesUploadCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &fileUploadFlags{RetentionHours: 72, MaxBytes: defaultFileUploadMax}
	cmd := &cobra.Command{
		Use:   "upload <path>",
		Short: withPreviewLabel("files.create", "Upload a stored file"),
		Long: withPreviewNotice("files.create", `Upload a stored file using multipart/form-data.

Uploads require an Idempotency-Key. The CLI creates a private local action
record before the live request and binds recovery to the file content plus the
effective filename, project and retention metadata, not the multipart boundary.
Use --idempotency-key to supply a caller-owned replay key; otherwise one is
generated. Uploads may consume paid storage and require confirmation through
--yes or an interactive prompt. In --no-prompt or non-interactive mode, use
--yes to acknowledge the upload. Use --wait to poll scanner readiness after the
server accepts the file.

Output modes: default human detail, --plain, --json.

Related commands:
  chab files wait
  chab convert file`),
		Args: cobra.ExactArgs(1),
		Example: `  chab files upload ./contract.pdf --project-id 42 --yes
  chab files upload ./notes.md --filename source.md --retention-hours 72 --wait --json
  chab files upload ./contract.pdf --idempotency-key upload-contract-001 --yes --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFileUpload(cmd, f, args[0], flags)
		},
	}
	cmd.Flags().Int64Var(&flags.ProjectID, "project-id", 0, "numeric Chab project id to make the file project-visible")
	cmd.Flags().Int64Var(&flags.RetentionHours, "retention-hours", 72, "retention window in hours, 24 through 720")
	cmd.Flags().StringVar(&flags.Filename, "filename", "", "trusted filename sent to Chab")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().BoolVar(&flags.Wait, "wait", false, "wait for scanner readiness after upload acceptance")
	cmd.Flags().Int64Var(&flags.MaxBytes, "max-bytes", defaultFileUploadMax, "maximum upload bytes to stream")
	return cmd
}

func runFileUpload(cmd *cobra.Command, f *cmdutil.Factory, sourcePath string, flags *fileUploadFlags) error {
	if flags.MaxBytes < 1 {
		return &usageError{detail: "--max-bytes must be positive"}
	}
	if flags.RetentionHours < 24 || flags.RetentionHours > 720 {
		return &usageError{detail: "--retention-hours must be between 24 and 720"}
	}
	filename := flags.Filename
	if filename == "" {
		filename = filepath.Base(sourcePath)
	}
	digest, size, err := fileSHA256(sourcePath, flags.MaxBytes)
	if err != nil {
		return &usageError{detail: "could not read upload file: " + err.Error()}
	}
	if size < 1 {
		return &usageError{detail: "upload file must not be empty"}
	}
	if size > flags.MaxBytes {
		return &usageError{detail: "upload file exceeds configured byte limit"}
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Uploading this file may consume paid storage. Continue?"); err != nil {
		return err
	}
	authn, err := resolveAuthContext(cmd, f)
	if err != nil {
		return err
	}
	key := flags.IdempotencyKey
	if key == "" {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return err
		}
	} else if err := api.ValidateIdempotencyKey(key); err != nil {
		return err
	}
	f.RegisterSecret(key)
	projectValue := any(nil)
	fields := map[string]string{}
	if cmd.Flags().Changed("project-id") {
		projectValue = flags.ProjectID
		fields["project_id"] = strconv.FormatInt(flags.ProjectID, 10)
	}
	if cmd.Flags().Changed("retention-hours") {
		fields["retention_hours"] = strconv.FormatInt(flags.RetentionHours, 10)
	}
	if cmd.Flags().Changed("filename") {
		fields["filename"] = filename
	}
	fingerprint, err := semanticFingerprint(map[string]any{
		"version":         1,
		"project_id":      projectValue,
		"retention_hours": flags.RetentionHours,
		"filename":        strings.TrimSpace(filename),
		"byte_size":       size,
		"sha256":          strings.ToLower(digest),
	})
	if err != nil {
		return err
	}
	store := operations.StoreForRuntime(authn.Runtime, f.Clock())
	prepared, err := store.Prepare(operations.PrepareInput{
		Profile:            authn.Runtime.Profile,
		Destination:        authn.Runtime.APIBaseURL,
		TokenPublicID:      authn.Identity.TokenPublicID,
		PrincipalID:        authn.Identity.PrincipalID,
		RequiredScope:      mustOperation("files.create").RequiredScope,
		OperationKey:       "files.create",
		Method:             http.MethodPost,
		Path:               "/v1/files",
		RequestFingerprint: fingerprint,
		IdempotencyKey:     key,
	})
	if err != nil {
		return err
	}
	if prepared.AlreadyCompleted {
		if flags.Wait && prepared.Record.AcceptedOperationID != "" && prepared.Record.LastStatus != "available" {
			if err := ensureRecordedActionContext(authn, prepared.Record); err != nil {
				return err
			}
			return resumeFileUploadLifecycle(cmd, f, store, prepared.Record, authn, true, "chab files resume "+prepared.Record.ID+" "+sourcePath)
		}
		out := fileOutputFromRecord(prepared.Record, nil, api.ResponseMeta{})
		return writeFileActionOutput(cmd, f, out)
	}
	if err := store.ReplayablePrepared(prepared); err != nil {
		return err
	}
	result, err := authn.Client.PostMultipart(cmd.Context(), "files", api.MultipartFileUpload{
		FilePath:      sourcePath,
		FileFieldName: "file",
		Filename:      filename,
		Fields:        fields,
		MaxBytes:      flags.MaxBytes,
		Idempotency:   api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing),
	})
	if err != nil {
		updated, persistence := markFileActionUnknown(store, prepared.Record, err, prepared.Existing)
		out := fileOutputFromRecord(updated, nil, api.ResponseMeta{})
		out.LocalPersistence = persistence
		setFileFailureRecovery(&out, persistence, "chab files resume "+prepared.Record.ID+" "+sourcePath)
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	receipt, receiptErr := parseFileReceipt(result.Data, result.Meta, "file upload")
	if receiptErr != nil {
		updated, persistence := markFileActionUnknown(store, prepared.Record, receiptErr, prepared.Existing)
		out := fileOutputFromRecord(updated, result.Data, result.Meta)
		out.LocalPersistence = persistence
		setFileFailureRecovery(&out, persistence, "chab files resume "+prepared.Record.ID+" "+sourcePath)
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return output.WithCredentialContext(receiptErr, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	fileID := receipt.ID
	state := receipt.State
	updated, persistErr := store.MarkCompleted(prepared.Record.ID, operations.OperationPayload{ID: fileID, OperationKey: "files.create", Family: "files", Status: state}, result.Meta)
	if persistErr != nil {
		out := fileOutputFromRecord(prepared.Record, result.Data, result.Meta)
		out.FileID = fileID
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return fmt.Errorf("file upload was accepted but failed to persist local action state: %w", persistErr)
	}
	raw := result.Data
	meta := result.Meta
	if flags.Wait && fileID != "" && !fileTerminal(state) {
		fmt.Fprintf(cmd.ErrOrStderr(), "accepted file %s; waiting for scanner readiness\n", fileID)
		raw, meta, err = waitForFile(cmd, f, authn, fileID, 10*time.Minute, false)
		if err != nil && len(raw) == 0 {
			return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
		}
	}
	out := fileOutputFromRecord(updated, raw, meta)
	out.FileID = fileID
	if out.FileID == "" {
		out.FileID = updated.AcceptedOperationID
	}
	if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return nil
}

func newFilesResumeCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &fileUploadFlags{RetentionHours: 72, MaxBytes: defaultFileUploadMax}
	cmd := &cobra.Command{
		Use:   "resume <action-id> <path>",
		Short: withPreviewLabel("files.create", "Resume a stored-file upload"),
		Long: withPreviewNotice("files.create", `Resume a stored-file upload from the private action journal.

The same file content and effective metadata must be supplied so the CLI can
verify the semantic fingerprint before replaying the original Idempotency-Key.
Completed actions are reported without resubmitting bytes. Replaying an upload
requires confirmation through --yes or an interactive prompt; in --no-prompt
or non-interactive mode, use --yes.

Output modes: default human detail, --plain, --json.

Related commands:
  chab files upload
  chab operations actions show`),
		Args: cobra.ExactArgs(2),
		Example: `  chab files resume act_123 ./contract.pdf --yes
  chab files resume act_123 ./contract.pdf --project-id 42 --retention-hours 72 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFileResume(cmd, f, args[0], args[1], flags)
		},
	}
	cmd.Flags().Int64Var(&flags.ProjectID, "project-id", 0, "numeric Chab project id used by the original upload")
	cmd.Flags().Int64Var(&flags.RetentionHours, "retention-hours", 72, "retention window used by the original upload")
	cmd.Flags().StringVar(&flags.Filename, "filename", "", "trusted filename used by the original upload")
	cmd.Flags().BoolVar(&flags.Wait, "wait", false, "wait for scanner readiness after upload acceptance")
	cmd.Flags().Int64Var(&flags.MaxBytes, "max-bytes", defaultFileUploadMax, "maximum upload bytes to stream")
	return cmd
}

func runFileResume(cmd *cobra.Command, f *cmdutil.Factory, actionID, sourcePath string, flags *fileUploadFlags) error {
	authn, err := resolveAuthContext(cmd, f)
	if err != nil {
		return err
	}
	store := operations.StoreForRuntime(authn.Runtime, f.Clock())
	record, err := store.Load(actionID)
	if err != nil {
		return err
	}
	if record.OperationKey != "files.create" || record.Method != http.MethodPost || record.Path != "/v1/files" {
		return &usageError{detail: "action is not a stored-file upload"}
	}
	filename := flags.Filename
	if filename == "" {
		filename = filepath.Base(sourcePath)
	}
	digest, size, err := fileSHA256(sourcePath, flags.MaxBytes)
	if err != nil {
		return &usageError{detail: "could not read upload file: " + err.Error()}
	}
	projectValue := any(nil)
	fields := map[string]string{}
	if cmd.Flags().Changed("project-id") {
		projectValue = flags.ProjectID
		fields["project_id"] = strconv.FormatInt(flags.ProjectID, 10)
	}
	if cmd.Flags().Changed("retention-hours") {
		fields["retention_hours"] = strconv.FormatInt(flags.RetentionHours, 10)
	}
	if cmd.Flags().Changed("filename") {
		fields["filename"] = filename
	}
	fingerprint, err := semanticFingerprint(map[string]any{
		"version":         1,
		"project_id":      projectValue,
		"retention_hours": flags.RetentionHours,
		"filename":        strings.TrimSpace(filename),
		"byte_size":       size,
		"sha256":          strings.ToLower(digest),
	})
	if err != nil {
		return err
	}
	if record.RequestSHA256 != fingerprint {
		return &usageError{detail: "action input does not match the original upload fingerprint"}
	}
	if err := store.Replayable(record); err != nil {
		return err
	}
	if record.State == operations.ActionCompleted {
		if flags.Wait && record.AcceptedOperationID != "" && record.LastStatus != "available" {
			if err := ensureRecordedActionContext(authn, record); err != nil {
				return err
			}
			return resumeFileUploadLifecycle(cmd, f, store, record, authn, true, "chab files resume "+record.ID+" "+sourcePath)
		}
		return writeFileActionOutput(cmd, f, fileOutputFromRecord(record, nil, api.ResponseMeta{}))
	}
	if err := ensureRecordedActionContext(authn, record); err != nil {
		return err
	}
	if record.AcceptedOperationID != "" {
		return resumeFileUploadLifecycle(cmd, f, store, record, authn, flags.Wait, "chab files resume "+record.ID+" "+sourcePath)
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Replaying this upload sends the original Idempotency-Key. Continue?"); err != nil {
		return err
	}
	f.RegisterSecret(record.IdempotencyKey)
	if err := store.BeginReplay(record); err != nil {
		return err
	}
	result, err := authn.Client.PostMultipart(cmd.Context(), "files", api.MultipartFileUpload{
		FilePath:      sourcePath,
		FileFieldName: "file",
		Filename:      filename,
		Fields:        fields,
		MaxBytes:      flags.MaxBytes,
		Idempotency:   api.JournaledIdempotency(record.IdempotencyKey, true),
	})
	if err != nil {
		updated, persistence := markFileActionUnknown(store, record, err, true)
		out := fileOutputFromRecord(updated, nil, api.ResponseMeta{})
		out.LocalPersistence = persistence
		setFileFailureRecovery(&out, persistence, "chab files resume "+record.ID+" "+sourcePath)
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	receipt, receiptErr := parseFileReceipt(result.Data, result.Meta, "file upload")
	if receiptErr != nil {
		updated, persistence := markFileActionUnknown(store, record, receiptErr, true)
		out := fileOutputFromRecord(updated, result.Data, result.Meta)
		out.LocalPersistence = persistence
		setFileFailureRecovery(&out, persistence, "chab files resume "+record.ID+" "+sourcePath)
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return output.WithCredentialContext(receiptErr, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	fileID := receipt.ID
	state := receipt.State
	updated, err := store.MarkCompleted(record.ID, operations.OperationPayload{ID: fileID, OperationKey: "files.create", Family: "files", Status: state}, result.Meta)
	if err != nil {
		out := fileOutputFromRecord(record, result.Data, result.Meta)
		out.FileID = fileID
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return fmt.Errorf("file upload was accepted but failed to persist local action state: %w", err)
	}
	raw := result.Data
	meta := result.Meta
	if flags.Wait && fileID != "" && !fileTerminal(state) {
		raw, meta, err = waitForFile(cmd, f, authn, fileID, 10*time.Minute, false)
		if err != nil && len(raw) == 0 {
			return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
		}
	}
	out := fileOutputFromRecord(updated, raw, meta)
	out.FileID = fileID
	if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return nil
}

func resumeFileUploadLifecycle(cmd *cobra.Command, f *cmdutil.Factory, store operations.Store, record operations.ActionRecord, authn authContext, wait bool, resumeHint string) error {
	fileID := record.AcceptedOperationID
	if fileID == "" {
		return writeFileActionOutput(cmd, f, fileOutputFromRecord(record, nil, api.ResponseMeta{}))
	}
	if !wait || record.LastStatus == "available" {
		out := fileOutputFromRecord(record, nil, api.ResponseMeta{})
		if record.State != operations.ActionCompleted {
			out.LocalRecovery.ResumeHint = resumeHint
		}
		return writeFileActionOutput(cmd, f, out)
	}
	if fileErrorState(record.LastStatus) {
		out := fileOutputFromRecord(record, nil, api.ResponseMeta{})
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return &usageError{detail: "file reached terminal state " + record.LastStatus}
	}
	raw, meta, waitErr := waitForFile(cmd, f, authn, fileID, 10*time.Minute, false)
	if waitErr == nil {
		receipt, receiptErr := parseFileReceipt(raw, meta, "file readiness")
		if receiptErr != nil {
			waitErr = receiptErr
		} else {
			updated, persistErr := store.MarkCompleted(record.ID, operations.OperationPayload{ID: receipt.ID, OperationKey: "files.create", Family: "files", Status: receipt.State}, meta)
			if persistErr != nil {
				out := fileOutputFromRecord(record, raw, meta)
				out.FileID = fileID
				out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
				if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
					return writeErr
				}
				return fmt.Errorf("file upload reached state %s but failed to persist local action state: %w", receipt.State, persistErr)
			}
			record = updated
		}
	}
	out := fileOutputFromRecord(record, raw, meta)
	out.FileID = fileID
	if waitErr != nil {
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.ResumeHint = resumeHint
	}
	if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if waitErr != nil {
		return output.WithCredentialContext(waitErr, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return nil
}

func newFilesWaitCommand(f *cmdutil.Factory) *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "wait <file-id>",
		Short: withPreviewLabel("files.get", "Wait for stored file readiness"),
		Long: withPreviewNotice("files.get", `Wait for a stored file to leave scanner-pending state.

The command polls the stored-file resource and honors Retry-After when the
server provides it. It treats available as success and returns a local failure
when the server reports scan_failed, quarantined, or expired.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab files upload
  chab files show`),
		Args: cobra.ExactArgs(1),
		Example: `  chab files wait fil_123
  chab files wait fil_123 --timeout 2m --json
  chab files wait fil_123 --jq .file.state
  chab files wait fil_123 --template '{{.file.state}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAPIClient(cmd, f, false)
			if err != nil {
				return err
			}
			raw, meta, err := waitForFile(cmd, f, authn, args[0], timeout, false)
			if err != nil && len(raw) == 0 {
				return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
			}
			if writeErr := writeRawJSONValue(cmd, f, raw, meta); writeErr != nil {
				return writeErr
			}
			if err != nil {
				return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum local wait duration")
	return cmd
}

func newFilesDownloadCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &downloadFlags{MaxBytes: defaultTransferMaxBytes}
	cmd := &cobra.Command{
		Use:   "download <file-id>",
		Short: withPreviewLabel("files.download", "Download stored file bytes"),
		Long: withPreviewNotice("files.download", `Download stored file bytes to an explicit new local path.

The command first reads trusted file metadata, resolves the documented
download_url below the configured API base, and streams bytes through the
stored-file download endpoint without adding project or provider query
context. Existing output files are refused. When a sha256:<hex> checksum is
available or supplied through --checksum, the file is published only after the
completed transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
  chab files show
  chab operations artifact download`),
		Args: cobra.ExactArgs(1),
		Example: `  chab files download fil_123 --output ./contract.pdf
  chab files download fil_123 -o ./contract.pdf --checksum sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --json
  chab files download fil_123 -o ./contract.pdf --jq .bytes
  chab files download fil_123 -o ./contract.pdf --template '{{.path}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAPIClient(cmd, f, false)
			if err != nil {
				return err
			}
			result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, api.Path("files", args[0]), nil, nil, api.IdempotencyNone)
			if err != nil {
				return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
			}
			link, checksum, err := downloadMetadata(result.Data, result.Meta)
			if err != nil {
				return err
			}
			err = downloadToLocalPath(cmd, f, authn.Client, func(w io.Writer) (int64, api.ResponseMeta, error) {
				return authn.Client.DownloadAPILink(cmd.Context(), link, w, api.DownloadOptions{MaxBytes: flags.MaxBytes})
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

func newFilesDeleteCommand(f *cmdutil.Factory) *cobra.Command {
	var idempotencyKey string
	var wait bool
	cmd := &cobra.Command{
		Use:   "delete <file-id>",
		Short: withPreviewLabel("files.delete", "Delete a stored file"),
		Long: withPreviewNotice("files.delete", `Delete a stored file.

Deletion requires confirmation through --yes or an interactive prompt and sends
one Idempotency-Key for the logical action. In --no-prompt or non-interactive
mode, use --yes. Use --idempotency-key to supply a
caller-owned replay key; otherwise one is generated. If the server returns
deletion_pending, --wait polls the file resource until it disappears or returns
a terminal deletion receipt; the command does not claim proof of physical byte
purge.

Output modes: default human detail, --plain, --json.

Related commands:
  chab files list
  chab operations actions show`),
		Args: cobra.ExactArgs(1),
		Example: `  chab files delete fil_123 --yes
  chab files delete fil_123 --idempotency-key delete-fil-123 --yes --json
  chab files delete fil_123 --yes --wait`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFileDelete(cmd, f, args[0], idempotencyKey, wait)
		},
	}
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for deletion_pending to become a terminal lifecycle result")
	return cmd
}

func runFileDelete(cmd *cobra.Command, f *cmdutil.Factory, fileID, idempotencyKey string, wait bool) error {
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Deleting this stored file changes retained file state. Continue?"); err != nil {
		return err
	}
	authn, err := resolveAuthContext(cmd, f)
	if err != nil {
		return err
	}
	key := idempotencyKey
	if key == "" {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return err
		}
	} else if err := api.ValidateIdempotencyKey(key); err != nil {
		return err
	}
	f.RegisterSecret(key)
	store := operations.StoreForRuntime(authn.Runtime, f.Clock())
	prepared, err := store.Prepare(operations.PrepareInput{
		Profile:        authn.Runtime.Profile,
		Destination:    authn.Runtime.APIBaseURL,
		TokenPublicID:  authn.Identity.TokenPublicID,
		PrincipalID:    authn.Identity.PrincipalID,
		RequiredScope:  mustOperation("files.delete").RequiredScope,
		OperationKey:   "files.delete",
		Method:         http.MethodDelete,
		Path:           "/v1/" + api.Path("files", fileID),
		IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	if prepared.AlreadyCompleted {
		if wait && prepared.Record.AcceptedOperationID == "" {
			return resumeFileDeletion(cmd, f, store, prepared.Record, authn, fileID, true, sameDeleteCommandResumeHint())
		}
		return writeFileActionOutput(cmd, f, fileOutputFromRecord(prepared.Record, nil, api.ResponseMeta{}))
	}
	if err := store.ReplayablePrepared(prepared); err != nil {
		return err
	}
	if prepared.AlreadyAccepted {
		return resumeFileDeletion(cmd, f, store, prepared.Record, authn, fileID, wait, sameDeleteCommandResumeHint())
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodDelete, api.Path("files", fileID), nil, nil, api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing))
	if err != nil {
		updated, persistence := markFileActionUnknown(store, prepared.Record, err, prepared.Existing)
		out := fileOutputFromRecord(updated, nil, api.ResponseMeta{})
		out.LocalPersistence = persistence
		setFileFailureRecovery(&out, persistence, sameDeleteCommandResumeHint())
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	receipt, receiptErr := parseFileReceipt(result.Data, result.Meta, "file deletion")
	if receiptErr == nil && receipt.ID != fileID {
		receiptErr = &api.ProtocolError{Detail: "file deletion response returned a different file id", Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Meta: result.Meta}
	}
	if receiptErr == nil && !fileDeletionReceiptState(receipt.State) {
		receiptErr = &api.ProtocolError{Detail: "file deletion response returned unsupported file state " + receipt.State, Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Meta: result.Meta}
	}
	if receiptErr != nil {
		updated, persistence := markFileActionUnknown(store, prepared.Record, receiptErr, prepared.Existing)
		out := fileOutputFromRecord(updated, result.Data, result.Meta)
		out.LocalPersistence = persistence
		setFileFailureRecovery(&out, persistence, sameDeleteCommandResumeHint())
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return output.WithCredentialContext(receiptErr, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	payload := operations.OperationPayload{ID: receipt.ID, OperationKey: "files.delete", Family: "files", Status: receipt.State}
	var updated operations.ActionRecord
	if fileTerminal(receipt.State) {
		updated, err = store.MarkCompleted(prepared.Record.ID, payload, result.Meta)
	} else {
		updated, err = store.MarkAccepted(prepared.Record.ID, payload, result.Meta)
	}
	if err != nil {
		out := fileOutputFromRecord(prepared.Record, result.Data, result.Meta)
		out.FileID = receipt.ID
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
		if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
			return writeErr
		}
		return fmt.Errorf("file deletion was accepted but failed to persist local action state: %w", err)
	}
	return finishFileDeletion(cmd, f, store, updated, authn, result.Data, result.Meta, wait)
}

func resumeFileDeletion(cmd *cobra.Command, f *cmdutil.Factory, store operations.Store, record operations.ActionRecord, authn authContext, fileID string, wait bool, resumeHint string) error {
	if record.AcceptedOperationID != "" && record.AcceptedOperationID != fileID {
		return &usageError{detail: "recorded deletion file id does not match the command file id"}
	}
	if !wait {
		out := fileOutputFromRecord(record, nil, api.ResponseMeta{})
		out.LocalRecovery.ResumeHint = resumeHint
		return writeFileActionOutput(cmd, f, out)
	}
	raw, meta, waitErr := waitForFile(cmd, f, authn, fileID, 10*time.Minute, true)
	if waitErr == nil {
		if receipt, err := parseFileReceipt(raw, meta, "file deletion"); err == nil {
			updated, persistErr := store.MarkCompleted(record.ID, operations.OperationPayload{ID: receipt.ID, OperationKey: "files.delete", Family: "files", Status: receipt.State}, meta)
			if persistErr != nil {
				out := fileOutputFromRecord(record, raw, meta)
				out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
				if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
					return writeErr
				}
				return fmt.Errorf("file deletion reached terminal state but failed to persist local action state: %w", persistErr)
			}
			record = updated
		} else {
			waitErr = err
		}
	}
	out := fileOutputFromRecord(record, raw, meta)
	if waitErr != nil {
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.ResumeHint = resumeHint
	}
	if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if waitErr != nil {
		return output.WithCredentialContext(waitErr, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return nil
}

func finishFileDeletion(cmd *cobra.Command, f *cmdutil.Factory, store operations.Store, record operations.ActionRecord, authn authContext, raw json.RawMessage, meta api.ResponseMeta, wait bool) error {
	var err error
	if wait && fileState(raw) == "deletion_pending" {
		raw, meta, err = waitForFile(cmd, f, authn, record.AcceptedOperationID, 10*time.Minute, true)
		if err == nil {
			receipt, receiptErr := parseFileReceipt(raw, meta, "file deletion")
			if receiptErr != nil {
				err = receiptErr
			} else if updated, persistErr := store.MarkCompleted(record.ID, operations.OperationPayload{ID: receipt.ID, OperationKey: "files.delete", Family: "files", Status: receipt.State}, meta); persistErr == nil {
				record = updated
			} else {
				out := fileOutputFromRecord(record, raw, meta)
				out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
				if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
					return writeErr
				}
				return fmt.Errorf("file deletion reached terminal state but failed to persist local action state: %w", persistErr)
			}
		}
	}
	out := fileOutputFromRecord(record, raw, meta)
	if record.State == operations.ActionAccepted {
		out.LocalRecovery.ResumeHint = sameDeleteCommandResumeHint()
	}
	if writeErr := writeFileActionOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return nil
}

func isFileLifecycleAction(record operations.ActionRecord) bool {
	return record.OperationKey == "files.create" || record.OperationKey == "files.delete"
}

func resumeFileLifecycleAction(cmd *cobra.Command, f *cmdutil.Factory, store operations.Store, record operations.ActionRecord, authn authContext) error {
	switch record.OperationKey {
	case "files.create":
		return resumeFileUploadLifecycle(cmd, f, store, record, authn, true, "chab files wait "+record.AcceptedOperationID)
	case "files.delete":
		return resumeFileDeletion(cmd, f, store, record, authn, record.AcceptedOperationID, true, "chab operations resume "+record.ID)
	default:
		return writeFileActionOutput(cmd, f, fileOutputFromRecord(record, nil, api.ResponseMeta{}))
	}
}

func fileDeletionReceiptState(state string) bool {
	return state == "deletion_pending" || state == "deleted"
}

func sameDeleteCommandResumeHint() string {
	return "rerun this command with --wait and the original --idempotency-key"
}

func waitForFile(cmd *cobra.Command, f *cmdutil.Factory, authn authContext, fileID string, timeout time.Duration, deletion bool) (json.RawMessage, api.ResponseMeta, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := f.Clock()().Add(timeout)
	var lastRaw json.RawMessage
	var lastMeta api.ResponseMeta
	for {
		result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, api.Path("files", fileID), nil, nil, api.IdempotencyNone)
		if err != nil {
			var apiErr *api.Error
			if deletion && errors.As(err, &apiErr) && apiErr.Code == "not_found" {
				raw := json.RawMessage(fmt.Sprintf(`{"file":{"id":%q,"state":"deleted"}}`, fileID))
				return raw, apiErr.Meta, nil
			}
			return lastRaw, lastMeta, err
		}
		lastRaw = result.Data
		lastMeta = result.Meta
		state := fileState(result.Data)
		if deletion {
			switch {
			case state == "deleted":
				return result.Data, result.Meta, nil
			case state == "deletion_pending":
			case fileTerminal(state):
				return result.Data, result.Meta, &usageError{detail: "file deletion reached terminal state " + state}
			case state == "":
				return result.Data, result.Meta, &api.ProtocolError{Detail: "file response missing file state", Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Meta: result.Meta}
			}
			wait := retryDelay(result.Meta, 5*time.Second)
			if f.Clock()().Add(wait).After(deadline) {
				return result.Data, result.Meta, &usageError{detail: "timed out waiting for file deletion"}
			}
			if err := f.Sleep(cmd.Context(), wait); err != nil {
				return result.Data, result.Meta, err
			}
			continue
		}
		if fileTerminal(state) {
			if fileErrorState(state) {
				return result.Data, result.Meta, &usageError{detail: "file reached terminal state " + state}
			}
			return result.Data, result.Meta, nil
		}
		wait := retryDelay(result.Meta, 5*time.Second)
		if f.Clock()().Add(wait).After(deadline) {
			return result.Data, result.Meta, &usageError{detail: "timed out waiting for file readiness"}
		}
		if err := f.Sleep(cmd.Context(), wait); err != nil {
			return result.Data, result.Meta, err
		}
	}
}

func fileOutputFromRecord(record operations.ActionRecord, raw json.RawMessage, meta api.ResponseMeta) fileActionOutput {
	projection := operations.Projection(record)
	return fileActionOutput{
		Action:    &projection,
		FileID:    record.AcceptedOperationID,
		Server:    raw,
		RequestID: meta.RequestID,
		Meta:      operationResponseMeta(meta),
		LocalRecovery: operations.RecoveryStatus{
			State:       string(record.State),
			CanResume:   record.State == operations.ActionUnknown || record.State == operations.ActionAccepted,
			KnownRemote: record.AcceptedOperationID != "",
		},
		LocalPersistence: &operations.PersistenceState{State: "persisted"},
	}
}

func markFileActionUnknown(store operations.Store, record operations.ActionRecord, cause error, priorUnknown bool) (operations.ActionRecord, *operations.PersistenceState) {
	var markErr error
	if operations.DefinitiveAdmissionDenialForSubmission(cause, priorUnknown) {
		markErr = store.MarkDenied(record.ID, cause)
	} else {
		markErr = store.MarkUnknown(record.ID, cause)
	}
	if markErr != nil {
		if cause != nil {
			record.LastError = cause.Error()
		}
		return record, &operations.PersistenceState{State: "failed", Detail: markErr.Error()}
	}
	updated, err := store.Load(record.ID)
	if err != nil {
		if cause != nil {
			record.LastError = cause.Error()
		}
		return record, &operations.PersistenceState{State: "failed", Detail: err.Error()}
	}
	return updated, &operations.PersistenceState{State: "persisted"}
}

func setFileFailureRecovery(out *fileActionOutput, persistence *operations.PersistenceState, hint string) {
	out.LocalRecovery.CanResume = out.LocalRecovery.CanResume && persistence.State == "persisted"
	if out.LocalRecovery.CanResume {
		out.LocalRecovery.ResumeHint = hint
	} else {
		out.LocalRecovery.ResumeHint = ""
	}
}

func writeFileActionOutput(cmd *cobra.Command, f *cmdutil.Factory, value fileActionOutput) error {
	return f.WriteResult(cmd, value, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			if value.Action != nil {
				fmt.Fprintf(w, "Action ID: %s\n", value.Action.ID)
			}
			if value.FileID != "" {
				fmt.Fprintf(w, "File ID: %s\n", value.FileID)
			}
			if value.LocalRecovery.State != "" {
				fmt.Fprintf(w, "Recovery: %s\n", value.LocalRecovery.State)
			}
			if value.LocalRecovery.ResumeHint != "" {
				fmt.Fprintf(w, "Resume: %s\n", value.LocalRecovery.ResumeHint)
			}
			if value.RequestID != "" {
				fmt.Fprintf(w, "Request ID: %s\n", value.RequestID)
			}
			if len(value.Server) > 0 {
				fmt.Fprintf(w, "Server: %s\n", string(value.Server))
			}
		},
		Plain: func(data, prose io.Writer) {
			if value.FileID != "" {
				fmt.Fprintln(data, value.FileID)
			} else if value.Action != nil {
				fmt.Fprintln(data, value.Action.ID)
			}
			if value.LocalRecovery.ResumeHint != "" {
				fmt.Fprintln(prose, value.LocalRecovery.ResumeHint)
			}
		},
	})
}
