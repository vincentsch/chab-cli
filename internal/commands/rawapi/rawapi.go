// Package rawapi implements the "chab api" escape hatch for calling configured
// /v1 endpoints without adding a first-class resource command first.
package rawapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/redact"
)

type commonFlags struct {
	Query       []string
	SecretField []string
	Raw         bool
}

// unsafeFlags are available only on methods that can carry a body and
// idempotency key. GET intentionally has only commonFlags.
type unsafeFlags struct {
	commonFlags
	Field          []string
	Body           string
	BodyFile       string
	IdempotencyKey string
}

// rawOp describes one leaf command after flag registration. unsafe is nil for
// GET, which keeps body/idempotency behavior out of read-only calls.
type rawOp struct {
	method string
	common *commonFlags
	unsafe *unsafeFlags
	page   *cmdutil.CursorPaginationFlags
}

type kv struct {
	key   string
	value string
}

type bodyField struct {
	key   string
	value string
}

type bodyMode int

const (
	bodyModeNone bodyMode = iota
	bodyModeField
	bodyModeJSON
)

type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "api command error"
	}
	return "api command error: " + e.detail
}

func (e *usageError) ExitCode() int {
	return 1
}

type rawAction struct {
	Operation chabcontract.Operation
	Prepared  operations.PreparedAction
	Store     operations.Store
}

func registerCommonFlags(cmd *cobra.Command, flags *commonFlags) {
	f := cmd.Flags()
	f.StringArrayVar(&flags.Query, "query", nil, "query parameter as key=value (repeatable)")
	f.StringArrayVar(&flags.SecretField, "secret-field", nil, "request field name whose value must be redacted in debug output and errors (repeatable)")
	f.BoolVar(&flags.Raw, "raw", false, "print the full API success envelope instead of only data")
}

func registerUnsafeFlags(cmd *cobra.Command, flags *unsafeFlags) {
	registerCommonFlags(cmd, &flags.commonFlags)
	f := cmd.Flags()
	f.StringArrayVar(&flags.Field, "field", nil, "body field as key=value (repeatable; mutually exclusive with --body and --body-file)")
	f.StringVar(&flags.Body, "body", "", "JSON request body (mutually exclusive with --field and --body-file)")
	f.StringVar(&flags.BodyFile, "body-file", "", "read the JSON request body from a file, or - for stdin")
	f.StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key (1-255 visible ASCII bytes); generated when omitted on replayable unsafe routes")
	f.Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
}

