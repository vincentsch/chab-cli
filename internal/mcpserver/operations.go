package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/localfile"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

const (
	defaultMCPTransferMaxBytes = int64(100 * 1024 * 1024)
	defaultMCPUploadMaxBytes   = int64(100 * 1024 * 1024)
)

type operationCallInput struct {
	Path    map[string]json.RawMessage
	Query   map[string]json.RawMessage
	Body    json.RawMessage
	BodySet bool
	Local   operationLocalInput
}

type operationLocalInput struct {
	Confirmation      *bool
	IdempotencyKey    string
	Wait              bool
	TimeoutSeconds    int
	InputPath         string
	OutputPath        string
	MaxBytes          int64
	ApprovalProofPath string
}

type transferOutput struct {
	Path      string           `json:"path"`
	Bytes     int64            `json:"bytes"`
	Checksum  string           `json:"checksum,omitempty"`
	RequestID string           `json:"request_id,omitempty"`
	Meta      *output.MetaView `json:"meta,omitempty"`
}

type genericOperationOutput struct {
	Operation       string           `json:"operation"`
	Server          json.RawMessage  `json:"server,omitempty"`
	RequestID       string           `json:"request_id,omitempty"`
	Meta            *output.MetaView `json:"meta,omitempty"`
	LocalRecovery   any              `json:"local_recovery,omitempty"`
	DownloadedBytes *int64           `json:"downloaded_bytes,omitempty"`
	OutputPath      string           `json:"output_path,omitempty"`
}

type resolvedOperationRuntime struct {
	f        *cmdutil.Factory
	cmd      commandLike
	runtime  config.Runtime
	client   *api.Client
	cred     auth.Credential
	identity api.WhoamiData
}

type commandLike interface {
	Context() context.Context
	ErrOrStderr() io.Writer
}

