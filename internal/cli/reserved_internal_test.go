package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSyntheticReservedCommandsOwnDurableBehavior(t *testing.T) {
	_, entries := syntheticReservedTree(t)
	if got := []string{entries[0].Spec.Path, entries[1].Spec.Path}; !reflect.DeepEqual(got, []string{"reserved", "reserved leaf"}) {
		t.Fatalf("synthetic reserved paths = %v", got)
	}

	for _, test := range []struct {
		args     []string
		sentence string
	}{
		{args: []string{"reserved"}, sentence: FamilyReservedSentence},
		{args: []string{"reserved", "--help"}, sentence: FamilyReservedSentence},
		{args: []string{"reserved", "leaf", "--help"}, sentence: LeafReservedSentence},
	} {
		stdout, stderr, err := executeFreshSynthetic(t, test.args...)
		if err != nil || stderr != "" {
			t.Fatalf("%v error = %v, stderr = %q", test.args, err, stderr)
		}
		for _, want := range []string{"Usage:", test.sentence, "Related commands:"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("%v help missing %q:\n%s", test.args, want, stdout)
			}
		}
		if strings.Contains(stdout, "Examples:") {
			t.Fatalf("%v reserved help contains examples:\n%s", test.args, stdout)
		}
	}

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.yml")
	authPath := filepath.Join(tmp, "auth.json")
	stdout, stderr, err := executeFreshSynthetic(t,
		"--config", configPath,
		"--auth-file", authPath,
		"--profile", "hostile-profile",
		"reserved", "leaf",
	)
	if stdout != "" || stderr != "" {
		t.Fatalf("reserved leaf wrote output before rendering: stdout=%q stderr=%q", stdout, stderr)
	}
	var reserved *ReservedCommandError
	if !errors.As(err, &reserved) || reserved.Path != "chab reserved leaf" {
		t.Fatalf("reserved leaf error = %T %v", err, err)
	}
	var rendered bytes.Buffer
	renderError(&rendered, err, false)
	if !strings.Contains(rendered.String(), `"chab reserved leaf" is not implemented yet`) ||
		!strings.Contains(rendered.String(), `Run "chab reserved leaf --help" for details.`) {
		t.Fatalf("rendered reserved error = %q", rendered.String())
	}
	for _, path := range []string{configPath, authPath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("reserved execution touched %s: %v", path, statErr)
		}
	}

	for _, args := range [][]string{
		{"reserved", "leaf", "--plain"},
		{"reserved", "leaf", "--jq", "."},
		{"reserved", "leaf", "--template", "{{.}}"},
	} {
		stdout, _, err = executeFreshSynthetic(t, args...)
		if err == nil || stdout != "" || !strings.Contains(err.Error(), "is not supported") {
			t.Fatalf("reserved output-mode result for %v: stdout=%q err=%v", args, stdout, err)
		}
	}
}

func syntheticReservedTree(t *testing.T) (*cobra.Command, []Entry) {
	t.Helper()
	entries := []Entry{
		{
			Spec: CommandSpec{Path: "reserved", Summary: "Manage reserved behavior"},
			Sidecar: Sidecar{
				Status:  StatusReserved,
				DocPath: "manual/commands/chab-reserved.md",
			},
		},
		{
			Spec: CommandSpec{Path: "reserved leaf", Summary: "Run reserved behavior"},
			Sidecar: Sidecar{
				Status:  StatusReserved,
				DocPath: "manual/commands/chab-reserved-leaf.md",
			},
		},
	}
	root := &cobra.Command{
		Use:           "chab",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	flags := &globalFlags{}
	registerGlobalFlags(root, flags)
	family := newReservedFamily(
		"reserved",
		entries[0].Spec.Summary,
		"Manage an exact synthetic reserved command family.",
		[]string{"chab help"},
	)
	family.AddCommand(newReservedLeaf(
		"leaf",
		entries[1].Spec.Summary,
		"Exercise an exact synthetic reserved leaf.",
		[]string{"chab reserved"},
	))
	root.AddCommand(family)
	installOutputModeGuard(root, entries)
	if err := validateTreeCatalog(root, entries); err != nil {
		t.Fatalf("synthetic tree/catalog mismatch: %v", err)
	}
	return root, entries
}

func executeSynthetic(root *cobra.Command, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func executeFreshSynthetic(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root, _ := syntheticReservedTree(t)
	return executeSynthetic(root, args...)
}