// runRaw keeps all local validation ahead of runtime, credential, and HTTP
// work. That ordering is what makes invalid input and --dry-run fully offline.
func runRaw(cmd *cobra.Command, f *cmdutil.Factory, op rawOp, pathArg string) (runErr error) {
	requestPath, displayPath, err := normalizeRawPath(pathArg)
	if err != nil {
		return err
	}
	if isOneTimeSecretRoute(op.method, requestPath) {
		return &usageError{detail: "raw API refuses routes that return one-time plaintext secrets or management proofs; use the dedicated private-output command for this workflow"}
	}

	queryEntries, query, err := parseQuery(op.common.Query)
	if err != nil {
		return err
	}
	secretFields, err := secretFieldSet(op.common.SecretField)
	if err != nil {
		return err
	}

	dryRun := op.unsafe != nil && dryRunEnabled(cmd)
	bodyMode, err := selectBodyMode(cmd, op.unsafe)
	if err != nil {
		return err
	}

	var plan cmdutil.CursorListPlan
	paginationActive := false
	if op.page != nil {
		// Cobra records --all=false as changed, but false must behave as if the
		// pagination mode was never enabled.
		allActive := cmd.Flags().Changed("all") && op.page.All
		limitActive := cmd.Flags().Changed("limit")
		paginationActive = allActive ||
			limitActive ||
			cmd.Flags().Changed("cursor") ||
			cmd.Flags().Changed("page-size")
		if paginationActive {
			if op.common.Raw && (allActive || limitActive) {
				return &usageError{detail: "--raw cannot be combined with --all or --limit"}
			}
			if len(query["cursor"]) > 0 || len(query["limit"]) > 0 {
				return &usageError{detail: "pagination flags cannot be combined with --query cursor=... or --query limit=..."}
			}
			plan, err = cmdutil.ResolveCursorListPlan(cmd, op.page)
			if err != nil {
				return err
			}
		}
	}
	if op.common.Raw && dryRun {
		return &usageError{detail: "--raw and --dry-run cannot be combined"}
	}

	explicitKey := op.unsafe != nil && cmd.Flags().Changed("idempotency-key")
	if explicitKey {
		if err := api.ValidateIdempotencyKey(op.unsafe.IdempotencyKey); err != nil {
			return err
		}
		f.RegisterSecret(op.unsafe.IdempotencyKey)
		defer func() {
			runErr = f.RedactError(runErr)
		}()
	}

	var bodyRaw json.RawMessage
	var bodyFields []bodyField
	var bodyTree *requestJSONNode
	if op.unsafe != nil {
		bodyRaw, bodyFields, err = assembleBody(cmd, f, op.unsafe, bodyMode)
		if err != nil {
			return err
		}
		if bodyMode != bodyModeNone {
			// Parse once before runtime work. The same ordered tree supplies
			// diagnostic secrets and the dry-run preview without changing the
			// validated bytes that will be sent.
			bodyTree, err = parseRequestJSON(bodyRaw)
			if err != nil {
				return &usageError{detail: "request body could not be processed safely"}
			}
		}
	}

	scope := collectSecretScope(queryEntries, bodyTree, secretFields)
	var maskedBody string
	if bodyMode == bodyModeJSON {
		maskedBody, err = maskedRequestJSON(bodyTree, secretFields)
		if err != nil {
			return &usageError{detail: "request body could not be processed safely"}
		}
	}

	if dryRun {
		// Dry-run renders only already-parsed command values. Do not resolve the
		// profile or credential here; callers use it to inspect unsafe requests
		// without touching local auth or the network.
		preview := output.DryRunPreview{
			Method:      op.method,
			Path:        displayPath,
			Query:       queryDryRunValues(queryEntries, secretFields),
			Body:        bodyDryRunValues(bodyMode, bodyFields, maskedBody, secretFields),
			Idempotency: &output.DryRunIdempotency{Source: idempotencySource(explicitKey, false)},
		}
		return f.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: func(w io.Writer) { preview.Render(w) },
			Plain:  preview.RenderPlain,
		})
	}

	var catalogOperation chabcontract.Operation
	useActionJournal := false
	serverDryRun := false
	if op.unsafe != nil {
		catalogOperation, useActionJournal, err = rawRecoverableOperation(op.method, requestPath)
		if err != nil {
			return err
		}
	}
	if useActionJournal && bodyMode != bodyModeNone {
		serverDryRun, err = operations.RequestDryRun(bodyRaw)
		if err != nil {
			return &usageError{detail: err.Error()}
		}
		if serverDryRun {
			if !catalogOperation.DryRunSupported {
				return &usageError{detail: "operation " + catalogOperation.ID + " does not support server dry runs"}
			}
			if explicitKey {
				return &usageError{detail: "--idempotency-key is not used for server dry runs"}
			}
			useActionJournal = false
		}
	}
	if useActionJournal {
		if len(queryEntries) > 0 {
			return &usageError{detail: "catalogued operation actions do not accept raw --query parameters"}
		}
		if err := requireRawAcknowledgement(cmd, f, catalogOperation); err != nil {
			return err
		}
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
		// Preserve credential redaction from the factory and add only the
		// request values the caller explicitly marked as sensitive.
		opts.SecretValues = append(opts.SecretValues, scope...)
	})
	if err != nil {
		return err
	}

	idem := api.IdempotencyNone
	var action *rawAction
	if useActionJournal {
		identity, _, err := client.Whoami(cmd.Context())
		if err != nil {
			return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
		}
		key := op.unsafe.IdempotencyKey
		if key == "" {
			key, err = api.GenerateIdempotencyKey()
			if err != nil {
				return err
			}
		}
		f.RegisterSecret(key)
		defer func() {
			runErr = f.RedactError(runErr)
		}()
		requestBytes := []byte(nil)
		if bodyMode != bodyModeNone {
			requestBytes = append([]byte(nil), bodyRaw...)
		}
		store := operations.StoreForRuntime(rt, f.Clock())
		prepared, err := store.Prepare(operations.PrepareInput{
			Profile:        rt.Profile,
			Destination:    rt.APIBaseURL,
			TokenPublicID:  identity.TokenPublicID,
			PrincipalID:    identity.PrincipalID,
			RequiredScope:  catalogOperation.RequiredScope,
			OperationKey:   catalogOperation.ID,
			Method:         catalogOperation.Method,
			Path:           catalogOperation.Path,
			EncoderVersion: operations.RawEncoderVersion,
			RequestBytes:   requestBytes,
			IdempotencyKey: key,
		})
		if err != nil {
			return err
		}
		if prepared.AlreadyAccepted || prepared.AlreadyCompleted {
			return writeRawActionRecovery(cmd, f, prepared.Record)
		}
		if err := store.ReplayablePrepared(prepared); err != nil {
			return err
		}
		action = &rawAction{Operation: catalogOperation, Prepared: prepared, Store: store}
		idem = api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing)
	} else if op.unsafe != nil && !serverDryRun {
		switch {
		case explicitKey:
			idem = api.ExplicitIdempotency(op.unsafe.IdempotencyKey)
		default:
			idem = api.AutoIdempotency()
		}
	}

	var body any
	if op.unsafe != nil && bodyMode != bodyModeNone {
		// Mark the already-validated bytes explicitly so the shared client does
		// not compact whitespace or rewrite JSON string escapes before sending.
		body = api.ExactJSONBody(bodyRaw)
	}

	if paginationActive {
		return runRawGetPaginated(cmd, f, client, cred, requestPath, query, plan, op.common.Raw)
	}

	result, err := client.DoRaw(cmd.Context(), op.method, requestPath, query, body, idem)
	if err != nil {
		if action != nil {
			record := action.Prepared.Record
			var markErr error
			if operations.DefinitiveAdmissionDenialForSubmission(err, action.Prepared.Existing) {
				markErr = action.Store.MarkDenied(record.ID, err)
			} else {
				markErr = action.Store.MarkUnknown(record.ID, err)
			}
			if markErr == nil {
				if updated, loadErr := action.Store.Load(action.Prepared.Record.ID); loadErr == nil {
					record = updated
				}
			} else if updated, loadErr := action.Store.Load(action.Prepared.Record.ID); loadErr == nil {
				record = updated
			}
			out := rawActionOutput(record, nil, api.ResponseMeta{})
			if markErr != nil {
				out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: markErr.Error()}
				out.LocalRecovery.CanResume = false
				out.LocalRecovery.ResumeHint = ""
			}
			if writeErr := writeRawActionOutput(cmd, f, out, api.ResponseMeta{RequestID: record.RequestID}); writeErr != nil {
				return writeErr
			}
		}
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	var localPersistenceErr error
	var updatedRecord operations.ActionRecord
	if action != nil {
		updatedRecord, err = updateRawAction(action, result)
		if err != nil {
			var protocolErr *api.ProtocolError
			if errors.As(err, &protocolErr) {
				return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
			}
			localPersistenceErr = fmt.Errorf("accepted raw action for %s but failed to persist local recovery state: %w", action.Operation.ID, err)
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v; request_id=%s\n", localPersistenceErr, result.Meta.RequestID)
			if updatedRecord.ID == "" {
				payload := rawOperationPayload(result.Data)
				updatedRecord = rawRecordWithResponse(action.Prepared.Record, action.Operation, payload, result.Meta)
			}
		}
	}

	machine := json.RawMessage(result.Data)
	if op.common.Raw {
		machine = json.RawMessage(result.Envelope)
	}
	machine, err = sanitizeRawSuccess(machine, result.Meta)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	if action != nil {
		if updatedRecord.ID == "" {
			updatedRecord = action.Prepared.Record
		}
		out := rawActionOutput(updatedRecord, machine, result.Meta)
		if localPersistenceErr != nil {
			out.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: localPersistenceErr.Error()}
		}
		if writeErr := writeRawActionOutput(cmd, f, out, result.Meta); writeErr != nil {
			return writeErr
		}
		if localPersistenceErr != nil {
			return output.WithCredentialContext(localPersistenceErr, cred.Profile, cred.DisplayID)
		}
		return nil
	}
	includeAPIMeta := op.method == http.MethodGet && !op.common.Raw
	if err := writeRawResult(cmd, f, machine, result.Meta, includeAPIMeta); err != nil {
		return err
	}
	if localPersistenceErr != nil {
		return output.WithCredentialContext(localPersistenceErr, cred.Profile, cred.DisplayID)
	}
	return nil
}

