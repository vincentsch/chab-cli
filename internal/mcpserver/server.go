package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
	"github.com/vincentsch/chab-cli/internal/redact"
)

const (
	ProtocolVersion = "2026-07-28"
	cacheTTLMS      = 3600000
)

// Options configures the local stdio MCP server.
type Options struct {
	Factory *cmdutil.Factory
	Command *cobra.Command
	Version string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// Serve runs the local MCP server over newline-delimited JSON on stdio.
func Serve(ctx context.Context, opts Options) error {
	if opts.Stdin == nil && opts.Factory != nil {
		opts.Stdin = opts.Factory.Stdin
	}
	if opts.Stdout == nil && opts.Command != nil {
		opts.Stdout = opts.Command.OutOrStdout()
	}
	if opts.Stderr == nil && opts.Command != nil {
		opts.Stderr = opts.Command.ErrOrStderr()
	}
	if opts.Stdin == nil {
		opts.Stdin = strings.NewReader("")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	err := NewServer(opts).Run(ctx, versionedTransport{Transport: &mcp.IOTransport{
		Reader: readCloser{Reader: opts.Stdin},
		Writer: writeCloser{Writer: opts.Stdout},
	}})
	if err == nil || errors.Is(err, io.EOF) || strings.Contains(err.Error(), "server is closing: EOF") {
		return nil
	}
	return err
}

// NewServer builds a server instance. It is exported for focused protocol tests.
func NewServer(opts Options) *mcp.Server {
	version := opts.Version
	if version == "" && opts.Factory != nil {
		version = opts.Factory.VersionString()
	}
	if version == "" {
		version = "dev"
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "chab", Version: version}, &mcp.ServerOptions{
		Instructions: instructions(),
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		PageSize:     1000,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server.AddReceivingMiddleware(cacheAndCursorMiddleware)
	server.AddReceivingMiddleware((registry{opts: opts}).guestCredentialMiddleware)
	registry{opts: opts}.register(server)
	return server
}

type registry struct {
	opts Options
}

func (r registry) register(server *mcp.Server) {
	for _, spec := range toolSpecs(r) {
		server.AddTool(spec.tool, spec.handler)
	}
}

type toolSpec struct {
	tool    *mcp.Tool
	handler mcp.ToolHandler
}

func toolSpecs(r registry) []toolSpec {
	authEnv := r.emptyTool("chab_auth_env", "Report the effective local non-secret authentication environment.", "Authentication environment", authEnvOutputSchema(), r.callAuthEnv)
	localOnly := false
	authEnv.tool.Annotations.OpenWorldHint = &localOnly
	actionList := r.emptyTool("chab_action_list", "List local Chab action receipts without exposing idempotency keys.", "Local actions", arraySchema(actionProjectionSchema()), r.callActionList)
	actionList.tool.Annotations.OpenWorldHint = &localOnly
	actionShow := r.toolWithInput("chab_action_show", "Show one local Chab action receipt without exposing idempotency keys.", "Local action", actionShowInputSchema(), actionProjectionSchema(), r.callActionShow)
	actionShow.tool.Annotations.OpenWorldHint = &localOnly
	actionResume := r.toolWithInput("chab_action_resume", "Resume one local Chab action using the stored idempotency key and matching input.", "Resume local action", actionResumeInputSchema(), commandOutputSchema(), r.callActionResume)
	actionResumeReadOnly := false
	actionResumeDestructive := true
	actionResume.tool.Annotations.ReadOnlyHint = actionResumeReadOnly
	actionResume.tool.Annotations.DestructiveHint = &actionResumeDestructive
	specs := []toolSpec{
		authEnv,
		r.emptyTool("chab_health", "Show public API health.", "Public API health", healthSchema(), r.callHealth),
		r.emptyTool("chab_errors", "List public API error codes.", "Public API errors", errorCatalogSchema(), r.callErrors),
		actionList,
		actionShow,
		actionResume,
	}
	registry, err := chabcontract.Load()
	if err != nil {
		return specs
	}
	for _, op := range registry.Operations() {
		if mcpExcludedOperation(op.ID) {
			continue
		}
		switch op.ID {
		case "system.health", "errors.list":
			continue
		}
		specs = append(specs, r.operationTool(op))
	}
	return specs
}

// ToolNames returns the registered local MCP tool names for generated
// operation-map freshness checks.
func ToolNames() []string {
	specs := toolSpecs(registry{})
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.tool.Name)
	}
	sort.Strings(names)
	return names
}

func (r registry) emptyTool(name, description, title string, outputSchema any, fn func(context.Context) (any, error)) toolSpec {
	return r.toolWithInput(name, description, title, emptyInputSchema(), outputSchema, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireEmptyArguments(req.Params.Arguments); err != nil {
			return nil, invalidParams(err)
		}
		value, err := fn(ctx)
		if err != nil {
			return toolError(r.opts, err), nil
		}
		return toolSuccess(value)
	})
}

