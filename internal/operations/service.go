package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/output"
)

type Client interface {
	Get(ctx context.Context, path string, query url.Values, out any) (api.ResponseMeta, error)
	Post(ctx context.Context, path string, query url.Values, body any, idem api.Idempotency, out any) (api.ResponseMeta, error)
	DoRaw(ctx context.Context, method, path string, query url.Values, body any, idem api.Idempotency) (api.RawResult, error)
	DoRawWithHeaders(ctx context.Context, method, path string, query url.Values, body any, idem api.Idempotency, header http.Header) (api.RawResult, error)
	Whoami(ctx context.Context) (api.WhoamiData, api.ResponseMeta, error)
}

type OperationPayload struct {
	ID              string          `json:"id"`
	OperationKey    string          `json:"operation_key"`
	Family          string          `json:"family"`
	Status          string          `json:"status"`
	ResultAvailable *bool           `json:"result_available,omitempty"`
	Raw             json.RawMessage `json:"-"`
}

type CommandOutput struct {
	Action           *ActionProjection `json:"action,omitempty"`
	OperationID      string            `json:"operation_id,omitempty"`
	Server           json.RawMessage   `json:"server,omitempty"`
	LocalRecovery    RecoveryStatus    `json:"local_recovery"`
	LocalPersistence *PersistenceState `json:"local_persistence,omitempty"`
	Meta             *output.MetaView  `json:"meta,omitempty"`
	Warning          string            `json:"warning,omitempty"`
	RequestID        string            `json:"request_id,omitempty"`
}

type RecoveryStatus struct {
	State       string `json:"state"`
	CanResume   bool   `json:"can_resume"`
	ResumeHint  string `json:"resume_hint,omitempty"`
	KnownRemote bool   `json:"known_remote"`
}

type PersistenceState struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type StartInput struct {
	Operation       chabcontract.Operation
	RuntimeProfile  string
	Destination     string
	TokenPublicID   string
	PrincipalID     string
	EncoderVersion  string
	RequestBytes    []byte
	IdempotencyKey  string
	Headers         http.Header
	Journal         Store
	UseExistingOnly bool
}

func Estimate(ctx context.Context, client Client, operationKey string, requestBytes []byte) (json.RawMessage, api.ResponseMeta, error) {
	body := json.RawMessage(fmt.Sprintf(`{"input":%s,"operation_key":%q}`, requestBytes, operationKey))
	result, err := client.DoRaw(ctx, "POST", "operations/estimate", nil, api.ExactJSONBody(body), api.IdempotencyNone)
	if err != nil {
		return nil, api.ResponseMeta{}, err
	}
	return result.Data, result.Meta, nil
}