// writeRawResult renders only a value that has already passed request-aware
// sanitation. The ordinary static structured redactor still runs inside output
// dispatch as a defense-in-depth pass.
func writeRawResult(cmd *cobra.Command, f *cmdutil.Factory, machine json.RawMessage, meta api.ResponseMeta, includeAPIMeta bool) error {
	// The active renderer does not consult Secrets for successful output. Clear
	// both registry fields on the copy anyway so retained compatibility state
	// cannot widen success sanitation if this helper is reused. The ordinary
	// static structured redactor still runs inside output dispatch.
	resultFactory := *f
	resultFactory.Secrets = redact.NewRegistry()
	resultFactory.Rungrad = nil
	return resultFactory.WriteResultWithMeta(cmd, machine, meta, includeAPIMeta, cmdutil.HumanOutput{
		Render: func(w io.Writer) { output.RawValueHuman(w, machine) },
		Plain:  func(data, prose io.Writer) { output.RawValuePlain(data, prose, machine) },
	})
}

// runRawGetPaginated handles exact-page and merged-page GET modes. Exact pages
// may retain their envelope; merged modes operate on decoded array rows and
// reject later shape drift before any accumulated row is rendered.
func runRawGetPaginated(cmd *cobra.Command, f *cmdutil.Factory, client *api.Client, cred auth.Credential, requestPath string, baseQuery url.Values, plan cmdutil.CursorListPlan, raw bool) error {
	if raw {
		q := cloneValues(baseQuery)
		if plan.Cursor != "" {
			q.Set("cursor", plan.Cursor)
		}
		if plan.PageSize > 0 {
			q.Set("limit", fmt.Sprintf("%d", plan.PageSize))
		}
		result, err := client.DoRaw(cmd.Context(), http.MethodGet, requestPath, q, nil, api.IdempotencyNone)
		if err != nil {
			return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
		}
		machine := json.RawMessage(result.Data)
		if raw {
			machine = json.RawMessage(result.Envelope)
		}
		machine, err = sanitizeRawSuccess(machine, result.Meta)
		if err != nil {
			return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
		}
		return writeRawResult(cmd, f, machine, result.Meta, !raw)
	}

	var fallbackData json.RawMessage
	fallback := false
	outcome, err := cmdutil.FetchCursorPages(plan, func(cursor string, limit int) ([]json.RawMessage, api.ResponseMeta, error) {
		q := cloneValues(baseQuery)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", limit))
		}
		result, err := client.DoRaw(cmd.Context(), http.MethodGet, requestPath, q, nil, api.IdempotencyNone)
		if err != nil {
			return nil, api.ResponseMeta{}, err
		}
		if jsonValueKind(result.Data) != jsonKindArray || result.Meta.CursorForTraversal() == nil {
			if cursor == plan.Cursor {
				// --all/--limit starts with list-style query parameters because
				// the response shape is unknown until after the first request. If
				// the first response is not a paginated array, render that single
				// data value as-is instead of inventing an array wrapper.
				fallback = true
				fallbackData = append(json.RawMessage(nil), result.Data...)
				return nil, result.Meta, nil
			}
			// Reaching a later page means an earlier response advertised more
			// pages. Treat a dropped pagination object or non-array data as a
			// protocol drift rather than silently mixing incompatible shapes.
			return nil, result.Meta, &api.ProtocolError{
				Detail:    "cursor-paginated GET response data must remain an array with cursor metadata",
				Status:    result.Meta.HTTPStatus,
				RequestID: result.Meta.RequestID,
				Meta:      result.Meta,
			}
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(result.Data, &rows); err != nil {
			return nil, result.Meta, &api.ProtocolError{
				Detail:    "paginated GET response data must be a JSON array",
				Status:    result.Meta.HTTPStatus,
				RequestID: result.Meta.RequestID,
				Err:       err,
				Meta:      result.Meta,
			}
		}
		return rows, result.Meta, nil
	})
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}

	machine := fallbackData
	if !fallback {
		merged, err := json.Marshal(outcome.Rows)
		if err != nil {
			return err
		}
		machine = json.RawMessage(merged)
	}
	machine, err = sanitizeRawSuccess(machine, outcome.Meta)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	return writeRawResult(cmd, f, machine, outcome.Meta, true)
}

