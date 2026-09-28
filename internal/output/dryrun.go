package output

import (
	"encoding/json"
	"io"
)

const redactedValue = "[REDACTED]"

// DryRunPreview describes the request a write command would send without
// contacting the API. It is built from values the command already resolved, so
// rendering a preview never needs credentials, flags, environment variables, or
// network access.
type DryRunPreview struct {
	Method      string             `json:"method"`
	Path        string             `json:"path"`
	Query       []DryRunValue      `json:"query,omitempty"`
	Body        []DryRunValue      `json:"body,omitempty"`
	Idempotency *DryRunIdempotency `json:"idempotency,omitempty"`
	Retry       *DryRunRetry       `json:"retry,omitempty"`
}

// DryRunValue is one query parameter or body field. Mark Secret when the raw
// value may contain a user secret; all preview renderers then show [REDACTED].
type DryRunValue struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

// DryRunIdempotency describes idempotency behavior for a previewed write. A
// generated key is described without creating the eventual random value.
type DryRunIdempotency struct {
	Source string `json:"source"`
}

// DryRunRetry describes whether the previewed request would use automatic
// retry. It is omitted for commands whose existing preview shape does not
// expose retry policy.
type DryRunRetry struct {
	Automatic bool   `json:"automatic"`
	Source    string `json:"source"`
}

// MarshalJSON masks secret-marked values in a copy so preview JSON cannot
// serialize raw secrets. The shared JSON writer still checks the final bytes
// for secret-looking text after this method returns.
func (p DryRunPreview) MarshalJSON() ([]byte, error) {
	type alias DryRunPreview
	safe := alias(p)
	safe.Query = maskValues(p.Query)
	safe.Body = maskValues(p.Body)
	return json.Marshal(safe)
}

// Render writes the human detail view.
func (p DryRunPreview) Render(w io.Writer) {
	Detail{Nodes: p.nodes()}.Render(w)
}

// RenderPlain writes the copy-safe view.
func (p DryRunPreview) RenderPlain(data, prose io.Writer) {
	Detail{Nodes: p.nodes()}.RenderPlain(data, prose)
}

func maskValues(in []DryRunValue) []DryRunValue {
	if in == nil {
		return nil
	}
	out := make([]DryRunValue, len(in))
	for i, v := range in {
		out[i] = v
		if v.Secret {
			out[i].Value = redactedValue
		}
	}
	return out
}

// nodes builds one detail tree for both human and plain output so the two
// views cannot drift apart as later commands add dry-run consumers.
func (p DryRunPreview) nodes() []Node {
	nodes := []Node{Field("Method", p.Method), Field("Path", p.Path)}
	if len(p.Query) > 0 {
		nodes = append(nodes, Section("Query", valueNodes(p.Query)...))
	}
	if len(p.Body) > 0 {
		nodes = append(nodes, Section("Body", valueNodes(p.Body)...))
	}
	if p.Idempotency != nil {
		nodes = append(nodes, p.Idempotency.node())
	}
	if p.Retry != nil {
		nodes = append(nodes, p.Retry.node())
	}
	return nodes
}

func valueNodes(values []DryRunValue) []Node {
	out := make([]Node, 0, len(values))
	for _, v := range values {
		out = append(out, Field(v.Name, v.display()))
	}
	return out
}

func (v DryRunValue) display() string {
	if v.Secret {
		return redactedValue
	}
	return v.Value
}

func (i DryRunIdempotency) node() Node {
	switch {
	case i.Source == "generated":
		return Field("Idempotency key", "generated on send")
	case i.Source == "explicit":
		// Explicit keys are acknowledged but not rendered; dry-run output can
		// be saved or shared and should not expose retry correlation values.
		return Field("Idempotency key", "explicit")
	case i.Source == "suppressed":
		return Field("Idempotency key", "not sent (route is not replayable)")
	default:
		return Field("Idempotency", i.Source)
	}
}

func (r DryRunRetry) node() Node {
	switch {
	case !r.Automatic && r.Source == "disabled_non_replayable":
		return Field("Automatic retry", "disabled (route is not replayable)")
	default:
		return Field("Automatic retry", r.Source)
	}
}