func operationToolName(operationID string) string {
	var b strings.Builder
	b.WriteString("chab_")
	lastUnderscore := false
	for _, r := range operationID {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

func mcpExcludedOperation(operationID string) bool {
	switch operationID {
	case "cli.compatibility",
		"billing.auto_recharge.update",
		"billing.purchases.create",
		"tokens.create",
		"tokens.management_approvals.get",
		"webhooks.endpoints.create",
		"webhooks.endpoints.rotate_secret":
		return true
	default:
		return false
	}
}

func operationToolDescription(op chabcontract.Operation) string {
	parts := []string{fmt.Sprintf("%s %s (%s).", op.Method, op.Path, op.ID)}
	if op.Availability == "preview" {
		parts = append(parts, "Preview operation.")
	}
	if operationRequiresConfirmation(op) {
		parts = append(parts, "Live calls require local.confirmation=true.")
	}
	if op.IdempotencyRequired {
		parts = append(parts, "The tool records a local action receipt before live submission and reuses one idempotency key.")
	}
	if op.RequiresPaidPlan && !op.NotBillable {
		parts = append(parts, "May spend credits.")
	}
	if byteDownloadOperation(op.ID) {
		parts = append(parts, "Requires local.output_path and refuses to overwrite existing files.")
	}
	if op.ID == "files.create" {
		parts = append(parts, "Requires local.input_path for the file bytes.")
	}
	return strings.Join(parts, " ")
}

func operationReadOnly(op chabcontract.Operation) bool {
	return op.Method == http.MethodGet && !byteDownloadOperation(op.ID)
}

func operationDestructive(op chabcontract.Operation) bool {
	switch op.ID {
	case "operations.cancel", "operations.bulk_cancel", "projects.delete", "files.delete", "mail.messages.send",
		"drive.items.update", "drive.items.move", "drive.items.trash", "drive.permissions.update",
		"tokens.update", "tokens.revoke", "webhooks.endpoints.delete",
		"webhooks.deliveries.replay", "webhooks.replays.create":
		return true
	default:
		return false
	}
}

func operationRequiresConfirmation(op chabcontract.Operation) bool {
	if op.RequiresPaidPlan && !op.NotBillable {
		return true
	}
	switch op.ID {
	case "operations.cancel", "operations.bulk_cancel", "projects.delete",
		"files.create", "files.delete",
		"mail.messages.send",
		"drive.items.update", "drive.items.move", "drive.items.trash", "drive.permissions.update",
		"tokens.update", "tokens.revoke", "webhooks.endpoints.delete",
		"webhooks.deliveries.replay", "webhooks.replays.create":
		return true
	default:
		return false
	}
}

func tokenProofEligible(operationID string) bool {
	return operationID == "tokens.update" || operationID == "tokens.revoke"
}

func byteDownloadOperation(operationID string) bool {
	switch operationID {
	case "operations.artifact_download", "files.download", "drive.items.download", "mail.attachments.download":
		return true
	default:
		return false
	}
}

func (r registry) callOperation(ctx context.Context, req *mcp.CallToolRequest, op chabcontract.Operation) (*mcp.CallToolResult, error) {
	input, err := decodeOperationInput(req.Params.Arguments)
	if err != nil {
		return nil, invalidParams(err)
	}
	path, err := resolveOperationPath(op, input.Path)
	if err != nil {
		return nil, invalidParams(err)
	}
	query, err := resolveOperationQuery(op, input.Query)
	if err != nil {
		return nil, invalidParams(err)
	}
	if err := validateLocalControls(op, input.Local); err != nil {
		return nil, invalidParams(err)
	}

	var value any
	switch {
	case op.ID == "files.create":
		value, err = r.callFileUpload(ctx, op, input)
	case op.ID == "files.delete":
		value, err = r.callFileDelete(ctx, op, path, input)
	case byteDownloadOperation(op.ID):
		value, err = r.callByteDownload(ctx, op, path, query, input.Local)
	default:
		value, err = r.callJSONOperation(ctx, op, path, query, input)
	}
	if err != nil {
		return toolErrorWithPartial(r.opts, value, err), nil
	}
	return toolSuccess(value)
}

func (r registry) callJSONOperation(ctx context.Context, op chabcontract.Operation, path string, query url.Values, input operationCallInput) (value any, err error) {
	request, hasBody, err := requestBytes(op, input)
	if err != nil {
		return nil, err
	}
	dryRun, err := requestDryRun(request)
	if err != nil {
		return nil, mcpUsageError(err.Error())
	}
	if dryRun && !op.DryRunSupported {
		return nil, mcpUsageError("operation " + op.ID + " does not support server dry runs")
	}
	if operationRequiresConfirmation(op) && !dryRun && !confirmed(input.Local) {
		return nil, mcpUsageError("operation " + op.ID + " requires local.confirmation=true")
	}

	rt, err := r.resolveRuntime(ctx, op.IdempotencyRequired && !dryRun)
	if err != nil {
		return nil, err
	}
	defer func() {
		value = redactOperationValue(rt.f, value)
	}()
	var body any
	if hasBody {
		body = api.ExactJSONBody(request)
	}
	if !op.IdempotencyRequired || dryRun {
		header, err := approvalHeaderFromLocal(rt.f, input.Local)
		if err != nil {
			return nil, err
		}
		result, err := rt.client.DoRawWithHeaders(ctx, op.Method, path, query, body, api.IdempotencyNone, header)
		if err != nil {
			return nil, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
		}
		return genericOperationOutput{
			Operation: op.ID,
			Server:    redactRawJSON(rt.f, result.Data),
			RequestID: result.Meta.RequestID,
			Meta:      metaView(result.Meta),
		}, nil
	}

	key := input.Local.IdempotencyKey
	if key == "" {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return nil, err
		}
	} else if err := api.ValidateIdempotencyKey(key); err != nil {
		return nil, err
	}
	rt.f.RegisterSecret(key)
	header, err := approvalHeaderFromLocal(rt.f, input.Local)
	if err != nil {
		return nil, err
	}
	actionOp := op
	actionOp.Path = "/v1/" + strings.TrimLeft(path, "/")
	out, err := operations.Start(ctx, rt.client, operations.StartInput{
		Operation:      actionOp,
		RuntimeProfile: rt.runtime.Profile,
		Destination:    rt.runtime.APIBaseURL,
		TokenPublicID:  rt.identity.TokenPublicID,
		PrincipalID:    rt.identity.PrincipalID,
		RequestBytes:   request,
		IdempotencyKey: key,
		Headers:        header,
		Journal:        operations.StoreForRuntime(rt.runtime, rt.f.Clock()),
	})
	if err != nil && !commandOutputPresent(out) {
		return nil, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	if input.Local.Wait && out.OperationID != "" {
		out, err = waitForOperation(ctx, rt, operations.StoreForRuntime(rt.runtime, rt.f.Clock()), out, timeoutFromLocal(input.Local))
		if err != nil && !commandOutputPresent(out) {
			return nil, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
		}
	}
	if err != nil {
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	return out, nil
}

func (r registry) callByteDownload(ctx context.Context, op chabcontract.Operation, path string, query url.Values, local operationLocalInput) (any, error) {
	if local.OutputPath == "" {
		return nil, mcpUsageError("byte download requires local.output_path")
	}
	maxBytes := local.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMCPTransferMaxBytes
	}
	rt, err := r.resolveRuntime(ctx, false)
	if err != nil {
		return nil, err
	}
	var written int64
	var meta api.ResponseMeta
	var checksum string
	err = localfile.CreateExclusive(local.OutputPath, func(w io.Writer) error {
		expected := ""
		download := func(dst io.Writer) (int64, api.ResponseMeta, error) {
			opts := api.DownloadOptions{Query: query, MaxBytes: maxBytes, FollowHTTPSRedirect: downloadFollowsRedirect(op.ID)}
			return rt.client.DownloadWithOptions(ctx, path, dst, opts)
		}
		if metadataPath, ok, err := byteDownloadMetadataPath(op.ID, path); err != nil {
			return err
		} else if ok {
			if len(query) > 0 {
				return mcpUsageError("metadata-backed byte downloads do not accept query parameters")
			}
			result, err := rt.client.DoRaw(ctx, http.MethodGet, metadataPath, nil, nil, api.IdempotencyNone)
			if err != nil {
				return err
			}
			link, metadataChecksum, err := byteDownloadMetadata(result.Data, result.Meta)
			if err != nil {
				return err
			}
			expected = metadataChecksum
			download = func(dst io.Writer) (int64, api.ResponseMeta, error) {
				opts := api.DownloadOptions{MaxBytes: maxBytes, FollowHTTPSRedirect: downloadFollowsRedirect(op.ID)}
				return rt.client.DownloadAPILink(ctx, link, dst, opts)
			}
		}
		n, responseMeta, actual, err := downloadWithChecksum(w, expected, download)
		written = n
		meta = responseMeta
		checksum = actual
		return err
	})
	if err != nil {
		return nil, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	return transferOutput{Path: local.OutputPath, Bytes: written, Checksum: checksum, RequestID: meta.RequestID, Meta: metaView(meta)}, nil
}

func downloadFollowsRedirect(operationID string) bool {
	return operationID == "operations.artifact_download" || operationID == "drive.items.download"
}

func redactRawJSON(f *cmdutil.Factory, raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || f == nil {
		return raw
	}
	return json.RawMessage(f.RedactJSON(raw))
}

func redactOperationValue(f *cmdutil.Factory, value any) any {
	switch v := value.(type) {
	case genericOperationOutput:
		v.Server = redactRawJSON(f, v.Server)
		return v
	case operations.CommandOutput:
		return redactCommandOutput(f, v)
	default:
		return value
	}
}

func redactCommandOutput(f *cmdutil.Factory, out operations.CommandOutput) operations.CommandOutput {
	if f == nil {
		return out
	}
	data, err := output.StableJSONBytes(out)
	if err != nil {
		out.Server = redactRawJSON(f, out.Server)
		return out
	}
	redacted := f.RedactJSON(data)
	var next operations.CommandOutput
	if err := json.Unmarshal(redacted, &next); err != nil {
		out.Server = redactRawJSON(f, out.Server)
		return out
	}
	return next
}

func byteDownloadMetadataPath(operationID, downloadPath string) (string, bool, error) {
	switch operationID {
	case "files.download", "operations.artifact_download":
		if !strings.HasSuffix(downloadPath, "/download") {
			return "", true, mcpUsageError("byte download metadata path could not be derived")
		}
		return strings.TrimSuffix(downloadPath, "/download"), true, nil
	default:
		return "", false, nil
	}
}

func byteDownloadMetadata(raw json.RawMessage, meta api.ResponseMeta) (downloadURL string, checksum string, err error) {
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
			return "", checksum, unavailableByteDownloadError(envelope.File.State, meta)
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

func unavailableByteDownloadError(state string, meta api.ResponseMeta) error {
	detail := "file is not downloadable while state is " + state
	if meta.RequestID != "" {
		detail += " (request id " + meta.RequestID + ")"
	}
	return mcpUsageError(detail)
}

func downloadWithChecksum(w io.Writer, checksum string, target func(io.Writer) (int64, api.ResponseMeta, error)) (int64, api.ResponseMeta, string, error) {
	expected, err := normalizeDownloadChecksum(checksum)
	if err != nil {
		return 0, api.ResponseMeta{}, "", err
	}
	var h hash.Hash
	dst := w
	if expected != "" {
		h = sha256.New()
		dst = io.MultiWriter(w, h)
	}
	n, meta, err := target(dst)
	if err != nil {
		return n, meta, "", err
	}
	if h == nil {
		return n, meta, "", nil
	}
	actual := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return n, meta, actual, mcpUsageError("download checksum mismatch")
	}
	return n, meta, actual, nil
}

func normalizeDownloadChecksum(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(strings.ToLower(value), "sha256:") {
		return "", mcpUsageError("download checksum must use sha256:<hex>")
	}
	hexPart := value[len("sha256:"):]
	if len(hexPart) != 64 {
		return "", mcpUsageError("download checksum sha256 value must be 64 hex characters")
	}
	for _, r := range hexPart {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return "", mcpUsageError("download checksum sha256 value must be hex")
	}
	return "sha256:" + strings.ToLower(hexPart), nil
}

func (r registry) callFileUpload(ctx context.Context, op chabcontract.Operation, input operationCallInput) (value any, err error) {
	if !confirmed(input.Local) {
		return nil, mcpUsageError("operation files.create requires local.confirmation=true")
	}
	sourcePath := input.Local.InputPath
	if sourcePath == "" {
		return nil, mcpUsageError("files.create requires local.input_path")
	}
	maxBytes := input.Local.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMCPUploadMaxBytes
	}
	fields, fingerprint, filename, err := fileUploadFields(input.Body, input.BodySet, sourcePath, maxBytes)
	if err != nil {
		return nil, err
	}
	key := input.Local.IdempotencyKey
	if key == "" {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return nil, err
		}
	} else if err := api.ValidateIdempotencyKey(key); err != nil {
		return nil, err
	}
	rt, err := r.resolveRuntime(ctx, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		value = redactOperationValue(rt.f, value)
	}()
	rt.f.RegisterSecret(key)
	store := operations.StoreForRuntime(rt.runtime, rt.f.Clock())
	prepared, err := store.Prepare(operations.PrepareInput{
		Profile:            rt.runtime.Profile,
		Destination:        rt.runtime.APIBaseURL,
		TokenPublicID:      rt.identity.TokenPublicID,
		PrincipalID:        rt.identity.PrincipalID,
		RequiredScope:      op.RequiredScope,
		OperationKey:       op.ID,
		Method:             http.MethodPost,
		Path:               op.Path,
		RequestFingerprint: fingerprint,
		IdempotencyKey:     key,
	})
	if err != nil {
		return nil, err
	}
	projection := operations.Projection(prepared.Record)
	if prepared.AlreadyCompleted || prepared.AlreadyAccepted {
		return operations.CommandOutput{
			Action:      &projection,
			OperationID: prepared.Record.AcceptedOperationID,
			RequestID:   prepared.Record.RequestID,
			LocalRecovery: operations.RecoveryStatus{
				State:       string(prepared.Record.State),
				CanResume:   prepared.Record.State != operations.ActionCompleted,
				KnownRemote: prepared.Record.AcceptedOperationID != "",
				ResumeHint:  "call chab_action_resume with the same local.input_path",
			},
			LocalPersistence: &operations.PersistenceState{State: "persisted"},
		}, nil
	}
	if err := store.ReplayablePrepared(prepared); err != nil {
		return nil, err
	}
	result, err := rt.client.PostMultipart(ctx, "files", api.MultipartFileUpload{
		FilePath:      sourcePath,
		FileFieldName: "file",
		Filename:      filename,
		Fields:        fields,
		MaxBytes:      maxBytes,
		Idempotency:   api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing),
	})
	if err != nil {
		updated, persistence := markActionUnknown(store, prepared.Record, err, prepared.Existing)
		out := outputFromRecord(updated)
		out.LocalPersistence = persistence
		setActionFailureRecovery(&out, persistence, "call chab_action_resume with the same local.input_path")
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	payload, err := fileUploadPayload(result.Data, result.Meta)
	if err != nil {
		updated, persistence := markActionUnknown(store, prepared.Record, err, prepared.Existing)
		out := outputFromRecord(updated)
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		out.LocalPersistence = persistence
		setActionFailureRecovery(&out, persistence, "call chab_action_resume with the same local.input_path")
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	updated, persistErr := store.MarkCompleted(prepared.Record.ID, payload, result.Meta)
	if persistErr != nil {
		out := outputFromRecord(prepared.Record)
		out.OperationID = payload.ID
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
		out.LocalRecovery.State = string(operations.ActionUnknown)
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.KnownRemote = payload.ID != ""
		out.LocalRecovery.ResumeHint = "call chab_action_resume with the same local.input_path"
		return out, persistErr
	}
	projection = operations.Projection(updated)
	out := operations.CommandOutput{
		Action:      &projection,
		OperationID: payload.ID,
		Server:      result.Data,
		RequestID:   result.Meta.RequestID,
		Meta:        metaView(result.Meta),
		LocalRecovery: operations.RecoveryStatus{
			State:       string(updated.State),
			CanResume:   false,
			KnownRemote: payload.ID != "",
		},
		LocalPersistence: &operations.PersistenceState{State: "persisted"},
	}
	if input.Local.Wait && payload.ID != "" && payload.Status != "available" {
		out, err = waitForFileReady(ctx, rt, store, out, timeoutFromLocal(input.Local))
	}
	return out, err
}

func (r registry) callFileDelete(ctx context.Context, op chabcontract.Operation, path string, input operationCallInput) (value any, err error) {
	if !confirmed(input.Local) {
		return nil, mcpUsageError("operation files.delete requires local.confirmation=true")
	}
	if _, _, err := requestBytes(op, input); err != nil {
		return nil, err
	}
	fileID := strings.TrimPrefix(path, "files/")
	if fileID == "" || fileID == path {
		return nil, mcpUsageError("files.delete requires path.file_id")
	}
	key := input.Local.IdempotencyKey
	if key == "" {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return nil, err
		}
	} else if err := api.ValidateIdempotencyKey(key); err != nil {
		return nil, err
	}
	rt, err := r.resolveRuntime(ctx, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		value = redactOperationValue(rt.f, value)
	}()
	rt.f.RegisterSecret(key)
	store := operations.StoreForRuntime(rt.runtime, rt.f.Clock())
	actionPath := "/v1/" + strings.TrimLeft(path, "/")
	prepared, err := store.Prepare(operations.PrepareInput{
		Profile:        rt.runtime.Profile,
		Destination:    rt.runtime.APIBaseURL,
		TokenPublicID:  rt.identity.TokenPublicID,
		PrincipalID:    rt.identity.PrincipalID,
		RequiredScope:  op.RequiredScope,
		OperationKey:   op.ID,
		Method:         http.MethodDelete,
		Path:           actionPath,
		IdempotencyKey: key,
	})
	if err != nil {
		return nil, err
	}
	if prepared.AlreadyCompleted || prepared.AlreadyAccepted {
		out := outputFromRecord(prepared.Record)
		out.OperationID = prepared.Record.AcceptedOperationID
		if prepared.AlreadyAccepted {
			out.LocalRecovery.CanResume = true
			out.LocalRecovery.KnownRemote = out.OperationID != ""
			out.LocalRecovery.ResumeHint = "call chab_action_resume"
		}
		if input.Local.Wait && out.OperationID != "" && prepared.AlreadyAccepted {
			return waitForFileDeleted(ctx, rt, store, out, timeoutFromLocal(input.Local))
		}
		return out, nil
	}
	if err := store.ReplayablePrepared(prepared); err != nil {
		return nil, err
	}
	result, err := rt.client.DoRaw(ctx, http.MethodDelete, path, nil, nil, api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing))
	if err != nil {
		updated, persistence := markActionUnknown(store, prepared.Record, err, prepared.Existing)
		out := outputFromRecord(updated)
		out.LocalPersistence = persistence
		setActionFailureRecovery(&out, persistence, "call chab_action_resume")
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	payload, err := fileDeletePayload(result.Data, result.Meta, fileID)
	if err != nil {
		updated, persistence := markActionUnknown(store, prepared.Record, err, prepared.Existing)
		out := outputFromRecord(updated)
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		out.LocalPersistence = persistence
		setActionFailureRecovery(&out, persistence, "call chab_action_resume")
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	var updated operations.ActionRecord
	if fileDeleteTerminal(payload.Status) {
		updated, err = store.MarkCompleted(prepared.Record.ID, payload, result.Meta)
	} else {
		updated, err = store.MarkAccepted(prepared.Record.ID, payload, result.Meta)
	}
	if err != nil {
		out := outputFromRecord(prepared.Record)
		out.OperationID = payload.ID
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
		return out, err
	}
	out := outputFromRecord(updated)
	out.OperationID = payload.ID
	out.Server = result.Data
	out.RequestID = result.Meta.RequestID
	out.Meta = metaView(result.Meta)
	out.LocalPersistence = &operations.PersistenceState{State: "persisted"}
	if updated.State == operations.ActionAccepted {
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.KnownRemote = true
		out.LocalRecovery.ResumeHint = "call chab_action_resume"
	}
	if input.Local.Wait && payload.Status == "deletion_pending" {
		return waitForFileDeleted(ctx, rt, store, out, timeoutFromLocal(input.Local))
	}
	return out, nil
}

func (r registry) resolveRuntime(ctx context.Context, identity bool) (resolvedOperationRuntime, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return resolvedOperationRuntime{}, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return resolvedOperationRuntime{}, withToolFactory(f, err)
	}
	client, cred, err := apiClient(f, cmd, rt)
	if err != nil {
		return resolvedOperationRuntime{}, withToolFactory(f, err)
	}
	out := resolvedOperationRuntime{f: f, cmd: cmd, runtime: rt, client: client, cred: cred}
	if identity {
		id, _, err := client.Whoami(ctx)
		if err != nil {
			return resolvedOperationRuntime{}, withToolFactory(f, output.WithCredentialContext(err, cred.Profile, cred.DisplayID))
		}
		out.identity = id
	}
	return out, nil
}

func requestBytes(op chabcontract.Operation, input operationCallInput) ([]byte, bool, error) {
	if len(op.RequestSchema) == 0 {
		if input.BodySet {
			return nil, false, mcpUsageError(op.ID + " does not accept a JSON body")
		}
		return nil, false, nil
	}
	if op.ID == "files.create" {
		return nil, false, nil
	}
	raw := input.Body
	if !input.BodySet {
		raw = json.RawMessage(`{}`)
	}
	request, err := operations.CanonicalizeJSON(raw)
	if err != nil {
		return nil, false, mcpUsageError(err.Error())
	}
	registry, err := chabcontract.Load()
	if err != nil {
		return nil, false, err
	}
	if err := registry.ValidateRequest(op.ID, request); err != nil {
		return nil, false, mcpUsageError(err.Error())
	}
	return request, true, nil
}

func requestDryRun(request []byte) (bool, error) {
	if len(request) == 0 {
		return false, nil
	}
	return operations.RequestDryRun(request)
}

func decodeOperationInput(raw json.RawMessage) (operationCallInput, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var obj rawObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return operationCallInput{}, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if obj == nil {
		return operationCallInput{}, errors.New("arguments must be a JSON object")
	}
	var input operationCallInput
	for key, value := range obj {
		if string(value) == "null" {
			return operationCallInput{}, fmt.Errorf("argument %q must not be null", key)
		}
		switch key {
		case "path":
			if err := decodeRawObject(value, &input.Path, key); err != nil {
				return operationCallInput{}, err
			}
		case "query":
			if err := decodeRawObject(value, &input.Query, key); err != nil {
				return operationCallInput{}, err
			}
		case "body":
			input.BodySet = true
			input.Body = append(json.RawMessage(nil), value...)
		case "local":
			local, err := decodeLocalInput(value)
			if err != nil {
				return operationCallInput{}, err
			}
			input.Local = local
		default:
			return operationCallInput{}, fmt.Errorf("unknown argument %q", key)
		}
	}
	return input, nil
}

func decodeRawObject(raw json.RawMessage, target *map[string]json.RawMessage, name string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("argument %q must be an object", name)
	}
	if obj == nil {
		return fmt.Errorf("argument %q must be an object", name)
	}
	*target = obj
	return nil
}

