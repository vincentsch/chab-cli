package projects

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// writeFlags holds Project mutation inputs. Create and update use the complete
// field/body set, while fixed-status mutations bind only the idempotency key.
type writeFlags struct {
	Name            string
	Description     string
	DescriptionNull bool
	URL             string
	URLNull         bool
	Status          string
	Timezone        string
	Language        string
	Limit           int64
	Automate        bool

	Body           string
	BodyFile       string
	IdempotencyKey string
}

type writeKind int

const (
	kindCreate writeKind = iota
	kindUpdate
	kindLifecycle
)

// writeOp carries the operation-specific pieces that differ after a Project
// mutation payload has been built and validated.
type writeOp struct {
	kind        writeKind
	method      string
	action      string
	requestPath string
	displayPath string
	// expectedStatus is set only for fixed-status operations so the shared
	// executor can reject an accidentally widened or mismatched payload.
	expectedStatus string
}

// payloadField is one validated outgoing body field plus the string form used
// in dry-run previews. Keeping both avoids re-parsing arbitrary JSON values
// while still letting the API receive typed values.
type payloadField struct {
	name    string
	value   any
	display string
}

// payloadBuilder delays every payload operation until explicit idempotency-key
// validation and secret registration have completed.
type payloadBuilder func() ([]payloadField, bool, error)

// orderedFieldNames is the allowed body field set and the canonical order for
// validation, request-body construction, and dry-run output.
var orderedFieldNames = []string{"name", "description", "url", "status", "timezone", "language", "limit", "automate"}

func registerWriteFlags(cmd *cobra.Command, flags *writeFlags) {
	f := cmd.Flags()
	f.StringVar(&flags.Name, "name", "", "project name (sent in the body)")
	f.StringVar(&flags.Description, "description", "", "project description (sent in the body; --description \"\" clears it)")
	f.BoolVar(&flags.DescriptionNull, "description-null", false, "send description as null")
	f.StringVar(&flags.URL, "url", "", "project URL (sent in the body)")
	f.BoolVar(&flags.URLNull, "url-null", false, "send url as null")
	f.StringVar(&flags.Status, "status", "", "project status: active, paused, or archived")
	f.StringVar(&flags.Timezone, "timezone", "", "project timezone (sent in the body; validated by the API)")
	f.StringVar(&flags.Language, "language", "", "project content language (sent in the body; distinct from the global --locale)")
	f.Int64Var(&flags.Limit, "limit", 0, "project resource limit field (sent in the body; not a pagination control)")
	f.BoolVar(&flags.Automate, "automate", false, "project automate flag (sent in the body)")
	f.StringVar(&flags.Body, "body", "", "JSON request body (mutually exclusive with field flags and --body-file)")
	f.StringVar(&flags.BodyFile, "body-file", "", "read the JSON request body from a file, or - for stdin")
	f.StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key (1-255 visible ASCII bytes); generated when omitted")
	f.Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
}