// normalizeRawPath accepts only API-relative path segments. It rejects query
// strings, fragments, host-like input, and empty segments, then delegates each
// opaque segment to api.Path so caller text cannot replace the configured base.
func normalizeRawPath(raw string) (requestPath, displayPath string, err error) {
	if strings.ContainsAny(raw, "?#") {
		return "", "", &usageError{detail: "path must not contain a query string or fragment; use --query"}
	}
	explicitLeadingSlash := strings.HasPrefix(raw, "/")
	if strings.HasPrefix(raw, "//") {
		return "", "", &usageError{detail: "path must be API-relative, not an absolute or scheme-relative URL"}
	}
	if strings.HasSuffix(raw, "/") {
		return "", "", &usageError{detail: "path must not end with a slash"}
	}
	trimmed := strings.TrimLeft(raw, "/")
	if trimmed == "" {
		return "", "", &usageError{detail: "path must not be empty"}
	}
	segments := strings.Split(trimmed, "/")
	if !explicitLeadingSlash && strings.Contains(segments[0], ":") {
		return "", "", &usageError{detail: "bare relative path must not contain URI scheme syntax in its first segment"}
	}
	for _, segment := range segments {
		if segment == "" {
			return "", "", &usageError{detail: "path must not contain empty segments"}
		}
	}
	requestPath = api.Path(segments...)
	return requestPath, "/" + requestPath, nil
}

