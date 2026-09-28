package examplecheck

import (
	"fmt"
	"strings"

	"github.com/vincentsch/chab-cli/internal/cli"
)

// ValidateDrift checks every manifest command use against the Cobra tree and
// catalog.
func ValidateDrift(m Manifest, nodes []cli.CommandNode, entries []cli.Entry) []error {
	// Cobra and catalog paths are display strings. Split them back into command
	// tokens before keying so display formatting never defines path identity.
	nodeByPath := make(map[string]cli.CommandNode, len(nodes))
	for _, node := range nodes {
		nodeByPath[commandPathKey(strings.Fields(node.Path))] = node
	}
	entryByPath := make(map[string]cli.Entry, len(entries))
	for _, entry := range entries {
		entryByPath[commandPathKey(strings.Fields(entry.Spec.Path))] = entry
	}

	var errs []error
	for _, example := range m.Examples {
		for _, use := range example.Commands {
			path := commandPathText(use.Path)
			key := commandPathKey(use.Path)
			node, ok := nodeByPath[key]
			if !ok {
				errs = append(errs, fmt.Errorf("%s: unknown command path %q", example.Script, path))
				continue
			}
			entry, ok := entryByPath[key]
			if !ok {
				errs = append(errs, fmt.Errorf("%s: command %q has no catalog entry", example.Script, path))
				continue
			}
			if entry.Sidecar.Status != cli.StatusFunctional {
				errs = append(errs, fmt.Errorf("%s: command %q has catalog status %q, want %q", example.Script, path, entry.Sidecar.Status, cli.StatusFunctional))
			}
			for _, flagName := range use.Flags {
				if flagName == "" || strings.HasPrefix(flagName, "-") {
					errs = append(errs, fmt.Errorf("%s: command %q has invalid manifest flag %q", example.Script, path, flagName))
					continue
				}
				// Examples can use both command-local flags and inherited root flags
				// such as --profile, so validate against both Cobra flag sets.
				if node.Command.Flags().Lookup(flagName) == nil && node.Command.InheritedFlags().Lookup(flagName) == nil {
					errs = append(errs, fmt.Errorf("%s: command %q has no flag %q", example.Script, path, "--"+flagName))
				}
			}
		}
	}
	return errs
}