func Start(ctx context.Context, client Client, input StartInput) (CommandOutput, error) {
	path := apiPath(input.Operation.Path)
	prepared, err := input.Journal.Prepare(PrepareInput{
		Profile:        input.RuntimeProfile,
		Destination:    input.Destination,
		TokenPublicID:  input.TokenPublicID,
		PrincipalID:    input.PrincipalID,
		RequiredScope:  input.Operation.RequiredScope,
		OperationKey:   input.Operation.ID,
		Method:         input.Operation.Method,
		Path:           input.Operation.Path,
		EncoderVersion: input.EncoderVersion,
		RequestBytes:   input.RequestBytes,
		IdempotencyKey: input.IdempotencyKey,
	})
	if err != nil {
		return CommandOutput{}, err
	}
	projection := Projection(prepared.Record)
	if prepared.AlreadyCompleted {
		return CommandOutput{
			Action:      &projection,
			OperationID: prepared.Record.AcceptedOperationID,
			RequestID:   prepared.Record.RequestID,
			LocalRecovery: RecoveryStatus{
				State:       string(prepared.Record.State),
				CanResume:   false,
				KnownRemote: prepared.Record.AcceptedOperationID != "",
			},
			LocalPersistence: &PersistenceState{State: "persisted"},
		}, nil
	}
	if prepared.AlreadyAccepted {
		return CommandOutput{
			Action:      &projection,
			OperationID: prepared.Record.AcceptedOperationID,
			LocalRecovery: RecoveryStatus{
				State:       string(prepared.Record.State),
				CanResume:   true,
				ResumeHint:  "chab operations wait " + prepared.Record.AcceptedOperationID,
				KnownRemote: true,
			},
		}, nil
	}
	if input.UseExistingOnly {
		return CommandOutput{}, fmt.Errorf("action %s has no accepted operation id to resume", prepared.Record.ID)
	}
	if err := input.Journal.ReplayablePrepared(prepared); err != nil {
		return CommandOutput{}, err
	}

	var body any
	if input.RequestBytes != nil {
		body = api.ExactJSONBody(input.RequestBytes)
	}
	var result api.RawResult
	if len(input.Headers) > 0 {
		result, err = client.DoRawWithHeaders(ctx, input.Operation.Method, path, nil, body, api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing), input.Headers)
	} else {
		result, err = client.DoRaw(ctx, input.Operation.Method, path, nil, body, api.JournaledIdempotency(prepared.Record.IdempotencyKey, prepared.Existing))
	}
	if err != nil {
		if DefinitiveAdmissionDenialForSubmission(err, prepared.Existing) {
			if markErr := input.Journal.MarkDenied(prepared.Record.ID, err); markErr != nil {
				return CommandOutput{Action: &projection, LocalRecovery: RecoveryStatus{State: string(ActionPrepared), CanResume: false}, LocalPersistence: &PersistenceState{State: "failed", Detail: markErr.Error()}}, fmt.Errorf("API denied action and local denial could not be persisted: %w (persistence: %v)", err, markErr)
			}
			if updated, loadErr := input.Journal.Load(prepared.Record.ID); loadErr == nil {
				projection = Projection(updated)
			}
			return CommandOutput{Action: &projection, LocalRecovery: RecoveryStatus{State: string(ActionDenied), CanResume: false}}, err
		}
		if markErr := input.Journal.MarkUnknown(prepared.Record.ID, err); markErr != nil {
			return CommandOutput{Action: &projection, LocalRecovery: RecoveryStatus{State: string(ActionPrepared), CanResume: false}, LocalPersistence: &PersistenceState{State: "failed", Detail: markErr.Error()}}, fmt.Errorf("API outcome uncertain and local recovery could not be persisted: %w (persistence: %v)", err, markErr)
		}
		if updated, loadErr := input.Journal.Load(prepared.Record.ID); loadErr == nil {
			projection = Projection(updated)
		}
		return CommandOutput{
			Action: &projection,
			LocalRecovery: RecoveryStatus{
				State:      string(ActionUnknown),
				CanResume:  true,
				ResumeHint: "chab operations resume " + prepared.Record.ID + " --input @request.json",
			},
		}, err
	}
	payload := decodeOperationPayload(result.Data)
	statusErr := ValidateStartPayload(input.Operation, payload, result.Meta)
	if statusErr != nil && payload.ID == "" {
		_ = input.Journal.MarkUnknown(prepared.Record.ID, statusErr)
		return CommandOutput{}, statusErr
	}
	var updated ActionRecord
	continues := ResponseContinuesByID(input.Operation, payload)
	if statusErr != nil {
		updated, err = input.Journal.MarkAccepted(prepared.Record.ID, payload, result.Meta)
	} else if !continues || isTerminal(payload.Status) {
		updated, err = input.Journal.MarkCompleted(prepared.Record.ID, payload, result.Meta)
	} else {
		updated, err = input.Journal.MarkAccepted(prepared.Record.ID, payload, result.Meta)
	}
	if updated.ID == "" {
		updated = recordWithPayload(prepared.Record, payload, result.Meta, actionStateForPayload(continues, payload))
	}
	outProjection := Projection(updated)
	output := CommandOutput{
		Action:      &outProjection,
		OperationID: payload.ID,
		Server:      result.Data,
		RequestID:   result.Meta.RequestID,
		Meta:        responseMetaView(result.Meta),
		LocalRecovery: RecoveryStatus{
			State:       string(updated.State),
			CanResume:   continues,
			KnownRemote: payload.ID != "",
		},
	}
	if continues {
		output.LocalRecovery.ResumeHint = "chab operations wait " + payload.ID
	}
	if err != nil {
		output.LocalPersistence = &PersistenceState{State: "failed", Detail: err.Error()}
		return output, fmt.Errorf("accepted operation %s but failed to persist local action state: %w", payload.ID, err)
	}
	output.LocalPersistence = &PersistenceState{State: "persisted"}
	if statusErr != nil {
		return output, statusErr
	}
	return output, nil
}

// DefinitiveAdmissionDenial distinguishes a terminal API rejection from an
// admission response that could be racing or preceding the idempotency claim.
// Keep this shared across typed, raw, file and MCP action journals.
func DefinitiveAdmissionDenial(err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		// A released/settled marker is authoritative even when the backend's
		// error body is empty or malformed and decoding yields ProtocolError.
		var protocolErr *api.ProtocolError
		return errors.As(err, &protocolErr) && protocolErr.Status >= 300 && protocolErr.Meta.IdempotencyOutcome != ""
	}
	if apiErr.Status >= 300 && apiErr.Meta.IdempotencyOutcome != "" {
		return true
	}
	if apiErr.Status < 400 || apiErr.Status >= 500 {
		return false
	}
	return apiErr.Status != http.StatusRequestTimeout &&
		apiErr.Code != "idempotency_request_in_progress" &&
		(apiErr.Status != http.StatusTooManyRequests || apiErr.Code != "rate_limited")
}

