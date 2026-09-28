package operationscmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

const (
	longPollTransportHeadroom = 5 * time.Second
	longPollRequestMax        = 30*time.Second - longPollTransportHeadroom
)

type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "operation command error"
	}
	return "operation command error: " + e.detail
}

func (e *usageError) ExitCode() int { return 1 }

type authContext struct {
	Runtime  config.Runtime
	Cred     auth.Credential
	Client   *api.Client
	Identity api.WhoamiData
}

func resolveAuthContext(cmd *cobra.Command, f *cmdutil.Factory) (authContext, error) {
	return resolveAPIClient(cmd, f, true)
}

func resolveAPIClient(cmd *cobra.Command, f *cmdutil.Factory, withIdentity bool) (authContext, error) {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return authContext{}, err
	}
	cred, findings, err := f.Credential(rt)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return authContext{}, fmt.Errorf("%w; run \"chab login\" or set CHAB_API_KEY", err)
	}
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return authContext{}, err
	}
	authn := authContext{Runtime: rt, Cred: cred, Client: client}
	if !withIdentity {
		return authn, nil
	}
	identity, _, err := client.Whoami(cmd.Context())
	if err != nil {
		return authContext{}, output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	authn.Identity = identity
	return authn, nil
}

func operationByKey(key string) (chabcontract.Operation, error) {
	registry := chabcontract.MustLoad()
	op, ok := registry.Find(key)
	if !ok {
		return chabcontract.Operation{}, &usageError{detail: "unknown operation key " + key}
	}
	return op, nil
}

func ensureRecordedActionContext(authn authContext, record operations.ActionRecord) error {
	if authn.Runtime.Profile != record.Profile || authn.Runtime.APIBaseURL != record.Destination || authn.Identity.TokenPublicID != record.TokenPublicID || authn.Identity.PrincipalID != record.PrincipalID {
		return &usageError{detail: "current credential context does not match the recorded action"}
	}
	return nil
}

func ensureStartable(op chabcontract.Operation) error {
	if !startableOperations[op.ID] {
		return &usageError{detail: "operation " + op.ID + " is not supported by the operation start entrypoint yet"}
	}
	if op.Method != http.MethodPost {
		return &usageError{detail: "operation " + op.ID + " is not a startable POST operation"}
	}
	return nil
}

func ensureEstimateSupported(operationKey string) error {
	if !estimateOperations[operationKey] {
		return &usageError{detail: "operation " + operationKey + " does not support generic estimates"}
	}
	return nil
}

func requireAcknowledgement(cmd *cobra.Command, f *cmdutil.Factory, op chabcontract.Operation, action string) error {
	if op.NotBillable && op.ID != "operations.cancel" && op.ID != "operations.bulk_cancel" {
		return nil
	}
	return cmdutil.ConfirmDestructive(f.Prompt(cmd), fmt.Sprintf("%s may spend credits or change operation state for %s. Continue?", action, op.ID))
}

