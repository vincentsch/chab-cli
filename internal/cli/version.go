package cli

import (
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/output"
)

// Build metadata for chab version. Release builds override these via
//
//	-ldflags "-X github.com/vincentsch/chab-cli/internal/cli.Version=..."
//
// Commit and Date use the same package path. go_version, os, and arch always
// come from the running Go runtime.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// versionInfo is ordered to match the stable JSON field order promised by
// version --json.
type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// newVersionCommand reports local build metadata only. It goes through the
// shared command-result writer so human, plain, JSON, jq, and template modes
// stay consistent with the rest of the active shell.
func newVersionCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   summaryFor("version"),
		GroupID: groupUtilities,
		Args:    cobra.NoArgs,
		Long: `Print build metadata for the chab binary.

With --json, output is a stable JSON object with exactly these fields:
  version     release version, or "dev" for local builds
  commit      source commit, or "none" for local builds
  date        build date, or "unknown" for local builds
  go_version  Go runtime version
  os          runtime operating system
  arch        runtime architecture

The same JSON shape can be filtered with --jq or rendered with --template.
Use --plain for copy-safe key/value rows.

Related commands:
  chab doctor
  chab completion`,
		Example: `  chab version
  chab version --json
  chab version --jq .version
  chab version --template '{{.version}}'
  chab version --plain`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionInfo{
				Version:   Version,
				Commit:    Commit,
				Date:      Date,
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
			}
			return f.WriteCommandResult(cmd, cmdutil.CommandResult{
				Machine: info,
				Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
					fmt.Fprintf(w, "chab %s (commit %s, built %s, %s %s/%s)\n",
						info.Version, info.Commit, info.Date, info.GoVersion, info.OS, info.Arch)
				}, Plain: func(data, _ io.Writer) {
					output.Detail{Nodes: []output.Node{
						output.Field("version", info.Version),
						output.Field("commit", info.Commit),
						output.Field("date", info.Date),
						output.Field("go_version", info.GoVersion),
						output.Field("os", info.OS),
						output.Field("arch", info.Arch),
					}}.RenderPlain(data, io.Discard)
				}},
				Supports: cmdutil.OutputSupport{Human: true, JSON: true, Plain: true, JQ: true, Template: true},
			})
		},
	}
}