// runWrite is the shared Project mutation executor. The ordering here is part
// of the command contract: explicit-key handling, payload construction and
// policy, dry-run, runtime and credentials, client construction, HTTP,
// response decoding and redaction, then rendering.
func runWrite(cmd *cobra.Command, f *cmdutil.Factory, flags *writeFlags, build payloadBuilder, op writeOp) (runErr error) {
	explicitKey := cmd.Flags().Changed("idempotency-key")
	if explicitKey {
		if err := api.ValidateIdempotencyKey(flags.IdempotencyKey); err != nil {
			return err
		}
		f.RegisterSecret(flags.IdempotencyKey)
		defer func() {
			runErr = f.RedactError(runErr)
		}()
	}

	fields, bodyMode, err := build()
	if err != nil {
		return err
	}

	if err := validateWritePayload(cmd, fields, bodyMode, op); err != nil {
		return err
	}

	if dryRunEnabled(cmd) {
		// Nothing that resolves config, credentials, or HTTP clients may move
		// above this branch; every Project mutation dry-run is an offline
		// preview after explicit-key, payload, and operation validation.
		preview := output.DryRunPreview{
			Method:      op.method,
			Path:        f.RedactValue(op.displayPath),
			Body:        dryRunBody(f, fields),
			Idempotency: dryRunIdempotency(explicitKey),
		}
		return f.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: func(w io.Writer) { preview.Render(w) },
			Plain:  preview.RenderPlain,
		})
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
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return err
	}

	idem := api.AutoIdempotency()
	if explicitKey {
		idem = api.ExplicitIdempotency(flags.IdempotencyKey)
	}

	// Build the request body after dry-run so previews do not depend on the
	// exact map encoding that will be used for the real API request.
	body := buildRequestBody(fields)
	var wrapped struct {
		Project *projectJSON `json:"project"`
	}
	var meta api.ResponseMeta
	// Select the transport explicitly so a new or unknown operation can never
	// inherit PATCH behavior by falling through a default branch.
	switch op.kind {
	case kindCreate:
		meta, err = client.Post(cmd.Context(), op.requestPath, nil, body, idem, &wrapped)
	case kindUpdate, kindLifecycle:
		meta, err = client.Patch(cmd.Context(), op.requestPath, nil, body, idem, &wrapped)
	default:
		return fmt.Errorf("unsupported project write operation")
	}
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	if wrapped.Project == nil {
		return &api.ProtocolError{Detail: "project response missing data.project", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	p := *wrapped.Project
	p = redactProjectValues(f, p)

	summary := output.MutationSummary{
		Action:   op.action,
		Resource: "project",
		Name:     p.Name,
		Fields:   projectDetailNodes(p),
	}
	return f.WriteResultWithMeta(cmd, p, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) { summary.Render(w) },
		Plain:  summary.RenderPlain,
	})
}

// validateWritePayload applies the operation-owned request policy before an
// offline preview or any runtime work. Fixed-status mutations are fail-closed:
// their internal builder must return exactly the command-owned status field.
func validateWritePayload(cmd *cobra.Command, fields []payloadField, bodyMode bool, op writeOp) error {
	switch op.kind {
	case kindCreate:
		// Field mode provides early guidance, while complete JSON bodies leave
		// required-field decisions to the API.
		if op.method != httpMethodPost {
			return fmt.Errorf("invalid project create method")
		}
		if !bodyMode && !cmd.Flags().Changed("name") {
			return &usageError{detail: "project create requires --name (or a JSON body via --body/--body-file)"}
		}
	case kindUpdate:
		// An empty update has no useful server-side meaning in either input
		// mode, so reject it before runtime work.
		if op.method != httpMethodPatch {
			return fmt.Errorf("invalid project update method")
		}
		if len(fields) == 0 {
			return &usageError{detail: "project update requires at least one field to change"}
		}
	case kindLifecycle:
		// The status is command-owned. Any internal mismatch must fail closed
		// instead of silently turning this into a generic update.
		if op.method != httpMethodPatch {
			return fmt.Errorf("invalid project lifecycle method")
		}
		if bodyMode || len(fields) != 1 ||
			fields[0].name != "status" ||
			fields[0].display != op.expectedStatus {
			return fmt.Errorf("invalid project lifecycle payload")
		}
		status, ok := fields[0].value.(string)
		if !ok || status != op.expectedStatus {
			return fmt.Errorf("invalid project lifecycle payload")
		}
	default:
		return fmt.Errorf("unsupported project write operation")
	}
	return nil
}

const (
	httpMethodPost  = "POST"
	httpMethodPatch = "PATCH"
)