// parseQuery keeps both the original entry order for dry-run output and the
// url.Values form needed by the HTTP client. Values may contain "=".
func parseQuery(entries []string) ([]kv, url.Values, error) {
	pairs := make([]kv, 0, len(entries))
	values := url.Values{}
	for _, entry := range entries {
		i := strings.IndexByte(entry, '=')
		if i < 0 {
			return nil, nil, &usageError{detail: fmt.Sprintf("--query %q must be key=value", entry)}
		}
		key := entry[:i]
		if key == "" {
			return nil, nil, &usageError{detail: "--query key must not be empty"}
		}
		value := entry[i+1:]
		pairs = append(pairs, kv{key: key, value: value})
		values.Add(key, value)
	}
	return pairs, values, nil
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, set := range values {
		cloned[key] = append([]string(nil), set...)
	}
	return cloned
}

// selectBodyMode resolves flag-only body conflicts without touching a selected
// file or stdin source.
func selectBodyMode(cmd *cobra.Command, flags *unsafeFlags) (bodyMode, error) {
	if flags == nil {
		return bodyModeNone, nil
	}
	fieldSet := len(flags.Field) > 0
	bodySet := cmd.Flags().Changed("body")
	bodyFileSet := cmd.Flags().Changed("body-file")

	if bodySet && bodyFileSet {
		return bodyModeNone, &usageError{detail: "--body and --body-file cannot be combined"}
	}
	if fieldSet && (bodySet || bodyFileSet) {
		return bodyModeNone, &usageError{detail: "--field cannot be combined with --body or --body-file"}
	}
	switch {
	case bodySet || bodyFileSet:
		return bodyModeJSON, nil
	case fieldSet:
		return bodyModeField, nil
	default:
		return bodyModeNone, nil
	}
}

