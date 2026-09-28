package projects

import (
	"strconv"

	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

type projectJSON = readservice.Project

// usageError marks local project-command validation failures as exit code 1.
type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "project command error"
	}
	return "project command error: " + e.detail
}

func (e *usageError) ExitCode() int {
	return 1
}

// projectTable selects the compact columns for list output. Full project
// fields stay available through project show and machine output.
func projectTable(rows []projectJSON) output.Table {
	tableRows := make([][]string, 0, len(rows))
	for _, p := range rows {
		tableRows = append(tableRows, []string{p.ID, p.Name, p.Status, optionalCell(p.URL), p.UpdatedAt})
	}
	return output.Table{
		Columns: []string{"ID", "NAME", "STATUS", "URL", "UPDATED"},
		Rows:    tableRows,
		Empty:   "No projects found.",
	}
}

// projectDetailNodes keeps the human detail field order aligned with the
// documented project JSON shape.
func projectDetailNodes(p projectJSON) []output.Node {
	return []output.Node{
		output.Field("id", p.ID),
		output.Field("name", p.Name),
		output.Field("description", optionalDetail(p.Description)),
		output.Field("url", optionalDetail(p.URL)),
		output.Field("status", p.Status),
		output.Field("timezone", p.Timezone),
		output.Field("language", p.Language),
		output.Field("limit", strconv.FormatInt(p.Limit, 10)),
		output.Field("automate", strconv.FormatBool(p.Automate)),
		output.Field("created_at", p.CreatedAt),
		output.Field("updated_at", p.UpdatedAt),
	}
}

func optionalCell(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalDetail(value *string) string {
	if value == nil {
		return "null"
	}
	return *value
}