func decodeLocalInput(raw json.RawMessage) (operationLocalInput, error) {
	var obj rawObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return operationLocalInput{}, fmt.Errorf("argument %q must be an object", "local")
	}
	if obj == nil {
		return operationLocalInput{}, fmt.Errorf("argument %q must be an object", "local")
	}
	var input operationLocalInput
	for key, value := range obj {
		if string(value) == "null" {
			return operationLocalInput{}, fmt.Errorf("local.%s must not be null", key)
		}
		switch key {
		case "confirmation":
			var v bool
			if err := json.Unmarshal(value, &v); err != nil {
				return operationLocalInput{}, errors.New("local.confirmation must be a boolean")
			}
			input.Confirmation = &v
		case "idempotency_key":
			if err := decodeLocalString(value, &input.IdempotencyKey, "idempotency_key"); err != nil {
				return operationLocalInput{}, err
			}
		case "wait":
			if err := json.Unmarshal(value, &input.Wait); err != nil {
				return operationLocalInput{}, errors.New("local.wait must be a boolean")
			}
		case "timeout_seconds":
			n, err := decodePositiveInt64(value, "timeout_seconds")
			if err != nil {
				return operationLocalInput{}, err
			}
			input.TimeoutSeconds = int(n)
		case "input_path":
			if err := decodeLocalString(value, &input.InputPath, "input_path"); err != nil {
				return operationLocalInput{}, err
			}
		case "output_path":
			if err := decodeLocalString(value, &input.OutputPath, "output_path"); err != nil {
				return operationLocalInput{}, err
			}
		case "max_bytes":
			n, err := decodePositiveInt64(value, "max_bytes")
			if err != nil {
				return operationLocalInput{}, err
			}
			input.MaxBytes = n
		case "approval_proof_path":
			if err := decodeLocalString(value, &input.ApprovalProofPath, "approval_proof_path"); err != nil {
				return operationLocalInput{}, err
			}
		default:
			return operationLocalInput{}, fmt.Errorf("unknown local control %q", key)
		}
	}
	return input, nil
}