// DefinitiveAdmissionDenialForSubmission is stricter when a prior submission
// is already unknown. A replay can receive a new, unmarked pre-claim denial
// even though the original submission committed work. Only "released" proves
// this idempotency claim was removed without an accepted effect; "settled"
// merely says a response was persisted and cannot clear prior uncertainty.
func DefinitiveAdmissionDenialForSubmission(err error, priorUnknown bool) bool {
	if !priorUnknown {
		return DefinitiveAdmissionDenial(err)
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 300 && apiErr.Meta.IdempotencyOutcome == "released"
	}
	var protocolErr *api.ProtocolError
	return errors.As(err, &protocolErr) && protocolErr.Status >= 300 && protocolErr.Meta.IdempotencyOutcome == "released"
}

func Status(ctx context.Context, client Client, operationID string, wait time.Duration) (json.RawMessage, OperationPayload, api.ResponseMeta, error) {
	q := url.Values{}
	if wait > 0 {
		seconds := int(wait.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		if seconds > 30 {
			seconds = 30
		}
		q.Set("wait", fmt.Sprintf("%ds", seconds))
	}
	result, err := client.DoRaw(ctx, "GET", api.Path("operations", operationID), q, nil, api.IdempotencyNone)
	if err != nil {
		return nil, OperationPayload{}, api.ResponseMeta{}, err
	}
	payload := decodeOperationPayload(result.Data)
	return result.Data, payload, result.Meta, nil
}

func Result(ctx context.Context, client Client, operationID string) (json.RawMessage, api.ResponseMeta, error) {
	result, err := client.DoRaw(ctx, "GET", api.Path("operations", operationID, "result"), nil, nil, api.IdempotencyNone)
	if err != nil {
		return nil, api.ResponseMeta{}, err
	}
	return result.Data, result.Meta, nil
}

func Artifact(ctx context.Context, client Client, operationID, artifactID string) (json.RawMessage, api.ResponseMeta, error) {
	result, err := client.DoRaw(ctx, "GET", api.Path("operations", operationID, "artifacts", artifactID), nil, nil, api.IdempotencyNone)
	if err != nil {
		return nil, api.ResponseMeta{}, err
	}
	return result.Data, result.Meta, nil
}

func Cancel(ctx context.Context, client Client, input StartInput) (CommandOutput, error) {
	return Start(ctx, client, input)
}

func decodeOperationPayload(raw json.RawMessage) OperationPayload {
	var payload OperationPayload
	_ = json.Unmarshal(raw, &payload)
	payload.Raw = append(json.RawMessage(nil), raw...)
	return payload
}

func isTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "cancelled", "timed_out", "expired":
		return true
	default:
		return false
	}
}

func ActiveStatus(status string) bool {
	switch status {
	case "queued", "running", "cancelling":
		return true
	default:
		return false
	}
}

func ValidateStatus(status, operationID string) error {
	switch status {
	case "queued", "running", "cancelling", "succeeded", "failed", "cancelled", "timed_out", "expired":
		return nil
	case "":
		return fmt.Errorf("operation %s response missing status", operationID)
	default:
		return fmt.Errorf("operation %s returned unsupported status %q", operationID, status)
	}
}

func apiPath(openAPIPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(openAPIPath, "/v1/"), "/")
}

// ValidateStartPayload checks whether a submitted operation response carries a
// valid lifecycle payload. Synchronous data responses are allowed to omit it.
func ValidateStartPayload(op chabcontract.Operation, payload OperationPayload, meta api.ResponseMeta) error {
	if !responseUsesOperationLifecycle(op, payload) {
		return nil
	}
	if payload.ID == "" {
		return &api.ProtocolError{Detail: "accepted operation response missing operation id", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	if err := ValidateStatus(payload.Status, payload.ID); err != nil {
		return &api.ProtocolError{Detail: err.Error(), Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return nil
}

// ResponseContinuesByID reports whether the response should be followed through
// the operation status endpoint instead of treated as a completed synchronous
// receipt.
func ResponseContinuesByID(op chabcontract.Operation, payload OperationPayload) bool {
	return payload.ID != "" && ActiveStatus(payload.Status) && responseUsesOperationLifecycle(op, payload)
}

func responseUsesOperationLifecycle(op chabcontract.Operation, payload OperationPayload) bool {
	return op.Behavior == "async" ||
		op.ID == "operations.cancel" ||
		payload.OperationKey != "" ||
		payload.Family != "" ||
		(payload.ID != "" && ActiveStatus(payload.Status))
}

func actionStateForPayload(continues bool, payload OperationPayload) ActionState {
	if continues && !isTerminal(payload.Status) {
		return ActionAccepted
	}
	return ActionCompleted
}

func recordWithPayload(record ActionRecord, payload OperationPayload, meta api.ResponseMeta, state ActionState) ActionRecord {
	record.State = state
	if payload.ID != "" {
		record.AcceptedOperationID = payload.ID
	}
	record.RequestID = meta.RequestID
	record.LastStatus = payload.Status
	return record
}

func responseMetaView(meta api.ResponseMeta) *output.MetaView {
	view := output.ViewMeta(meta, output.MetaOptions{IncludeAPIMeta: true})
	if view.Empty() {
		return nil
	}
	return &view
}
