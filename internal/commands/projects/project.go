package projects

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewProjectCommand builds the functional Project command family. Bare
// invocation shows help; an unmatched subcommand becomes a typed usage error.
func NewProjectCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
		Long: `Manage projects visible to the active team API key.

List projects with shared pagination and API-authoritative filters, show one by
an opaque id, create or update projects, use fixed-status pause, resume, and
archive mutations, or delete projects.

Related commands:
  chab login
  chab whoami
  chab doctor`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.Example = `  chab project list
  chab project show <project-id>
  chab project create --name "Demo"
  chab project update <project-id> --status paused
  chab project pause <project-id>
  chab project resume <project-id>
  chab project archive <project-id>
  chab project delete <project-id>`
	cmd.AddCommand(
		NewArchiveCommand(f),
		NewCreateCommand(f),
		NewDeleteCommand(f),
		NewListCommand(f),
		NewPauseCommand(f),
		NewResumeCommand(f),
		NewShowCommand(f),
		NewUpdateCommand(f),
	)
	return cmd
}