func (r registry) toolWithInput(name, description, title string, inputSchema, outputSchema any, handler mcp.ToolHandler) toolSpec {
	readOnly := true
	openWorld := true
	destructive := false
	return toolSpec{
		tool: &mcp.Tool{
			Name:         name,
			Title:        title,
			Description:  description,
			InputSchema:  inputSchema,
			OutputSchema: oneOf(outputSchema, toolErrorSchema()),
			Annotations: &mcp.ToolAnnotations{
				Title:           title,
				ReadOnlyHint:    readOnly,
				DestructiveHint: &destructive,
				OpenWorldHint:   &openWorld,
			},
		},
		handler: handler,
	}
}

func (r registry) operationTool(op chabcontract.Operation) toolSpec {
	name := operationToolName(op.ID)
	title := strings.ReplaceAll(strings.TrimPrefix(name, "chab_"), "_", " ")
	description := operationToolDescription(op)
	readOnly := operationReadOnly(op)
	destructive := operationDestructive(op)
	openWorld := true
	return toolSpec{
		tool: &mcp.Tool{
			Name:         name,
			Title:        title,
			Description:  description,
			InputSchema:  operationInputSchema(op),
			OutputSchema: operationToolOutputSchema(op),
			Annotations: &mcp.ToolAnnotations{
				Title:           title,
				ReadOnlyHint:    readOnly,
				DestructiveHint: &destructive,
				OpenWorldHint:   &openWorld,
			},
		},
		handler: func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return r.callOperation(ctx, req, op)
		},
	}
}

func (r registry) callWhoami(ctx context.Context) (any, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return nil, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	client, cred, err := apiClient(f, cmd, rt)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	value, _, err := readservice.GetMCPWhoami(ctx, client)
	if err != nil {
		return nil, withToolFactory(f, output.WithCredentialContext(err, cred.Profile, cred.DisplayID))
	}
	return value, nil
}

func (r registry) callAuthEnv(ctx context.Context) (any, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return nil, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	select {
	case <-ctx.Done():
		return nil, withToolFactory(f, ctx.Err())
	default:
	}
	return readservice.AuthEnv(rt), nil
}

func (r registry) callHealth(ctx context.Context) (any, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return nil, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	client, err := f.PublicAPIClient(rt, cmd)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	var value map[string]any
	if _, err := client.Get(ctx, "health", nil, &value); err != nil {
		return nil, withToolFactory(f, err)
	}
	return value, nil
}

func (r registry) callErrors(ctx context.Context) (any, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return nil, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	client, err := f.PublicAPIClient(rt, cmd)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	var value map[string]any
	if _, err := client.Get(ctx, "errors", nil, &value); err != nil {
		return nil, withToolFactory(f, err)
	}
	return value, nil
}

func (r registry) callProjectList(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := decodeProjectListInput(req.Params.Arguments)
	if err != nil {
		return nil, invalidParams(err)
	}
	opts := input.toOptions()
	query, err := readservice.ProjectListQuery(opts)
	if err != nil {
		return nil, invalidParams(err)
	}
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return toolError(r.opts, err), nil
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	plan, err := readservice.ProjectListPlan(rt, opts)
	if err != nil {
		return nil, invalidParams(err)
	}
	client, cred, err := apiClient(f, cmd, rt)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	value, err := readservice.FetchProjectList(ctx, client, plan, query)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, output.WithCredentialContext(err, cred.Profile, cred.DisplayID)), nil
	}
	return toolSuccess(value.Rows)
}

func (r registry) callProjectShow(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := decodeProjectShowInput(req.Params.Arguments)
	if err != nil {
		return nil, invalidParams(err)
	}
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return toolError(r.opts, err), nil
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	client, cred, err := apiClient(f, cmd, rt)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	value, _, err := readservice.GetProject(ctx, client, input.ProjectID)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, output.WithCredentialContext(err, cred.Profile, cred.DisplayID)), nil
	}
	return toolSuccess(value)
}

func (r registry) callCreditsBalance(ctx context.Context) (any, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return nil, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	client, cred, err := apiClient(f, cmd, rt)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	value, _, err := readservice.GetBalance(ctx, client)
	if err != nil {
		return nil, withToolFactory(f, output.WithCredentialContext(err, cred.Profile, cred.DisplayID))
	}
	return value, nil
}

func (r registry) callCreditsTransactions(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := decodeTransactionsInput(req.Params.Arguments)
	if err != nil {
		return nil, invalidParams(err)
	}
	opts := input.toOptions()
	query, err := readservice.TransactionQuery(opts)
	if err != nil {
		return nil, invalidParams(err)
	}
	plan, err := readservice.TransactionListPlan(opts)
	if err != nil {
		return nil, invalidParams(err)
	}
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return toolError(r.opts, err), nil
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	client, cred, err := apiClient(f, cmd, rt)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	value, err := readservice.FetchTransactions(ctx, client, plan, query)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, output.WithCredentialContext(err, cred.Profile, cred.DisplayID)), nil
	}
	return toolSuccess(value.Rows)
}

