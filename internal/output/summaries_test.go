package output_test

import (
	"testing"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/output"
)

func TestCapabilitySummary(t *testing.T) {
	if got := output.CapabilitySummary(nil); got != "none reported" {
		t.Fatalf("empty summary = %q", got)
	}
	caps := []auth.Capability{
		{ID: "projects", Read: true, Write: true},
		{ID: "credits", Read: true},
	}
	if got, want := output.CapabilitySummary(caps), "projects (read/write), credits (read)"; got != want {
		t.Fatalf("CapabilitySummary() = %q, want %q", got, want)
	}
}

func TestProjectScopeSummary(t *testing.T) {
	if got := output.ProjectScopeSummary(auth.ProjectScope{Mode: "all_projects"}); got != "all_projects" {
		t.Fatalf("scope without selection = %q", got)
	}
	scope := auth.ProjectScope{
		Mode:          "selected_projects",
		SelectedCount: 5,
		SelectedProjects: &auth.SelectedProjects{
			HasMore: true,
		},
	}
	if got, want := output.ProjectScopeSummary(scope), "selected_projects (5 selected, capped summary)"; got != want {
		t.Fatalf("ProjectScopeSummary() = %q, want %q", got, want)
	}
}
