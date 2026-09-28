package cmdutil

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/outputtransform"
)

// HumanOutput renders the human form for one command result.
type HumanOutput struct {
	Render     func(w io.Writer)
	RenderMode func(w io.Writer, mode output.TerminalMode)
	Plain      func(data, prose io.Writer)
}

// OutputSupport declares the output modes a command result explicitly supports.
type OutputSupport struct {
	Human    bool
	JSON     bool
	Plain    bool
	JQ       bool
	Template bool
}

// CommandResult is the support-aware rendering handoff for active commands.
type CommandResult struct {
	Machine  any
	Human    HumanOutput
	Supports OutputSupport
}

// WriteCommandResult renders a command result through the shared output layer.
func (f *Factory) WriteCommandResult(cmd *cobra.Command, result CommandResult) error {
	// Requested modes are terminal decisions: if a command result does not
	// support the requested mode, do not fall through to a different format.
	// Active commands are normally rejected earlier by the root guard.
	if expr, ok := JQExpr(cmd); ok {
		if !result.Supports.JQ {
			return nil
		}
		return f.writeJQ(cmd.Context(), cmd.OutOrStdout(), result.Machine, expr)
	}
	if text, ok := TemplateExpr(cmd); ok {
		if !result.Supports.Template {
			return nil
		}
		return f.writeTemplate(cmd.OutOrStdout(), result.Machine, text)
	}
	if JSONEnabled(cmd) && result.Supports.JSON {
		return f.writeJSON(cmd.OutOrStdout(), result.Machine)
	}
	if PlainEnabled(cmd) {
		if !result.Supports.Plain || result.Human.Plain == nil {
			return nil
		}
		return f.writePlain(cmd.OutOrStdout(), cmd.ErrOrStderr(), result.Human.Plain)
	}
	if result.Supports.Human && (result.Human.Render != nil || result.Human.RenderMode != nil) {
		return f.writeHuman(cmd, result.Human)
	}
	return nil
}

// WriteResult keeps retained callers source-compatible. A non-nil machine
// value keeps the historical JSON-capable behavior; a nil machine value is
// human-only.
func (f *Factory) WriteResult(cmd *cobra.Command, machine any, human HumanOutput) error {
	return f.WriteCommandResult(cmd, commandResult(machine, machine, human))
}

// commandResult derives supported modes from the command's original value but
// carries the value selected for rendering. Keeping those inputs separate
// prevents a metadata wrapper from making a nil machine result JSON-capable.
func commandResult(machine, rendered any, human HumanOutput) CommandResult {
	support := OutputSupport{
		Human: human.Render != nil || human.RenderMode != nil || human.Plain != nil,
		Plain: human.Plain != nil,
	}
	if machine != nil {
		support.JSON = true
		support.JQ = true
		support.Template = true
	}
	return CommandResult{
		Machine:  rendered,
		Human:    human,
		Supports: support,
	}
}

// WriteResultWithMeta wraps supported machine output when response metadata
// was explicitly requested. Output support remains derived from the original
// command value so metadata cannot make a human-only result machine-capable.
func (f *Factory) WriteResultWithMeta(cmd *cobra.Command, machine any, meta api.ResponseMeta, includeAPIMeta bool, human HumanOutput) error {
	rendered := machine
	if IncludeMetaEnabled(cmd) {
		rendered = output.WrapMeta(machine, meta, output.MetaOptions{IncludeAPIMeta: includeAPIMeta})
	}
	return meta.RedactError(f.WriteCommandResult(cmd, commandResult(machine, rendered, human)))
}

