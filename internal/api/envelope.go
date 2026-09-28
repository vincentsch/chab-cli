package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// Pagination is the complete meta.pagination shape returned by list
// envelopes. From and To are nil when the API returns JSON null.
type Pagination struct {
	CurrentPage int  `json:"current_page"`
	PerPage     int  `json:"per_page"`
	Total       int  `json:"total"`
	LastPage    int  `json:"last_page"`
	From        *int `json:"from"`
	To          *int `json:"to"`
	HasMore     bool `json:"has_more"`
}

// successEnvelope keeps data and meta raw until the client knows which parts
// need typed decoding. That preserves unknown meta keys while still detecting
// a missing data envelope for callers that expect a body.
type successEnvelope struct {
	Data      json.RawMessage            `json:"data"`
	Operation json.RawMessage            `json:"operation"`
	Meta      map[string]json.RawMessage `json:"meta"`
	RequestID string                     `json:"request_id"`
}

// errorEnvelope mirrors the public error contract. Details stay raw first
// because older and newer API versions may disagree on their exact shape.
type errorEnvelope struct {
	Error *struct {
		Code       string          `json:"code"`
		Message    string          `json:"message"`
		UserAction string          `json:"user_action"`
		Retryable  *bool           `json:"retryable"`
		Details    json.RawMessage `json:"details"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// RawResult carries a raw escape-hatch decode of a 2xx response: decoded data,
// the full success envelope, and response metadata for optional machine output.
type RawResult struct {
	Data     json.RawMessage
	Envelope json.RawMessage
	Meta     ResponseMeta
}

// decodeResponse routes by status because both successful and failed API
// responses are expected to use envelopes.
func (c *Client) decodeResponse(status int, body []byte, meta ResponseMeta, out any, redactor requestRedactor) (ResponseMeta, error) {
	if status >= 200 && status < 300 {
		return c.decodeSuccess(status, body, meta, out, redactor)
	}
	return c.decodeError(status, body, meta, redactor)
}

// decodeSuccess validates the success envelope and decodes only the requested
// data type. Metadata is retained even when the data shape is invalid.
func (c *Client) decodeSuccess(status int, body []byte, meta ResponseMeta, out any, redactor requestRedactor) (ResponseMeta, error) {
	meta = redactor.redactMeta(meta)
	if len(bytes.TrimSpace(body)) == 0 {
		return ResponseMeta{}, &ProtocolError{Detail: "empty response body", Status: status, RequestID: meta.RequestID, Meta: meta}
	}

	var env successEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ResponseMeta{}, &ProtocolError{Detail: "malformed JSON response body", Status: status, RequestID: meta.RequestID, Err: redactor.redactErr(err), Meta: meta}
	}

	if perr := c.applyEnvelopeMeta(&meta, env, status, redactor); perr != nil {
		return ResponseMeta{}, perr
	}

	if out != nil {
		payload, perr := selectSuccessPayload(env, false, status, meta)
		if perr != nil {
			return ResponseMeta{}, perr
		}
		if err := json.Unmarshal(payload, out); err != nil {
			return ResponseMeta{}, &ProtocolError{Detail: "response data does not match the expected shape", Status: status, RequestID: meta.RequestID, Err: redactor.redactErr(err), Meta: meta}
		}
	}

	return meta, nil
}

// decodeRaw keeps the API envelope opaque except for the data presence check
// and shared metadata extraction. Unlike typed decoders, it allows data:null.
func (c *Client) decodeRaw(status int, body []byte, meta ResponseMeta, redactor requestRedactor) (RawResult, error) {
	meta = redactor.redactMeta(meta)
	if status < 200 || status >= 300 {
		_, err := c.decodeError(status, body, meta, redactor)
		return RawResult{}, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return RawResult{}, &ProtocolError{Detail: "empty response body", Status: status, RequestID: meta.RequestID, Meta: meta}
	}

	var env successEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return RawResult{}, &ProtocolError{Detail: "malformed JSON response body", Status: status, RequestID: meta.RequestID, Err: redactor.redactErr(err), Meta: meta}
	}
	if perr := c.applyEnvelopeMeta(&meta, env, status, redactor); perr != nil {
		return RawResult{}, perr
	}
	payload, perr := selectSuccessPayload(env, true, status, meta)
	if perr != nil {
		return RawResult{}, perr
	}

	return RawResult{
		Data:     append(json.RawMessage(nil), payload...),
		Envelope: append(json.RawMessage(nil), body...),
		Meta:     meta,
	}, nil
}

func selectSuccessPayload(env successEnvelope, allowDataNull bool, status int, meta ResponseMeta) (json.RawMessage, *ProtocolError) {
	dataPresent := env.Data != nil
	operationPresent := env.Operation != nil
	if dataPresent && operationPresent {
		return nil, &ProtocolError{Detail: "response envelope must not include both data and operation", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	if !dataPresent && !operationPresent {
		return nil, &ProtocolError{Detail: "response envelope missing data or operation", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	if operationPresent {
		if bytes.Equal(bytes.TrimSpace(env.Operation), []byte("null")) {
			return nil, &ProtocolError{Detail: "response envelope operation is null", Status: status, RequestID: meta.RequestID, Meta: meta}
		}
		return env.Operation, nil
	}
	if !allowDataNull && bytes.Equal(bytes.TrimSpace(env.Data), []byte("null")) {
		return nil, &ProtocolError{Detail: "response envelope payload is null", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	return env.Data, nil
}

// decodeError turns a valid API error envelope into a typed Error. Malformed
// error envelopes are protocol failures, not API business errors.
func (c *Client) decodeError(status int, body []byte, meta ResponseMeta, redactor requestRedactor) (ResponseMeta, error) {
	meta = redactor.redactMeta(meta)
	if len(bytes.TrimSpace(body)) == 0 {
		return ResponseMeta{}, &ProtocolError{Detail: "empty response body", Status: status, RequestID: meta.RequestID, Meta: meta}
	}

	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ResponseMeta{}, &ProtocolError{Detail: "malformed JSON response body", Status: status, RequestID: meta.RequestID, Err: redactor.redactErr(err), Meta: meta}
	}

	c.applyEnvelopeRequestID(&meta, env.RequestID, redactor)
	if env.Error == nil || env.Error.Code == "" {
		return ResponseMeta{}, &ProtocolError{Detail: "error envelope missing error code", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	if env.Error.Retryable == nil {
		return ResponseMeta{}, &ProtocolError{Detail: "error envelope missing retryable", Status: status, RequestID: meta.RequestID, Meta: meta}
	}

	details := redactValidationDetails(decodeDetails(env.Error.Details), redactor.redactText)
	detailsValue := redactStructuredDetails(decodeAnyDetails(env.Error.Details), redactor.redactText)
	if env.Error.Details != nil && detailsValue == nil && !bytes.Equal(bytes.TrimSpace(env.Error.Details), []byte("null")) {
		c.debugfWith(redactor, "error details not decodable")
	}
	// Classify before redaction. A server may echo a request secret as its code;
	// callers must see the redacted code without losing the original exit class.
	classifiedExitCode := 2
	if exitCode, ok := exitCodes[env.Error.Code]; ok {
		classifiedExitCode = exitCode
	}

	return ResponseMeta{}, &Error{
		Code:               redactor.redactText(env.Error.Code),
		Message:            redactor.redactText(env.Error.Message),
		UserAction:         redactor.redactText(env.Error.UserAction),
		Retryable:          *env.Error.Retryable,
		Details:            details,
		DetailsValue:       detailsValue,
		DetailsPresent:     env.Error.Details != nil,
		RawDetails:         redactRawDetails(env.Error.Details, redactor.redactText),
		RequestID:          meta.RequestID,
		Status:             status,
		Meta:               meta,
		classifiedExitCode: classifiedExitCode,
	}
}

// applyEnvelopeMeta enriches metadata for typed and raw decode paths. It
// sanitizes the copy available to renderers, but validates pagination from the
// original response. The validated value remains private when exposing it
// would reveal a request secret.
func (c *Client) applyEnvelopeMeta(meta *ResponseMeta, env successEnvelope, status int, redactor requestRedactor) *ProtocolError {
	c.applyEnvelopeRequestID(meta, env.RequestID, redactor)
	if env.Meta == nil {
		return nil
	}
	if rawRequestID, ok := env.Meta["request_id"]; ok {
		var requestID string
		if err := json.Unmarshal(rawRequestID, &requestID); err != nil {
			return &ProtocolError{
				Detail:    "malformed response metadata",
				Status:    status,
				RequestID: meta.RequestID,
				Err:       redactor.redactErr(err),
				Meta:      *meta,
			}
		}
		c.applyEnvelopeRequestID(meta, requestID, redactor)
	}
	rawMeta, err := sanitizeRawMeta(cloneRawMeta(env.Meta), redactor.redactText)
	if err != nil {
		return &ProtocolError{
			Detail:    "malformed response metadata",
			Status:    status,
			RequestID: meta.RequestID,
			Err:       redactor.redactErr(err),
			Meta:      *meta,
		}
	}
	meta.RawMeta = rawMeta
	if hasFlatCursorMeta(env.Meta) {
		rawFlatMeta, err := json.Marshal(env.Meta)
		if err != nil {
			return &ProtocolError{
				Detail:    "malformed response metadata",
				Status:    status,
				RequestID: meta.RequestID,
				Err:       redactor.redactErr(err),
				Meta:      *meta,
			}
		}
		redactedPagination, err := rawMetadataValueNeedsRedaction(rawFlatMeta, redactor.redactText)
		if err != nil {
			return &ProtocolError{
				Detail:    "malformed response metadata",
				Status:    status,
				RequestID: meta.RequestID,
				Err:       redactor.redactErr(err),
				Meta:      *meta,
			}
		}
		cursor, perr := decodeCursorPagination(rawFlatMeta, status, *meta)
		if perr != nil {
			return perr
		}
		meta.traversalCursor = cursor
		if !redactedPagination {
			meta.CursorPagination = cursor
		}
		return nil
	}
	if rawPagination, ok := env.Meta["pagination"]; ok {
		redactedPagination, err := rawMetadataValueNeedsRedaction(rawPagination, redactor.redactText)
		if err != nil {
			return &ProtocolError{
				Detail:    "malformed response metadata",
				Status:    status,
				RequestID: meta.RequestID,
				Err:       redactor.redactErr(err),
				Meta:      *meta,
			}
		}
		var paginationShape map[string]json.RawMessage
		_ = json.Unmarshal(rawPagination, &paginationShape)
		if _, cursorShape := paginationShape["next_cursor"]; cursorShape {
			if _, mixed := paginationShape["current_page"]; mixed {
				return paginationProtocolError(status, *meta, nil)
			}
			cursor, perr := decodeCursorPagination(rawPagination, status, *meta)
			if perr != nil {
				return perr
			}
			meta.traversalCursor = cursor
			if !redactedPagination {
				meta.CursorPagination = cursor
			}
			return nil
		}
		pagination, perr := decodePagination(rawPagination, status, *meta, redactor.redactErr)
		if perr != nil {
			return perr
		}
		meta.traversalPagination = pagination
		if !redactedPagination {
			meta.Pagination = pagination
		}
	}
	return nil
}

// applyEnvelopeRequestID gives the /v1 envelope value precedence over the
// optional transport header. Header capture remains diagnostic-only.
func (c *Client) applyEnvelopeRequestID(meta *ResponseMeta, envelopeID string, redactor requestRedactor) {
	envelopeID = redactor.redactText(envelopeID)
	meta.EnvelopeRequestID = envelopeID
	if envelopeID != "" {
		if meta.HeaderRequestID != "" && envelopeID != meta.HeaderRequestID {
			c.debugfWith(redactor, "request-id mismatch header=%s envelope=%s", meta.HeaderRequestID, envelopeID)
		}
		meta.RequestID = envelopeID
		return
	}
	meta.RequestID = meta.HeaderRequestID
}

// decodeDetails accepts the documented validation-details shape and otherwise
// returns nil so error decoding remains forward-compatible.
func decodeDetails(raw json.RawMessage) map[string][]string {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var details map[string][]string
	if err := json.Unmarshal(raw, &details); err != nil {
		return nil
	}
	return details
}

func decodeAnyDetails(raw json.RawMessage) any {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil
	}
	return value
}

func redactStructuredDetails(value any, redactText func(string) string) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactText(typed)
	case json.Number:
		redacted := redactText(typed.String())
		if redacted != typed.String() {
			return redacted
		}
		return typed
	case bool:
		text := strconv.FormatBool(typed)
		redacted := redactText(text)
		if redacted != text {
			return redacted
		}
		return typed
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactStructuredDetails(item, redactText)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[redactText(key)] = redactStructuredDetails(item, redactText)
		}
		return out
	default:
		text := fmt.Sprint(typed)
		redacted := redactText(text)
		if redacted != text {
			return redacted
		}
		return typed
	}
}

func hasFlatCursorMeta(meta map[string]json.RawMessage) bool {
	for _, key := range []string{"next_cursor", "prev_cursor", "has_more", "limit"} {
		if _, ok := meta[key]; ok {
			return true
		}
	}
	return false
}

func redactRawDetails(raw json.RawMessage, redactText func(string) string) json.RawMessage {
	value := redactStructuredDetails(decodeAnyDetails(raw), redactText)
	if value == nil {
		if len(raw) != 0 && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return json.RawMessage("null")
		}
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

// redactValidationDetails copies the validation-detail map before replacing
// dynamic secrets so decoded API errors cannot expose request values marked
// sensitive.
func redactValidationDetails(details map[string][]string, redactText func(string) string) map[string][]string {
	if len(details) == 0 {
		return details
	}
	out := make(map[string][]string, len(details))
	for key, values := range details {
		redactedKey := key
		if redactText != nil {
			redactedKey = redactText(key)
		}
		cloned := make([]string, len(values))
		for i, value := range values {
			if redactText != nil {
				cloned[i] = redactText(value)
			} else {
				cloned[i] = value
			}
		}
		out[redactedKey] = append(out[redactedKey], cloned...)
	}
	return out
}

// decodePagination validates the metadata shape explicitly instead of decoding
// straight into Pagination. That prevents missing fields from silently becoming
// zero values while still allowing the documented nullable bounds.
func decodePagination(raw json.RawMessage, status int, meta ResponseMeta, redactErr func(error) error) (*Pagination, *ProtocolError) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, paginationProtocolError(status, meta, nil)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	if fields == nil {
		return nil, paginationProtocolError(status, meta, nil)
	}

	currentPage, err := requiredIntField(fields, "current_page")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	perPage, err := requiredIntField(fields, "per_page")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	total, err := requiredIntField(fields, "total")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	lastPage, err := requiredIntField(fields, "last_page")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	from, err := requiredNullableIntField(fields, "from")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	to, err := requiredNullableIntField(fields, "to")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}
	hasMore, err := requiredBoolField(fields, "has_more")
	if err != nil {
		return nil, paginationProtocolError(status, meta, redactOptionalErr(redactErr, err))
	}

	return &Pagination{
		CurrentPage: currentPage,
		PerPage:     perPage,
		Total:       total,
		LastPage:    lastPage,
		From:        from,
		To:          to,
		HasMore:     hasMore,
	}, nil
}

// requiredIntField distinguishes missing, null, and wrong-type fields so a
// malformed pagination object cannot look like a valid first page.
func requiredIntField(fields map[string]json.RawMessage, name string) (int, error) {
	raw, ok := fields[name]
	if !ok {
		return 0, fmt.Errorf("pagination field %q is required", name)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, fmt.Errorf("pagination field %q must be a number", name)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return value, nil
}

// requiredNullableIntField keeps from/to required while allowing JSON null for
// empty result windows.
func requiredNullableIntField(fields map[string]json.RawMessage, name string) (*int, error) {
	raw, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("pagination field %q is required", name)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

// requiredBoolField validates booleans separately so null cannot be mistaken
// for the default false value.
func requiredBoolField(fields map[string]json.RawMessage, name string) (bool, error) {
	raw, ok := fields[name]
	if !ok {
		return false, fmt.Errorf("pagination field %q is required", name)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, fmt.Errorf("pagination field %q must be a boolean", name)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	return value, nil
}

func paginationProtocolError(status int, meta ResponseMeta, err error) *ProtocolError {
	return &ProtocolError{Detail: "malformed pagination metadata", Status: status, RequestID: meta.RequestID, Err: err, Meta: meta}
}

func redactOptionalErr(redactErr func(error) error, err error) error {
	if err == nil || redactErr == nil {
		return err
	}
	return redactErr(err)
}

func cloneRawMeta(values map[string]json.RawMessage) map[string]json.RawMessage {
	if values == nil {
		return nil
	}
	cloned := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}

// sanitizeRawMeta returns a fresh map whose keys and JSON values have passed
// through the response redactor. Keys are processed in lexical order so
// collision suffixes are stable even though Go map iteration is not.
func sanitizeRawMeta(values map[string]json.RawMessage, transform func(string) string) (map[string]json.RawMessage, error) {
	if values == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	type entry struct {
		name  string
		value json.RawMessage
	}
	entries := make([]entry, 0, len(keys))
	used := make(map[string]struct{}, len(keys))
	// Pagination owns its public name. A different key that redacts to the same
	// name receives a suffix instead of displacing the structural field.
	if _, ok := values["pagination"]; ok {
		used["pagination"] = struct{}{}
	}
	for _, key := range keys {
		value, err := redact.TransformOrderedJSON(values[key], transform)
		if err != nil {
			return nil, fmt.Errorf("sanitize response metadata %q: %w", key, err)
		}
		if key == "pagination" {
			entries = append(entries, entry{name: "pagination", value: value})
			continue
		}
		base := transform(key)
		name := base
		for suffix := 2; ; suffix++ {
			if _, exists := used[name]; !exists {
				break
			}
			name = base + "~" + strconv.Itoa(suffix)
		}
		used[name] = struct{}{}
		entries = append(entries, entry{name: name, value: value})
	}

	sanitized := make(map[string]json.RawMessage, len(entries))
	for _, entry := range entries {
		sanitized[entry.name] = entry.value
	}
	return sanitized, nil
}

// rawMetadataValueNeedsRedaction checks whether any key or scalar token would
// change without using the transformed bytes. Callers can therefore keep the
// original validated value for execution while suppressing its public copy.
func rawMetadataValueNeedsRedaction(value json.RawMessage, transform func(string) string) (bool, error) {
	changed := false
	_, err := redact.TransformOrderedJSON(value, func(source string) string {
		transformed := transform(source)
		if transformed != source {
			changed = true
		}
		return transformed
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}