func (r registry) callActionList(ctx context.Context) (any, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return nil, err
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	select {
	case <-ctx.Done():
		return nil, withToolFactory(f, ctx.Err())
	default:
	}
	records, err := operations.StoreForRuntime(rt, f.Clock()).List()
	if err != nil {
		return nil, withToolFactory(f, err)
	}
	cred, _, err := f.Credential(rt)
	if err == nil && cred.PrincipalType == "guest_trial" {
		client, clientErr := f.APIClient(rt, cred, cmd)
		if clientErr != nil {
			return nil, withToolFactory(f, clientErr)
		}
		identity, _, identityErr := client.Whoami(ctx)
		if identityErr != nil {
			return nil, withToolFactory(f, identityErr)
		}
		filtered := records[:0]
		for _, record := range records {
			if record.Profile == rt.Profile && record.Destination == rt.APIBaseURL && record.PrincipalID != "" && record.PrincipalID == identity.PrincipalID {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	} else {
		// Guest receipts require a live matching guest principal even though
		// ordinary local action metadata may be listed without a credential.
		filtered := records[:0]
		for _, record := range records {
			if record.PrincipalID == "" {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}
	return operations.Projections(records), nil
}

func (r registry) callActionShow(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := decodeActionIDInput(req.Params.Arguments)
	if err != nil {
		return nil, invalidParams(err)
	}
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return toolError(r.opts, err), nil
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	select {
	case <-ctx.Done():
		return toolErrorWithFactory(r.opts, f, ctx.Err()), nil
	default:
	}
	record, err := operations.StoreForRuntime(rt, f.Clock()).Load(input)
	if err != nil {
		return toolErrorWithFactory(r.opts, f, err), nil
	}
	cred, _, err := f.Credential(rt)
	if record.PrincipalID != "" && (err != nil || cred.PrincipalType != "guest_trial") {
		return toolError(r.opts, mcpUsageError("guest action requires its matching credential")), nil
	}
	if err == nil && cred.PrincipalType == "guest_trial" {
		client, clientErr := f.APIClient(rt, cred, cmd)
		if clientErr != nil {
			return toolErrorWithFactory(r.opts, f, clientErr), nil
		}
		identity, _, identityErr := client.Whoami(ctx)
		if identityErr != nil {
			return toolErrorWithFactory(r.opts, f, identityErr), nil
		}
		if record.Profile != rt.Profile || record.Destination != rt.APIBaseURL || record.PrincipalID == "" || record.PrincipalID != identity.PrincipalID {
			return toolError(r.opts, mcpUsageError("action belongs to a different guest principal")), nil
		}
	}
	return toolSuccess(operations.Projection(record))
}

func (r registry) callActionResume(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := r.resumeAction(ctx, req.Params.Arguments)
	if err != nil {
		if _, ok := err.(*jsonrpc.Error); ok {
			return nil, err
		}
		return toolErrorWithPartial(r.opts, out, err), nil
	}
	return toolSuccess(out)
}

func (r registry) runtimeFactory() (*cmdutil.Factory, *cobra.Command, error) {
	if r.opts.Factory == nil {
		return nil, nil, errors.New("missing command factory")
	}
	clone := *r.opts.Factory
	clone.Secrets = redact.NewRegistry()
	cmd := r.opts.Command
	if cmd == nil {
		cmd = &cobra.Command{Use: "mcp serve"}
	}
	return &clone, cmd, nil
}

func apiClient(f *cmdutil.Factory, cmd *cobra.Command, rt config.Runtime) (*api.Client, auth.Credential, error) {
	cred, findings, err := f.Credential(rt)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return nil, auth.Credential{}, err
	}
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return nil, auth.Credential{}, err
	}
	return client, cred, nil
}

func toolSuccess(value any) (*mcp.CallToolResult, error) {
	text, err := output.StableJSONBytes(value)
	if err != nil {
		return nil, fmt.Errorf("marshal tool result: %w", err)
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: value,
	}, nil
}

func toolError(opts Options, err error) *mcp.CallToolResult {
	value := structuredToolError(opts.Factory, err)
	text, marshalErr := output.StableJSONBytes(value)
	if marshalErr != nil {
		text = []byte(`{"code":"internal_error","exit_code":2,"message":"could not encode tool error"}`)
	}
	return &mcp.CallToolResult{
		IsError:           true,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: value,
	}
}

type commandOutputWithError struct {
	operations.CommandOutput
	Error ToolError `json:"error"`
}

func toolErrorWithPartial(opts Options, partial any, err error) *mcp.CallToolResult {
	out, ok := partial.(operations.CommandOutput)
	if !ok || !commandOutputPresent(out) {
		return toolError(opts, err)
	}
	value := commandOutputWithError{
		CommandOutput: out,
		Error:         structuredToolError(opts.Factory, err),
	}
	text, marshalErr := output.StableJSONBytes(value)
	if marshalErr != nil {
		return toolError(opts, err)
	}
	return &mcp.CallToolResult{
		IsError:           true,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: value,
	}
}

func toolErrorWithFactory(opts Options, factory *cmdutil.Factory, err error) *mcp.CallToolResult {
	opts.Factory = factory
	return toolError(opts, err)
}

type toolCallError struct {
	factory *cmdutil.Factory
	err     error
}

func withToolFactory(factory *cmdutil.Factory, err error) error {
	if err == nil {
		return nil
	}
	return toolCallError{factory: factory, err: err}
}

func (e toolCallError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e toolCallError) Unwrap() error {
	return e.err
}

type ToolError struct {
	Code       string     `json:"code"`
	Message    string     `json:"message"`
	ExitCode   int        `json:"exit_code"`
	RequestID  string     `json:"request_id,omitempty"`
	HTTPStatus int        `json:"http_status,omitempty"`
	Retryable  bool       `json:"retryable"`
	UserAction string     `json:"user_action,omitempty"`
	Details    *any       `json:"details,omitempty"`
	RetryAfter *string    `json:"retry_after,omitempty"`
	RateLimit  *rateLimit `json:"rate_limit,omitempty"`
}

type rateLimit struct {
	Limit     *int64            `json:"limit,omitempty"`
	Remaining *int64            `json:"remaining,omitempty"`
	Reset     *int64            `json:"reset,omitempty"`
	Raw       map[string]string `json:"raw,omitempty"`
}

func structuredToolError(factory *cmdutil.Factory, err error) ToolError {
	if err == nil {
		return ToolError{Code: "internal_error", Message: "unknown error", ExitCode: 2}
	}
	var callErr toolCallError
	if errors.As(err, &callErr) {
		if callErr.factory != nil {
			factory = callErr.factory
		}
		err = callErr.err
	}
	redacted := err
	if factory != nil {
		redacted = factory.RedactError(err)
	}
	redactField := fieldRedactor(factory)
	message := redactField(redacted.Error())
	code := "local_failure"
	exitCode := exitCodeOf(redacted)

	var authErr *auth.Error
	if errors.As(err, &authErr) {
		switch authErr.Kind {
		case auth.ErrMissingCredential:
			return ToolError{
				Code:     "missing_credentials",
				Message:  `run "chab login" or set CHAB_API_KEY in your shell or CI secret store`,
				ExitCode: 3,
			}
		case auth.ErrMissingProfile:
			code = "profile_not_found"
		case auth.ErrInvalidURL:
			code = "invalid_profile_url"
		case auth.ErrMalformedConfig, auth.ErrUnsupportedVersion:
			code = "auth_parse_failed"
		default:
			code = "auth_read_failed"
		}
		return ToolError{Code: code, Message: message, ExitCode: exitCode}
	}

	var cfgErr *config.Error
	if errors.As(err, &cfgErr) {
		switch cfgErr.Kind {
		case config.ErrMissingProfile:
			code = "profile_not_found"
		case config.ErrInvalidURL:
			code = "invalid_profile_url"
		case config.ErrMalformedConfig, config.ErrUnsupportedVersion:
			code = "config_parse_failed"
		default:
			code = "config_read_failed"
		}
		return ToolError{Code: code, Message: message, ExitCode: exitCode}
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		apiMessage := apiErr.Message
		if apiMessage == "" {
			apiMessage = message
		}
		value := ToolError{
			Code:       redactField(apiErr.Code),
			Message:    redactField(apiMessage),
			ExitCode:   exitCode,
			RequestID:  redactField(apiErr.RequestID),
			HTTPStatus: apiErr.Status,
			Retryable:  apiErr.Retryable,
			UserAction: redactField(apiErr.UserAction),
			Details:    apiToolDetails(apiErr, redactField),
		}
		addToolOperationalContext(&value, apiErr.Meta, redactField)
		return value
	}
	var protoErr *api.ProtocolError
	var guestUnavailable guestToolUnavailable
	if errors.As(err, &guestUnavailable) {
		return ToolError{Code: "operation_not_available_on_free", Message: message, ExitCode: 4}
	}
	if errors.As(err, &protoErr) {
		value := ToolError{Code: "api_failure", Message: message, ExitCode: exitCode, RequestID: redactField(protoErr.RequestID), HTTPStatus: protoErr.Status}
		addToolOperationalContext(&value, protoErr.Meta, redactField)
		return value
	}
	var transportErr *api.TransportError
	if errors.As(err, &transportErr) {
		return ToolError{Code: "api_failure", Message: message, ExitCode: exitCode}
	}
	var usageErr *api.UsageError
	if errors.As(err, &usageErr) {
		if usageErr.Field == "base_url" {
			code = "invalid_profile_url"
		} else if usageErr.Field == "mcp" {
			code = "invalid_params"
		} else {
			code = "api_failure"
		}
		return ToolError{Code: code, Message: message, ExitCode: exitCode}
	}
	var projectErr *readservice.ProjectUsageError
	if errors.As(err, &projectErr) {
		return ToolError{Code: "invalid_params", Message: message, ExitCode: exitCode}
	}
	var creditsErr *readservice.CreditsUsageError
	if errors.As(err, &creditsErr) {
		return ToolError{Code: "invalid_params", Message: message, ExitCode: exitCode}
	}
	if errors.Is(err, context.Canceled) {
		return ToolError{Code: "cancelled", Message: "request cancelled", ExitCode: 2}
	}
	return ToolError{Code: code, Message: message, ExitCode: exitCode}
}

func fieldRedactor(factory *cmdutil.Factory) func(string) string {
	return func(value string) string {
		if factory != nil {
			value = factory.RedactValue(value)
		}
		return redact.String(value)
	}
}

func addToolOperationalContext(value *ToolError, meta api.ResponseMeta, redactField func(string) string) {
	if meta.RetryAfter.Present || meta.RetryAfter.Raw != "" {
		retryAfter := redactField(meta.RetryAfter.Raw)
		value.RetryAfter = &retryAfter
	}
	if rateLimit, ok := buildToolRateLimit(meta.RateLimit, redactField); ok {
		value.RateLimit = rateLimit
	}
}

func buildToolRateLimit(rl api.RateLimit, redactField func(string) string) (*rateLimit, bool) {
	if rl.Limit == nil && rl.Remaining == nil && rl.Reset == nil &&
		rl.RawLimit == "" && rl.RawRemaining == "" && rl.RawReset == "" &&
		!rl.LimitPresent && !rl.RemainingPresent && !rl.ResetPresent {
		return nil, false
	}
	view := &rateLimit{
		Limit:     rl.Limit,
		Remaining: rl.Remaining,
		Reset:     rl.Reset,
	}
	raw := make(map[string]string)
	if rl.LimitPresent || rl.RawLimit != "" {
		redacted := redactField(rl.RawLimit)
		raw["limit"] = redacted
		if redacted != rl.RawLimit {
			view.Limit = nil
		}
	}
	if rl.RemainingPresent || rl.RawRemaining != "" {
		redacted := redactField(rl.RawRemaining)
		raw["remaining"] = redacted
		if redacted != rl.RawRemaining {
			view.Remaining = nil
		}
	}
	if rl.ResetPresent || rl.RawReset != "" {
		redacted := redactField(rl.RawReset)
		raw["reset"] = redacted
		if redacted != rl.RawReset {
			view.Reset = nil
		}
	}
	if len(raw) > 0 {
		view.Raw = raw
	}
	return view, true
}

func exitCodeOf(err error) int {
	type exitCoder interface{ ExitCode() int }
	var coded exitCoder
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}

func apiToolDetails(apiErr *api.Error, redactField func(string) string) *any {
	if apiErr == nil {
		return nil
	}
	if apiErr.DetailsPresent {
		value := redactToolDetail(apiErr.DetailsValue, redactField)
		return &value
	}
	if len(apiErr.Details) == 0 {
		return nil
	}
	value := any(redactDetails(apiErr.Details, redactField))
	return &value
}

func redactDetails(details map[string][]string, redactField func(string) string) map[string][]string {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string][]string, len(details))
	for key, values := range details {
		copied := make([]string, len(values))
		for i, value := range values {
			copied[i] = redactField(value)
		}
		out[redactField(key)] = copied
	}
	return out
}

