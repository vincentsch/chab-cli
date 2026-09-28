package output

import (
	"fmt"
	"io"
	"strings"
)

// Node is one renderable line with optional nested children.
type Node struct {
	Label string
	Value string
	// hasValue distinguishes Field("Label", "") from Section("Label").
	hasValue bool
	// Prose marks guidance text that stays in default human output but moves
	// to stderr for --plain so stdout remains data-only.
	Prose    bool
	Children []Node
}

// Line returns a raw output line.
func Line(text string) Node {
	return Node{Value: text}
}

// Guidance returns a prose line. Default human rendering matches Line, while
// plain rendering routes prose to stderr.
func Guidance(text string) Node {
	return Node{Value: text, Prose: true}
}

// Field returns a labeled output line.
func Field(label, value string) Node {
	return Node{Label: label, Value: value, hasValue: true}
}

// Section returns a labeled section with nested children.
func Section(label string, children ...Node) Node {
	return Node{Label: label, Children: children}
}

// Detail renders ordered nodes with two-space indentation per depth level.
type Detail struct {
	Nodes []Node
}

// Render writes the human detail document.
func (d Detail) Render(w io.Writer) {
	for _, node := range d.Nodes {
		renderNode(w, node, 0)
	}
}

// RenderPlain writes copy-safe detail data as label/value TSV lines. Prose
// nodes are routed to prose.
func (d Detail) RenderPlain(data, prose io.Writer) {
	for _, node := range d.Nodes {
		renderPlainNode(data, prose, node, nil, false)
	}
}

func renderNode(w io.Writer, node Node, depth int) {
	indent := strings.Repeat("  ", depth)
	switch {
	case nodeHasValue(node):
		fmt.Fprintf(w, "%s%s: %s\n", indent, humanCell(node.Label), humanCell(node.Value))
	case node.Label != "":
		fmt.Fprintf(w, "%s%s:\n", indent, humanCell(node.Label))
	default:
		fmt.Fprintf(w, "%s%s\n", indent, humanCell(node.Value))
	}

	for _, child := range node.Children {
		renderNode(w, child, depth+1)
	}
}

// renderPlainNode maps the nested human detail tree to flat plain rows. prefix
// carries the dotted key built from parent labels.
func renderPlainNode(data, prose io.Writer, node Node, prefix []string, proseMode bool) {
	if proseMode || node.Prose {
		// Once a node is prose, its whole subtree stays on the prose stream so
		// plain stdout remains data-only.
		writeProseNode(prose, node)
		for _, child := range node.Children {
			renderPlainNode(data, prose, child, prefix, true)
		}
		return
	}

	switch {
	case nodeHasValue(node):
		parts := appendPrefix(prefix, node.Label)
		fmt.Fprintf(data, "%s\t%s\n", dotted(parts), plainCell(node.Value))
		for _, child := range node.Children {
			renderPlainNode(data, prose, child, parts, false)
		}
	case node.Label != "":
		// A label-only node is a section. It does not emit its own row in plain
		// mode, but its label becomes part of each child row's dotted key.
		parts := appendPrefix(prefix, node.Label)
		for _, child := range node.Children {
			renderPlainNode(data, prose, child, parts, false)
		}
	default:
		if len(prefix) == 0 {
			if node.Value != "" {
				fmt.Fprintln(data, plainCell(node.Value))
			}
		} else {
			fmt.Fprintf(data, "%s\t%s\n", dotted(prefix), plainCell(node.Value))
		}
		for _, child := range node.Children {
			renderPlainNode(data, prose, child, prefix, false)
		}
	}
}

func writeProseNode(w io.Writer, node Node) {
	switch {
	case nodeHasValue(node):
		fmt.Fprintf(w, "%s: %s\n", plainCell(node.Label), plainCell(node.Value))
	case node.Label != "":
		fmt.Fprintf(w, "%s:\n", plainCell(node.Label))
	case node.Value != "":
		fmt.Fprintln(w, plainCell(node.Value))
	}
}

func nodeHasValue(node Node) bool {
	return node.Label != "" && (node.hasValue || node.Value != "")
}

func appendPrefix(prefix []string, label string) []string {
	next := make([]string, 0, len(prefix)+1)
	next = append(next, prefix...)
	next = append(next, plainCell(label))
	return next
}

func dotted(parts []string) string {
	return strings.Join(parts, ".")
}

// plainCell returns one safe TSV cell by removing control bytes and collapsing
// embedded tab/newline separators into spaces.
func plainCell(value string) string {
	// Tabs and newlines are structural in plain output, so collapse them inside
	// cells after control-byte sanitization.
	sanitized := sanitizeControlString(value)
	return strings.Join(strings.FieldsFunc(sanitized, func(r rune) bool {
		return r == '\t' || r == '\r' || r == '\n'
	}), " ")
}

func humanCell(value string) string {
	// A cell is one output record. Provider-controlled tabs and newlines must
	// not create sibling records even though the document renderer itself uses
	// those delimiters structurally.
	return SanitizeInlineText(value)
}