// assembleBody loads and validates the already-selected body mode. No-body
// unsafe calls return nil so the API client receives no JSON null body.
func assembleBody(cmd *cobra.Command, f *cmdutil.Factory, flags *unsafeFlags, mode bodyMode) (json.RawMessage, []bodyField, error) {
	switch mode {
	case bodyModeJSON:
		bodySet := cmd.Flags().Changed("body")
		raw, err := readBodyInput(cmd, f, flags, bodySet)
		if err != nil {
			return nil, nil, err
		}
		body, err := validateJSONBody(raw)
		if err != nil {
			return nil, nil, err
		}
		return body, nil, nil
	case bodyModeField:
		fields, err := parseFields(flags.Field)
		if err != nil {
			return nil, nil, err
		}
		return buildFieldJSON(fields), fields, nil
	default:
		return nil, nil, nil
	}
}

func readBodyInput(cmd *cobra.Command, f *cmdutil.Factory, flags *unsafeFlags, bodySet bool) ([]byte, error) {
	if bodySet {
		return []byte(flags.Body), nil
	}
	if flags.BodyFile == "-" {
		raw, err := f.Prompt(cmd).ReadPiped()
		if err != nil {
			return nil, &usageError{detail: "could not read request body from stdin"}
		}
		return []byte(raw), nil
	}
	data, err := os.ReadFile(flags.BodyFile)
	if err != nil {
		return nil, &usageError{detail: "could not read request body file"}
	}
	return data, nil
}

func validateJSONBody(raw []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if !json.Valid(trimmed) {
		return nil, &usageError{detail: "request body must be valid JSON"}
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func parseFields(entries []string) ([]bodyField, error) {
	fields := make([]bodyField, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		i := strings.IndexByte(entry, '=')
		if i < 0 {
			return nil, &usageError{detail: fmt.Sprintf("--field %q must be key=value", entry)}
		}
		key := entry[:i]
		if key == "" {
			return nil, &usageError{detail: "--field key must not be empty"}
		}
		if seen[key] {
			return nil, &usageError{detail: fmt.Sprintf("--field %q was provided more than once", key)}
		}
		seen[key] = true
		fields = append(fields, bodyField{key: key, value: entry[i+1:]})
	}
	return fields, nil
}

func buildFieldJSON(fields []bodyField) json.RawMessage {
	var b strings.Builder
	b.WriteByte('{')
	for i, field := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(field.key)
		value, _ := json.Marshal(field.value)
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}

func secretFieldSet(values []string) (map[string]bool, error) {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			return nil, &usageError{detail: "--secret-field name must not be empty"}
		}
		out[value] = true
	}
	return out, nil
}

func queryDryRunValues(entries []kv, secret map[string]bool) []output.DryRunValue {
	if len(entries) == 0 {
		return nil
	}
	values := make([]output.DryRunValue, 0, len(entries))
	for _, entry := range entries {
		values = append(values, output.DryRunValue{Name: entry.key, Value: entry.value, Secret: secret[entry.key]})
	}
	return values
}

func bodyDryRunValues(mode bodyMode, fields []bodyField, maskedJSON string, secret map[string]bool) []output.DryRunValue {
	switch mode {
	case bodyModeField:
		values := make([]output.DryRunValue, 0, len(fields))
		for _, field := range fields {
			values = append(values, output.DryRunValue{Name: field.key, Value: field.value, Secret: secret[field.key]})
		}
		return values
	case bodyModeJSON:
		return []output.DryRunValue{{Name: "json", Value: maskedJSON}}
	default:
		return nil
	}
}

func idempotencySource(explicit bool, suppressed bool) string {
	if explicit {
		return "explicit"
	}
	if suppressed {
		return "suppressed"
	}
	return "generated"
}

func dryRunRetry(disabled bool) *output.DryRunRetry {
	if !disabled {
		return nil
	}
	return &output.DryRunRetry{
		Automatic: false,
		Source:    "disabled_non_replayable",
	}
}

// collectSecretScope builds request-only redaction inputs. These values are
// passed to the API client for diagnostics, errors, and CLI-owned response
// metadata, but never widen onto API-owned successful response data.
func collectSecretScope(query []kv, body *requestJSONNode, secret map[string]bool) []string {
	var out []string
	add := func(value string) {
		if value == "" {
			return
		}
		// Different sinks see the same value in different forms: raw text in
		// error messages, URL-escaped text in request URLs, and JSON-escaped
		// text inside serialized API error details.
		out = append(out, value, url.QueryEscape(value))
		if encoded, err := json.Marshal(value); err == nil && len(encoded) >= 2 {
			out = append(out, string(encoded[1:len(encoded)-1]))
		}
	}
	for _, entry := range query {
		if secret[entry.key] {
			add(entry.value)
		}
	}
	if body != nil {
		collectRequestJSONSecrets(body, secret, add)
	}
	return out
}