func (f *Factory) writeJSON(w io.Writer, value any) error {
	data, err := f.stableJSON(value)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (f *Factory) writeJQ(ctx context.Context, w io.Writer, value any, expr string) error {
	data, err := f.stableJSON(value)
	if err != nil {
		return err
	}
	out, err := outputtransform.ExecuteJQ(ctx, data, expr)
	if err != nil {
		return err
	}
	if registry := f.retainedSecretRegistry(); registry != nil {
		// jq emits a stream of JSON values. Redact only decoded string values
		// so exact secrets cannot rewrite JSON syntax, property names, or types.
		out = registry.RedactJSON(out)
	}
	_, err = w.Write(out)
	return err
}

func (f *Factory) writeTemplate(w io.Writer, value any, text string) error {
	data, err := f.stableJSON(value)
	if err != nil {
		return err
	}
	out, err := outputtransform.ExecuteTemplate(data, text)
	if err != nil {
		return err
	}
	out = f.redactRetainedText(out)
	_, err = w.Write(out)
	return err
}

func (f *Factory) writePlain(stdout, stderr io.Writer, render func(data, prose io.Writer)) error {
	data, prose := output.PlainBytes(render)
	data = f.redactRetainedText(data)
	prose = f.redactRetainedText(prose)
	if _, err := stdout.Write(data); err != nil {
		return err
	}
	if _, err := stderr.Write(prose); err != nil {
		return err
	}
	return nil
}

func (f *Factory) writeHuman(cmd *cobra.Command, human HumanOutput) error {
	mode := f.terminalMode(cmd)
	render := human.RenderMode
	if render == nil {
		render = func(w io.Writer, _ output.TerminalMode) {
			human.Render(w)
		}
	}
	data := output.RenderHuman(mode, render)
	data = f.redactRetainedText(data)
	paged, err := f.maybePageHuman(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), data, NoANSIEnabled(cmd), NoPagerEnabled(cmd))
	if err != nil {
		return err
	}
	if paged {
		return nil
	}
	_, err = cmd.OutOrStdout().Write(data)
	return err
}

func (f *Factory) terminalMode(cmd *cobra.Command) output.TerminalMode {
	// Color is deliberately not enabled by default in the current shell. The
	// suppressor flags and NO_COLOR keep future color support off without
	// changing today's plain human output.
	color := false
	ansi := false
	if NoANSIEnabled(cmd) {
		return output.TerminalMode{Color: false, ANSI: false, Sanitize: true}
	}
	if value, ok := f.lookupEnv("NO_COLOR"); ok && value != "" {
		color = false
	}
	if NoColorEnabled(cmd) {
		color = false
	}
	return output.TerminalMode{Color: color, ANSI: ansi}
}

func (f *Factory) stableJSON(value any) ([]byte, error) {
	data, err := output.StableJSONBytes(value)
	if err != nil {
		return nil, err
	}
	if registry := f.retainedSecretRegistry(); registry != nil {
		// Retained callers historically supply already-rendered values. Active
		// commands redact semantic data values before this shared renderer so
		// structural strings cannot be mistaken for secrets.
		data = registry.RedactJSON(data)
	}
	return data, nil
}

func (f *Factory) redactRetainedText(data []byte) []byte {
	if registry := f.retainedSecretRegistry(); registry != nil {
		return registry.RedactText(data)
	}
	return data
}

// retainedSecretRegistry is intentionally separate from secretRegistry.
// Active commands redact data values before rendering; applying their exact
// secrets to completed output could rewrite labels or JSON syntax. Older
// callers still need their historical final-output pass.
func (f *Factory) retainedSecretRegistry() SecretRegistry {
	if f == nil {
		return nil
	}
	return f.Rungrad
}

// RenderAPIError writes API/protocol/transport errors in the command output
// mode. It returns false for local errors so the CLI runner can keep the local
// error renderer.
func RenderAPIError(w io.Writer, err error, jsonMode bool) bool {
	if w == nil {
		// A nil writer is a detection-only call used by tests and classifiers.
		return isAPIError(err)
	}
	if jsonMode {
		return output.WriteAPIErrorJSON(w, err)
	}
	return output.WriteAPIErrorHuman(w, err)
}

func isAPIError(err error) bool {
	var apiErr *api.Error
	var protoErr *api.ProtocolError
	var transportErr *api.TransportError
	return errors.As(err, &apiErr) ||
		errors.As(err, &protoErr) ||
		errors.As(err, &transportErr)
}
