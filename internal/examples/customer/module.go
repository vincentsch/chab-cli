package customer

import (
	"github.com/vincentsch/chab-cli/internal/commandmeta"
	"github.com/vincentsch/chab-cli/internal/feature"
	"github.com/vincentsch/rungrad"
)

// customersGroup gives the guide its own visible help section. Product resource
// modules normally use feature.GroupResources unless they add a new root family.
var customersGroup = feature.RootGroup{ID: "customers", Title: "Customers:"}

// module is the compiled-in customer example module, not a runtime plugin.
type module struct{}

// NewModule returns the customer example feature module for tests and docs
// demonstrations. Production code must not register this module.
func NewModule() feature.FeatureModule {
	return module{}
}

func (module) Groups() []feature.RootGroup {
	return []feature.RootGroup{customersGroup}
}

// Commands returns the customer family parent.
func (module) Commands(runtime feature.ModuleRuntime) ([]*rungrad.Command, error) {
	cmd := NewCustomerCommand(runtime.Factory)
	cmd.GroupID = customersGroup.ID
	return []*rungrad.Command{cmd}, nil
}

// CatalogEntries returns the customer example catalog rows. DocPath is
// deliberately left empty; the CLI catalog merge derives it for the tree.
func (module) CatalogEntries() []commandmeta.Entry {
	modes := commandmeta.CanonicalOutputTokens(commandmeta.OutputHuman, commandmeta.OutputJSON, commandmeta.OutputPlain, commandmeta.OutputJQ, commandmeta.OutputTemplate)
	return []commandmeta.Entry{
		{Spec: rungrad.CommandSpec{Path: "customer", Summary: "Manage example customers", GroupID: customersGroup.ID}, Sidecar: commandmeta.Sidecar{Owner: "customer example module", Status: commandmeta.StatusFunctional, DocOnlyOutputModes: []commandmeta.OutputMode{commandmeta.OutputHelp}}},
		{Spec: rungrad.CommandSpec{Path: "customer create", Summary: "Create an example customer", OutputModes: modes, RequiresAuth: true, SupportsMeta: true, Mutates: true}, Sidecar: commandmeta.Sidecar{Owner: "customer example module", Status: commandmeta.StatusFunctional, SupportsDryRun: true, HelpRequirements: []commandmeta.HelpRequirement{commandmeta.HelpDryRun, commandmeta.HelpIdempotency, commandmeta.HelpMetadata}}},
		{Spec: rungrad.CommandSpec{Path: "customer list", Summary: "List example customers", OutputModes: modes, RequiresAuth: true, SupportsMeta: true}, Sidecar: commandmeta.Sidecar{Owner: "customer example module", Status: commandmeta.StatusFunctional, HelpRequirements: []commandmeta.HelpRequirement{commandmeta.HelpPagination, commandmeta.HelpMetadata}}},
	}
}
