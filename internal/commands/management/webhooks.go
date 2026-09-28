package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewWebhooksCommand builds chab webhooks.
func NewWebhooksCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("webhooks", "Manage webhook endpoints and replays", `Manage persistent webhook endpoints, deliveries and replay requests.

Webhook endpoint creation and secret rotation return one-time signing secrets.
Those commands require --secret-out and save the secret only to that private
file. Replay and delete commands require confirmation; --yes confirms only the
prompt and does not supply missing request fields.

Browser device login may not grant webhook scopes. Use a manually created team
API key when the server rejects broader scopes.

Related commands:
  chab operations wait
  chab credits balance`)
	cmd.AddCommand(newWebhookEndpointsCommand(f), newWebhookDeliveriesCommand(f), newWebhookReplaysCommand(f))
	return cmd
}

type endpointFlags struct {
	URL            string
	EventTypes     []string
	FilterProjects []string
	FilterKeys     []string
	FilterFamilies []string
	Enabled        bool
	Body           string
	BodyFile       string
	SecretOut      string
	IdempotencyKey string
}

func newWebhookEndpointsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("endpoints", "Manage webhook endpoints", `Manage persistent webhook endpoints.

Endpoint list and show never include signing secrets. Create and rotate-secret
write one-time secrets only to --secret-out.

Related commands:
  chab webhooks deliveries list
  chab webhooks replays create`)
	cmd.AddCommand(
		newEndpointListCommand(f),
		newEndpointShowCommand(f),
		newEndpointCreateCommand(f),
		newEndpointUpdateCommand(f),
		newEndpointDeleteCommand(f),
		newEndpointRotateSecretCommand(f),
	)
	return cmd
}

func newEndpointListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &cmdutil.CursorPaginationFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhook endpoints",
		Long: `List webhook endpoints owned by the current token.

The list is cursor-paginated. Use --limit, --cursor, --page-size, or --all.
--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks endpoints show
  chab webhooks deliveries list`,
		Args:    cobra.NoArgs,
		Example: "  chab webhooks endpoints list\n  chab webhooks endpoints list --limit 20 --json\n  chab webhooks endpoints list --all --json --include-meta\n  chab webhooks endpoints list --jq '.[].id'\n  chab webhooks endpoints list --template '{{range .}}{{.id}}{{\"\\n\"}}{{end}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cursorListRaw(cmd, f, "webhooks/endpoints", nil, flags)
		},
	}
	registerCursorFlags(cmd, flags)
	return cmd
}

func newEndpointShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <endpoint-id>",
		Short: "Show a webhook endpoint",
		Long: `Show one webhook endpoint without its signing secret.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks endpoints update
  chab webhooks endpoints rotate-secret`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab webhooks endpoints show <endpoint-id>\n  chab webhooks endpoints show <endpoint-id> --json\n  chab webhooks endpoints show <endpoint-id> --jq .endpoint.enabled\n  chab webhooks endpoints show <endpoint-id> --template '{{.endpoint.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOnlyRaw(cmd, f, api.Path("webhooks", "endpoints", args[0]), nil)
		},
	}
}

func newEndpointCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &endpointFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook endpoint",
		Long: `Create a webhook endpoint and save its one-time signing secret.

Build the request from flags or provide an exact JSON object with --body or
--body-file. Flag mode requires --url and at least one --event-type. Optional
filters are --filter-project, --filter-operation-key, and --filter-family.
--enabled can be set explicitly; omitted lets the server apply its default.

The one-time signing secret is written only to --secret-out. The CLI reserves
that private path before the HTTP request and prints a safe receipt. Use
--dry-run to preview the request without reserving --secret-out, resolving
credentials or contacting the API. Mutating requests send an idempotency key;
pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks endpoints list
  chab webhooks endpoints rotate-secret`,
		Args:    cobra.NoArgs,
		Example: "  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --filter-family search --secret-out webhook-secret.json\n  chab webhooks endpoints create --body-file endpoint.json --secret-out webhook-secret.json --json\n  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --dry-run\n  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --secret-out webhook-secret.json --jq .secret_out\n  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --secret-out webhook-secret.json --template '{{.secret_out}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEndpointCreate(cmd, f, flags)
		},
	}
	registerEndpointMutationFlags(cmd, flags, true)
	cmd.Flags().StringVar(&flags.SecretOut, "secret-out", "", "private path for the one-time signing secret")
	return cmd
}

func newEndpointUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &endpointFlags{}
	cmd := &cobra.Command{
		Use:   "update <endpoint-id>",
		Short: "Update a webhook endpoint",
		Long: `Update a webhook endpoint.

Build the request from flags or provide an exact JSON object with --body or
--body-file. At least one field must change. The server owns URL safety,
enabled endpoint quotas, filter limits and current authorization checks.

Use --dry-run to preview the request without credentials or HTTP. Mutating
requests send an idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks endpoints show
  chab webhooks deliveries list`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab webhooks endpoints update <endpoint-id> --event-type operation.succeeded --event-type operation.failed\n  chab webhooks endpoints update <endpoint-id> --enabled=false --json\n  chab webhooks endpoints update <endpoint-id> --body-file endpoint-update.json\n  chab webhooks endpoints update <endpoint-id> --enabled=false --jq .endpoint.enabled\n  chab webhooks endpoints update <endpoint-id> --enabled=false --template '{{.endpoint.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEndpointUpdate(cmd, f, flags, args[0])
		},
	}
	registerEndpointMutationFlags(cmd, flags, false)
	return cmd
}

func newEndpointDeleteCommand(f *cmdutil.Factory) *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "delete <endpoint-id>",
		Short: "Delete a webhook endpoint",
		Long: `Delete a webhook endpoint.

Deleting disables future deliveries and prevents replay. This action requires
confirmation; in --no-prompt or non-interactive mode, pass --yes after
providing the endpoint id. Use --dry-run to preview the request without
credentials, confirmation or HTTP. Mutating requests send an idempotency key;
pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks endpoints list
  chab webhooks deliveries list`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab webhooks endpoints delete <endpoint-id> --yes\n  chab webhooks endpoints delete <endpoint-id> --dry-run\n  chab webhooks endpoints delete <endpoint-id> --yes --jq .deleted\n  chab webhooks endpoints delete <endpoint-id> --yes --template '{{.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSimpleMutation(cmd, f, simpleMutation{
				OperationKey:    "webhooks.endpoints.delete",
				Method:          http.MethodDelete,
				Path:            api.Path("webhooks", "endpoints", args[0]),
				DisplayPath:     "/" + api.Path("webhooks", "endpoints", args[0]),
				IdempotencyKey:  key,
				ConfirmQuestion: "Delete webhook endpoint " + args[0] + "?",
			})
		},
	}
	cmd.Flags().StringVar(&key, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

func newEndpointRotateSecretCommand(f *cmdutil.Factory) *cobra.Command {
	var secretOut, key string
	cmd := &cobra.Command{
		Use:   "rotate-secret <endpoint-id>",
		Short: "Rotate a webhook secret",
		Long: `Rotate a webhook endpoint signing secret.

The new one-time secret is written only to --secret-out. The CLI reserves that
private path before the HTTP request and prints a safe receipt. This action
requires confirmation; --yes confirms only the rotation prompt.

Use --dry-run to preview the request without reserving --secret-out,
credentials, confirmation or HTTP. Automatic retries are disabled for the
secret-producing request. Mutating requests send an idempotency key; pass
--idempotency-key to provide your own.
In --no-prompt or non-interactive mode, pass --yes after providing the endpoint
id and --secret-out.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks endpoints show
  chab webhooks endpoints update`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes\n  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes --json\n  chab webhooks endpoints rotate-secret <endpoint-id> --dry-run\n  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes --jq .secret_out\n  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes --template '{{.secret_out}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEndpointRotateSecret(cmd, f, args[0], secretOut, key)
		},
	}
	cmd.Flags().StringVar(&secretOut, "secret-out", "", "private path for the one-time signing secret")
	cmd.Flags().StringVar(&key, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

func registerEndpointMutationFlags(cmd *cobra.Command, flags *endpointFlags, _ bool) {
	cmd.Flags().StringVar(&flags.URL, "url", "", "webhook HTTPS URL")
	cmd.Flags().StringArrayVar(&flags.EventTypes, "event-type", nil, "event type subscription (repeatable)")
	cmd.Flags().StringArrayVar(&flags.FilterProjects, "filter-project", nil, "project filter id (repeatable)")
	cmd.Flags().StringArrayVar(&flags.FilterKeys, "filter-operation-key", nil, "operation-key filter (repeatable)")
	cmd.Flags().StringArrayVar(&flags.FilterFamilies, "filter-family", nil, "operation family filter (repeatable)")
	cmd.Flags().BoolVar(&flags.Enabled, "enabled", false, "enable or disable the endpoint")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	registerBodyFlags(cmd, &flags.Body, &flags.BodyFile)
}

func runEndpointCreate(cmd *cobra.Command, f *cmdutil.Factory, flags *endpointFlags) error {
	body, err := endpointBody(cmd, f, flags, true)
	if err != nil {
		return err
	}
	if err := validateRequest("webhooks.endpoints.create", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPost, "/webhooks/endpoints", body, explicit)
	}
	if flags.SecretOut == "" {
		return &usageError{detail: "webhook endpoint create requires --secret-out"}
	}
	reserved, err := reservePrivateOutput(flags.SecretOut)
	if err != nil {
		return err
	}
	authn, err := oneShotSecretClient(cmd, f, "")
	if err != nil {
		_ = reserved.Abort(true)
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodPost, "webhooks/endpoints", nil, api.ExactJSONBody(body), idem)
	if err != nil {
		_ = reserved.Abort(false)
		return apiCredentialError(err, authn.Cred)
	}
	var decoded struct {
		Endpoint webhookEndpoint `json:"endpoint"`
		Secret   string          `json:"secret"`
	}
	if err := json.Unmarshal(result.Data, &decoded); err != nil || decoded.Secret == "" {
		_ = reserved.Abort(false)
		if err == nil {
			err = fmt.Errorf("missing secret")
		}
		return &api.ProtocolError{Detail: "webhook endpoint create response data does not match the expected shape", Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Err: err, Meta: result.Meta}
	}
	f.RegisterSecret(decoded.Secret)
	if err := commitSecretJSON(reserved, map[string]any{
		"operation":   "webhooks.endpoints.create",
		"endpoint_id": decoded.Endpoint.ID,
		"secret":      decoded.Secret,
		"request_id":  result.Meta.RequestID,
	}); err != nil {
		return &usageError{detail: "webhook signing secret may have been consumed; saving private output failed: " + err.Error()}
	}
	receipt := secretReceipt{Operation: "webhooks.endpoints.create", ResourceID: decoded.Endpoint.ID, SecretOut: reserved.Path(), RequestID: result.Meta.RequestID, Status: "saved"}
	return writeSecretReceipt(cmd, f, receipt, result.Meta)
}

func runEndpointUpdate(cmd *cobra.Command, f *cmdutil.Factory, flags *endpointFlags, id string) error {
	body, err := endpointBody(cmd, f, flags, false)
	if err != nil {
		return err
	}
	if err := validateRequest("webhooks.endpoints.update", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPatch, "/"+api.Path("webhooks", "endpoints", id), body, explicit)
	}
	authn, err := resolveAuth(cmd, f, false, false)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodPatch, api.Path("webhooks", "endpoints", id), nil, api.ExactJSONBody(body), idem)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func runEndpointRotateSecret(cmd *cobra.Command, f *cmdutil.Factory, id, secretOut, key string) error {
	idem, explicit, err := idemFromFlags(cmd, f, key)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPost, "/"+api.Path("webhooks", "endpoints", id, "rotate-secret"), nil, explicit)
	}
	if secretOut == "" {
		return &usageError{detail: "webhook secret rotation requires --secret-out"}
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Rotate webhook signing secret for "+id+"?"); err != nil {
		return err
	}
	reserved, err := reservePrivateOutput(secretOut)
	if err != nil {
		return err
	}
	authn, err := oneShotSecretClient(cmd, f, "")
	if err != nil {
		_ = reserved.Abort(true)
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodPost, api.Path("webhooks", "endpoints", id, "rotate-secret"), nil, nil, idem)
	if err != nil {
		_ = reserved.Abort(false)
		return apiCredentialError(err, authn.Cred)
	}
	var decoded struct {
		Endpoint webhookEndpoint `json:"endpoint"`
		Secret   string          `json:"secret"`
	}
	if err := json.Unmarshal(result.Data, &decoded); err != nil || decoded.Secret == "" {
		_ = reserved.Abort(false)
		if err == nil {
			err = fmt.Errorf("missing secret")
		}
		return &api.ProtocolError{Detail: "webhook secret rotation response data does not match the expected shape", Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Err: err, Meta: result.Meta}
	}
	f.RegisterSecret(decoded.Secret)
	if err := commitSecretJSON(reserved, map[string]any{
		"operation":   "webhooks.endpoints.rotate_secret",
		"endpoint_id": decoded.Endpoint.ID,
		"secret":      decoded.Secret,
		"request_id":  result.Meta.RequestID,
	}); err != nil {
		return &usageError{detail: "webhook signing secret may have been consumed; saving private output failed: " + err.Error()}
	}
	receipt := secretReceipt{Operation: "webhooks.endpoints.rotate_secret", ResourceID: decoded.Endpoint.ID, SecretOut: reserved.Path(), RequestID: result.Meta.RequestID, Status: "saved"}
	return writeSecretReceipt(cmd, f, receipt, result.Meta)
}

