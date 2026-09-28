package examplecheck

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cli"
)

func TestValidateDriftRejectsInvalidCommandContracts(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	nodes := cli.VisibleCommands(root, false)
	entries := cli.Catalog()
	tests := []struct {
		name string
		use  CommandUse
		want string
	}{
		{name: "unknown", use: CommandUse{Path: []string{"api", "lst"}}, want: `unknown command path "api lst"`},
		{name: "combined path element", use: CommandUse{Path: []string{"api get"}}, want: `unknown command path "api get"`},
		{name: "alias", use: CommandUse{Path: []string{"login"}}, want: `catalog status "alias"`},
		{name: "malformed flag", use: CommandUse{Path: []string{"api", "get"}, Flags: []string{"--json"}}, want: `invalid manifest flag "--json"`},
		{name: "unknown flag", use: CommandUse{Path: []string{"api", "get"}, Flags: []string{"lmit"}}, want: `has no flag "--lmit"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := Manifest{Examples: []Example{{Script: "examples/ci/test.sh", Commands: []CommandUse{tt.use}}}}
			got := joinErrors(ValidateDrift(manifest, nodes, entries))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("errors missing %q:\n%s", tt.want, got)
			}
		})
	}
}

func TestValidateDriftRejectsReservedAndMissingCatalogEntries(t *testing.T) {
	future := &cobra.Command{Use: "future"}
	future.Flags().Bool("local", false, "")
	nodes := []cli.CommandNode{{Path: "future", Command: future}}
	manifest := Manifest{Examples: []Example{{
		Script:   "examples/ci/future.sh",
		Commands: []CommandUse{{Path: []string{"future"}, Flags: []string{"local"}}},
	}}}
	entries := []cli.Entry{{
		Spec:    cli.CommandSpec{Path: "future"},
		Sidecar: cli.Sidecar{Status: cli.StatusReserved},
	}}
	if got := joinErrors(ValidateDrift(manifest, nodes, entries)); !strings.Contains(got, `catalog status "reserved"`) {
		t.Fatalf("reserved errors = %q", got)
	}
	if got := joinErrors(ValidateDrift(manifest, nodes, nil)); !strings.Contains(got, "has no catalog entry") {
		t.Fatalf("missing-catalog errors = %q", got)
	}
}

func TestValidateDriftAcceptsLocalAndInheritedFlags(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	manifest := Manifest{Examples: []Example{{
		Script: "examples/ci/good.sh",
		Commands: []CommandUse{{
			Path:  []string{"api", "get"},
			Flags: []string{"include-meta", "json", "query"},
		}},
	}}}
	if errs := ValidateDrift(manifest, cli.VisibleCommands(root, false), cli.Catalog()); len(errs) != 0 {
		t.Fatalf("valid drift errors = %#v", errs)
	}
}