// dryRunEnabled reads only the leaf-owned flag. Looking at inherited or root
// flags could make an absent preview flag appear enabled on the wrong command.
func dryRunEnabled(cmd *cobra.Command) bool {
	if cmd == nil || cmd.LocalNonPersistentFlags().Lookup("dry-run") == nil {
		return false
	}
	enabled, err := cmd.LocalNonPersistentFlags().GetBool("dry-run")
	return err == nil && enabled
}

func rawRecoverableOperation(method, requestPath string) (chabcontract.Operation, bool, error) {
	registry, err := chabcontract.Load()
	if err != nil {
		return chabcontract.Operation{}, false, err
	}
	fullPath := "/v1/" + strings.TrimLeft(requestPath, "/")
	for _, op := range registry.Operations() {
		if op.Method != method || !rawOperationSupported(op) || !matchContractPath(op.Path, fullPath) {
			continue
		}
		op.Path = fullPath
		return op, true, nil
	}
	return chabcontract.Operation{}, false, nil
}

func rawOperationSupported(op chabcontract.Operation) bool {
	return op.IdempotencyRequired && !apiMethodIsSafe(op.Method)
}

func apiMethodIsSafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func matchContractPath(template, concrete string) bool {
	templateParts := strings.Split(strings.Trim(template, "/"), "/")
	concreteParts := strings.Split(strings.Trim(concrete, "/"), "/")
	if len(templateParts) != len(concreteParts) {
		return false
	}
	for i, part := range templateParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			if concreteParts[i] == "" {
				return false
			}
			continue
		}
		if part != concreteParts[i] {
			return false
		}
	}
	return true
}

func requireRawAcknowledgement(cmd *cobra.Command, f *cmdutil.Factory, op chabcontract.Operation) error {
	if op.NotBillable && op.ID != "operations.cancel" && op.ID != "operations.bulk_cancel" {
		return nil
	}
	return cmdutil.ConfirmDestructive(f.Prompt(cmd), fmt.Sprintf("Raw submission for %s may spend credits or change operation state. Continue?", op.ID))
}

func updateRawAction(action *rawAction, result api.RawResult) (operations.ActionRecord, error) {
	payload := rawOperationPayload(result.Data)
	statusErr := operations.ValidateStartPayload(action.Operation, payload, result.Meta)
	if statusErr != nil && payload.ID == "" {
		_ = action.Store.MarkUnknown(action.Prepared.Record.ID, statusErr)
		return operations.ActionRecord{}, statusErr
	}
	continues := operations.ResponseContinuesByID(action.Operation, payload)
	if statusErr != nil {
		updated, markErr := action.Store.MarkAccepted(action.Prepared.Record.ID, payload, result.Meta)
		if markErr != nil {
			return updated, markErr
		}
		return updated, statusErr
	}
	if continues {
		return action.Store.MarkAccepted(action.Prepared.Record.ID, payload, result.Meta)
	}
	return action.Store.MarkCompleted(action.Prepared.Record.ID, payload, result.Meta)
}

func rawRecordWithResponse(record operations.ActionRecord, op chabcontract.Operation, payload operations.OperationPayload, meta api.ResponseMeta) operations.ActionRecord {
	state := operations.ActionCompleted
	if operations.ResponseContinuesByID(op, payload) {
		state = operations.ActionAccepted
	}
	record.State = state
	if payload.ID != "" {
		record.AcceptedOperationID = payload.ID
	}
	record.RequestID = meta.RequestID
	record.LastStatus = payload.Status
	return record
}

func rawOperationPayload(raw json.RawMessage) operations.OperationPayload {
	var payload operations.OperationPayload
	_ = json.Unmarshal(raw, &payload)
	payload.Raw = append(json.RawMessage(nil), raw...)
	return payload
}