func redactToolDetail(value any, redactField func(string) string) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactField(typed)
	case json.Number:
		text := typed.String()
		redacted := redactField(text)
		if redacted != text {
			return redacted
		}
		return typed
	case bool:
		text := strconv.FormatBool(typed)
		redacted := redactField(text)
		if redacted != text {
			return redacted
		}
		return typed
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactToolDetail(item, redactField)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[redactField(key)] = redactToolDetail(item, redactField)
		}
		return out
	case map[string][]string:
		return redactDetails(typed, redactField)
	default:
		text := fmt.Sprint(typed)
		redacted := redactField(text)
		if redacted != text {
			return redacted
		}
		return typed
	}
}

func invalidParams(err error) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: redact.String(err.Error())}
}

func cacheAndCursorMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if err := enforceRequestMeta(method, req); err != nil {
			return nil, err
		}
		if method == "tools/list" {
			if params, ok := req.GetParams().(*mcp.ListToolsParams); ok && params != nil && params.Cursor != "" {
				return nil, invalidParams(errors.New("tools/list cursor is not supported"))
			}
		}
		result, err := next(ctx, method, req)
		if err != nil {
			return nil, err
		}
		switch typed := result.(type) {
		case *mcp.DiscoverResult:
			typed.SupportedVersions = []string{ProtocolVersion}
			typed.TTLMs = cacheTTLMS
			typed.CacheScope = "private"
		case *mcp.ListToolsResult:
			// Visibility changes with the active credential and server policy.
			// Clients must re-list rather than cache an old guest or team set.
			typed.TTLMs = 0
			typed.CacheScope = "private"
			typed.NextCursor = ""
			sort.SliceStable(typed.Tools, func(i, j int) bool { return typed.Tools[i].Name < typed.Tools[j].Name })
			if requestProtocolVersion(req) < ProtocolVersion {
				typed.Tools = legacyCompatibleTools(typed.Tools)
			}
		}
		return result, nil
	}
}

