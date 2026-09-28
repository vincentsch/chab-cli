package projects

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

// NewShowCommand builds chab project show.
func NewShowCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <project-id>",
		Short: "Show a project",
		Long: `Show a project visible to the active team API key.

The project id is opaque: every non-empty value is sent to the API as one path
segment without trimming, decoding, normalization, or local shape validation.
Malformed, deleted, cross-team, and out-of-scope ids all return the API's
not_found response.

JSON output is one project object with fields id, name, description, url,
status, timezone, language, limit, automate, created_at, updated_at.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab project list
  chab credits`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, f, args[0])
		},
	}
	cmd.Example = `  chab project show <project-id>
  chab project show <project-id> --json
  chab project show <project-id> --json --include-meta
  chab project show <project-id> --jq .status
  chab project show <project-id> --template '{{.id}} {{.name}}'`
	return cmd
}

// runShow resolves credentials, fetches one project by opaque id, and renders
// the same stable project value used by JSON and transform output.
func runShow(cmd *cobra.Command, f *cmdutil.Factory, id string) error {
	// Project ids are opaque. Only an exactly empty value is rejected locally;
	// every other byte sequence is escaped as one path segment.
	if id == "" {
		return &usageError{detail: "project id must not be empty"}
	}
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	cred, findings, err := f.Credential(rt)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return fmt.Errorf("%w; run \"chab login\" or set CHAB_API_KEY", err)
	}
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return err
	}

	p, meta, err := readservice.GetProject(cmd.Context(), client, id)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	p = redactProjectValues(f, p)

	detail := output.Detail{Nodes: projectDetailNodes(p)}
	return f.WriteResultWithMeta(cmd, p, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			detail.Render(w)
		},
		Plain: detail.RenderPlain,
	})
}