func endpointBody(cmd *cobra.Command, f *cmdutil.Factory, flags *endpointFlags, create bool) (json.RawMessage, error) {
	body, bodyMode, err := readExclusiveBodyMode(cmd, f, flags.Body, flags.BodyFile,
		"url",
		"event-type",
		"filter-project",
		"filter-operation-key",
		"filter-family",
		"enabled",
	)
	if err != nil || bodyMode {
		return body, err
	}
	fields := map[string]any{}
	if cmd.Flags().Changed("url") {
		fields["url"] = flags.URL
	}
	if cmd.Flags().Changed("event-type") {
		fields["event_types"] = flags.EventTypes
	}
	filters := map[string]any{}
	if cmd.Flags().Changed("filter-project") {
		ids, err := parseInt64List(flags.FilterProjects, "filter-project")
		if err != nil {
			return nil, err
		}
		filters["projects"] = ids
	}
	if cmd.Flags().Changed("filter-operation-key") {
		filters["operation_keys"] = flags.FilterKeys
	}
	if cmd.Flags().Changed("filter-family") {
		filters["families"] = flags.FilterFamilies
	}
	if len(filters) > 0 {
		fields["filters"] = filters
	}
	if cmd.Flags().Changed("enabled") {
		fields["enabled"] = flags.Enabled
	}
	if create {
		if !cmd.Flags().Changed("url") {
			return nil, &usageError{detail: "webhook endpoint create requires --url or a JSON body"}
		}
		if !cmd.Flags().Changed("event-type") {
			return nil, &usageError{detail: "webhook endpoint create requires at least one --event-type or a JSON body"}
		}
	} else if len(fields) == 0 {
		return nil, &usageError{detail: "webhook endpoint update requires at least one change"}
	}
	return canonicalObject(fields)
}

