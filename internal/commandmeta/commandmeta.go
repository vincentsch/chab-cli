// Package commandmeta defines neutral command catalog metadata shared by the
// CLI shell and feature packages.
//
// Feature packages use these names to describe catalog entries without
// importing internal/cli. The cli package aliases them for compatibility with
// existing catalog consumers.
package commandmeta

import (
	"fmt"

	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/rungrad/manifest"
)

// CommandStatus describes whether a catalog entry is executable today.
type CommandStatus string

const (
	StatusFunctional CommandStatus = "functional"
	StatusReserved   CommandStatus = "reserved"
	StatusAlias      CommandStatus = "alias"
)

// ExtensionNamespace is the stable namespace for Chab's machine-visible
// product metadata in the rungrad manifest. It satisfies rungrad's namespace
// pattern and avoids the reserved rungrad/ and rungrad. prefixes.
const ExtensionNamespace = "chab/saas-cli"

// OutputMode names a successful output mode implemented by a command.
// OutputHuman through OutputTemplate are rungrad canonical output tokens.
// OutputHelp and OutputShellScript are documentation-only categories and must
// stay in Sidecar.DocOnlyOutputModes.
type OutputMode string

const (
	OutputHuman       OutputMode = "human"
	OutputJSON        OutputMode = "json"
	OutputPlain       OutputMode = "plain"
	OutputJQ          OutputMode = "jq"
	OutputTemplate    OutputMode = "template"
	OutputHelp        OutputMode = "help"
	OutputShellScript OutputMode = "shell-script"
)

// HelpRequirement names extra help content a command promises to carry.
type HelpRequirement string

const (
	HelpAPIKeySetup  HelpRequirement = "api-key-setup"
	HelpPagination   HelpRequirement = "pagination"
	HelpDryRun       HelpRequirement = "dry-run"
	HelpConfirmation HelpRequirement = "confirmation"
	HelpIdempotency  HelpRequirement = "idempotency"
	HelpMetadata     HelpRequirement = "metadata"
	HelpRawEnvelope  HelpRequirement = "raw-envelope"
	HelpUpdateCheck  HelpRequirement = "update-check"
)

// Sidecar holds Chab-only command metadata that rungrad.CommandSpec does not
// own. DocPath is filled by the CLI shell's catalog merge. Module authors
// should leave it empty in CatalogEntries results.
type Sidecar struct {
	Owner                string
	Status               CommandStatus
	DocPath              string
	DocOnlyOutputModes   []OutputMode
	FrameworkOwned       bool
	SupportsDryRun       bool
	RequiresConfirmation bool
	HelpRequirements     []HelpRequirement
}

// Entry pairs rungrad-owned generic command metadata with Chab-only sidecar
// metadata.
type Entry struct {
	Spec    rungrad.CommandSpec
	Sidecar Sidecar
}

// SidecarExtensionObject projects the machine-visible sidecar facts into the
// chab/saas-cli extension object. Status is stored as a plain string so
// manifest.RequireExtensionEnum (which type-asserts value.(string)) accepts it;
// the named CommandStatus type would fail that assertion.
func SidecarExtensionObject(s Sidecar) manifest.ExtensionObject {
	return manifest.ExtensionObject{
		"owner":     s.Owner,
		"status":    string(s.Status),
		"docs_path": s.DocPath,
	}
}

// CanonicalOutputTokens converts readable Chab output-mode constants to the
// rungrad token slice expected by CommandSpec.OutputModes.
func CanonicalOutputTokens(modes ...OutputMode) []string {
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		switch mode {
		case OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate:
			out = append(out, string(mode))
		case OutputHelp, OutputShellScript:
			panic(fmt.Sprintf("commandmeta: %q is documentation-only, not a rungrad output mode", mode))
		default:
			panic(fmt.Sprintf("commandmeta: unsupported output mode %q", mode))
		}
	}
	return out
}

// RungradSpecs returns deep-copied rungrad specs for non-framework-owned
// catalog rows.
func RungradSpecs(entries []Entry) []rungrad.CommandSpec {
	out := make([]rungrad.CommandSpec, 0, len(entries))
	for _, entry := range entries {
		if entry.Sidecar.FrameworkOwned {
			continue
		}
		out = append(out, CloneRungradSpec(entry.Spec))
	}
	return out
}

// CloneRungradSpec deep-copies the reference fields on a rungrad CommandSpec.
func CloneRungradSpec(spec rungrad.CommandSpec) rungrad.CommandSpec {
	spec.OutputModes = append([]string(nil), spec.OutputModes...)
	spec.Examples = append([]string(nil), spec.Examples...)
	spec.Related = append([]string(nil), spec.Related...)
	spec.Extensions = CloneExtensions(spec.Extensions)
	return spec
}

// CloneExtensions deep-copies the extension set shape Chab emits: the set and
// each namespace's object. Projected values are immutable strings.
func CloneExtensions(in manifest.ExtensionSet) manifest.ExtensionSet {
	if in == nil {
		return nil
	}
	out := make(manifest.ExtensionSet, len(in))
	for ns, obj := range in {
		if obj == nil {
			out[ns] = nil
			continue
		}
		cloned := make(manifest.ExtensionObject, len(obj))
		for field, value := range obj {
			cloned[field] = value
		}
		out[ns] = cloned
	}
	return out
}

// CloneEntry deep-copies an Entry.
func CloneEntry(entry Entry) Entry {
	entry.Spec = CloneRungradSpec(entry.Spec)
	entry.Sidecar.DocOnlyOutputModes = append([]OutputMode(nil), entry.Sidecar.DocOnlyOutputModes...)
	entry.Sidecar.HelpRequirements = append([]HelpRequirement(nil), entry.Sidecar.HelpRequirements...)
	return entry
}
