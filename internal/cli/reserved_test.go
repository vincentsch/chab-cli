package cli_test

import (
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestProductionCatalogHasNoReservedCommands(t *testing.T) {
	for _, entry := range cli.Catalog() {
		if entry.Sidecar.Status == cli.StatusReserved {
			t.Fatalf("%s remains reserved", entry.Spec.Path)
		}
	}
}

func TestCreditsTransactionsHelpExposesActiveFlags(t *testing.T) {
	result := testutil.RunCommand(t, "credits", "transactions", "--help")
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("credits transactions help result = %#v", result)
	}
	for _, flag := range []string{"--limit", "--all", "--cursor", "--page-size"} {
		if !strings.Contains(result.Stdout, flag) {
			t.Fatalf("credits transactions help missing active %s:\n%s", flag, result.Stdout)
		}
	}
	for _, flag := range []string{"`--type`", "`--since`", "`--until`", "`--sort`", "`--page`", "`--per-page`"} {
		if strings.Contains(result.Stdout, flag) {
			t.Fatalf("credits transactions help contains unsupported %s:\n%s", flag, result.Stdout)
		}
	}
	for _, want := range []string{"--include-meta", "safe transport context", "under data"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("credits transactions help missing %q:\n%s", want, result.Stdout)
		}
	}
	for _, forbidden := range []string{cli.LeafReservedSentence, "none (planned command)", "planned for a later release"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("credits transactions help contains %q:\n%s", forbidden, result.Stdout)
		}
	}

	root := cli.NewRootCommand(io.Discard, io.Discard)
	command, remaining, err := root.Find([]string{"credits", "transactions"})
	if err != nil || len(remaining) != 0 || command == nil {
		t.Fatalf("Find(credits transactions) = %v, %v, %v", command, remaining, err)
	}
	var localFlags []string
	command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name != "help" {
			localFlags = append(localFlags, flag.Name)
		}
	})
	sort.Strings(localFlags)
	wantFlags := []string{"all", "cursor", "limit", "page-size"}
	if !reflect.DeepEqual(localFlags, wantFlags) {
		t.Fatalf("credits transactions local flags = %v, want %v", localFlags, wantFlags)
	}
}
