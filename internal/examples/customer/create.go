package customer

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/retainedcmdutil"
	"github.com/vincentsch/rungrad"
)

// createFlags holds only the body and safety controls this example supports.
// The example deliberately avoids body-file and arbitrary JSON input modes.
type createFlags struct {
	Name           string
	IdempotencyKey string
}

// NewCreateCommand builds chab customer create for test-owned command trees.
func NewCreateCommand(f *cmdutil.Factory) *rungrad.Command {
	flags := &createFlags{}
	return &rungrad.Command{
		Use:   "create",
		Short: "Create an example customer",
		Long: `Create an example customer through a teaching artifact for the SaaS CLI
authoring guide, compiled only into test command trees.

--name is required and is the only field sent in the request body.

Every real create sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive and are never
printed; output reports only whether a key is generated or explicit.

Use --dry-run to preview the method, path, body fields, and idempotency
behavior without resolving credentials or contacting the API. Dry-run cannot
be combined with --include-meta.

JSON output is one customer object with fields id, name, status, created_at.
Default JSON omits request and idempotency metadata.

Add --include-meta to JSON, --jq, or --template output to wrap this command's
value under data and expose request id, idempotency replay, rate-limit, and
retry metadata under meta.

Output modes: default human summary, --plain tab-separated fields, --json
stable JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab customer list`,
		Args: cobra.NoArgs,
		Configure: func(cmd *cobra.Command) {
			cmd.Example = `  chab customer create --name "Demo"
  chab customer create --name "Demo" --dry-run
  chab customer create --name "Demo" --idempotency-key <key>
  chab customer create --name "Demo" --json`
			cmd.Flags().StringVar(&flags.Name, "name", "", "customer name (sent in the body)")
			cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key (1-255 visible ASCII bytes); generated when omitted")
		},
		Run: func(_ *rungrad.Factory, cmd *cobra.Command, _ []string) error {
			return runCreate(cmd, f, flags)
		},
	}
}

// runCreate preserves the write-command safety order: validate local input,
// validate an explicit idempotency key, allow a fully offline dry-run, and only
// then resolve credentials and contact the API.
func runCreate(cmd *cobra.Command, f *cmdutil.Factory, flags *createFlags) error {
	if !cmd.Flags().Changed("name") || strings.TrimSpace(flags.Name) == "" {
		return &usageError{detail: "customer create requires --name"}
	}

	explicitKey := cmd.Flags().Changed("idempotency-key")
	if explicitKey {
		if err := api.ValidateIdempotencyKey(flags.IdempotencyKey); err != nil {
			return err
		}
	}

	if retainedcmdutil.DryRunEnabled(cmd) {
		// Nothing that resolves config, credentials, environment, or HTTP may
		// move above this branch; dry-run is an offline preview.
		preview := output.DryRunPreview{
			Method: "POST",
			Path:   "/" + api.Path("customers"),
			Body: []output.DryRunValue{
				{Name: "name", Value: flags.Name},
			},
			Idempotency: dryRunIdempotency(explicitKey),
		}
		return f.WriteResult(cmd, preview, cmdutil.HumanOutput{
			Render: func(w io.Writer) { preview.Render(w) },
			Plain:  preview.RenderPlain,
		})
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

	idem := api.AutoIdempotency()
	if explicitKey {
		idem = api.ExplicitIdempotency(flags.IdempotencyKey)
	}
	var row customerJSON
	meta, err := client.Post(cmd.Context(), api.Path("customers"), nil, map[string]string{"name": flags.Name}, idem, &row)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	summary := output.MutationSummary{
		Action:   "Created",
		Resource: "customer",
		Name:     row.Name,
		Fields:   customerDetailNodes(row),
	}
	return f.WriteResultWithMeta(cmd, row, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) { summary.Render(w) },
		Plain:  summary.RenderPlain,
	})
}

// dryRunIdempotency describes the request that would be sent without creating
// or exposing the eventual idempotency key value.
func dryRunIdempotency(explicit bool) *output.DryRunIdempotency {
	if explicit {
		return &output.DryRunIdempotency{Source: "explicit"}
	}
	return &output.DryRunIdempotency{Source: "generated"}
}
