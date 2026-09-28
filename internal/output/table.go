package output

import (
	"fmt"
	"io"
	"strings"
)

// Table renders compact aligned list output.
type Table struct {
	Columns []string
	Rows    [][]string
	Empty   string
}

// Render writes the table or its deterministic empty state.
func (t Table) Render(w io.Writer) {
	if len(t.Rows) == 0 {
		if t.Empty != "" {
			fmt.Fprintln(w, t.Empty)
		}
		return
	}
	if len(t.Columns) == 0 {
		return
	}

	header := make([]string, len(t.Columns))
	widths := make([]int, len(t.Columns))
	for i, column := range t.Columns {
		// Alignment is intentionally byte-count based for now. Multi-byte or
		// wide cells may under- or over-pad in terminals, while content order and
		// output-boundary sanitization remain stable.
		header[i] = humanCell(column)
		widths[i] = len(header[i])
	}
	normalized := make([][]string, len(t.Rows))
	for rowIndex, row := range t.Rows {
		normalized[rowIndex] = normalizeRow(row, len(t.Columns))
		for i, cell := range normalized[rowIndex] {
			safeCell := humanCell(cell)
			normalized[rowIndex][i] = safeCell
			if len(safeCell) > widths[i] {
				widths[i] = len(safeCell)
			}
		}
	}

	renderCells(w, header, widths)
	for _, row := range normalized {
		renderCells(w, row, widths)
	}
}

// RenderPlain writes a TSV table. Empty tables write no data and route the
// friendly empty-state sentence to prose.
func (t Table) RenderPlain(data, prose io.Writer) {
	if len(t.Rows) == 0 {
		if t.Empty != "" {
			fmt.Fprintln(prose, plainCell(t.Empty))
		}
		return
	}
	if len(t.Columns) == 0 {
		return
	}

	writePlainCells(data, t.Columns)
	for _, row := range t.Rows {
		writePlainCells(data, normalizeRow(row, len(t.Columns)))
	}
}

func normalizeRow(row []string, count int) []string {
	out := make([]string, count)
	copy(out, row)
	return out
}

func renderCells(w io.Writer, cells []string, widths []int) {
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == len(cells)-1 {
			b.WriteString(cell)
			continue
		}
		b.WriteString(cell)
		b.WriteString(strings.Repeat(" ", widths[i]-len(cell)))
	}
	// Short rows can leave empty trailing cells; trim only the padding, not the
	// cell content already written for the last non-empty column.
	fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
}

func writePlainCells(w io.Writer, cells []string) {
	for i, cell := range cells {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, plainCell(cell))
	}
	fmt.Fprintln(w)
}