func enforceRequestMeta(method string, req mcp.Request) error {
	if strings.HasPrefix(method, "notifications/") {
		return nil
	}
	if method == "server/discover" || sessionProtocolVersion(req) >= ProtocolVersion {
		return requireModernRequestMeta(req)
	}
	return nil
}

func sessionProtocolVersion(req mcp.Request) string {
	session, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || session == nil {
		return ""
	}
	params := session.InitializeParams()
	if params == nil {
		return ""
	}
	return params.ProtocolVersion
}

func requireModernRequestMeta(req mcp.Request) error {
	meta := paramsMeta(req)
	version, ok := meta[mcp.MetaKeyProtocolVersion].(string)
	if !ok || version == "" {
		return invalidParams(fmt.Errorf("request _meta must include %q", mcp.MetaKeyProtocolVersion))
	}
	if version != ProtocolVersion {
		return unsupportedProtocol(version)
	}
	if _, ok := meta[mcp.MetaKeyClientCapabilities]; !ok {
		return invalidParams(fmt.Errorf("request _meta must include %q", mcp.MetaKeyClientCapabilities))
	}
	return nil
}

func paramsMeta(req mcp.Request) map[string]any {
	params := req.GetParams()
	if params == nil {
		return nil
	}
	value := reflect.ValueOf(params)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil
	}
	return params.GetMeta()
}

