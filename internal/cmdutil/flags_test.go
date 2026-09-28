package cmdutil_test

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

func TestOutputFlagAccessorsUseChangedForTransforms(t *testing.T) {
	cmd := flagCommand()

	if expr, ok := cmdutil.JQExpr(cmd); ok || expr != "" {
		t.Fatalf("unset jq = %q, %t", expr, ok)
	}
	if text, ok := cmdutil.TemplateExpr(cmd); ok || text != "" {
		t.Fatalf("unset template = %q, %t", text, ok)
	}
	if cmdutil.MachineOutputEnabled(cmd) {
		t.Fatalf("unset flags reported machine output")
	}

	if err := cmd.Root().PersistentFlags().Set("jq", ""); err != nil {
		t.Fatal(err)
	}
	if expr, ok := cmdutil.JQExpr(cmd); !ok || expr != "" {
		t.Fatalf("empty jq = %q, %t, want changed empty", expr, ok)
	}
	if !cmdutil.MachineOutputEnabled(cmd) {
		t.Fatalf("changed jq did not report machine output")
	}

	cmd = flagCommand()
	if err := cmd.Root().PersistentFlags().Set("template", ""); err != nil {
		t.Fatal(err)
	}
	if text, ok := cmdutil.TemplateExpr(cmd); !ok || text != "" {
		t.Fatalf("empty template = %q, %t, want changed empty", text, ok)
	}
	if !cmdutil.MachineOutputEnabled(cmd) {
		t.Fatalf("changed template did not report machine output")
	}
}

func TestOutputBooleanFlagAccessors(t *testing.T) {
	cmd := flagCommand()
	if cmdutil.PlainEnabled(cmd) || cmdutil.NoColorEnabled(cmd) || cmdutil.NoANSIEnabled(cmd) || cmdutil.NoPagerEnabled(cmd) || cmdutil.YesEnabled(cmd) {
		t.Fatalf("unset booleans reported true")
	}
	for _, name := range []string{"plain", "no-color", "no-ansi", "no-pager", "json", "yes"} {
		if err := cmd.Root().PersistentFlags().Set(name, "true"); err != nil {
			t.Fatal(err)
		}
	}
	if !cmdutil.PlainEnabled(cmd) || !cmdutil.NoColorEnabled(cmd) || !cmdutil.NoANSIEnabled(cmd) || !cmdutil.NoPagerEnabled(cmd) || !cmdutil.YesEnabled(cmd) {
		t.Fatalf("set boolean accessors did not report true")
	}
	if !cmdutil.MachineOutputEnabled(cmd) {
		t.Fatalf("json did not report machine output")
	}
}

func flagCommand() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().Bool("json", false, "")
	root.PersistentFlags().String("jq", "", "")
	root.PersistentFlags().String("template", "", "")
	root.PersistentFlags().Bool("plain", false, "")
	root.PersistentFlags().Bool("no-color", false, "")
	root.PersistentFlags().Bool("no-ansi", false, "")
	root.PersistentFlags().Bool("no-pager", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	return child
}
