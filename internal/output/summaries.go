package output

import (
	"fmt"
	"strings"

	"github.com/vincentsch/chab-cli/internal/auth"
)

// CapabilitySummary summarizes flat whoami capabilities for compact output.
func CapabilitySummary(caps []auth.Capability) string {
	if len(caps) == 0 {
		return "none reported"
	}
	parts := make([]string, 0, len(caps))
	for _, cap := range caps {
		abilities := make([]string, 0, 2)
		if cap.Read {
			abilities = append(abilities, "read")
		}
		if cap.Write {
			abilities = append(abilities, "write")
		}
		if len(abilities) == 0 {
			abilities = append(abilities, "no access")
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", cap.ID, strings.Join(abilities, "/")))
	}
	return strings.Join(parts, ", ")
}

// ProjectScopeSummary summarizes a project scope for compact output.
func ProjectScopeSummary(scope auth.ProjectScope) string {
	if scope.SelectedProjects == nil {
		return scope.Mode
	}
	suffix := ""
	if scope.SelectedProjects.HasMore {
		suffix = ", capped summary"
	}
	return fmt.Sprintf("%s (%d selected%s)", scope.Mode, scope.SelectedCount, suffix)
}