func unsupportedProtocol(requested string) error {
	data, _ := json.Marshal(mcp.UnsupportedProtocolVersionData{
		Supported: []string{ProtocolVersion},
		Requested: requested,
	})
	return &jsonrpc.Error{Code: mcp.CodeUnsupportedProtocolVersion, Message: "unsupported protocol version", Data: data}
}

func requestProtocolVersion(req mcp.Request) string {
	switch typed := req.(type) {
	case *mcp.ServerRequest[*mcp.ListToolsParams]:
		return typed.ProtocolVersion()
	case *mcp.ServerRequest[*mcp.DiscoverParams]:
		return typed.ProtocolVersion()
	}
	return ""
}

func legacyCompatibleTools(tools []*mcp.Tool) []*mcp.Tool {
	if len(tools) == 0 {
		return tools
	}
	out := make([]*mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			out = append(out, nil)
			continue
		}
		clone := *tool
		clone.OutputSchema = nil
		out = append(out, &clone)
	}
	return out
}

type versionedTransport struct {
	mcp.Transport
}

func (versionedTransport) SupportsProtocolVersion(version string) bool {
	return version == ProtocolVersion
}

type readCloser struct {
	io.Reader
}

func (readCloser) Close() error {
	return nil
}

type writeCloser struct {
	io.Writer
}

func (writeCloser) Close() error {
	return nil
}

func instructions() string {
	return "Use Chab tools to inspect local auth context, token identity, public API health, error codes, credits, operations, files and connected resources. Credentials are read at each call using the CLI's profile, environment and auth-file rules. Do not pass API keys as tool arguments. Live effectful tools require explicit local confirmation input and record local recovery receipts before the request."
}

type rawObject map[string]json.RawMessage

func decodeObject(raw json.RawMessage, allowed map[string]func(json.RawMessage) error) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var obj rawObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if obj == nil {
		return errors.New("arguments must be a JSON object")
	}
	for key, value := range obj {
		fn, ok := allowed[key]
		if !ok {
			return fmt.Errorf("unknown argument %q", key)
		}
		if string(value) == "null" {
			return fmt.Errorf("argument %q must not be null", key)
		}
		if err := fn(value); err != nil {
			return err
		}
	}
	return nil
}

func requireEmptyArguments(raw json.RawMessage) error {
	return decodeObject(raw, map[string]func(json.RawMessage) error{})
}

type projectListInput struct {
	Limit    *int
	All      *bool
	Cursor   *string
	PageSize *int
}

func decodeProjectListInput(raw json.RawMessage) (projectListInput, error) {
	var input projectListInput
	err := decodeObject(raw, map[string]func(json.RawMessage) error{
		"limit":     decodeIntPtr(&input.Limit, "limit"),
		"all":       decodeBoolPtr(&input.All, "all"),
		"cursor":    decodeStringPtr(&input.Cursor, "cursor"),
		"page_size": decodeIntPtr(&input.PageSize, "page_size"),
	})
	return input, err
}

func (i projectListInput) toOptions() readservice.ProjectListOptions {
	return readservice.ProjectListOptions{
		Page: readservice.CursorPaginationOptions{
			Limit:    optionalInt(i.Limit),
			All:      optionalBool(i.All),
			Cursor:   optionalString(i.Cursor),
			PageSize: optionalInt(i.PageSize),
		},
	}
}

type projectShowInput struct {
	ProjectID string
}

func decodeProjectShowInput(raw json.RawMessage) (projectShowInput, error) {
	var input projectShowInput
	seen := false
	err := decodeObject(raw, map[string]func(json.RawMessage) error{
		"project_id": func(raw json.RawMessage) error {
			seen = true
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("argument %q must be a string", "project_id")
			}
			if value == "" {
				return errors.New("argument \"project_id\" must not be empty")
			}
			input.ProjectID = value
			return nil
		},
	})
	if err != nil {
		return projectShowInput{}, err
	}
	if !seen {
		return projectShowInput{}, errors.New("missing required argument \"project_id\"")
	}
	return input, nil
}