func decodeLocalString(raw json.RawMessage, target *string, name string) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("local.%s must be a string", name)
	}
	if value == "" {
		return fmt.Errorf("local.%s must not be empty", name)
	}
	*target = value
	return nil
}

func decodePositiveInt64(raw json.RawMessage, name string) (int64, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var n json.Number
	if err := dec.Decode(&n); err != nil {
		return 0, fmt.Errorf("local.%s must be an integer", name)
	}
	value, err := n.Int64()
	if err != nil || value < 1 {
		return 0, fmt.Errorf("local.%s must be a positive integer", name)
	}
	return value, nil
}

func validateLocalControls(op chabcontract.Operation, local operationLocalInput) error {
	if local.IdempotencyKey != "" && !op.IdempotencyRequired {
		return errors.New("local.idempotency_key is only valid for idempotent mutation tools")
	}
	if local.InputPath != "" && op.ID != "files.create" {
		return errors.New("local.input_path is only valid for upload tools")
	}
	if local.OutputPath != "" && !byteDownloadOperation(op.ID) {
		return errors.New("local.output_path is only valid for byte-download tools")
	}
	if local.ApprovalProofPath != "" && !tokenProofEligible(op.ID) {
		return errors.New("local.approval_proof_path is only valid for protected token mutation tools")
	}
	if local.Confirmation != nil && !operationRequiresConfirmation(op) {
		return errors.New("local.confirmation does not apply to this tool")
	}
	if local.Wait && !op.IdempotencyRequired && op.ID != "files.create" {
		return errors.New("local.wait is only valid for recoverable action tools")
	}
	return nil
}

func confirmed(local operationLocalInput) bool {
	return local.Confirmation != nil && *local.Confirmation
}

func resolveOperationPath(op chabcontract.Operation, values map[string]json.RawMessage) (string, error) {
	allowed := map[string]bool{}
	var segments []string
	for _, segment := range strings.Split(strings.TrimPrefix(op.Path, "/v1/"), "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			allowed[name] = true
			raw, ok := values[name]
			if !ok {
				return "", fmt.Errorf("path.%s is required", name)
			}
			text, err := scalarPathValue(raw, name)
			if err != nil {
				return "", err
			}
			segments = append(segments, text)
			continue
		}
		segments = append(segments, segment)
	}
	for name := range values {
		if !allowed[name] {
			return "", fmt.Errorf("unknown path parameter %q", name)
		}
	}
	return api.Path(segments...), nil
}

func scalarPathValue(raw json.RawMessage, name string) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", fmt.Errorf("path.%s must be a scalar", name)
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("path.%s must not be empty", name)
		}
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", fmt.Errorf("path.%s must be a string, number, or boolean", name)
	}
}

func resolveOperationQuery(op chabcontract.Operation, values map[string]json.RawMessage) (url.Values, error) {
	allowed := map[string]chabcontract.Parameter{}
	for _, param := range op.Parameters {
		if param.In == "query" {
			allowed[param.Name] = param
		}
	}
	query := url.Values{}
	for _, param := range allowed {
		raw, ok := values[param.Name]
		if !ok {
			if param.Required {
				return nil, fmt.Errorf("query.%s is required", param.Name)
			}
			continue
		}
		items, err := queryValues(raw, param.Name)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			query.Add(param.Name, item)
		}
	}
	for name := range values {
		if _, ok := allowed[name]; !ok {
			return nil, fmt.Errorf("unknown query parameter %q", name)
		}
	}
	return query, nil
}

func queryValues(raw json.RawMessage, name string) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("query.%s must be a scalar or array of scalars", name)
	}
	switch v := value.(type) {
	case string:
		return []string{v}, nil
	case json.Number:
		return []string{v.String()}, nil
	case bool:
		return []string{strconv.FormatBool(v)}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			switch scalar := item.(type) {
			case string:
				out = append(out, scalar)
			case json.Number:
				out = append(out, scalar.String())
			case bool:
				out = append(out, strconv.FormatBool(scalar))
			default:
				return nil, fmt.Errorf("query.%s array items must be scalars", name)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("query.%s must be a scalar or array of scalars", name)
	}
}

func approvalHeaderFromLocal(f *cmdutil.Factory, local operationLocalInput) (http.Header, error) {
	if local.ApprovalProofPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(local.ApprovalProofPath)
	if err != nil {
		return nil, mcpUsageError("could not read approval proof file")
	}
	proof, err := decodeApprovalProof(data)
	if err != nil {
		return nil, err
	}
	f.RegisterSecret(proof)
	return http.Header{"X-Chab-Management-Approval": []string{proof}}, nil
}

func decodeApprovalProof(data []byte) (string, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) == nil {
		if raw := object["proof"]; len(raw) > 0 {
			var proof string
			if json.Unmarshal(raw, &proof) == nil && proof != "" {
				return proof, nil
			}
		}
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n\t ") || strings.HasPrefix(trimmed, "{") {
		return "", mcpUsageError("approval proof file must contain the private JSON proof format")
	}
	return trimmed, nil
}

func timeoutFromLocal(local operationLocalInput) time.Duration {
	if local.TimeoutSeconds > 0 {
		return time.Duration(local.TimeoutSeconds) * time.Second
	}
	return 10 * time.Minute
}

func waitForOperation(ctx context.Context, rt resolvedOperationRuntime, store operations.Store, out operations.CommandOutput, timeout time.Duration) (operations.CommandOutput, error) {
	deadline := rt.f.Clock()().Add(timeout)
	for {
		remaining := deadline.Sub(rt.f.Clock()())
		if remaining <= 0 {
			out.LocalRecovery.State = "timeout"
			out.LocalRecovery.CanResume = true
			out.LocalRecovery.KnownRemote = out.OperationID != ""
			out.LocalRecovery.ResumeHint = "call chab_action_resume or poll " + out.OperationID
			return out, nil
		}
		wait := remaining
		if wait > 30*time.Second {
			wait = 30 * time.Second
		}
		requestCtx, cancel := context.WithTimeout(ctx, remaining)
		raw, payload, meta, err := operations.Status(requestCtx, rt.client, out.OperationID, wait)
		cancel()
		if err != nil {
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				out.LocalRecovery.State = "timeout"
				out.LocalRecovery.CanResume = true
				out.LocalRecovery.KnownRemote = out.OperationID != ""
				out.LocalRecovery.ResumeHint = "call chab_action_resume or poll " + out.OperationID
				return out, nil
			}
			return out, err
		}
		out.Server = raw
		out.RequestID = meta.RequestID
		out.Meta = metaView(meta)
		if err := operations.ValidateStatus(payload.Status, out.OperationID); err != nil {
			return out, err
		}
		if operations.ActiveStatus(payload.Status) {
			out.LocalRecovery.State = payload.Status
			out.LocalRecovery.CanResume = true
			out.LocalRecovery.KnownRemote = true
			out.LocalRecovery.ResumeHint = "poll " + out.OperationID
			sleep := retryDelay(meta, 2*time.Second)
			remaining = deadline.Sub(rt.f.Clock()())
			if sleep > remaining {
				sleep = remaining
			}
			if sleep > 0 {
				if err := rt.f.Sleep(ctx, sleep); err != nil {
					return out, err
				}
			}
			continue
		}
		if out.Action != nil {
			updated, err := store.MarkCompleted(out.Action.ID, payload, meta)
			if err != nil {
				out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
				return out, err
			}
			projection := operations.Projection(updated)
			out.Action = &projection
			out.LocalPersistence = &operations.PersistenceState{State: "persisted"}
			out.LocalRecovery.State = string(updated.State)
		}
		out.LocalRecovery.CanResume = false
		out.LocalRecovery.KnownRemote = true
		out.LocalRecovery.ResumeHint = ""
		return out, nil
	}
}

