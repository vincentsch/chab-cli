// Package outputtransform holds jq and Go-template execution for output modes.
package outputtransform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"text/template"

	"github.com/itchyny/gojq"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/redact"
)

// TransformError is a local output-mode error for jq/template validation or
// execution failures.
type TransformError struct {
	Mode  string
	Stage string
	// Err is set by all construction sites; Error formats it into the
	// user-facing jq/template message.
	Err error
}

func (e *TransformError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Mode == "jq" && e.Stage == "parse":
		return "invalid --jq expression: " + e.Err.Error()
	case e.Mode == "jq":
		return "--jq expression failed: " + e.Err.Error()
	case e.Mode == "template" && e.Stage == "parse":
		return "invalid --template: " + e.Err.Error()
	default:
		return "--template rendering failed: " + e.Err.Error()
	}
}

func (e *TransformError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *TransformError) ExitCode() int {
	return 1
}

// ValidateJQ parses and compiles a jq expression without executing command
// logic. Compilation catches undefined functions.
func ValidateJQ(expr string) error {
	if _, err := compileJQ(expr); err != nil {
		return err
	}
	return nil
}

// ValidateTemplate parses a Go text/template without executing command logic.
func ValidateTemplate(text string) error {
	if _, err := parseTemplate(text); err != nil {
		return err
	}
	return nil
}

// ExecuteJQ runs expr over the exact stable JSON bytes that --json would emit.
// Results are buffered and returned only after the full iteration succeeds.
func ExecuteJQ(ctx context.Context, stableJSON []byte, expr string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	value, err := decodeStableJSON(stableJSON, "jq")
	if err != nil {
		return nil, err
	}
	code, err := compileJQ(expr)
	if err != nil {
		return nil, err
	}
	iter := code.RunWithContext(ctx, value)
	var buf bytes.Buffer
	for {
		next, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := next.(error); ok {
			return nil, &TransformError{Mode: "jq", Stage: "run", Err: err}
		}
		data, err := gojq.Marshal(output.RedactJSONValue(next))
		if err != nil {
			return nil, &TransformError{Mode: "jq", Stage: "run", Err: err}
		}
		buf.Write(output.EscapeJSONC1Controls(data))
		buf.WriteByte('\n')
	}
	return output.SanitizeControlBytes(buf.Bytes()), nil
}

// ExecuteTemplate renders text over the exact stable JSON bytes that --json
// would emit. Partial template output is discarded on failure.
func ExecuteTemplate(stableJSON []byte, text string) ([]byte, error) {
	value, err := decodeStableJSON(stableJSON, "template")
	if err != nil {
		return nil, err
	}
	// Sanitize provider-owned scalar values before interpolation. Otherwise an
	// incomplete escape at the end of a value can combine with and consume
	// template-owned delimiters before the final output sanitizer sees it.
	value = sanitizeTemplateStrings(value)
	tmpl, err := parseTemplate(text)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, value); err != nil {
		return nil, &TransformError{Mode: "template", Stage: "run", Err: err}
	}
	if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
		buf.WriteByte('\n')
	}
	// Redaction happens before transforms when stable JSON is rendered, but run
	// it again here because templates can reshape values into new strings.
	return output.SanitizeControlBytes(redact.Bytes(buf.Bytes())), nil
}

func sanitizeTemplateStrings(value any) any {
	switch typed := value.(type) {
	case string:
		return string(output.SanitizeControlBytes([]byte(typed)))
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeTemplateStrings(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = sanitizeTemplateStrings(item)
		}
		return out
	default:
		return value
	}
}

// compileJQ preserves Chab's legacy validation contract: only an exactly empty
// expression is identity, and parse/compile errors use local TransformError text.
func compileJQ(expr string) (*gojq.Code, error) {
	if expr == "" {
		expr = "."
	}
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, &TransformError{Mode: "jq", Stage: "parse", Err: err}
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, &TransformError{Mode: "jq", Stage: "parse", Err: err}
	}
	return code, nil
}

func parseTemplate(text string) (*template.Template, error) {
	tmpl, err := template.New("--template").Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, &TransformError{Mode: "template", Stage: "parse", Err: err}
	}
	return tmpl, nil
}

// decodeStableJSON preserves json.Number values for transform execution so
// large integers keep the same text form that stable JSON emitted. It also
// rejects trailing values because transforms must consume one stable JSON
// document, not a stream.
func decodeStableJSON(stableJSON []byte, mode string) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(stableJSON))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, &TransformError{Mode: mode, Stage: "run", Err: err}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("invalid JSON after top-level value")
		}
		return nil, &TransformError{Mode: mode, Stage: "run", Err: err}
	}
	return value, nil
}