type transactionsInput struct {
	Limit    *int
	All      *bool
	Cursor   *string
	PageSize *int
}

func decodeTransactionsInput(raw json.RawMessage) (transactionsInput, error) {
	var input transactionsInput
	err := decodeObject(raw, map[string]func(json.RawMessage) error{
		"limit":     decodeIntPtr(&input.Limit, "limit"),
		"all":       decodeBoolPtr(&input.All, "all"),
		"cursor":    decodeStringPtr(&input.Cursor, "cursor"),
		"page_size": decodeIntPtr(&input.PageSize, "page_size"),
	})
	return input, err
}

func (i transactionsInput) toOptions() readservice.TransactionListOptions {
	return readservice.TransactionListOptions{
		Page: readservice.CursorPaginationOptions{
			Limit:    optionalInt(i.Limit),
			All:      optionalBool(i.All),
			Cursor:   optionalString(i.Cursor),
			PageSize: optionalInt(i.PageSize),
		},
	}
}

func decodeStringPtr(target **string, name string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("argument %q must be a string", name)
		}
		*target = &value
		return nil
	}
}

func decodeIntPtr(target **int, name string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("argument %q must be an integer", name)
		}
		*target = &value
		return nil
	}
}

func decodeBoolPtr(target **bool, name string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("argument %q must be a boolean", name)
		}
		*target = &value
		return nil
	}
}

func optionalString(value *string) readservice.OptionalString {
	if value == nil {
		return readservice.OptionalString{}
	}
	return readservice.OptionalString{Value: *value, Set: true}
}

func optionalInt(value *int) readservice.OptionalInt {
	if value == nil {
		return readservice.OptionalInt{}
	}
	return readservice.OptionalInt{Value: *value, Set: true}
}

func optionalBool(value *bool) readservice.OptionalBool {
	if value == nil {
		return readservice.OptionalBool{}
	}
	return readservice.OptionalBool{Value: *value, Set: true}
}

func paginationOptions(limit *int, all *bool, page *int, perPage *int) readservice.PaginationOptions {
	return readservice.PaginationOptions{
		Limit:   optionalInt(limit),
		All:     optionalBool(all),
		Page:    optionalInt(page),
		PerPage: optionalInt(perPage),
	}
}

func addPaginationProperties(properties map[string]any) {
	properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "description": "Maximum total items to fetch across pages."}
	properties["all"] = map[string]any{"type": "boolean", "description": "Fetch every page. false behaves like absence."}
	properties["page"] = map[string]any{"type": "integer", "minimum": 1, "description": "Fetch one exact API page."}
	properties["per_page"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Items per page for exact page mode."}
}

func emptyInputSchema() map[string]any {
	return objectSchema(map[string]any{}, []string{})
}

func projectListInputSchema() map[string]any {
	props := map[string]any{
		"limit":     map[string]any{"type": "integer", "minimum": 1, "description": "Maximum total items to fetch."},
		"all":       map[string]any{"type": "boolean", "description": "Fetch every cursor page. false behaves like absence."},
		"cursor":    map[string]any{"type": "string", "minLength": 1, "description": "Opaque cursor returned by the API."},
		"page_size": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Items per API request."},
	}
	return objectSchema(props, []string{})
}

func projectShowInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"project_id": map[string]any{"type": "string", "minLength": 1},
	}, []string{"project_id"})
}

func transactionsInputSchema() map[string]any {
	props := map[string]any{
		"limit":     map[string]any{"type": "integer", "minimum": 1, "description": "Maximum total items to fetch."},
		"all":       map[string]any{"type": "boolean", "description": "Fetch every cursor page. false behaves like absence."},
		"cursor":    map[string]any{"type": "string", "minLength": 1, "description": "Opaque cursor returned by the API."},
		"page_size": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Items per API request."},
	}
	return objectSchema(props, []string{})
}

func authEnvOutputSchema() map[string]any {
	return objectSchema(stringProps("profile", "base_url", "api_base_url", "locale", "secret_variable"), []string{"profile", "base_url", "api_base_url", "locale", "secret_variable"})
}

func healthSchema() map[string]any {
	family := objectSchema(map[string]any{
		"family":                 stringSchema(),
		"state":                  stringSchema(),
		"covered_operation_keys": nullable(arraySchema(stringSchema())),
		"remediation":            nullable(stringSchema()),
	}, []string{"family", "state", "covered_operation_keys", "remediation"})
	freeMode := objectSchema(map[string]any{
		"state": stringSchema(),
	}, []string{"state"})
	return objectSchema(map[string]any{
		"service":     stringSchema(),
		"api_version": stringSchema(),
		"status":      stringSchema(),
		"updated_at":  stringSchema(),
		"families":    arraySchema(family),
		"free_mode":   freeMode,
	}, []string{"service", "api_version", "status", "updated_at", "families", "free_mode"})
}

