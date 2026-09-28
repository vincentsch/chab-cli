package rawapi

import (
	"net/http"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewPostCommand builds chab api post <path>.
func NewPostCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &unsafeFlags{}
	cmd := &cobra.Command{
		Use:   "post <path>",
		Short: "Send a POST request to an API path",
		Long:  postLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRaw(cmd, f, rawOp{method: http.MethodPost, common: &flags.commonFlags, unsafe: flags}, args[0])
		},
	}
	cmd.Example = postExample
	registerUnsafeFlags(cmd, flags)
	return cmd
}

const postLong = `Send a POST request to an API path using the active profile and credential.

<path> is API-relative and is joined below the resolved API base. Give a path
like /projects or projects. A bare first segment containing a colon is rejected
as ambiguous URI syntax; a colon after an explicit leading slash or in a later
segment remains opaque path data. Absolute and scheme-relative URLs, query or
fragment markers, trailing slashes, and empty segments fail locally. Put query
parameters in --query key=value (repeatable); values may be empty or contain
commas and equals signs, and repeated values retain their order.

Build a JSON body with --field key=value (repeatable), pass any valid JSON value
with --body '<json>', or read one with --body-file <path>. Use --body-file - to
read from stdin. These body modes are mutually exclusive, and duplicate --field
keys fail locally. A POST with no body sends no Content-Type and no JSON null
body.

Replayable POST routes send an idempotency key so a retried request is not
applied twice. A key is generated automatically; pass --idempotency-key <key>
to supply your own (1-255 visible ASCII bytes). Key values are sensitive:
output reports only whether a key is generated, explicit, or suppressed and
never prints the value.
Catalogued idempotent routes create a private recovery record after resolving
credential identity and before the effectful request.
Those catalogued recoverable routes print the stable action and recovery object,
with the API success payload under server. Other raw POST routes keep printing
the selected API success value.
Catalogued operation routes that support JSON body "dry_run": true send
authenticated server validation without confirmation, a recovery record, or an
idempotency key.

Raw API refuses v1 routes that return one-time plaintext secrets: /tokens,
/webhooks/endpoints, and /webhooks/endpoints/{endpoint_id}/rotate-secret. Use
the dedicated private-output token or webhook command for those workflows.

Use --dry-run to preview the method, path, query, body, idempotency behavior,
and any non-replayable retry policy without resolving credentials or contacting
the API.

POST is an unsafe raw action. Most raw POST routes treat choosing this leaf as
the explicit decision to send it. Catalogued paid operation starts and
operation cancellation use the same confirmation rule as native commands; pass
--yes with --no-prompt in non-interactive scripts.

For non-catalogued raw routes, the decoded "data" value of the API success
envelope is printed by default. Use --raw to print the full success envelope,
including data, meta, request_id, and unknown envelope keys. --raw and
--dry-run cannot be combined.

Mark a request value as sensitive with --secret-field <name> (repeatable). Named
query values, field values, and matching JSON body keys are shown as [REDACTED]
in dry-run previews, --debug output, and rendered errors; the flag does not add
a request value.

JSON output is the selected machine value as stable JSON. Default human and
--plain output print scalar values without JSON quotes; objects, arrays, and
null print as JSON because the raw shape is not known ahead of time.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human, --plain, --json stable JSON, and --jq/--template
transforms over the selected JSON value.

Related commands:
  chab api
  chab login`

const postExample = `  chab api post /projects --field name=Demo --field status=active
  chab api post /projects --body '{"name":"Demo","status":"active"}' --json
  chab api post /projects --field name=Demo --json --include-meta
  chab api post /projects --body-file payload.json --dry-run
  chab api post /projects --field name=Demo --idempotency-key <key> --json
  chab api post /projects --field name=Demo --jq .id
  chab api post /projects --field name=Demo --template '{{.id}}'
  chab api post /projects --field token=<token> --secret-field token --debug`
