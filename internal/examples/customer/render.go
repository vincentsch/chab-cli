package customer

import "github.com/vincentsch/chab-cli/internal/output"

// customerJSON is the stable JSON shape owned by the customer example module:
// it is used for --json, jq/template input, and the human renderers.
type customerJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// usageError marks local customer-command validation failures as exit code 1.
type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "customer command error"
	}
	return "customer command error: " + e.detail
}

func (e *usageError) ExitCode() int {
	return 1
}

// customerTable keeps list output intentionally compact; the full stable shape
// remains available to JSON, jq, and template consumers.
func customerTable(rows []customerJSON) output.Table {
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{row.ID, row.Name, row.Status, row.CreatedAt})
	}
	return output.Table{
		Columns: []string{"ID", "NAME", "STATUS", "CREATED"},
		Rows:    tableRows,
		Empty:   "No customers found.",
	}
}

// customerDetailNodes keeps mutation summaries in the same field order as the
// documented JSON object.
func customerDetailNodes(c customerJSON) []output.Node {
	return []output.Node{
		output.Field("id", c.ID),
		output.Field("name", c.Name),
		output.Field("status", c.Status),
		output.Field("created_at", c.CreatedAt),
	}
}