// assembleWritePayload selects exactly one input mode. Field flags use Cobra's
// Changed state so explicit zero values are sent; body mode validates only the
// JSON shape and field types, leaving semantic rules to the API.
func assembleWritePayload(cmd *cobra.Command, f *cmdutil.Factory, flags *writeFlags) ([]payloadField, bool, error) {
	bodySet := cmd.Flags().Changed("body")
	bodyFileSet := cmd.Flags().Changed("body-file")
	if bodySet && bodyFileSet {
		return nil, false, &usageError{detail: "--body and --body-file cannot be combined"}
	}
	bodyMode := bodySet || bodyFileSet
	if bodyMode && anyFieldFlagChanged(cmd) {
		return nil, true, &usageError{detail: "--body/--body-file cannot be combined with field flags"}
	}

	if bodyMode {
		raw, err := readBodyInput(cmd, f, flags, bodySet)
		if err != nil {
			return nil, true, err
		}
		fields, err := parseJSONBody(raw)
		return fields, true, err
	}

	if cmd.Flags().Changed("status") && !validProjectStatus(flags.Status) {
		return nil, false, &usageError{detail: "--status must be one of active, paused, or archived"}
	}
	if cmd.Flags().Changed("description") && flags.DescriptionNull {
		return nil, false, &usageError{detail: "--description and --description-null cannot be combined"}
	}
	if cmd.Flags().Changed("url") && flags.URLNull {
		return nil, false, &usageError{detail: "--url and --url-null cannot be combined"}
	}
	return assembleFlagFields(cmd, flags), false, nil
}