func writeRawActionRecovery(cmd *cobra.Command, f *cmdutil.Factory, record operations.ActionRecord) error {
	return writeRawActionOutput(cmd, f, rawActionOutput(record, nil, api.ResponseMeta{}), api.ResponseMeta{RequestID: record.RequestID})
}

func rawActionOutput(record operations.ActionRecord, server json.RawMessage, meta api.ResponseMeta) operations.CommandOutput {
	projection := operations.Projection(record)
	out := operations.CommandOutput{
		Action:      &projection,
		OperationID: record.AcceptedOperationID,
		Server:      server,
		RequestID:   firstNonEmpty(meta.RequestID, record.RequestID),
		Meta:        rawResponseMeta(meta),
		LocalRecovery: operations.RecoveryStatus{
			State:       string(record.State),
			CanResume:   record.State == operations.ActionUnknown || record.State == operations.ActionAccepted,
			KnownRemote: record.AcceptedOperationID != "",
		},
		LocalPersistence: &operations.PersistenceState{State: "persisted"},
	}
	if record.AcceptedOperationID != "" && record.State != operations.ActionCompleted {
		out.LocalRecovery.ResumeHint = "chab operations wait " + record.AcceptedOperationID
	} else if record.State == operations.ActionUnknown {
		out.LocalRecovery.ResumeHint = "chab operations resume " + record.ID + " --input @request.json"
	}
	return out
}

func writeRawActionOutput(cmd *cobra.Command, f *cmdutil.Factory, out operations.CommandOutput, meta api.ResponseMeta) error {
	return f.WriteResultWithMeta(cmd, out, meta, true, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			if out.Action != nil {
				fmt.Fprintf(w, "Action ID: %s\n", out.Action.ID)
			}
			if out.OperationID != "" {
				fmt.Fprintf(w, "Operation ID: %s\n", out.OperationID)
			}
			if out.LocalRecovery.ResumeHint != "" {
				fmt.Fprintf(w, "Resume: %s\n", out.LocalRecovery.ResumeHint)
			}
			if len(out.Server) > 0 {
				fmt.Fprintf(w, "Server: %s\n", string(out.Server))
			}
		},
		Plain: func(data, prose io.Writer) {
			if out.OperationID != "" {
				fmt.Fprintln(data, out.OperationID)
			} else if out.Action != nil {
				fmt.Fprintln(data, out.Action.ID)
			}
			if out.LocalRecovery.ResumeHint != "" {
				fmt.Fprintln(prose, out.LocalRecovery.ResumeHint)
			}
		},
	})
}

func rawResponseMeta(meta api.ResponseMeta) *output.MetaView {
	view := output.ViewMeta(meta, output.MetaOptions{IncludeAPIMeta: true})
	if view.Empty() {
		return nil
	}
	return &view
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// sanitizeRawSuccess removes the credential, request-marked secrets, and
// request idempotency key from semantic JSON strings and keys. A transform
// failure becomes a protocol error carrying response metadata, because
// returning the original bytes would fail open.
func sanitizeRawSuccess(raw json.RawMessage, meta api.ResponseMeta) (json.RawMessage, error) {
	sanitized, err := meta.RedactJSON(raw)
	if err != nil {
		return nil, &api.ProtocolError{
			Detail:    "raw API response could not be sanitized safely",
			Status:    meta.HTTPStatus,
			RequestID: meta.RequestID,
			Err:       err,
			Meta:      meta,
		}
	}
	return json.RawMessage(sanitized), nil
}

type jsonKind int

const (
	jsonKindInvalid jsonKind = iota
	jsonKindObject
	jsonKindArray
	jsonKindString
	jsonKindNumber
	jsonKindBool
	jsonKindNull
)

func jsonValueKind(raw []byte) jsonKind {
	// Classify from the first non-space byte so callers can choose a cheap path
	// without decoding the whole JSON value.
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return jsonKindObject
		case '[':
			return jsonKindArray
		case '"':
			return jsonKindString
		case 't', 'f':
			return jsonKindBool
		case 'n':
			return jsonKindNull
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return jsonKindNumber
		default:
			return jsonKindInvalid
		}
	}
	return jsonKindInvalid
}
