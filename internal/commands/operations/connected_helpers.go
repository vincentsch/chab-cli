package operationscmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/localfile"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

const (
	defaultTransferMaxBytes = int64(100 * 1024 * 1024)
	defaultFileUploadMax    = int64(100 * 1024 * 1024)
)

type jsonRequestFlags struct {
	Input          string
	Set            []string
	IdempotencyKey string
	Wait           bool
	ServerDryRun   bool
	fields         map[string]*fieldValue
}

type downloadFlags struct {
	Output   string
	Checksum string
	MaxBytes int64
}

type transferOutput struct {
	Path      string           `json:"path"`
	Bytes     int64            `json:"bytes"`
	Checksum  string           `json:"checksum,omitempty"`
	RequestID string           `json:"request_id,omitempty"`
	Meta      *output.MetaView `json:"meta,omitempty"`
}

func connectedFamily(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  short + ".\n\nRequest the required scopes during browser device login. Project access, spending and connected provider permissions are checked separately.\n\nRelated commands:\n  chab setup\n  chab operations",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
}

func readOnlyRaw(cmd *cobra.Command, f *cmdutil.Factory, path string, query url.Values) error {
	authn, err := resolveAPIClient(cmd, f, false)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, path, query, nil, api.IdempotencyNone)
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func cursorListRaw(cmd *cobra.Command, f *cmdutil.Factory, path string, baseQuery url.Values, flags *cmdutil.CursorPaginationFlags) error {
	plan, err := cmdutil.ResolveCursorListPlan(cmd, flags)
	if err != nil {
		return &usageError{detail: strings.TrimPrefix(err.Error(), "invalid list options: ")}
	}
	authn, err := resolveAPIClient(cmd, f, false)
	if err != nil {
		return err
	}
	outcome, err := cmdutil.FetchCursorPages[json.RawMessage](plan, func(cursor string, limit int) ([]json.RawMessage, api.ResponseMeta, error) {
		q := cloneValues(baseQuery)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		var rows []json.RawMessage
		meta, err := authn.Client.Get(cmd.Context(), path, q, &rows)
		return rows, meta, err
	})
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	data, err := json.Marshal(outcome.Rows)
	if err != nil {
		return err
	}
	hint := cmdutil.CursorPaginationHint(plan, len(outcome.Rows), outcome.Meta.CursorPagination)
	return f.WriteResultWithMeta(cmd, json.RawMessage(data), outcome.Meta, true, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			output.RawValueHuman(w, json.RawMessage(data))
			if hint != "" {
				fmt.Fprintln(w, hint)
			}
		},
		Plain: func(dataOut, prose io.Writer) {
			output.RawValuePlain(dataOut, prose, json.RawMessage(data))
			if hint != "" {
				fmt.Fprintln(prose, hint)
			}
		},
	})
}

func registerCursorFlags(cmd *cobra.Command, flags *cmdutil.CursorPaginationFlags) {
	cmdutil.RegisterCursorPaginationFlags(cmd, flags)
}

func registerJSONRequestFlags(cmd *cobra.Command, flags *jsonRequestFlags, fields []fieldFlag, includeMutation bool, includeWait bool, includeServerDryRun bool) {
	flags.fields = map[string]*fieldValue{}
	f := cmd.Flags()
	f.StringVar(&flags.Input, "input", "", "JSON request body, @path, or @- for stdin")
	f.StringArrayVar(&flags.Set, "set", nil, "top-level request field as name=json (repeatable)")
	if includeMutation {
		f.StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	}
	if includeWait {
		f.BoolVar(&flags.Wait, "wait", false, "wait for an accepted operation to reach a terminal state")
	}
	if includeServerDryRun {
		f.BoolVar(&flags.ServerDryRun, "dry-run", false, "ask the server to validate without mutating provider data")
	}
	for _, spec := range fields {
		value := &fieldValue{kind: spec.Kind, field: spec.Field}
		flags.fields[spec.Name] = value
		switch spec.Kind {
		case valueString, valueJSON, valueArtifactID:
			f.StringVar(&value.s, spec.Name, "", spec.Usage)
		case valueInt:
			f.Int64Var(&value.i, spec.Name, 0, spec.Usage)
		case valueBool:
			f.BoolVar(&value.b, spec.Name, false, spec.Usage)
		case valueStringSlice:
			f.StringArrayVar(&value.values, spec.Name, nil, spec.Usage)
		}
	}
}