func fileUploadFields(raw json.RawMessage, bodySet bool, sourcePath string, maxBytes int64) (map[string]string, string, string, error) {
	body := map[string]json.RawMessage{}
	if bodySet {
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, "", "", mcpUsageError("files.create body must be an object")
		}
		for name := range body {
			switch name {
			case "project_id", "retention_hours", "filename":
			default:
				return nil, "", "", mcpUsageError("files.create body supports only project_id, retention_hours and filename; file bytes must use local.input_path")
			}
		}
	}
	retention := int64(72)
	fields := map[string]string{}
	projectValue := any(nil)
	if rawProject, ok := body["project_id"]; ok {
		projectID, err := rawInt64(rawProject, "body.project_id")
		if err != nil {
			return nil, "", "", err
		}
		if projectID < 1 {
			return nil, "", "", mcpUsageError("body.project_id must be at least 1")
		}
		projectValue = projectID
		fields["project_id"] = strconv.FormatInt(projectID, 10)
	}
	if rawRetention, ok := body["retention_hours"]; ok {
		value, err := rawInt64(rawRetention, "body.retention_hours")
		if err != nil {
			return nil, "", "", err
		}
		if value < 24 || value > 720 {
			return nil, "", "", mcpUsageError("body.retention_hours must be between 24 and 720")
		}
		retention = value
		fields["retention_hours"] = strconv.FormatInt(value, 10)
	}
	filename := filepath.Base(sourcePath)
	if rawFilename, ok := body["filename"]; ok {
		if err := json.Unmarshal(rawFilename, &filename); err != nil || strings.TrimSpace(filename) == "" {
			return nil, "", "", mcpUsageError("body.filename must be a non-empty string")
		}
		fields["filename"] = filename
	}
	digest, size, err := fileSHA256(sourcePath, maxBytes)
	if err != nil {
		return nil, "", "", mcpUsageError("could not read upload file: " + err.Error())
	}
	if size < 1 {
		return nil, "", "", mcpUsageError("upload file must not be empty")
	}
	fingerprint, err := semanticFingerprint(map[string]any{
		"version":         1,
		"project_id":      projectValue,
		"retention_hours": retention,
		"filename":        strings.TrimSpace(filename),
		"byte_size":       size,
		"sha256":          strings.ToLower(digest),
	})
	if err != nil {
		return nil, "", "", err
	}
	return fields, fingerprint, filename, nil
}

func rawInt64(raw json.RawMessage, name string) (int64, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var n json.Number
	if err := dec.Decode(&n); err != nil {
		return 0, mcpUsageError(name + " must be an integer")
	}
	value, err := n.Int64()
	if err != nil {
		return 0, mcpUsageError(name + " must be an integer")
	}
	return value, nil
}

