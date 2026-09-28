package system

import (
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

type Health struct {
	Service    string         `json:"service"`
	APIVersion string         `json:"api_version"`
	Status     string         `json:"status"`
	UpdatedAt  string         `json:"updated_at"`
	Families   []HealthFamily `json:"families"`
	FreeMode   HealthFreeMode `json:"free_mode"`
}

type HealthFamily struct {
	Family               string    `json:"family"`
	State                string    `json:"state"`
	CoveredOperationKeys *[]string `json:"covered_operation_keys"`
	Remediation          *string   `json:"remediation"`
}

type HealthFreeMode struct {
	State string `json:"state"`
}

func NewHealthCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Show public API health",
		Long: `Show public API health.

This command calls unauthenticated GET /v1/health and never reads credentials.

Related commands:
  chab errors
  chab doctor
  chab api get /health`,
		Example: `  chab health
  chab health --plain
  chab health --json
  chab health --jq .status
  chab health --template '{{.status}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHealth(cmd, f)
		},
	}
}

func runHealth(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	client, err := f.PublicAPIClient(rt, cmd)
	if err != nil {
		return err
	}
	var data Health
	meta, err := client.Get(cmd.Context(), "health", nil, &data)
	if err != nil {
		return err
	}
	detail := output.Detail{Nodes: healthNodes(data)}
	return f.WriteResultWithMeta(cmd, data, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) { detail.Render(w) },
		Plain:  detail.RenderPlain,
	})
}

func healthNodes(data Health) []output.Node {
	nodes := []output.Node{
		output.Field("service", data.Service),
		output.Field("api_version", data.APIVersion),
		output.Field("status", data.Status),
	}
	if data.UpdatedAt != "" {
		nodes = append(nodes, output.Field("updated_at", data.UpdatedAt))
	}
	if data.FreeMode.State != "" {
		nodes = append(nodes, output.Field("free_mode", data.FreeMode.State))
	}
	if len(data.Families) > 0 {
		var parts []string
		for _, family := range data.Families {
			part := family.Family
			if family.State != "" {
				part += "=" + family.State
			}
			if family.CoveredOperationKeys != nil {
				part += " [" + strings.Join(*family.CoveredOperationKeys, ", ") + "]"
			}
			if family.Remediation != nil && *family.Remediation != "" {
				part += " (" + *family.Remediation + ")"
			}
			parts = append(parts, part)
		}
		nodes = append(nodes, output.Field("families", strings.Join(parts, ", ")))
	}
	return nodes
}