func registerDownloadFlags(cmd *cobra.Command, flags *downloadFlags) {
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "new local output path")
	cmd.Flags().StringVar(&flags.Checksum, "checksum", "", "expected sha256:<hex> checksum")
	defaultMaxBytes := flags.MaxBytes
	if defaultMaxBytes < 1 {
		defaultMaxBytes = defaultTransferMaxBytes
	}
	flags.MaxBytes = defaultMaxBytes
	cmd.Flags().Int64Var(&flags.MaxBytes, "max-bytes", defaultMaxBytes, "maximum bytes to write")
	_ = cmd.MarkFlagRequired("output")
}

func resolveJSONRequest(cmd *cobra.Command, f *cmdutil.Factory, flags *jsonRequestFlags, operationKey string) ([]byte, error) {
	object := map[string]json.RawMessage{}
	if flags.Input != "" {
		raw, err := readInput(cmd, f, flags.Input)
		if err != nil {
			return nil, err
		}
		canon, err := operations.CanonicalizeJSON(raw)
		if err != nil {
			return nil, &usageError{detail: err.Error()}
		}
		if err := json.Unmarshal(canon, &object); err != nil {
			return nil, &usageError{detail: "--input must be a JSON object"}
		}
	}
	for _, entry := range flags.Set {
		key, raw, err := parseSet(entry)
		if err != nil {
			return nil, err
		}
		if err := putField(object, key, raw); err != nil {
			return nil, err
		}
	}
	for name, field := range flags.fields {
		if !cmd.Flags().Changed(name) {
			continue
		}
		raw, err := field.raw()
		if err != nil {
			return nil, err
		}
		if err := putField(object, field.field, raw); err != nil {
			return nil, err
		}
	}
	if flags.ServerDryRun {
		if err := putField(object, "dry_run", json.RawMessage("true")); err != nil {
			return nil, err
		}
	}
	request, err := marshalObject(object)
	if err != nil {
		return nil, err
	}
	registry, err := chabcontract.Load()
	if err != nil {
		return nil, err
	}
	if op, ok := registry.Find(operationKey); ok {
		serverDryRun, err := operations.RequestDryRun(request)
		if err != nil {
			return nil, &usageError{detail: err.Error()}
		}
		if serverDryRun && !op.DryRunSupported {
			return nil, &usageError{detail: "operation " + operationKey + " does not support server dry runs"}
		}
	}
	if err := registry.ValidateRequest(operationKey, request); err != nil {
		return nil, &usageError{detail: err.Error()}
	}
	return request, nil
}

func runJSONRead(cmd *cobra.Command, f *cmdutil.Factory, method string, path string, body []byte) error {
	authn, err := resolveAPIClient(cmd, f, false)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), method, path, nil, api.ExactJSONBody(body), api.IdempotencyNone)
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func runRecoverableJSONAction(cmd *cobra.Command, f *cmdutil.Factory, operationKey string, method string, path string, request []byte, flags *jsonRequestFlags, requireAck bool) error {
	op, err := operationByKey(operationKey)
	if err != nil {
		return err
	}
	op.Method = method
	op.Path = "/v1/" + strings.TrimLeft(path, "/")
	serverDryRun, err := operations.RequestDryRun(request)
	if err != nil {
		return &usageError{detail: err.Error()}
	}
	if serverDryRun {
		if flags.IdempotencyKey != "" {
			return &usageError{detail: "--idempotency-key is not used for server dry runs"}
		}
		return runJSONRead(cmd, f, method, path, request)
	}
	if requireAck {
		if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), fmt.Sprintf("This request changes connected data for %s. Continue?", operationKey)); err != nil {
			return err
		}
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
	store := operations.StoreForRuntime(authn.Runtime, f.Clock())
	out, err := operations.Start(cmd.Context(), authn.Client, operations.StartInput{
		Operation:      op,
		RuntimeProfile: authn.Runtime.Profile,
		Destination:    authn.Runtime.APIBaseURL,
		TokenPublicID:  authn.Identity.TokenPublicID,
		PrincipalID:    authn.Identity.PrincipalID,
		RequestBytes:   request,
		IdempotencyKey: key,
		Journal:        store,
	})
	if err != nil && !hasCommandOutput(out) {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	if flags.Wait && out.OperationID != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "accepted operation %s; resume with chab operations wait %s\n", out.OperationID, out.OperationID)
		waitRaw, payload, meta, waitErr := waitUntilTerminal(cmd, f, authn.Client, out.OperationID, 10*time.Minute)
		out = mergeWaitOutput(out, waitRaw, payload, meta)
		if waitErr != nil {
			if reason, ok := waitRecoveryReason(waitErr); ok {
				out.LocalRecovery.State = reason
				waitErr = waitRecoveryErrorForReason(out.OperationID, reason, waitErr)
			}
			if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
				return writeErr
			}
			return output.WithCredentialContext(waitErr, authn.Cred.Profile, authn.Cred.DisplayID)
		}
		if payload.ID != "" {
			out.OperationID = payload.ID
		}
		if out.Action != nil {
			updated, persistErr := store.MarkCompleted(out.Action.ID, payload, meta)
			if persistErr != nil {
				out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
				if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
					return writeErr
				}
				return output.WithCredentialContext(fmt.Errorf("operation %s reached terminal status %s but failed to persist local action state: %w", out.OperationID, payload.Status, persistErr), authn.Cred.Profile, authn.Cred.DisplayID)
			}
			projection := operations.Projection(updated)
			out.Action = &projection
			out.LocalRecovery.State = string(updated.State)
			out.LocalPersistence = &operations.PersistenceState{State: "persisted"}
		}
		if payload.Status != "succeeded" {
			err = &usageError{detail: fmt.Sprintf("operation %s finished with status %s", out.OperationID, payload.Status)}
		}
	}
	if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	return nil
}

