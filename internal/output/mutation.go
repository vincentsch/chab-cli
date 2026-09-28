package output

import (
	"fmt"
	"io"
)

// MutationSummary renders a concise human summary for write operations.
type MutationSummary struct {
	Action   string
	Resource string
	Name     string
	Fields   []Node
	Notes    []Node
}

// Render writes the mutation summary.
func (m MutationSummary) Render(w io.Writer) {
	headline := stringsJoinNonEmpty(" ", m.Action, m.Resource, m.Name)
	nodes := make([]Node, 0, 1+len(m.Notes))
	if headline != "" {
		// Fields describe the mutated resource, so they hang from the headline.
		// Callers should provide at least one headline part when fields matter.
		nodes = append(nodes, Node{Value: headline, Children: m.Fields})
	}
	nodes = append(nodes, m.Notes...)
	Detail{Nodes: nodes}.Render(w)
}

// RenderPlain writes a copy-safe mutation summary.
func (m MutationSummary) RenderPlain(data, prose io.Writer) {
	resource := stringsJoinNonEmpty(" ", m.Resource, m.Name)
	switch {
	case m.Action != "" && resource != "":
		fmt.Fprintf(data, "%s\t%s\n", plainCell(m.Action), plainCell(resource))
	case m.Action != "":
		fmt.Fprintln(data, plainCell(m.Action))
	case resource != "":
		fmt.Fprintln(data, plainCell(resource))
	}
	if len(m.Fields) > 0 {
		Detail{Nodes: m.Fields}.RenderPlain(data, prose)
	}
	if len(m.Notes) > 0 {
		Detail{Nodes: m.Notes}.RenderPlain(data, prose)
	}
}

func stringsJoinNonEmpty(sep string, parts ...string) string {
	out := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += part
	}
	return out
}