type webhookEndpoint struct {
	ID string `json:"id"`
}

type deliveryFlags struct {
	EndpointID string
	Status     string
	Page       cmdutil.CursorPaginationFlags
}

func newWebhookDeliveriesCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("deliveries", "Manage webhook deliveries", `Manage webhook deliveries and individual replay requests.

Delivery replay checks current endpoint ownership, subscription, scope,
project and connected-data authority on the server. Replay commands require
local confirmation.

Related commands:
  chab webhooks endpoints list
  chab webhooks replays create`)
	cmd.AddCommand(newDeliveryListCommand(f), newDeliveryShowCommand(f), newDeliveryReplayCommand(f))
	return cmd
}

func newDeliveryListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &deliveryFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhook deliveries",
		Long: `List webhook delivery attempts for current-token endpoints.

Optional filters are --endpoint-id and --status. The list is cursor-paginated;
use --limit, --cursor, --page-size, or --all. --include-meta adds safe
transport context under meta when --json, --jq, or --template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks deliveries show
  chab webhooks deliveries replay`,
		Args:    cobra.NoArgs,
		Example: "  chab webhooks deliveries list\n  chab webhooks deliveries list --endpoint-id whe_example --status failed --json\n  chab webhooks deliveries list --all --json --include-meta\n  chab webhooks deliveries list --jq '.[].id'\n  chab webhooks deliveries list --template '{{range .}}{{.id}}{{\"\\n\"}}{{end}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if flags.EndpointID != "" {
				q.Set("endpoint_id", flags.EndpointID)
			}
			if flags.Status != "" {
				q.Set("status", flags.Status)
			}
			return cursorListRaw(cmd, f, "webhooks/deliveries", q, &flags.Page)
		},
	}
	cmd.Flags().StringVar(&flags.EndpointID, "endpoint-id", "", "filter by endpoint id")
	cmd.Flags().StringVar(&flags.Status, "status", "", "filter by delivery status")
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newDeliveryShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <delivery-id>",
		Short: "Show a webhook delivery",
		Long: `Show one webhook delivery attempt.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks deliveries replay
  chab operations show`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab webhooks deliveries show <delivery-id>\n  chab webhooks deliveries show <delivery-id> --json\n  chab webhooks deliveries show <delivery-id> --jq .delivery.status\n  chab webhooks deliveries show <delivery-id> --template '{{.delivery.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOnlyRaw(cmd, f, api.Path("webhooks", "deliveries", args[0]), nil)
		},
	}
}

