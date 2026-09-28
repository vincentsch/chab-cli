package doctor

import (
	"fmt"
	"io"
	"strconv"

	"github.com/vincentsch/chab-cli/internal/output"
)

// finding is one stable doctor result. IDs stay within the documented
// readiness vocabulary so automation can match them directly.
type finding struct {
	ID        string `json:"id"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// report is the complete doctor JSON payload and the source for human output.
type report struct {
	Status   string    `json:"status"`
	Findings []finding `json:"findings"`
}

// failError is returned after the report has already been rendered when one or
// more findings are hard readiness failures.
type failError struct {
	count int
}

func (e failError) Error() string {
	return fmt.Sprintf("doctor found %d failing readiness check(s)", e.count)
}

func (e failError) ExitCode() int {
	return 1
}

// renderHuman prints findings in scan-friendly order and suppresses duplicate
// request-id detail when RequestID already has its own line.
func renderHuman(w io.Writer, r report) {
	for _, f := range r.Findings {
		label := "OK"
		switch f.Severity {
		case "warning":
			label = "WARN"
		case "fail":
			label = "FAIL"
		}
		fmt.Fprintf(w, "%s  %s  %s\n", label, f.ID, f.Summary)
		if f.Detail != "" && !(f.RequestID != "" && f.Detail == "request id "+f.RequestID) {
			fmt.Fprintf(w, "  %s\n", f.Detail)
		}
		if f.RequestID != "" {
			fmt.Fprintf(w, "  Request ID: %s\n", f.RequestID)
		}
	}
	ok, warning, fail := countSeverities(r.Findings)
	fmt.Fprintf(w, "Summary: %s (%d ok, %d warning, %d fail)\n", r.Status, ok, warning, fail)
}

// renderPlain keeps findings and the summary in one fixed TSV schema. Fields
// that do not apply to a row type are intentionally empty cells.
func renderPlain(data, prose io.Writer, r report) {
	ok, warning, fail := countSeverities(r.Findings)
	rows := make([][]string, 0, len(r.Findings)+1)
	for _, f := range r.Findings {
		rows = append(rows, []string{
			"finding",
			f.ID,
			f.Severity,
			f.Summary,
			f.Detail,
			f.RequestID,
			"",
			"",
			"",
			"",
		})
	}
	rows = append(rows, []string{
		"summary",
		"",
		"",
		"",
		"",
		"",
		r.Status,
		strconv.Itoa(ok),
		strconv.Itoa(warning),
		strconv.Itoa(fail),
	})
	output.Table{
		Columns: []string{"record", "id", "severity", "summary", "detail", "request_id", "status", "ok", "warning", "fail"},
		Rows:    rows,
	}.RenderPlain(data, prose)
}

func countSeverities(findings []finding) (ok, warning, fail int) {
	for _, f := range findings {
		switch f.Severity {
		case "fail":
			fail++
		case "warning":
			warning++
		default:
			ok++
		}
	}
	return ok, warning, fail
}

// finishReport derives the top-level status from finding severities.
func finishReport(findings []finding) (report, int) {
	_, _, failures := countSeverities(findings)
	status := "pass"
	if failures > 0 {
		status = "fail"
	}
	return report{Status: status, Findings: findings}, failures
}