func anyFieldFlagChanged(cmd *cobra.Command) bool {
	for _, name := range orderedFieldNames {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	if cmd.Flags().Changed("description-null") || cmd.Flags().Changed("url-null") {
		return true
	}
	return false
}

// assembleFlagFields keeps omitted flags out of the body while preserving
// intentionally supplied zero values such as --description "" or
// --automate=false.
func assembleFlagFields(cmd *cobra.Command, flags *writeFlags) []payloadField {
	var fields []payloadField
	add := func(name string, value any, display string) {
		if cmd.Flags().Changed(name) {
			fields = append(fields, payloadField{name: name, value: value, display: display})
		}
	}
	add("name", flags.Name, flags.Name)
	add("description", flags.Description, flags.Description)
	if flags.DescriptionNull {
		fields = append(fields, payloadField{name: "description", value: nil, display: "null"})
	}
	add("url", flags.URL, flags.URL)
	if flags.URLNull {
		fields = append(fields, payloadField{name: "url", value: nil, display: "null"})
	}
	add("status", flags.Status, flags.Status)
	add("timezone", flags.Timezone, flags.Timezone)
	add("language", flags.Language, flags.Language)
	add("limit", flags.Limit, strconv.FormatInt(flags.Limit, 10))
	add("automate", flags.Automate, strconv.FormatBool(flags.Automate))
	return fields
}

// readBodyInput treats --body-file as explicit request data. The stdin case
// still goes through the injected prompt reader so command tests can prove no
// real terminal or process stdin is required.
func readBodyInput(cmd *cobra.Command, f *cmdutil.Factory, flags *writeFlags, bodySet bool) ([]byte, error) {
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

// parseJSONBody rejects malformed or structurally unsupported request bodies
// before credentials are resolved. It sorts unknown fields for stable errors
// and returns accepted fields in the canonical dry-run/body order.
func parseJSONBody(raw []byte) ([]payloadField, error) {
	// encoding/json replaces malformed UTF-8 inside quoted strings with U+FFFD.
	// Reject it first so body mode never silently changes request data while
	// claiming to accept strict JSON.
	if !utf8.Valid(raw) {
		return nil, &usageError{detail: "request body must be valid JSON"}
	}
	if !validJSONUnicodeEscapes(raw) {
		return nil, &usageError{detail: "request body must be valid JSON"}
	}
	trimmed := bytes.TrimSpace(raw)
	if jsonValueKind(trimmed) != kindObject {
		if !json.Valid(trimmed) {
			return nil, &usageError{detail: "request body must be valid JSON"}
		}
		return nil, &usageError{detail: "request body must be a JSON object"}
	}
	obj, duplicate, hasDuplicate, err := decodeJSONObject(trimmed)
	if err != nil {
		return nil, &usageError{detail: "request body must be valid JSON"}
	}
	if hasDuplicate {
		return nil, &usageError{detail: fmt.Sprintf("duplicate field %q in request body", duplicate)}
	}

	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !isAllowedField(key) {
			return nil, &usageError{detail: fmt.Sprintf("unknown field %q in request body", key)}
		}
	}

	var fields []payloadField
	for _, name := range orderedFieldNames {
		rawVal, ok := obj[name]
		if !ok {
			continue
		}
		field, err := validateBodyField(name, rawVal)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// validJSONUnicodeEscapes prevents encoding/json from replacing unpaired
// UTF-16 surrogate escapes with U+FFFD. Other JSON syntax remains owned by the
// duplicate-aware decoder so all malformed input keeps the same public error.
func validJSONUnicodeEscapes(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString {
				continue
			}
			i++
			if i >= len(raw) {
				return false
			}
			if raw[i] != 'u' {
				continue
			}
			if i+5 > len(raw) {
				return false
			}
			code, ok := jsonHexQuad(raw[i+1 : i+5])
			if !ok {
				return false
			}
			i += 4
			switch {
			case code >= 0xd800 && code <= 0xdbff:
				if i+7 > len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := jsonHexQuad(raw[i+3 : i+7])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			case code >= 0xdc00 && code <= 0xdfff:
				return false
			}
		}
	}
	return true
}

func jsonHexQuad(raw []byte) (uint16, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	var value uint16
	for _, b := range raw {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			value |= uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			value |= uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

// decodeJSONObject consumes exactly one complete top-level object, retaining
// the first raw value for each decoded key while recording the first duplicate.
// Duplicate reporting is delayed until structural validation and EOF succeed.
func decodeJSONObject(raw []byte) (map[string]json.RawMessage, string, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, "", false, fmt.Errorf("invalid object")
	}

	values := make(map[string]json.RawMessage)
	seen := make(map[string]struct{})
	var duplicate string
	hasDuplicate := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, "", false, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, "", false, fmt.Errorf("invalid object key")
		}
		// Decode the complete value before checking the key. This keeps nested
		// objects out of the top-level duplicate scan and makes malformed input
		// win over a duplicate recorded earlier in the document.
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, "", false, err
		}
		if _, exists := seen[key]; exists {
			if !hasDuplicate {
				duplicate = key
				hasDuplicate = true
			}
			continue
		}
		seen[key] = struct{}{}
		values[key] = append(json.RawMessage(nil), value...)
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, "", false, fmt.Errorf("invalid object close")
	}
	// A valid closing brace is not enough: body mode accepts exactly one JSON
	// document, so any token after the object is an error.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing JSON value")
		}
		return nil, "", false, err
	}
	return values, duplicate, hasDuplicate, nil
}

// validateBodyField accepts only the scalar type owned by each project field.
// It deliberately does not validate business values such as timezone strings
// or body-mode status enums; those remain API validation rules.
func validateBodyField(name string, raw json.RawMessage) (payloadField, error) {
	switch name {
	case "limit":
		num, err := jsonIntegerToken(raw)
		if err != nil {
			return payloadField{}, &usageError{detail: fmt.Sprintf("field %q must be an integer", name)}
		}
		return payloadField{name: name, value: num, display: num.String()}, nil
	case "automate":
		if jsonValueKind(raw) != kindBool {
			return payloadField{}, &usageError{detail: fmt.Sprintf("field %q must be a boolean", name)}
		}
		var b bool
		_ = json.Unmarshal(raw, &b)
		return payloadField{name: name, value: b, display: strconv.FormatBool(b)}, nil
	default:
		if (name == "description" || name == "url") && jsonValueKind(raw) == kindNull {
			return payloadField{name: name, value: nil, display: "null"}, nil
		}
		if jsonValueKind(raw) != kindString {
			return payloadField{}, &usageError{detail: fmt.Sprintf("field %q must be a string", name)}
		}
		var s string
		_ = json.Unmarshal(raw, &s)
		return payloadField{name: name, value: s, display: s}, nil
	}
}