func startOperation(cmd *cobra.Command, f *cmdutil.Factory, op chabcontract.Operation, requestBytes []byte, flags *requestFlags) (operations.CommandOutput, api.ResponseMeta, error) {
	if flags.OfflinePreview {
		preview := map[string]any{
			"operation_key": op.ID,
			"method":        op.Method,
			"path":          op.Path,
			"request":       json.RawMessage(requestBytes),
		}
		err := f.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: func(w io.Writer) {
				fmt.Fprintf(w, "Operation: %s\nPath: %s %s\nRequest: %s\n", op.ID, op.Method, op.Path, string(requestBytes))
			},
			Plain: func(data, prose io.Writer) {
				fmt.Fprintf(data, "%s\t%s\t%s\n", op.ID, op.Method, op.Path)
			},
		})
		return operations.CommandOutput{}, api.ResponseMeta{}, err
	}
	serverDryRun, err := operations.RequestDryRun(requestBytes)
	if err != nil {
		return operations.CommandOutput{}, api.ResponseMeta{}, &usageError{detail: err.Error()}
	}
	if serverDryRun {
		if !op.DryRunSupported {
			return operations.CommandOutput{}, api.ResponseMeta{}, &usageError{detail: "operation " + op.ID + " does not support server dry runs"}
		}
		if flags.IdempotencyKey != "" {
			return operations.CommandOutput{}, api.ResponseMeta{}, &usageError{detail: "--idempotency-key is not used for server dry runs"}
		}
		authn, err := resolveAuthContext(cmd, f)
		if err != nil {
			return operations.CommandOutput{}, api.ResponseMeta{}, err
		}
		result, err := authn.Client.DoRaw(cmd.Context(), op.Method, operationEndpointPath(op.Path), nil, api.ExactJSONBody(requestBytes), api.IdempotencyNone)
		if err != nil {
			return operations.CommandOutput{}, api.ResponseMeta{}, output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
		}
		return operations.CommandOutput{
			Server:    result.Data,
			RequestID: result.Meta.RequestID,
			Meta:      operationResponseMeta(result.Meta),
			LocalRecovery: operations.RecoveryStatus{
				State:     "server_dry_run",
				CanResume: false,
			},
			LocalPersistence: &operations.PersistenceState{State: "not_recorded"},
		}, result.Meta, nil
	}
	if err := requireAcknowledgement(cmd, f, op, "Starting this operation"); err != nil {
		return operations.CommandOutput{}, api.ResponseMeta{}, err
	}
	authn, err := resolveAuthContext(cmd, f)
	if err != nil {
		return operations.CommandOutput{}, api.ResponseMeta{}, err
	}
	key := flags.IdempotencyKey
	if key == "" {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return operations.CommandOutput{}, api.ResponseMeta{}, err
		}
	} else if err := api.ValidateIdempotencyKey(key); err != nil {
		return operations.CommandOutput{}, api.ResponseMeta{}, err
	}
	f.RegisterSecret(key)
	store := operations.StoreForRuntime(authn.Runtime, f.Clock())
	result, err := operations.Start(cmd.Context(), authn.Client, operations.StartInput{
		Operation:      op,
		RuntimeProfile: authn.Runtime.Profile,
		Destination:    authn.Runtime.APIBaseURL,
		TokenPublicID:  authn.Identity.TokenPublicID,
		PrincipalID:    authn.Identity.PrincipalID,
		RequestBytes:   requestBytes,
		IdempotencyKey: key,
		Journal:        store,
	})
	if err != nil {
		return result, api.ResponseMeta{}, output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	if flags.Wait && result.OperationID != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "accepted operation %s; resume with chab operations wait %s\n", result.OperationID, result.OperationID)
		waitRaw, payload, meta, err := waitUntilTerminal(cmd, f, authn.Client, result.OperationID, 10*time.Minute)
		if err != nil {
			result = mergeWaitOutput(result, waitRaw, payload, meta)
			if reason, ok := waitRecoveryReason(err); ok {
				result.LocalRecovery.State = reason
				err = waitRecoveryErrorForReason(result.OperationID, reason, err)
			}
			return result, meta, output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
		}
		result = mergeWaitOutput(result, waitRaw, payload, meta)
		if payload.ID != "" {
			result.OperationID = payload.ID
		}
		if result.Action != nil {
			updated, persistErr := store.MarkCompleted(result.Action.ID, payload, meta)
			if persistErr != nil {
				result.LocalPersistence = &operations.PersistenceState{State: "failed", Detail: persistErr.Error()}
				return result, meta, output.WithCredentialContext(
					fmt.Errorf("operation %s reached terminal status %s but failed to persist local action state: %w", result.OperationID, payload.Status, persistErr),
					authn.Cred.Profile,
					authn.Cred.DisplayID,
				)
			}
			projection := operations.Projection(updated)
			result.Action = &projection
			result.LocalRecovery.State = string(updated.State)
			result.LocalPersistence = &operations.PersistenceState{State: "persisted"}
		} else {
			result.LocalRecovery.State = payload.Status
		}
		if payload.Status != "succeeded" {
			return result, meta, &usageError{detail: fmt.Sprintf("operation %s finished with status %s", result.OperationID, payload.Status)}
		}
		return result, meta, nil
	}
	return result, api.ResponseMeta{}, nil
}

func operationEndpointPath(openAPIPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(openAPIPath, "/v1/"), "/")
}

