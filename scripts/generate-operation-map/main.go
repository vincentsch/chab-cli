package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/mcpserver"
	"github.com/vincentsch/chab-cli/internal/operationmap"
)

const outputPath = "docs/operation-map.md"

func main() {
	check := flag.Bool("check", false, "check that docs/operation-map.md is current")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "generate-operation-map: unexpected positional arguments")
		os.Exit(1)
	}
	data, err := generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		current, err := os.ReadFile(outputPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !bytes.Equal(current, data) {
			fmt.Fprintf(os.Stderr, "%s is stale; run go run ./scripts/generate-operation-map\n", outputPath)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() ([]byte, error) {
	registry, err := chabcontract.Load()
	if err != nil {
		return nil, err
	}
	bindings, err := operationmap.Bindings(registry)
	if err != nil {
		return nil, err
	}
	if err := validateSurfaces(bindings); err != nil {
		return nil, err
	}
	var b strings.Builder
	provenance := registry.Provenance()
	fmt.Fprintf(&b, "# Chab Operation Map\n\n")
	fmt.Fprintf(&b, "Generated from the pinned public API fixture at backend revision `%s`.\n\n", provenance.BackendRevision)
	fmt.Fprintf(&b, "The Local MCP column shows registered tools for a signed-in API key. With a browser-issued guest trial credential, discovery and calls are constrained by the live `/v1/cli/compatibility` guest-local-MCP list for the full beta catalog, plus identity, credits, own-file/model discovery, current-operation status/result/artifact/download/cancel and local action helpers. Actual health, scopes, allowances and the shared free budget govern starts. Guest credentials do not authorize management, connected-account, billing, operation-history or hosted MCP tools.\n\n")
	fmt.Fprintf(&b, "| Operation | Method | Path | Owner | CLI | Local MCP | Availability | Behavior | Output | Scope | Idempotency |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	bindingByID := map[string]operationmap.Binding{}
	for _, binding := range bindings {
		bindingByID[binding.OperationID] = binding
	}
	for _, op := range registry.Operations() {
		binding := bindingByID[op.ID]
		scope := op.RequiredScope
		if scope == "" {
			scope = "-"
		}
		idempotency := "no"
		if op.IdempotencyRequired {
			idempotency = "yes"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s | %s | %s | %s | %s | %s | `%s` | %s |\n",
			escape(op.ID),
			escape(op.Method),
			escape(op.Path),
			escape(binding.Owner),
			operationmap.RenderSurface(binding.CLI),
			operationmap.RenderSurface(binding.MCP),
			escape(op.Availability),
			escape(op.Behavior),
			escape(op.OutputKind),
			escape(scope),
			idempotency,
		)
	}
	return []byte(b.String()), nil
}

func validateSurfaces(bindings []operationmap.Binding) error {
	root, _, err := cli.NewRootCommandWithCatalog(io.Discard, io.Discard, cli.Options{
		LookupEnv:        func(string) (string, bool) { return "", false },
		Stdin:            strings.NewReader(""),
		StdinIsTerminal:  func() bool { return false },
		StdoutIsTerminal: func() bool { return false },
		TerminalHeight:   func() (int, bool) { return 0, false },
	})
	if err != nil {
		return err
	}
	commands := map[string]bool{}
	for _, node := range cli.VisibleCommands(root, false) {
		commands[node.Path] = true
	}
	tools := map[string]bool{}
	for _, name := range mcpserver.ToolNames() {
		tools[name] = true
	}
	var problems []string
	for _, binding := range bindings {
		if binding.CLI.State == operationmap.StateImplemented && binding.CLI.Command != "" && !commands[binding.CLI.Command] {
			problems = append(problems, fmt.Sprintf("%s CLI command %q is not registered", binding.OperationID, binding.CLI.Command))
		}
		if binding.MCP.State == operationmap.StateImplemented && binding.MCP.Tool != "" && !tools[binding.MCP.Tool] {
			problems = append(problems, fmt.Sprintf("%s MCP tool %q is not registered", binding.OperationID, binding.MCP.Tool))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return fmt.Errorf("operation map binding drift:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

func escape(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
