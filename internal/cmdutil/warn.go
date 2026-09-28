package cmdutil

import (
	"fmt"
	"io"

	"github.com/vincentsch/chab-cli/internal/auth"
)

// WarnPermissionFindings writes one redacted warning line per broad auth-file
// permission finding.
func WarnPermissionFindings(w io.Writer, findings []auth.PermissionFinding) {
	for _, finding := range findings {
		fmt.Fprintf(w, "Warning: auth file %s has permissions %04o; expected %04o.\n",
			finding.Path, finding.ActualMode.Perm(), finding.ExpectedMode.Perm())
		fmt.Fprintf(w, "Tighten it with: chmod %03o %s\n", finding.ExpectedMode.Perm(), finding.Path)
	}
}