func writeCommandOutput(cmd *cobra.Command, f *cmdutil.Factory, value operations.CommandOutput) error {
	return f.WriteResult(cmd, value, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			if value.Action != nil {
				fmt.Fprintf(w, "Action ID: %s\n", value.Action.ID)
			}
			if value.OperationID != "" {
				fmt.Fprintf(w, "Operation ID: %s\n", value.OperationID)
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
			if value.Meta != nil {
				if value.Meta.RateLimit != nil && value.Meta.RateLimit.Remaining != nil {
					fmt.Fprintf(w, "Rate limit remaining: %d\n", *value.Meta.RateLimit.Remaining)
				}
				if value.Meta.APIMeta != nil {
					if credits := value.Meta.APIMeta["credits"]; len(credits) > 0 {
						fmt.Fprintf(w, "Credits: %s\n", string(credits))
					}
				}
			}
			if len(value.Server) > 0 {
				fmt.Fprintf(w, "Server: %s\n", string(value.Server))
			}
			if value.Warning != "" {
				fmt.Fprintf(w, "Warning: %s\n", value.Warning)
			}
		},
		Plain: func(data, prose io.Writer) {
			if value.OperationID != "" {
				fmt.Fprintln(data, value.OperationID)
			} else if value.Action != nil {
				fmt.Fprintln(data, value.Action.ID)
			}
			if value.LocalRecovery.ResumeHint != "" {
				fmt.Fprintln(prose, value.LocalRecovery.ResumeHint)
			}
		},
	})
}

func hasCommandOutput(value operations.CommandOutput) bool {
	return value.Action != nil ||
		value.OperationID != "" ||
		len(value.Server) > 0 ||
		value.Warning != "" ||
		value.RequestID != "" ||
		value.Meta != nil ||
		value.LocalPersistence != nil ||
		value.LocalRecovery.State != "" ||
		value.LocalRecovery.ResumeHint != "" ||
		value.LocalRecovery.CanResume ||
		value.LocalRecovery.KnownRemote
}

func writeRawJSONValue(cmd *cobra.Command, f *cmdutil.Factory, value json.RawMessage, meta api.ResponseMeta) error {
	return f.WriteResultWithMeta(cmd, value, meta, true, cmdutil.HumanOutput{
		Render: func(w io.Writer) { output.RawValueHuman(w, value) },
		Plain:  func(data, prose io.Writer) { output.RawValuePlain(data, prose, value) },
	})
}

func operationResponseMeta(meta api.ResponseMeta) *output.MetaView {
	view := output.ViewMeta(meta, output.MetaOptions{IncludeAPIMeta: true})
	if view.Empty() {
		return nil
	}
	return &view
}

type waitRecoveryError struct {
	reason string
	err    error
	msg    string
}

func (e *waitRecoveryError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

func (e *waitRecoveryError) Unwrap() error {
	return nil
}

func waitTimeoutError(operationID string) error {
	return &waitRecoveryError{
		reason: "timeout",
		msg:    fmt.Sprintf("timed out waiting for operation %s; resume with chab operations wait %s", operationID, operationID),
	}
}

func waitInterruptedError(operationID string, err error) error {
	return &waitRecoveryError{
		reason: "interrupted",
		err:    err,
		msg:    fmt.Sprintf("stopped waiting for operation %s; resume with chab operations wait %s: %v", operationID, operationID, err),
	}
}

func waitRecoveryReason(err error) (string, bool) {
	var recoveryErr *waitRecoveryError
	if errors.As(err, &recoveryErr) && recoveryErr.reason != "" {
		return recoveryErr.reason, true
	}
	switch {
	case errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled"):
		return "interrupted", true
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") || strings.HasPrefix(err.Error(), "timed out waiting for operation "):
		return "timeout", true
	default:
		return "", false
	}
}

func waitRecoveryErrorForReason(operationID, reason string, err error) error {
	var recoveryErr *waitRecoveryError
	if errors.As(err, &recoveryErr) {
		return err
	}
	if reason == "timeout" {
		return waitTimeoutError(operationID)
	}
	return waitInterruptedError(operationID, err)
}

func waitUntilTerminal(cmd *cobra.Command, f *cmdutil.Factory, client operations.Client, operationID string, timeout time.Duration) (json.RawMessage, operations.OperationPayload, api.ResponseMeta, error) {
	deadline := f.Clock()().Add(timeout)
	for {
		remaining := deadline.Sub(f.Clock()())
		if remaining <= 0 {
			return nil, operations.OperationPayload{}, api.ResponseMeta{}, waitTimeoutError(operationID)
		}
		wait := pollWaitDuration(remaining)
		requestCtx, cancel := context.WithTimeout(cmd.Context(), remaining)
		raw, payload, meta, err := operations.Status(requestCtx, client, operationID, wait)
		cancel()
		if err != nil {
			if errors.Is(cmd.Context().Err(), context.Canceled) {
				return nil, operations.OperationPayload{}, meta, waitInterruptedError(operationID, err)
			}
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				return nil, operations.OperationPayload{}, meta, waitTimeoutError(operationID)
			}
			if errors.Is(requestCtx.Err(), context.Canceled) || errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") {
				return nil, operations.OperationPayload{}, meta, waitInterruptedError(operationID, err)
			}
			return nil, operations.OperationPayload{}, meta, err
		}
		if err := operations.ValidateStatus(payload.Status, operationID); err != nil {
			return nil, payload, meta, err
		}
		if !operations.ActiveStatus(payload.Status) {
			return raw, payload, meta, nil
		}
		sleep := 2 * time.Second
		if meta.RetryAfter.Wait != nil {
			sleep = *meta.RetryAfter.Wait
		}
		remaining = deadline.Sub(f.Clock()())
		if sleep > remaining {
			sleep = remaining
		}
		if sleep <= 0 {
			continue
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "operation %s is %s; waiting %s\n", operationID, payload.Status, sleep)
		if err := f.Sleep(cmd.Context(), sleep); err != nil {
			return raw, payload, meta, waitInterruptedError(operationID, err)
		}
	}
}