func fileSHA256(path string, maxBytes int64) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("must be a regular file")
	}
	if info.Size() > maxBytes {
		return "", info.Size(), fmt.Errorf("upload file exceeds configured byte limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", n, err
	}
	if n > maxBytes {
		return "", n, fmt.Errorf("upload file exceeds configured byte limit")
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
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

func fileUploadPayload(raw json.RawMessage, meta api.ResponseMeta) (operations.OperationPayload, error) {
	return filePayload(raw, meta, "files.create", "file upload")
}

func fileDeletePayload(raw json.RawMessage, meta api.ResponseMeta, expectedID string) (operations.OperationPayload, error) {
	payload, err := filePayload(raw, meta, "files.delete", "file deletion")
	if err != nil {
		return operations.OperationPayload{}, err
	}
	if payload.ID != expectedID {
		return operations.OperationPayload{}, &api.ProtocolError{Detail: "file deletion response returned a different file id", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	if payload.Status != "deletion_pending" && payload.Status != "deleted" {
		return operations.OperationPayload{}, &api.ProtocolError{Detail: "file deletion response returned unsupported file state " + payload.Status, Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return payload, nil
}

func filePayload(raw json.RawMessage, meta api.ResponseMeta, operationKey, context string) (operations.OperationPayload, error) {
	var decoded struct {
		File *struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"file"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.File == nil || decoded.File.ID == "" {
		if err == nil {
			err = errors.New("missing file.id")
		}
		return operations.OperationPayload{}, &api.ProtocolError{Detail: context + " response data does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: meta.RedactError(err), Meta: meta}
	}
	return operations.OperationPayload{ID: decoded.File.ID, OperationKey: operationKey, Family: "files", Status: decoded.File.State}, nil
}

func fileDeleteTerminal(state string) bool {
	return state == "deleted"
}

func markActionUnknown(store operations.Store, record operations.ActionRecord, cause error, priorUnknown bool) (operations.ActionRecord, *operations.PersistenceState) {
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

func setActionFailureRecovery(out *operations.CommandOutput, persistence *operations.PersistenceState, hint string) {
	out.LocalRecovery.CanResume = out.LocalRecovery.CanResume && persistence.State == "persisted"
	if out.LocalRecovery.CanResume {
		out.LocalRecovery.ResumeHint = hint
	} else {
		out.LocalRecovery.ResumeHint = ""
	}
}

func waitForFileReady(ctx context.Context, rt resolvedOperationRuntime, store operations.Store, out operations.CommandOutput, timeout time.Duration) (operations.CommandOutput, error) {
	deadline := rt.f.Clock()().Add(timeout)
	for {
		remaining := deadline.Sub(rt.f.Clock()())
		if remaining <= 0 {
			out.LocalRecovery.State = "timeout"
			out.LocalRecovery.CanResume = true
			out.LocalRecovery.KnownRemote = true
			out.LocalRecovery.ResumeHint = "call chab_action_resume with the same local.input_path"
			return out, nil
		}
		var wrapped struct {
			File *struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"file"`
		}
		requestCtx, cancel := context.WithTimeout(ctx, remaining)
		meta, err := rt.client.Get(requestCtx, api.Path("files", out.OperationID), nil, &wrapped)
		cancel()
		if err != nil {
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				out.LocalRecovery.State = "timeout"
				out.LocalRecovery.CanResume = true
				out.LocalRecovery.KnownRemote = true
				out.LocalRecovery.ResumeHint = "call chab_action_resume with the same local.input_path"
				return out, nil
			}
			return out, err
		}
		raw, _ := json.Marshal(wrapped)
		out.Server = raw
		out.RequestID = meta.RequestID
		out.Meta = metaView(meta)
		if wrapped.File == nil || wrapped.File.ID == "" {
			return out, &api.ProtocolError{Detail: "file status response missing data.file", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
		}
		switch wrapped.File.State {
		case "available", "failed", "expired":
			if out.Action != nil {
				payload := operations.OperationPayload{ID: wrapped.File.ID, OperationKey: "files.create", Family: "files", Status: wrapped.File.State}
				updated, err := store.MarkCompleted(out.Action.ID, payload, meta)
				if err != nil {
					out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
					return out, err
				}
				projection := operations.Projection(updated)
				out.Action = &projection
			}
			out.LocalRecovery.CanResume = false
			out.LocalRecovery.KnownRemote = true
			return out, nil
		default:
			wait := retryDelay(meta, 5*time.Second)
			remaining = deadline.Sub(rt.f.Clock()())
			if wait > remaining {
				wait = remaining
			}
			if wait <= 0 {
				continue
			}
			if err := rt.f.Sleep(ctx, wait); err != nil {
				return out, err
			}
		}
	}
}

func waitForFileDeleted(ctx context.Context, rt resolvedOperationRuntime, store operations.Store, out operations.CommandOutput, timeout time.Duration) (operations.CommandOutput, error) {
	deadline := rt.f.Clock()().Add(timeout)
	fileID := out.OperationID
	if fileID == "" && out.Action != nil {
		fileID = out.Action.AcceptedOperationID
	}
	for {
		remaining := deadline.Sub(rt.f.Clock()())
		if remaining <= 0 {
			out.LocalRecovery.State = "timeout"
			out.LocalRecovery.CanResume = true
			out.LocalRecovery.KnownRemote = fileID != ""
			out.LocalRecovery.ResumeHint = "call chab_action_resume"
			return out, nil
		}
		requestCtx, cancel := context.WithTimeout(ctx, remaining)
		result, err := rt.client.DoRaw(requestCtx, http.MethodGet, api.Path("files", fileID), nil, nil, api.IdempotencyNone)
		cancel()
		if err != nil {
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				out.LocalRecovery.State = "timeout"
				out.LocalRecovery.CanResume = true
				out.LocalRecovery.KnownRemote = fileID != ""
				out.LocalRecovery.ResumeHint = "call chab_action_resume"
				return out, nil
			}
			var apiErr *api.Error
			if errors.As(err, &apiErr) && apiErr.Code == "not_found" {
				raw := json.RawMessage(fmt.Sprintf(`{"file":{"id":%q,"state":"deleted"}}`, fileID))
				payload := operations.OperationPayload{ID: fileID, OperationKey: "files.delete", Family: "files", Status: "deleted"}
				return completeFileDeletion(store, out, raw, payload, apiErr.Meta)
			}
			return out, err
		}
		payload, err := fileDeletePayload(result.Data, result.Meta, fileID)
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		if err != nil {
			return out, err
		}
		if fileDeleteTerminal(payload.Status) {
			return completeFileDeletion(store, out, result.Data, payload, result.Meta)
		}
		out.LocalRecovery.State = payload.Status
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.KnownRemote = true
		out.LocalRecovery.ResumeHint = "call chab_action_resume"
		sleep := retryDelay(result.Meta, 5*time.Second)
		remaining = deadline.Sub(rt.f.Clock()())
		if sleep > remaining {
			sleep = remaining
		}
		if sleep <= 0 {
			continue
		}
		if err := rt.f.Sleep(ctx, sleep); err != nil {
			return out, err
		}
	}
}

func completeFileDeletion(store operations.Store, out operations.CommandOutput, raw json.RawMessage, payload operations.OperationPayload, meta api.ResponseMeta) (operations.CommandOutput, error) {
	out.OperationID = payload.ID
	out.Server = raw
	out.RequestID = meta.RequestID
	out.Meta = metaView(meta)
	if out.Action != nil {
		updated, err := store.MarkCompleted(out.Action.ID, payload, meta)
		if err != nil {
			out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
			return out, err
		}
		projection := operations.Projection(updated)
		out.Action = &projection
		out.LocalPersistence = &operations.PersistenceState{State: "persisted"}
		out.LocalRecovery.State = string(updated.State)
	}
	out.LocalRecovery.CanResume = false
	out.LocalRecovery.KnownRemote = true
	out.LocalRecovery.ResumeHint = ""
	return out, nil
}

func (r registry) resumeAction(ctx context.Context, raw json.RawMessage) (out operations.CommandOutput, err error) {
	input, err := decodeActionResumeInput(raw)
	if err != nil {
		return operations.CommandOutput{}, invalidParams(err)
	}
	rt, err := r.resolveRuntime(ctx, true)
	if err != nil {
		return operations.CommandOutput{}, err
	}
	defer func() {
		out = redactCommandOutput(rt.f, out)
	}()
	store := operations.StoreForRuntime(rt.runtime, rt.f.Clock())
	record, err := store.Load(input.ActionID)
	if err != nil {
		return operations.CommandOutput{}, withToolFactory(rt.f, err)
	}
	if err := ensureRecordContext(rt, record); err != nil {
		return operations.CommandOutput{}, err
	}
	if rt.cred.PrincipalType == "guest_trial" {
		_, allowed, policyErr := r.guestAllowedTools(ctx)
		if policyErr != nil {
			return operations.CommandOutput{}, policyErr
		}
		allowedToReconcile := allowed[operationToolName(record.OperationKey)]
		if record.AcceptedOperationID != "" {
			// A paused promotional starter must not hide an already accepted
			// operation. The backend remains authoritative for status/ownership.
			allowedToReconcile = allowed["chab_operations_get"]
		}
		if !allowedToReconcile {
			return operations.CommandOutput{}, mcpUsageError("recorded operation is not available for guest trial credentials")
		}
	}
	if err := store.Replayable(record); err != nil {
		return operations.CommandOutput{}, err
	}
	if record.OperationKey == "files.create" {
		return r.resumeFileUploadAction(ctx, rt, store, record, input)
	}
	if record.OperationKey == "files.delete" {
		return r.resumeFileDeleteAction(ctx, rt, store, record, input)
	}
	if record.AcceptedOperationID != "" {
		out := outputFromRecord(record)
		out.OperationID = record.AcceptedOperationID
		if input.Local.Wait {
			return waitForOperation(ctx, rt, store, out, timeoutFromLocal(input.Local))
		}
		return out, nil
	}
	op, ok := chabcontract.MustLoad().Find(record.OperationKey)
	if !ok {
		return operations.CommandOutput{}, mcpUsageError("unknown recorded operation " + record.OperationKey)
	}
	if err := validateLocalControls(op, input.Local); err != nil {
		return operations.CommandOutput{}, mcpUsageError(err.Error())
	}
	if operationRequiresConfirmation(op) && !confirmed(input.Local) {
		return operations.CommandOutput{}, mcpUsageError("operation " + op.ID + " requires local.confirmation=true")
	}
	if len(op.RequestSchema) > 0 && !input.BodySet {
		return operations.CommandOutput{}, mcpUsageError("action resume requires body for unknown JSON action outcomes")
	}
	request, _, err := requestBytes(op, operationCallInput{Body: input.Body, BodySet: input.BodySet})
	if err != nil {
		return operations.CommandOutput{}, err
	}
	if err := store.VerifyInput(record, request); err != nil {
		return operations.CommandOutput{}, err
	}
	header, err := approvalHeaderFromLocal(rt.f, input.Local)
	if err != nil {
		return operations.CommandOutput{}, err
	}
	actionOp := op
	actionOp.Method = record.Method
	actionOp.Path = record.Path
	out, err = operations.Start(ctx, rt.client, operations.StartInput{
		Operation:      actionOp,
		RuntimeProfile: rt.runtime.Profile,
		Destination:    rt.runtime.APIBaseURL,
		TokenPublicID:  rt.identity.TokenPublicID,
		PrincipalID:    rt.identity.PrincipalID,
		EncoderVersion: record.EncoderVersion,
		RequestBytes:   request,
		IdempotencyKey: record.IdempotencyKey,
		Headers:        header,
		Journal:        store,
	})
	if err != nil && !commandOutputPresent(out) {
		return operations.CommandOutput{}, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	if input.Local.Wait && out.OperationID != "" {
		out, err = waitForOperation(ctx, rt, store, out, timeoutFromLocal(input.Local))
	}
	if err != nil {
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	return out, nil
}

func (r registry) resumeFileDeleteAction(ctx context.Context, rt resolvedOperationRuntime, store operations.Store, record operations.ActionRecord, input actionResumeInput) (operations.CommandOutput, error) {
	op, ok := chabcontract.MustLoad().Find("files.delete")
	if !ok {
		return operations.CommandOutput{}, mcpUsageError("unknown recorded operation files.delete")
	}
	if err := validateLocalControls(op, input.Local); err != nil {
		return operations.CommandOutput{}, mcpUsageError(err.Error())
	}
	if !confirmed(input.Local) {
		return operations.CommandOutput{}, mcpUsageError("operation files.delete requires local.confirmation=true")
	}
	if input.BodySet {
		if _, _, err := requestBytes(op, operationCallInput{Body: input.Body, BodySet: true}); err != nil {
			return operations.CommandOutput{}, err
		}
	}
	fileID := record.AcceptedOperationID
	if fileID == "" {
		fileID = strings.TrimPrefix(strings.TrimPrefix(record.Path, "/v1/"), "files/")
		if fileID == "" || fileID == record.Path {
			return operations.CommandOutput{}, mcpUsageError("recorded files.delete action is missing a file id")
		}
		if err := store.VerifyInput(record, nil); err != nil {
			return operations.CommandOutput{}, err
		}
		if err := store.BeginReplay(record); err != nil {
			return operations.CommandOutput{}, err
		}
		result, err := rt.client.DoRaw(ctx, http.MethodDelete, api.Path("files", fileID), nil, nil, api.JournaledIdempotency(record.IdempotencyKey, true))
		if err != nil {
			updated, persistence := markActionUnknown(store, record, err, true)
			out := outputFromRecord(updated)
			out.LocalPersistence = persistence
			setActionFailureRecovery(&out, persistence, "call chab_action_resume")
			return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
		}
		payload, err := fileDeletePayload(result.Data, result.Meta, fileID)
		if err != nil {
			updated, persistence := markActionUnknown(store, record, err, true)
			out := outputFromRecord(updated)
			out.Server = result.Data
			out.RequestID = result.Meta.RequestID
			out.Meta = metaView(result.Meta)
			out.LocalPersistence = persistence
			setActionFailureRecovery(&out, persistence, "call chab_action_resume")
			return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
		}
		var updated operations.ActionRecord
		if fileDeleteTerminal(payload.Status) {
			updated, err = store.MarkCompleted(record.ID, payload, result.Meta)
		} else {
			updated, err = store.MarkAccepted(record.ID, payload, result.Meta)
		}
		if err != nil {
			out := outputFromRecord(record)
			out.OperationID = payload.ID
			out.Server = result.Data
			out.RequestID = result.Meta.RequestID
			out.Meta = metaView(result.Meta)
			out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
			return out, err
		}
		record = updated
		fileID = payload.ID
		out := outputFromRecord(record)
		out.OperationID = payload.ID
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		if input.Local.Wait && payload.Status == "deletion_pending" {
			return waitForFileDeleted(ctx, rt, store, out, timeoutFromLocal(input.Local))
		}
		if record.State == operations.ActionAccepted {
			out.LocalRecovery.CanResume = true
			out.LocalRecovery.KnownRemote = true
			out.LocalRecovery.ResumeHint = "call chab_action_resume"
		}
		return out, nil
	}
	out := outputFromRecord(record)
	out.OperationID = fileID
	if input.Local.Wait {
		return waitForFileDeleted(ctx, rt, store, out, timeoutFromLocal(input.Local))
	}
	return out, nil
}

type actionResumeInput struct {
	ActionID string
	Body     json.RawMessage
	BodySet  bool
	Local    operationLocalInput
}

func decodeActionResumeInput(raw json.RawMessage) (actionResumeInput, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var obj rawObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return actionResumeInput{}, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	var input actionResumeInput
	for key, value := range obj {
		switch key {
		case "action_id":
			var id string
			if err := json.Unmarshal(value, &id); err != nil || id == "" {
				return actionResumeInput{}, errors.New("action_id must be a non-empty string")
			}
			input.ActionID = id
		case "body":
			input.BodySet = true
			input.Body = append(json.RawMessage(nil), value...)
		case "local":
			local, err := decodeLocalInput(value)
			if err != nil {
				return actionResumeInput{}, err
			}
			input.Local = local
		default:
			return actionResumeInput{}, fmt.Errorf("unknown argument %q", key)
		}
	}
	if input.ActionID == "" {
		return actionResumeInput{}, errors.New("missing required argument \"action_id\"")
	}
	return input, nil
}

func (r registry) resumeFileUploadAction(ctx context.Context, rt resolvedOperationRuntime, store operations.Store, record operations.ActionRecord, input actionResumeInput) (operations.CommandOutput, error) {
	if record.AcceptedOperationID != "" {
		out := outputFromRecord(record)
		out.OperationID = record.AcceptedOperationID
		if input.Local.Wait {
			return waitForFileReady(ctx, rt, store, out, timeoutFromLocal(input.Local))
		}
		return out, nil
	}
	if input.Local.InputPath == "" {
		return operations.CommandOutput{}, mcpUsageError("files.create action resume requires local.input_path")
	}
	if !confirmed(input.Local) {
		return operations.CommandOutput{}, mcpUsageError("files.create action resume requires local.confirmation=true")
	}
	maxBytes := input.Local.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMCPUploadMaxBytes
	}
	fields, fingerprint, filename, err := fileUploadFields(input.Body, input.BodySet, input.Local.InputPath, maxBytes)
	if err != nil {
		return operations.CommandOutput{}, err
	}
	if record.RequestSHA256 != fingerprint {
		return operations.CommandOutput{}, mcpUsageError("action input does not match the original upload fingerprint")
	}
	if err := store.BeginReplay(record); err != nil {
		return operations.CommandOutput{}, err
	}
	result, err := rt.client.PostMultipart(ctx, "files", api.MultipartFileUpload{
		FilePath:      input.Local.InputPath,
		FileFieldName: "file",
		Filename:      filename,
		Fields:        fields,
		MaxBytes:      maxBytes,
		Idempotency:   api.JournaledIdempotency(record.IdempotencyKey, true),
	})
	if err != nil {
		updated, persistence := markActionUnknown(store, record, err, true)
		out := outputFromRecord(updated)
		out.LocalPersistence = persistence
		setActionFailureRecovery(&out, persistence, "call chab_action_resume with the same local.input_path")
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	payload, err := fileUploadPayload(result.Data, result.Meta)
	if err != nil {
		updated, persistence := markActionUnknown(store, record, err, true)
		out := outputFromRecord(updated)
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		out.LocalPersistence = persistence
		setActionFailureRecovery(&out, persistence, "call chab_action_resume with the same local.input_path")
		return out, withToolFactory(rt.f, output.WithCredentialContext(err, rt.cred.Profile, rt.cred.DisplayID))
	}
	updated, err := store.MarkCompleted(record.ID, payload, result.Meta)
	if err != nil {
		out := outputFromRecord(record)
		out.OperationID = payload.ID
		out.Server = result.Data
		out.RequestID = result.Meta.RequestID
		out.Meta = metaView(result.Meta)
		out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: err.Error()}
		out.LocalRecovery.State = string(operations.ActionUnknown)
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.KnownRemote = payload.ID != ""
		out.LocalRecovery.ResumeHint = "call chab_action_resume with the same local.input_path"
		return out, err
	}
	projection := operations.Projection(updated)
	out := operations.CommandOutput{
		Action:      &projection,
		OperationID: payload.ID,
		Server:      result.Data,
		RequestID:   result.Meta.RequestID,
		Meta:        metaView(result.Meta),
		LocalRecovery: operations.RecoveryStatus{
			State:       string(updated.State),
			CanResume:   false,
			KnownRemote: payload.ID != "",
		},
		LocalPersistence: &operations.PersistenceState{State: "persisted"},
	}
	if input.Local.Wait && payload.ID != "" && payload.Status != "available" {
		return waitForFileReady(ctx, rt, store, out, timeoutFromLocal(input.Local))
	}
	return out, nil
}

func outputFromRecord(record operations.ActionRecord) operations.CommandOutput {
	projection := operations.Projection(record)
	return operations.CommandOutput{
		Action:      &projection,
		OperationID: record.AcceptedOperationID,
		RequestID:   record.RequestID,
		LocalRecovery: operations.RecoveryStatus{
			State:       string(record.State),
			CanResume:   record.State == operations.ActionUnknown || record.State == operations.ActionAccepted,
			KnownRemote: record.AcceptedOperationID != "",
		},
		LocalPersistence: &operations.PersistenceState{State: "persisted"},
	}
}

func ensureRecordContext(rt resolvedOperationRuntime, record operations.ActionRecord) error {
	switch {
	case record.Profile != rt.runtime.Profile:
		return mcpUsageError("action belongs to profile " + record.Profile)
	case record.Destination != rt.runtime.APIBaseURL:
		return mcpUsageError("action belongs to a different API destination")
	case record.TokenPublicID != rt.identity.TokenPublicID:
		return mcpUsageError("action belongs to a different token")
	case record.PrincipalID != rt.identity.PrincipalID:
		return mcpUsageError("action belongs to a different principal")
	default:
		return nil
	}
}

func commandOutputPresent(out operations.CommandOutput) bool {
	return out.Action != nil || out.OperationID != "" || len(out.Server) > 0 || out.RequestID != ""
}

func metaView(meta api.ResponseMeta) *output.MetaView {
	view := output.ViewMeta(meta, output.MetaOptions{IncludeAPIMeta: true})
	if view.Empty() {
		return nil
	}
	return &view
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func retryDelay(meta api.ResponseMeta, fallback time.Duration) time.Duration {
	if meta.RetryAfter.Wait != nil {
		return *meta.RetryAfter.Wait
	}
	return fallback
}

func mcpUsageError(detail string) error {
	return &api.UsageError{Field: "mcp", Detail: detail}
}

func operationInputSchema(op chabcontract.Operation) map[string]any {
	props := map[string]any{}
	var required []string
	if schema := parameterGroupSchema(op, "path"); schema != nil {
		props["path"] = schema
		if len(schemaRequired(schema)) > 0 {
			required = append(required, "path")
		}
	}
	if schema := parameterGroupSchema(op, "query"); schema != nil {
		props["query"] = schema
		if len(schemaRequired(schema)) > 0 {
			required = append(required, "query")
		}
	}
	if len(op.RequestSchema) > 0 {
		props["body"] = bodySchemaForTool(op)
	}
	if schema, need := localControlsSchema(op); schema != nil {
		props["local"] = schema
		if need {
			required = append(required, "local")
		}
	}
	sort.Strings(required)
	schema := objectSchema(props, required)
	addReferencedComponents(op, schema)
	return schema
}

func parameterGroupSchema(op chabcontract.Operation, in string) map[string]any {
	props := map[string]any{}
	var required []string
	for _, param := range op.Parameters {
		if param.In != in {
			continue
		}
		props[param.Name] = schemaValue(param.Schema, stringSchema())
		if param.Description != "" {
			if m, ok := props[param.Name].(map[string]any); ok {
				m["description"] = param.Description
			}
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}
	if len(props) == 0 {
		return nil
	}
	sort.Strings(required)
	return objectSchema(props, required)
}

func bodySchemaForTool(op chabcontract.Operation) any {
	schema := cloneSchemaMap(op.RequestSchema)
	if op.ID == "files.create" {
		props, _ := schema["properties"].(map[string]any)
		delete(props, "file")
		schema["properties"] = props
		var required []any
		if current, ok := schema["required"].([]any); ok {
			for _, value := range current {
				if value != "file" {
					required = append(required, value)
				}
			}
		}
		if len(required) > 0 {
			schema["required"] = required
		} else {
			delete(schema, "required")
		}
	}
	return schema
}

func addReferencedComponents(op chabcontract.Operation, schema map[string]any) {
	registry, err := chabcontract.Load()
	if err != nil {
		return
	}
	doc, ok := registry.SchemaDocument(op.ID)
	if !ok || len(doc.ReferencedSchemas) == 0 {
		return
	}
	components := map[string]any{}
	names := make([]string, 0, len(doc.ReferencedSchemas))
	for name := range doc.ReferencedSchemas {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		components[name] = schemaValue(doc.ReferencedSchemas[name], map[string]any{})
	}
	schema["components"] = map[string]any{"schemas": components}
}

func localControlsSchema(op chabcontract.Operation) (map[string]any, bool) {
	props := map[string]any{}
	var required []string
	if operationRequiresConfirmation(op) {
		props["confirmation"] = map[string]any{"type": "boolean", "const": true, "description": "Explicit local acknowledgement for the live effect."}
		required = append(required, "confirmation")
	}
	if op.IdempotencyRequired {
		props["idempotency_key"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 255, "description": "Optional caller-owned key; generated when omitted."}
		props["wait"] = map[string]any{"type": "boolean", "description": "Poll accepted operations until terminal or timeout."}
		props["timeout_seconds"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 3600}
	}
	if op.ID == "files.create" {
		props["input_path"] = map[string]any{"type": "string", "minLength": 1, "description": "Local file path to upload."}
		props["max_bytes"] = map[string]any{"type": "integer", "minimum": 1}
		required = append(required, "input_path")
	}
	if byteDownloadOperation(op.ID) {
		props["output_path"] = map[string]any{"type": "string", "minLength": 1, "description": "New local path to write downloaded bytes."}
		props["max_bytes"] = map[string]any{"type": "integer", "minimum": 1}
		required = append(required, "output_path")
	}
	if tokenProofEligible(op.ID) {
		props["approval_proof_path"] = map[string]any{"type": "string", "minLength": 1, "description": "Private approval proof file; proof is sent only as X-Chab-Management-Approval."}
	}
	if len(props) == 0 {
		return nil, false
	}
	sort.Strings(required)
	return objectSchema(props, required), len(required) > 0
}

func operationOutputSchema(op chabcontract.Operation) any {
	if byteDownloadOperation(op.ID) {
		return objectSchema(map[string]any{
			"path":       stringSchema(),
			"bytes":      integerSchema(),
			"checksum":   stringSchema(),
			"request_id": stringSchema(),
			"meta":       map[string]any{"type": "object"},
		}, []string{"path", "bytes"})
	}
	if op.IdempotencyRequired || op.ID == "files.create" {
		if op.DryRunSupported {
			return oneOf(commandOutputSchema(), genericOperationOutputSchema(op, dryRunResultSchema(op)))
		}
		return commandOutputSchema()
	}
	return genericOperationOutputSchema(op, op.ResultSchema)
}

func operationToolOutputSchema(op chabcontract.Operation) map[string]any {
	schema := oneOf(operationOutputSchema(op), toolErrorSchema())
	addReferencedComponents(op, schema)
	return schema
}

func dryRunResultSchema(op chabcontract.Operation) json.RawMessage {
	if len(op.DryRunResultSchema) > 0 {
		return op.DryRunResultSchema
	}
	return op.ResultSchema
}

func genericOperationOutputSchema(op chabcontract.Operation, resultSchema json.RawMessage) map[string]any {
	return objectSchema(map[string]any{
		"operation":  stringSchema(),
		"server":     schemaValue(resultSchema, map[string]any{}),
		"request_id": stringSchema(),
		"meta":       map[string]any{"type": "object"},
	}, []string{"operation"})
}

func actionShowInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"action_id": map[string]any{"type": "string", "minLength": 1},
	}, []string{"action_id"})
}

func decodeActionIDInput(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var obj rawObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	var id string
	for key, value := range obj {
		switch key {
		case "action_id":
			if err := json.Unmarshal(value, &id); err != nil || id == "" {
				return "", errors.New("action_id must be a non-empty string")
			}
		default:
			return "", fmt.Errorf("unknown argument %q", key)
		}
	}
	if id == "" {
		return "", errors.New("missing required argument \"action_id\"")
	}
	return id, nil
}

func actionResumeInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"action_id": map[string]any{"type": "string", "minLength": 1},
		"body":      map[string]any{},
		"local": objectSchema(map[string]any{
			"approval_proof_path": map[string]any{"type": "string", "minLength": 1},
			"confirmation":        boolSchema(),
			"wait":                boolSchema(),
			"timeout_seconds":     map[string]any{"type": "integer", "minimum": 1},
			"input_path":          map[string]any{"type": "string", "minLength": 1},
			"max_bytes":           map[string]any{"type": "integer", "minimum": 1},
		}, []string{}),
	}, []string{"action_id"})
}

func commandOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"action":            actionProjectionSchema(),
		"error":             toolErrorSchema(),
		"operation_id":      stringSchema(),
		"server":            map[string]any{},
		"local_recovery":    recoverySchema(),
		"local_persistence": objectSchema(map[string]any{"state": stringSchema(), "detail": stringSchema()}, []string{"state"}),
		"meta":              map[string]any{"type": "object"},
		"warning":           stringSchema(),
		"request_id":        stringSchema(),
	}, []string{"local_recovery"})
}

func actionProjectionSchema() map[string]any {
	return objectSchema(map[string]any{
		"id":                    stringSchema(),
		"created_at":            stringSchema(),
		"updated_at":            stringSchema(),
		"profile":               stringSchema(),
		"destination":           stringSchema(),
		"token_public_id":       stringSchema(),
		"principal_id":          stringSchema(),
		"required_scope":        stringSchema(),
		"operation_key":         stringSchema(),
		"method":                stringSchema(),
		"path":                  stringSchema(),
		"encoder_version":       stringSchema(),
		"state":                 stringSchema(),
		"accepted_operation_id": stringSchema(),
		"request_id":            stringSchema(),
		"last_status":           stringSchema(),
		"last_error":            stringSchema(),
	}, []string{"id", "created_at", "updated_at", "profile", "destination", "operation_key", "method", "path", "encoder_version", "state"})
}

func recoverySchema() map[string]any {
	return objectSchema(map[string]any{
		"state":        stringSchema(),
		"can_resume":   boolSchema(),
		"resume_hint":  stringSchema(),
		"known_remote": boolSchema(),
	}, []string{"state", "can_resume", "known_remote"})
}

func schemaValue(raw json.RawMessage, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback
	}
	return value
}

func cloneSchemaMap(raw json.RawMessage) map[string]any {
	value := schemaValue(raw, map[string]any{}).(map[string]any)
	copied := make(map[string]any, len(value))
	for k, v := range value {
		copied[k] = v
	}
	return copied
}

func schemaRequired(schema map[string]any) []string {
	raw, _ := schema["required"].([]string)
	if raw != nil {
		return raw
	}
	var out []string
	if anyRaw, ok := schema["required"].([]any); ok {
		for _, value := range anyRaw {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