func downloadToLocalPath(cmd *cobra.Command, f *cmdutil.Factory, client *api.Client, target func(io.Writer) (int64, api.ResponseMeta, error), flags downloadFlags, checksum string) error {
	if flags.Output == "" {
		return &usageError{detail: "--output is required"}
	}
	if flags.MaxBytes < 1 {
		return &usageError{detail: "--max-bytes must be positive"}
	}
	expected, err := normalizeChecksum(firstNonEmpty(flags.Checksum, checksum))
	if err != nil {
		return err
	}
	var written int64
	var meta api.ResponseMeta
	var actual string
	err = localfile.CreateExclusive(flags.Output, func(w io.Writer) error {
		var h hash.Hash
		dst := w
		if expected != "" {
			h = sha256.New()
			dst = io.MultiWriter(w, h)
		}
		n, responseMeta, err := target(dst)
		written = n
		meta = responseMeta
		if err != nil {
			return err
		}
		if h != nil {
			actual = "sha256:" + hex.EncodeToString(h.Sum(nil))
			if !strings.EqualFold(actual, expected) {
				return &usageError{detail: "download checksum mismatch"}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	view := operationResponseMeta(meta)
	return f.WriteResultWithMeta(cmd, transferOutput{Path: flags.Output, Bytes: written, Checksum: actual, RequestID: meta.RequestID, Meta: view}, meta, true, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			fmt.Fprintf(w, "Path: %s\nBytes: %d\n", flags.Output, written)
			if actual != "" {
				fmt.Fprintf(w, "Checksum: %s\n", actual)
			}
			if meta.RequestID != "" {
				fmt.Fprintf(w, "Request ID: %s\n", meta.RequestID)
			}
		},
		Plain: func(data, prose io.Writer) {
			fmt.Fprintln(data, flags.Output)
		},
	})
}

func normalizeChecksum(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(strings.ToLower(value), "sha256:") {
		return "", &usageError{detail: "--checksum must use sha256:<hex>"}
	}
	hexPart := value[len("sha256:"):]
	if len(hexPart) != 64 {
		return "", &usageError{detail: "--checksum sha256 value must be 64 hex characters"}
	}
	for _, r := range hexPart {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return "", &usageError{detail: "--checksum sha256 value must be hex"}
	}
	return "sha256:" + strings.ToLower(hexPart), nil
}

func fileSHA256(path string, maxBytes int64) (string, int64, error) {
	if maxBytes < 1 {
		return "", 0, fmt.Errorf("maximum upload size must be positive")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("upload file must be a regular file")
	}
	if info.Size() < 1 {
		return "", 0, fmt.Errorf("upload file must not be empty")
	}
	if info.Size() > maxBytes {
		return "", 0, fmt.Errorf("upload file exceeds configured byte limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(file, maxBytes))
	if err != nil {
		return "", n, err
	}
	if n == maxBytes {
		var extra [1]byte
		read, err := file.Read(extra[:])
		if read > 0 {
			return "", n, fmt.Errorf("upload file exceeds configured byte limit")
		}
		if err != nil && err != io.EOF {
			return "", n, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func semanticFingerprint(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	canon, err := operations.CanonicalizeJSON(data)
	if err != nil {
		return "", err
	}
	return operations.RequestSHA256(canon), nil
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func setIntQuery(q url.Values, name string, value int64, changed bool) {
	if changed {
		q.Set(name, strconv.FormatInt(value, 10))
	}
}

func setStringQuery(q url.Values, name string, value string) {
	if value != "" {
		q.Set(name, value)
	}
}

func resolvedFileID(raw json.RawMessage) string {
	var envelope struct {
		File struct {
			ID string `json:"id"`
		} `json:"file"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return envelope.File.ID
}

func fileState(raw json.RawMessage) string {
	var envelope struct {
		File struct {
			State string `json:"state"`
		} `json:"file"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return envelope.File.State
}

type fileReceipt struct {
	ID    string
	State string
}

func parseFileReceipt(raw json.RawMessage, meta api.ResponseMeta, detail string) (fileReceipt, error) {
	var envelope struct {
		File *struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"file"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fileReceipt{}, &api.ProtocolError{Detail: detail + " response data does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: meta.RedactError(err), Meta: meta}
	}
	if envelope.File == nil || envelope.File.ID == "" || envelope.File.State == "" {
		return fileReceipt{}, &api.ProtocolError{Detail: detail + " response missing file id or state", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return fileReceipt{ID: envelope.File.ID, State: envelope.File.State}, nil
}

func downloadMetadata(raw json.RawMessage, meta api.ResponseMeta) (downloadURL string, checksum string, err error) {
	var envelope struct {
		File *struct {
			DownloadURL *string `json:"download_url"`
			Checksum    string  `json:"checksum"`
			State       string  `json:"state"`
		} `json:"file"`
		Artifact *struct {
			DownloadURL *string `json:"download_url"`
			Checksum    string  `json:"checksum"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", "", &api.ProtocolError{Detail: "download metadata response does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: meta.RedactError(err), Meta: meta}
	}
	switch {
	case envelope.File != nil:
		if envelope.File.DownloadURL != nil {
			downloadURL = *envelope.File.DownloadURL
		}
		checksum = envelope.File.Checksum
		if downloadURL == "" && envelope.File.State != "" && envelope.File.State != "available" {
			return "", checksum, unavailableFileDownloadError(envelope.File.State, meta)
		}
	case envelope.Artifact != nil:
		if envelope.Artifact.DownloadURL != nil {
			downloadURL = *envelope.Artifact.DownloadURL
		}
		checksum = envelope.Artifact.Checksum
	}
	if downloadURL == "" {
		return "", checksum, &api.ProtocolError{Detail: "download metadata missing download_url", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return downloadURL, checksum, nil
}

func unavailableFileDownloadError(state string, meta api.ResponseMeta) error {
	detail := "file is not downloadable while state is " + state
	if meta.RequestID != "" {
		detail += " (request id " + meta.RequestID + ")"
	}
	return &usageError{detail: detail}
}

func mustOperation(operationKey string) chabcontract.Operation {
	op, err := operationByKey(operationKey)
	if err != nil {
		panic(err)
	}
	return op
}

func addProjectConnectionFlags(cmd *cobra.Command, projectID *int64, connectionID *int64) {
	cmd.Flags().Int64Var(projectID, "project-id", 0, "numeric Chab project id")
	cmd.Flags().Int64Var(connectionID, "connection-id", 0, "numeric connected provider connection id")
	_ = cmd.MarkFlagRequired("project-id")
	_ = cmd.MarkFlagRequired("connection-id")
}

func addProjectFlag(cmd *cobra.Command, projectID *int64) {
	cmd.Flags().Int64Var(projectID, "project-id", 0, "numeric Chab project id")
	_ = cmd.MarkFlagRequired("project-id")
}

func connectionQuery(projectID, connectionID int64) url.Values {
	q := url.Values{}
	q.Set("project_id", strconv.FormatInt(projectID, 10))
	q.Set("connection_id", strconv.FormatInt(connectionID, 10))
	return q
}

func projectQuery(projectID int64) url.Values {
	q := url.Values{}
	q.Set("project_id", strconv.FormatInt(projectID, 10))
	return q
}

func serverDryRunBody(request []byte) bool {
	dry, err := operations.RequestDryRun(request)
	return err == nil && dry
}

func fileTerminal(state string) bool {
	switch state {
	case "available", "scan_failed", "quarantined", "expired", "deleted":
		return true
	default:
		return false
	}
}

func fileErrorState(state string) bool {
	switch state {
	case "scan_failed", "quarantined", "expired":
		return true
	default:
		return false
	}
}

func retryDelay(meta api.ResponseMeta, fallback time.Duration) time.Duration {
	if meta.RetryAfter.Wait != nil {
		return *meta.RetryAfter.Wait
	}
	return fallback
}
