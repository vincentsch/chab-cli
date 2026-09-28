package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

var guestPublicTools = map[string]bool{
	"chab_auth_env": true,
	"chab_health":   true,
	"chab_errors":   true,
}

var guestLocalTools = map[string]bool{
	"chab_action_list":   true,
	"chab_action_show":   true,
	"chab_action_resume": true,
}

// guestCredentialMiddleware is a local visibility fence. The backend remains
// authoritative for every request, but guests must never be offered or call
// a team/paid-only tool merely because it exists in the pinned OpenAPI file.
func (r registry) guestCredentialMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != "tools/list" && method != "tools/call" {
			return next(ctx, method, req)
		}
		name := ""
		if method == "tools/call" {
			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok || params == nil {
				return next(ctx, method, req)
			}
			name = params.Name
			if guestPublicTools[name] {
				return next(ctx, method, req)
			}
		}
		guest, allowed, err := r.guestAllowedTools(ctx)
		if !guest {
			return next(ctx, method, req)
		}
		if method == "tools/call" {
			if err != nil {
				return toolError(r.opts, err), nil
			}
			if !allowed[name] {
				return toolError(r.opts, guestToolUnavailable{}), nil
			}
			return next(ctx, method, req)
		}
		result, nextErr := next(ctx, method, req)
		if nextErr != nil {
			return result, nextErr
		}
		if listed, ok := result.(*mcp.ListToolsResult); ok {
			filtered := listed.Tools[:0]
			for _, tool := range listed.Tools {
				if tool != nil && (guestPublicTools[tool.Name] || (err == nil && allowed[tool.Name])) {
					filtered = append(filtered, tool)
				}
			}
			listed.Tools = filtered
		}
		return result, nil
	}
}

func (r registry) guestAllowedTools(ctx context.Context) (bool, map[string]bool, error) {
	f, cmd, err := r.runtimeFactory()
	if err != nil {
		return false, nil, nil
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return false, nil, nil
	}
	cred, _, err := f.Credential(rt)
	if err != nil {
		var authErr *auth.Error
		if errors.As(err, &authErr) && authErr.Kind == auth.ErrMissingCredential {
			return false, nil, nil
		}
		return false, nil, nil
	}
	if cred.PrincipalType != "guest_trial" {
		return false, nil, nil
	}
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return true, nil, err
	}
	identity, _, err := client.Whoami(ctx)
	if err != nil {
		return true, nil, output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	if identity.PrincipalType != "guest_trial" || identity.PrincipalID == "" {
		return true, nil, fmt.Errorf("guest credential identity mismatch")
	}
	bootstrap, err := api.NewBootstrap(api.OptionsForBootstrap(rt, f.VersionString(), false, nil))
	if err != nil {
		return true, nil, err
	}
	compatibility, _, err := bootstrap.Compatibility(ctx, api.EffectiveClientVersion(f.VersionString()))
	if err != nil {
		return true, nil, err
	}
	guest, err := compatibility.GuestLocalMCP()
	if err != nil {
		return true, nil, err
	}
	allowed := map[string]bool{}
	for name := range guestLocalTools {
		allowed[name] = true
	}
	registry, err := chabcontract.Load()
	if err != nil {
		return true, nil, err
	}
	for _, key := range guest.SupportedRouteOperationKeys {
		switch key {
		case "auth.me", "credits.get", "operations.get", "operations.result", "operations.artifact", "operations.artifact_download", "operations.cancel",
			"files.create", "files.list", "files.get", "files.delete", "files.download", "llm.models":
			if op, ok := registry.Find(key); ok && (op.RequiredScope == "" || slices.Contains(identity.Scopes, op.RequiredScope)) {
				allowed[operationToolName(op.ID)] = true
			}
		}
	}
	if guest.SpendEnabled {
		supported := make(map[string]bool, len(guest.SupportedRouteOperationKeys))
		for _, key := range guest.SupportedRouteOperationKeys {
			supported[key] = true
		}
		for _, tool := range guest.Tools {
			op, ok := registry.Find(tool.OperationKey)
			if !ok { // New server tools do not invalidate older pinned clients.
				continue
			}
			if !supported[tool.OperationKey] || mcpExcludedOperation(op.ID) || operationToolName(op.ID) != tool.ToolName || op.RequiredScope != tool.Scope ||
				!strings.EqualFold(op.Method, tool.Method) || op.Path != tool.Path ||
				!op.IdempotencyRequired || !tool.IdempotencyRequired ||
				tool.FundingMode != "promotional_only" || tool.CreditSource != "guest_trial" {
				return true, nil, fmt.Errorf("guest tool compatibility does not match pinned operation contract")
			}
			if slices.Contains(identity.Scopes, op.RequiredScope) {
				allowed[tool.ToolName] = true
			}
		}
	}
	return true, allowed, nil
}

type guestToolUnavailable struct{}

func (guestToolUnavailable) Error() string {
	return "operation is not available on the free guest trial"
}
func (guestToolUnavailable) ExitCode() int { return 4 }