func mergeWaitOutput(out operations.CommandOutput, raw json.RawMessage, payload operations.OperationPayload, meta api.ResponseMeta) operations.CommandOutput {
	if payload.ID != "" {
		out.OperationID = payload.ID
	}
	if len(raw) > 0 {
		out.Server = raw
	}
	if meta.RequestID != "" {
		out.RequestID = meta.RequestID
		out.Meta = operationResponseMeta(meta)
	}
	out.LocalRecovery.KnownRemote = out.OperationID != ""
	if out.OperationID != "" && operations.ActiveStatus(payload.Status) {
		out.LocalRecovery.CanResume = true
		out.LocalRecovery.ResumeHint = "chab operations wait " + out.OperationID
	}
	return out
}

func waitRecoveryOutput(operationID string, raw json.RawMessage, payload operations.OperationPayload, meta api.ResponseMeta, reason string) operations.CommandOutput {
	if payload.ID != "" {
		operationID = payload.ID
	}
	out := operations.CommandOutput{
		OperationID: operationID,
		Server:      raw,
		RequestID:   meta.RequestID,
		Meta:        operationResponseMeta(meta),
		LocalRecovery: operations.RecoveryStatus{
			State:       reason,
			CanResume:   operationID != "",
			KnownRemote: operationID != "",
		},
	}
	if operationID != "" {
		out.LocalRecovery.ResumeHint = "chab operations wait " + operationID
	}
	return out
}

func pollWaitDuration(remaining time.Duration) time.Duration {
	if remaining <= longPollTransportHeadroom {
		return 0
	}
	wait := remaining - longPollTransportHeadroom
	if wait > longPollRequestMax {
		wait = longPollRequestMax
	}
	if wait < 0 {
		return 0
	}
	return wait
}

var startableOperations = map[string]bool{
	"examples.echo":              true,
	"search.web":                 true,
	"search.serp":                true,
	"seo.keywords.ideas":         true,
	"seo.keywords.metrics":       true,
	"seo.domains.overview":       true,
	"seo.domains.backlinks":      true,
	"business.search":            true,
	"business.details":           true,
	"contacts.domain_search":     true,
	"contacts.email_finder":      true,
	"contacts.email_verify":      true,
	"scrape.markdown":            true,
	"scrape.dom":                 true,
	"screenshots.url":            true,
	"convert.file":               true,
	"research.deep":              true,
	"translate.text_or_document": true,
	"llm.generate":               true,
	"llm.embeddings":             true,
}

var estimateOperations = map[string]bool{
	"search.web":                 true,
	"search.serp":                true,
	"seo.keywords.ideas":         true,
	"seo.keywords.metrics":       true,
	"seo.domains.overview":       true,
	"seo.domains.backlinks":      true,
	"business.search":            true,
	"business.details":           true,
	"contacts.domain_search":     true,
	"contacts.email_finder":      true,
	"contacts.email_verify":      true,
	"scrape.markdown":            true,
	"scrape.dom":                 true,
	"screenshots.url":            true,
	"convert.file":               true,
	"translate.text_or_document": true,
	"llm.generate":               true,
	"llm.embeddings":             true,
	"research.deep":              true,
}
