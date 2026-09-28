// Package feature defines the compiled-in feature module contract for chab.
//
// Feature modules are ordinary Go values linked into the binary, not runtime
// plugins. Module lists are explicit values passed to command construction or
// execution; modules must not register themselves through init functions or
// blank imports. Runtime dependencies arrive through ModuleRuntime, so modules
// do not read environment variables, config/auth files, prompts, or HTTP
// settings directly. The root command owns persistent pre-run hooks such as
// the output-mode guard, so module commands must not set persistent hooks.
package feature

import (
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/commandmeta"
	"github.com/vincentsch/rungrad"
)

// RootGroup declares one root help group a command tree registers.
type RootGroup struct {
	ID    string
	Title string
}

var (
	GroupAuth      = RootGroup{ID: "auth", Title: "Authentication & identity:"}
	GroupResources = RootGroup{ID: "resources", Title: "Projects & credits:"}
	GroupConfig    = RootGroup{ID: "config", Title: "Profiles & configuration:"}
	GroupAPI       = RootGroup{ID: "api", Title: "Raw API access:"}
	GroupUtilities = RootGroup{ID: "utilities", Title: "Diagnostics & utilities:"}
)

// ShellOwnedGroups returns the root help groups the CLI shell registers
// unconditionally, in the golden-tested registration order.
func ShellOwnedGroups() []RootGroup {
	return []RootGroup{GroupAuth, GroupResources, GroupConfig, GroupAPI, GroupUtilities}
}

// ModuleRuntime carries the shared runtime dependencies a feature module's
// commands may use.
type ModuleRuntime struct {
	Factory *cmdutil.Factory
}

// FeatureModule is one compiled-in resource command group.
type FeatureModule interface {
	// Groups declares the root help groups the module's top-level commands
	// reference.
	Groups() []RootGroup
	// Commands returns the module's native rungrad commands, setting GroupID on
	// top-level commands. Modules must not set persistent pre-run hooks; the
	// root command owns invocation-wide guards.
	Commands(runtime ModuleRuntime) ([]*rungrad.Command, error)
	// CatalogEntries returns fresh catalog rows with Sidecar.DocPath left empty.
	// Entry.Spec is the rungrad-owned generic command contract; Entry.Sidecar
	// contains Chab-only metadata.
	CatalogEntries() []commandmeta.Entry
}

// RungradSpecModule adapts Chab feature metadata into a spec-only rungrad
// module. Command construction is native rungrad, but catalog metadata remains
// Chab-owned and is registered separately for validation.
func RungradSpecModule(name string, groups []RootGroup, entries []commandmeta.Entry) rungrad.FeatureModule {
	return rungradSpecModule{name: name, groups: groups, entries: entries}
}

// rungradSpecModule is deliberately metadata-only. It lets rungrad build and
// validate a catalog view of Chab commands without taking over command
// construction or execution.
type rungradSpecModule struct {
	name    string
	groups  []RootGroup
	entries []commandmeta.Entry
}

func (m rungradSpecModule) Groups() []rungrad.Group {
	out := make([]rungrad.Group, len(m.groups))
	for i, group := range m.groups {
		out[i] = rungrad.Group{ID: group.ID, Title: group.Title}
	}
	return out
}

func (m rungradSpecModule) Commands() []*rungrad.Command {
	return nil
}

func (m rungradSpecModule) Catalog() []rungrad.CommandSpec {
	return commandmeta.RungradSpecs(m.entries)
}