func isAllowedField(name string) bool {
	for _, n := range orderedFieldNames {
		if n == name {
			return true
		}
	}
	return false
}

func buildRequestBody(fields []payloadField) map[string]any {
	body := make(map[string]any, len(fields))
	for _, field := range fields {
		body[field.name] = field.value
	}
	return body
}

// dryRunBody preserves each documented field name and redacts only its display
// value. A field name may happen to match a key without being secret data.
func dryRunBody(f *cmdutil.Factory, fields []payloadField) []output.DryRunValue {
	if len(fields) == 0 {
		return nil
	}
	values := make([]output.DryRunValue, 0, len(fields))
	for _, field := range fields {
		values = append(values, output.DryRunValue{Name: field.name, Value: f.RedactValue(field.display)})
	}
	return values
}

// redactProjectValues copies the API result and redacts only semantic string
// fields. Keeping this step before rendering protects every output mode without
// mistaking field names, JSON delimiters, numbers, or booleans for secrets.
func redactProjectValues(f *cmdutil.Factory, p projectJSON) projectJSON {
	p.ID = f.RedactValue(p.ID)
	p.Name = f.RedactValue(p.Name)
	p.Description = redactOptionalString(f, p.Description)
	p.URL = redactOptionalString(f, p.URL)
	p.Status = f.RedactValue(p.Status)
	p.Timezone = f.RedactValue(p.Timezone)
	p.Language = f.RedactValue(p.Language)
	p.CreatedAt = f.RedactValue(p.CreatedAt)
	p.UpdatedAt = f.RedactValue(p.UpdatedAt)
	return p
}

func redactProjectRows(f *cmdutil.Factory, rows []projectJSON) []projectJSON {
	if len(rows) == 0 {
		return rows
	}
	redacted := make([]projectJSON, len(rows))
	for i, row := range rows {
		redacted[i] = redactProjectValues(f, row)
	}
	return redacted
}

func redactOptionalString(f *cmdutil.Factory, value *string) *string {
	if value == nil {
		return nil
	}
	redacted := f.RedactValue(*value)
	return &redacted
}

func dryRunIdempotency(explicit bool) *output.DryRunIdempotency {
	if explicit {
		return &output.DryRunIdempotency{Source: "explicit"}
	}
	return &output.DryRunIdempotency{Source: "generated"}
}

type jsonKind int

const (
	kindInvalid jsonKind = iota
	kindObject
	kindArray
	kindString
	kindNumber
	kindBool
	kindNull
)

// jsonValueKind classifies a raw JSON value by its first non-space byte. That
// is enough for local shape checks and avoids decoding numbers into float64.
func jsonValueKind(raw []byte) jsonKind {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return kindObject
		case '[':
			return kindArray
		case '"':
			return kindString
		case 't', 'f':
			return kindBool
		case 'n':
			return kindNull
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return kindNumber
		default:
			return kindInvalid
		}
	}
	return kindInvalid
}

// jsonIntegerToken keeps the original integer token as json.Number so the API
// owns range validation and receives the user's exact decimal text.
func jsonIntegerToken(raw json.RawMessage) (json.Number, error) {
	if jsonValueKind(raw) != kindNumber {
		return "", fmt.Errorf("not a number")
	}
	token := strings.TrimSpace(string(raw))
	if strings.ContainsAny(token, ".eE") {
		return "", fmt.Errorf("not an integer token")
	}
	return json.Number(token), nil
}