func newDeliveryReplayCommand(f *cmdutil.Factory) *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "replay <delivery-id>",
		Short: "Replay a webhook delivery",
		Long: `Replay one webhook delivery after current server authorization checks.

This action requires confirmation; in --no-prompt or non-interactive mode,
pass --yes after providing the delivery id. Use --dry-run to preview the
request without credentials, confirmation or HTTP. Mutating requests send an
idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks deliveries show
  chab webhooks replays create`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab webhooks deliveries replay <delivery-id> --yes\n  chab webhooks deliveries replay <delivery-id> --yes --json\n  chab webhooks deliveries replay <delivery-id> --dry-run\n  chab webhooks deliveries replay <delivery-id> --yes --jq .replay.id\n  chab webhooks deliveries replay <delivery-id> --yes --template '{{.replay.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSimpleMutation(cmd, f, simpleMutation{
				OperationKey:    "webhooks.deliveries.replay",
				Method:          http.MethodPost,
				Path:            api.Path("webhooks", "deliveries", args[0], "replay"),
				DisplayPath:     "/" + api.Path("webhooks", "deliveries", args[0], "replay"),
				IdempotencyKey:  key,
				ConfirmQuestion: "Replay webhook delivery " + args[0] + "?",
			})
		},
	}
	cmd.Flags().StringVar(&key, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

type replayFlags struct {
	EndpointID     string
	EventTypes     []string
	CreatedAfter   string
	CreatedBefore  string
	Body           string
	BodyFile       string
	IdempotencyKey string
}

func newWebhookReplaysCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("replays", "Manage webhook replay windows", `Manage webhook replay windows.

Replay windows create new delivery attempts for an endpoint, event set and
time range after current server authorization checks.

Related commands:
  chab webhooks endpoints show
  chab webhooks deliveries list`)
	cmd.AddCommand(newReplayCreateCommand(f))
	return cmd
}

func newReplayCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &replayFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook replay window",
		Long: `Create a webhook replay window.

Build the request from flags or provide an exact JSON object with --body or
--body-file. Flag mode requires --endpoint-id, at least one --event-type,
--created-after and --created-before. The server owns replay-window size,
retention, endpoint subscription and current authorization checks.

This action requires confirmation. In --no-prompt or non-interactive mode,
pass --yes after providing all request fields. Use --dry-run to preview the
request without credentials, confirmation or HTTP. Mutating requests send an
idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab webhooks deliveries list
  chab webhooks endpoints show`,
		Args:    cobra.NoArgs,
		Example: "  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --yes\n  chab webhooks replays create --body-file replay.json --yes --json\n  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --dry-run\n  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --yes --jq .replay.id\n  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --yes --template '{{.replay.id}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReplayCreate(cmd, f, flags)
		},
	}
	cmd.Flags().StringVar(&flags.EndpointID, "endpoint-id", "", "webhook endpoint id")
	cmd.Flags().StringArrayVar(&flags.EventTypes, "event-type", nil, "event type to replay (repeatable)")
	cmd.Flags().StringVar(&flags.CreatedAfter, "created-after", "", "replay window start date-time")
	cmd.Flags().StringVar(&flags.CreatedBefore, "created-before", "", "replay window end date-time")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	registerBodyFlags(cmd, &flags.Body, &flags.BodyFile)
	return cmd
}

func runReplayCreate(cmd *cobra.Command, f *cmdutil.Factory, flags *replayFlags) error {
	body, err := replayBody(cmd, f, flags)
	if err != nil {
		return err
	}
	if err := validateRequest("webhooks.replays.create", body); err != nil {
		return err
	}
	return runSimpleMutation(cmd, f, simpleMutation{
		OperationKey:    "webhooks.replays.create",
		Method:          http.MethodPost,
		Path:            "webhooks/replays",
		DisplayPath:     "/webhooks/replays",
		Body:            body,
		IdempotencyKey:  flags.IdempotencyKey,
		ConfirmQuestion: "Create webhook replay window for endpoint " + replayEndpointForQuestion(body) + "?",
	})
}

func replayBody(cmd *cobra.Command, f *cmdutil.Factory, flags *replayFlags) (json.RawMessage, error) {
	body, bodyMode, err := readExclusiveBodyMode(cmd, f, flags.Body, flags.BodyFile,
		"endpoint-id",
		"event-type",
		"created-after",
		"created-before",
	)
	if err != nil || bodyMode {
		return body, err
	}
	if flags.EndpointID == "" {
		return nil, &usageError{detail: "webhook replay create requires --endpoint-id or a JSON body"}
	}
	if !cmd.Flags().Changed("event-type") {
		return nil, &usageError{detail: "webhook replay create requires at least one --event-type or a JSON body"}
	}
	if flags.CreatedAfter == "" || flags.CreatedBefore == "" {
		return nil, &usageError{detail: "webhook replay create requires --created-after and --created-before or a JSON body"}
	}
	return canonicalObject(map[string]any{
		"endpoint_id":    flags.EndpointID,
		"event_types":    flags.EventTypes,
		"created_after":  flags.CreatedAfter,
		"created_before": flags.CreatedBefore,
	})
}

func replayEndpointForQuestion(body json.RawMessage) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) == nil {
		var id string
		if json.Unmarshal(object["endpoint_id"], &id) == nil && id != "" {
			return id
		}
	}
	return "<endpoint>"
}

type simpleMutation struct {
	OperationKey    string
	Method          string
	Path            string
	DisplayPath     string
	Body            json.RawMessage
	IdempotencyKey  string
	ConfirmQuestion string
}

func runSimpleMutation(cmd *cobra.Command, f *cmdutil.Factory, input simpleMutation) error {
	idem, explicit, err := idemFromFlags(cmd, f, input.IdempotencyKey)
	if err != nil {
		return err
	}
	body := input.Body
	if len(body) > 0 {
		if err := validateRequest(input.OperationKey, body); err != nil {
			return err
		}
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, input.Method, input.DisplayPath, input.Body, explicit)
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), input.ConfirmQuestion); err != nil {
		return err
	}
	authn, err := resolveAuth(cmd, f, false, false)
	if err != nil {
		return err
	}
	var requestBody any
	if len(body) > 0 {
		requestBody = api.ExactJSONBody(body)
	}
	result, err := authn.Client.DoRaw(cmd.Context(), input.Method, input.Path, nil, requestBody, idem)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}
