package customer

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/rungrad"
)

// NewCustomerCommand builds chab customer for test-owned command trees.
func NewCustomerCommand(f *cmdutil.Factory) *rungrad.Command {
	cmd := &rungrad.Command{
		Use:   "customer",
		Short: "Manage example customers",
		Long: `Manage example customers through a teaching artifact for the SaaS CLI
authoring guide, compiled only into test command trees; it is not Chab-SaaS
product behavior.

Related commands:
  chab customer list
  chab customer create`,
		Args: cobra.ArbitraryArgs,
		Configure: func(cmd *cobra.Command) {
			cmd.Example = `  chab customer list
  chab customer create --name "Demo"`
		},
		Run: func(_ *rungrad.Factory, cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(NewListCommand(f), NewCreateCommand(f))
	return cmd
}