func errorCatalogSchema() map[string]any {
	entry := objectSchema(map[string]any{
		"code":         stringSchema(),
		"http_status":  integerSchema(),
		"retryable":    boolSchema(),
		"chargeable":   boolSchema(),
		"safe_to_show": boolSchema(),
		"message":      stringSchema(),
		"user_action":  stringSchema(),
		"docs_url":     stringSchema(),
		"emitted_by":   stringSchema(),
	}, []string{"code", "http_status", "retryable", "chargeable", "safe_to_show", "message", "user_action", "docs_url", "emitted_by"})
	return objectSchema(map[string]any{"errors": arraySchema(entry)}, []string{"errors"})
}

func whoamiOutputSchema() map[string]any {
	allowance := objectSchema(map[string]any{
		"version":           integerSchema(),
		"limit_credits":     integerSchema(),
		"settled_credits":   integerSchema(),
		"held_credits":      integerSchema(),
		"remaining_credits": integerSchema(),
	}, []string{"version", "limit_credits", "settled_credits", "held_credits", "remaining_credits"})
	tokenControls := objectSchema(map[string]any{
		"policy_revision": integerSchema(),
		"feature_access": objectSchema(map[string]any{
			"mode":           stringSchema(),
			"snapshot_stale": boolSchema(),
		}, []string{"mode", "snapshot_stale"}),
		"ip_restrictions": objectSchema(map[string]any{
			"restricted":       boolSchema(),
			"allow_rule_count": integerSchema(),
			"deny_rule_count":  integerSchema(),
		}, []string{"restricted", "allow_rule_count", "deny_rule_count"}),
		"spending": objectSchema(map[string]any{
			"mode":                    stringSchema(),
			"active_reserved_credits": integerSchema(),
			"allowance":               nullable(allowance),
		}, []string{"mode", "active_reserved_credits", "allowance"}),
		"project_access": objectSchema(map[string]any{
			"mode":                 stringSchema(),
			"selected_project_ids": arraySchema(integerSchema()),
			"selected_count":       integerSchema(),
		}, []string{"mode", "selected_project_ids", "selected_count"}),
	}, []string{"policy_revision", "feature_access", "ip_restrictions", "spending", "project_access"})
	return objectSchema(map[string]any{
		"principal_type":  stringSchema(),
		"team_id":         integerSchema(),
		"token_id":        stringSchema(),
		"token_public_id": stringSchema(),
		"scopes":          arraySchema(stringSchema()),
		"token_controls":  tokenControls,
		"request_id":      stringSchema(),
	}, []string{"principal_type", "team_id", "token_id", "token_public_id", "scopes", "token_controls", "request_id"})
}

func projectSchema() map[string]any {
	props := stringProps("id", "name", "description", "url", "status", "timezone", "language", "created_at", "updated_at")
	props["description"] = nullable(stringSchema())
	props["url"] = nullable(stringSchema())
	props["limit"] = integerSchema()
	props["automate"] = boolSchema()
	return objectSchema(props, []string{"id", "name", "description", "url", "status", "timezone", "language", "limit", "automate", "created_at", "updated_at"})
}

func balanceSchema() map[string]any {
	return objectSchema(map[string]any{
		"spendable_balance": integerSchema(),
		"debt":              integerSchema(),
		"expires":           nullable(stringSchema()),
	}, []string{"spendable_balance", "debt", "expires"})
}

func transactionSchema() map[string]any {
	props := stringProps("id", "occurred_at", "kind", "description")
	props["amount"] = integerSchema()
	props["resulting_spendable_balance"] = integerSchema()
	props["expires_at"] = nullable(stringSchema())
	props["operation_id"] = nullable(stringSchema())
	props["purchase_id"] = nullable(stringSchema())
	return objectSchema(props, []string{"id", "occurred_at", "kind", "amount", "resulting_spendable_balance", "expires_at", "operation_id", "purchase_id", "description"})
}

func toolErrorSchema() map[string]any {
	return objectSchema(map[string]any{
		"code":        stringSchema(),
		"message":     stringSchema(),
		"exit_code":   integerSchema(),
		"request_id":  stringSchema(),
		"http_status": integerSchema(),
		"retryable":   boolSchema(),
		"user_action": stringSchema(),
		"details":     map[string]any{},
		"retry_after": stringSchema(),
		"rate_limit": objectSchema(map[string]any{
			"limit":     integerSchema(),
			"remaining": integerSchema(),
			"reset":     integerSchema(),
			"raw": map[string]any{
				"type":                 "object",
				"additionalProperties": stringSchema(),
			},
		}, []string{}),
	}, []string{"code", "message", "exit_code", "retryable"})
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProps(names ...string) map[string]any {
	props := make(map[string]any, len(names))
	for _, name := range names {
		props[name] = stringSchema()
	}
	return props
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func boolSchema() map[string]any {
	return map[string]any{"type": "boolean"}
}

func integerSchema() map[string]any {
	return map[string]any{"type": "integer"}
}

func arraySchema(item any) map[string]any {
	return map[string]any{"type": "array", "items": item}
}

func nullable(schema any) map[string]any {
	return map[string]any{"oneOf": []any{schema, map[string]any{"type": "null"}}}
}

func oneOf(schemas ...any) map[string]any {
	return map[string]any{"oneOf": schemas}
}
