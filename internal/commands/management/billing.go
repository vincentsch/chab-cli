package management

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewBillingCommand builds chab billing.
func NewBillingCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("billing", "Manage billing settings", `Manage billing settings and purchases.

Billing write operations require manually created team API tokens with the
server-required billing scopes when browser device login cannot obtain them.
The server owns plan gates, purchase policy, auto-recharge authority and
management approval validation.

Related commands:
  chab credits balance
  chab tokens approvals create`)
	cmd.AddCommand(
		newBillingShowCommand(f),
		newBillingPackagesCommand(f),
		newBillingReconciliationCommand(f),
		newAutoRechargeCommand(f),
		newPurchasesCommand(f),
	)
	return cmd
}

func newBillingShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show billing context",
		Long: `Show billing context for the active team API key.

The response includes the server-owned plan name and current auto-recharge
summary. Use a manually created team API key when the server rejects broader
billing scopes for browser/device credentials.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab billing auto-recharge show
  chab billing packages`,
		Args:    cobra.NoArgs,
		Example: "  chab billing show\n  chab billing show --json\n  chab billing show --json --include-meta\n  chab billing show --jq .plan\n  chab billing show --template '{{.plan}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return readOnlyRaw(cmd, f, "billing", nil)
		},
	}
}

func newBillingPackagesCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "packages",
		Short: "List billing packages",
		Long: `List credit packages configured by the server.

The server decides which packages are purchaseable by the team. This command is
read-only and does not create a purchase.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab billing purchases create
  chab credits balance`,
		Args:    cobra.NoArgs,
		Example: "  chab billing packages\n  chab billing packages --json\n  chab billing packages --jq '.packages[].id'\n  chab billing packages --template '{{range .packages}}{{.id}}{{\"\\n\"}}{{end}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return readOnlyRaw(cmd, f, "billing/packages", nil)
		},
	}
}

func newBillingReconciliationCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "reconciliation",
		Short: "Show billing reconciliation",
		Long: `Show the latest billing reconciliation report visible to the active API key.

The server returns either the latest clean or differences-found report, or a
not_yet_available state. The CLI does not run reconciliation work locally.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab billing purchases show
  chab credits transactions`,
		Args:    cobra.NoArgs,
		Example: "  chab billing reconciliation\n  chab billing reconciliation --json\n  chab billing reconciliation --jq .state\n  chab billing reconciliation --template '{{.state}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return readOnlyRaw(cmd, f, "billing/reconciliation", nil)
		},
	}
}

type autoRechargeFlags struct {
	Enabled          bool
	PackageID        string
	PackageIDNull    bool
	Threshold        int64
	ThresholdNull    bool
	DailyLimit       int64
	DailyLimitNull   bool
	MaxPerPeriod     int64
	MaxPerPeriodNull bool
	ProofFile        string
	ProofPrompt      bool
	Body             string
	BodyFile         string
	IdempotencyKey   string
}

func newAutoRechargeCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("auto-recharge", "Manage auto-recharge", `Manage billing auto-recharge.

Auto-recharge updates require an exact management approval proof. Create one
with chab tokens approvals create, complete verification in the product web
app, wait for a private proof file, then submit the update with
--approval-proof-file. --yes confirms only the update; it does not supply a
proof, package, budget, or missing request field.

Related commands:
  chab tokens approvals create
  chab billing purchases create`)
	cmd.AddCommand(newAutoRechargeShowCommand(f), newAutoRechargeUpdateCommand(f))
	return cmd
}

func newAutoRechargeShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show auto-recharge",
		Long: `Show auto-recharge settings and current period counters.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab billing auto-recharge update
  chab billing show`,
		Args:    cobra.NoArgs,
		Example: "  chab billing auto-recharge show\n  chab billing auto-recharge show --json\n  chab billing auto-recharge show --jq .enabled\n  chab billing auto-recharge show --template '{{.enabled}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return readOnlyRaw(cmd, f, "billing/auto-recharge", nil)
		},
	}
}

func newAutoRechargeUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &autoRechargeFlags{}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update auto-recharge",
		Long: `Update auto-recharge settings after management approval.

Build the request with flags or provide the exact JSON body via --body or
--body-file. Flag mode requires --enabled and sends only fields explicitly set:
--package-id, --package-id-null, --threshold, --threshold-null, --daily-limit,
--daily-limit-null, --max-per-period, and --max-per-period-null.

Live updates require --approval-proof-file or --approval-proof-prompt. The proof
is sent only in the X-Chab-Management-Approval header and never printed. Use
--dry-run to preview the request without credentials, proof input, confirmation
or HTTP. Mutating requests send an idempotency key; pass --idempotency-key to
provide your own.

This action requires confirmation. In --no-prompt or non-interactive mode,
pass --yes after providing all required request fields and approval proof.
Browser device login may not grant billing write scope; use a manually created
team API key when the server rejects broader scopes.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab tokens approvals create
  chab tokens approvals wait`,
		Args:    cobra.NoArgs,
		Example: "  chab billing auto-recharge update --enabled --package-id credits_1000 --threshold 100 --daily-limit 2000 --max-per-period 5 --approval-proof-file proof.json --yes\n  chab billing auto-recharge update --body-file auto-recharge.json --approval-proof-file proof.json --yes --json\n  chab billing auto-recharge update --enabled=false --package-id-null --threshold-null --daily-limit-null --max-per-period-null --dry-run\n  chab billing auto-recharge update --enabled=false --approval-proof-file proof.json --yes --jq .enabled\n  chab billing auto-recharge update --enabled=false --approval-proof-file proof.json --yes --template '{{.enabled}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAutoRechargeUpdate(cmd, f, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.Enabled, "enabled", false, "enable or disable auto-recharge")
	cmd.Flags().StringVar(&flags.PackageID, "package-id", "", "credit package id")
	cmd.Flags().BoolVar(&flags.PackageIDNull, "package-id-null", false, "send package_id as null")
	cmd.Flags().Int64Var(&flags.Threshold, "threshold", 0, "credit threshold")
	cmd.Flags().BoolVar(&flags.ThresholdNull, "threshold-null", false, "send threshold as null")
	cmd.Flags().Int64Var(&flags.DailyLimit, "daily-limit", 0, "daily recharge limit")
	cmd.Flags().BoolVar(&flags.DailyLimitNull, "daily-limit-null", false, "send daily_limit as null")
	cmd.Flags().Int64Var(&flags.MaxPerPeriod, "max-per-period", 0, "maximum attempts per billing period")
	cmd.Flags().BoolVar(&flags.MaxPerPeriodNull, "max-per-period-null", false, "send max_per_period as null")
	cmd.Flags().StringVar(&flags.ProofFile, "approval-proof-file", "", "private approval proof JSON file")
	cmd.Flags().BoolVar(&flags.ProofPrompt, "approval-proof-prompt", false, "read approval proof from a hidden prompt")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	registerBodyFlags(cmd, &flags.Body, &flags.BodyFile)
	return cmd
}

func runAutoRechargeUpdate(cmd *cobra.Command, f *cmdutil.Factory, flags *autoRechargeFlags) error {
	body, err := autoRechargeBody(cmd, f, flags)
	if err != nil {
		return err
	}
	if err := validateRequest("billing.auto_recharge.update", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPatch, "/billing/auto-recharge", body, explicit)
	}
	header, proof, err := mutationHeader(flags.ProofFile, flags.ProofPrompt, cmd, f)
	if err != nil {
		return err
	}
	if proof == "" {
		return &usageError{detail: "auto-recharge update requires --approval-proof-file or --approval-proof-prompt"}
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Update billing auto-recharge settings?"); err != nil {
		return err
	}
	authn, err := resolveAuth(cmd, f, false, false, proof)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRawWithHeaders(cmd.Context(), http.MethodPatch, "billing/auto-recharge", nil, api.ExactJSONBody(body), idem, header)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func autoRechargeBody(cmd *cobra.Command, f *cmdutil.Factory, flags *autoRechargeFlags) (json.RawMessage, error) {
	body, bodyMode, err := readExclusiveBodyMode(cmd, f, flags.Body, flags.BodyFile,
		"enabled",
		"package-id",
		"package-id-null",
		"threshold",
		"threshold-null",
		"daily-limit",
		"daily-limit-null",
		"max-per-period",
		"max-per-period-null",
	)
	if err != nil || bodyMode {
		return body, err
	}
	if !cmd.Flags().Changed("enabled") {
		return nil, &usageError{detail: "auto-recharge update requires --enabled or a JSON body"}
	}
	fields := map[string]any{"enabled": flags.Enabled}
	if cmd.Flags().Changed("package-id") && flags.PackageIDNull {
		return nil, &usageError{detail: "--package-id and --package-id-null cannot be combined"}
	}
	if flags.PackageIDNull {
		fields["package_id"] = nil
	} else if cmd.Flags().Changed("package-id") {
		fields["package_id"] = flags.PackageID
	}
	if err := addNullableInt(cmd, fields, "threshold", flags.Threshold, flags.ThresholdNull); err != nil {
		return nil, err
	}
	if err := addNullableInt(cmd, fields, "daily-limit", flags.DailyLimit, flags.DailyLimitNull); err != nil {
		return nil, err
	}
	if err := addNullableInt(cmd, fields, "max-per-period", flags.MaxPerPeriod, flags.MaxPerPeriodNull); err != nil {
		return nil, err
	}
	return canonicalObject(fields)
}

func addNullableInt(cmd *cobra.Command, fields map[string]any, flag string, value int64, nullSet bool) error {
	jsonName := strings.ReplaceAll(flag, "-", "_")
	if cmd.Flags().Changed(flag) && nullSet {
		return &usageError{detail: "--" + flag + " and --" + flag + "-null cannot be combined"}
	}
	if nullSet {
		fields[jsonName] = nil
	} else if cmd.Flags().Changed(flag) {
		fields[jsonName] = value
	}
	return nil
}

type purchaseFlags struct {
	PackageID      string
	ProofFile      string
	ProofPrompt    bool
	IdempotencyKey string
	Wait           bool
}

func newPurchasesCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("purchases", "Manage billing purchases", `Manage credit package purchases.

Purchase creation is a resource lifecycle. The create command records a local
idempotent action before submission, follows the returned purchase resource,
and never creates a replacement purchase to resolve an ambiguous outcome.
Purchase mutations require exact human approval and local confirmation.

Related commands:
  chab billing packages
  chab tokens approvals create`)
	cmd.AddCommand(newPurchaseCreateCommand(f), newPurchaseShowCommand(f), newPurchaseWaitCommand(f))
	return cmd
}

func newPurchaseCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &purchaseFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a credit purchase",
		Long: `Create a credit package purchase after management approval.

Provide --package-id and a private approval proof from chab tokens approvals
wait. The proof is sent only in X-Chab-Management-Approval. The command records
one local action and one idempotency key before the HTTP request; pass
--idempotency-key to provide your own. Rerunning with the same key reuses the
purchase resource instead of creating another charge. Pass --wait to poll
pending and pending_reconciliation until fulfilled or failed.

This action requires confirmation. In --no-prompt or non-interactive mode,
pass --yes after providing all required inputs and proof. Browser device login
may not grant billing write scope; use a manually created team API key when
needed.

Use --dry-run to preview the request without credentials, proof input,
confirmation or HTTP.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default human summary, --plain, --json, --jq, and --template.

Related commands:
  chab billing purchases show
  chab billing purchases wait`,
		Args:    cobra.NoArgs,
		Example: "  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes\n  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes --wait --json\n  chab billing purchases create --package-id credits_1000 --dry-run\n  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes --jq .purchase.id\n  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes --template '{{.purchase.id}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPurchaseCreate(cmd, f, flags)
		},
	}
	cmd.Flags().StringVar(&flags.PackageID, "package-id", "", "credit package id")
	cmd.Flags().StringVar(&flags.ProofFile, "approval-proof-file", "", "private approval proof JSON file")
	cmd.Flags().BoolVar(&flags.ProofPrompt, "approval-proof-prompt", false, "read approval proof from a hidden prompt")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().BoolVar(&flags.Wait, "wait", false, "poll the returned purchase until terminal")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

type purchase struct {
	ID          string  `json:"id"`
	PackageID   string  `json:"package_id"`
	State       string  `json:"state"`
	NextCheckAt *string `json:"next_check_at"`
	CreatedAt   string  `json:"created_at"`
}

type purchaseOutput struct {
	Action       *operations.ActionProjection `json:"action,omitempty"`
	Purchase     purchase                     `json:"purchase"`
	RequestID    string                       `json:"request_id,omitempty"`
	ResponseMeta api.ResponseMeta             `json:"-"`
}

func runPurchaseCreate(cmd *cobra.Command, f *cmdutil.Factory, flags *purchaseFlags) error {
	if flags.PackageID == "" {
		return &usageError{detail: "purchase create requires --package-id"}
	}
	body, err := canonicalObject(map[string]any{"package_id": flags.PackageID})
	if err != nil {
		return err
	}
	if err := validateRequest("billing.purchases.create", body); err != nil {
		return err
	}
	key := flags.IdempotencyKey
	explicit := cmd.Flags().Changed("idempotency-key")
	if explicit {
		if err := api.ValidateIdempotencyKey(key); err != nil {
			return err
		}
	} else {
		key, err = api.GenerateIdempotencyKey()
		if err != nil {
			return err
		}
	}
	f.RegisterSecret(key)
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPost, "/billing/purchases", body, explicit)
	}
	header, proof, err := mutationHeader(flags.ProofFile, flags.ProofPrompt, cmd, f)
	if err != nil {
		return err
	}
	if proof == "" {
		return &usageError{detail: "purchase create requires --approval-proof-file or --approval-proof-prompt"}
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Create a billing purchase for package "+flags.PackageID+"?"); err != nil {
		return err
	}
	authn, err := resolveAuth(cmd, f, true, false, proof, key)
	if err != nil {
		return err
	}
	store := operations.StoreForRuntime(authn.Runtime, f.Clock())
	prepared, err := store.Prepare(operations.PrepareInput{
		Profile:        authn.Runtime.Profile,
		Destination:    authn.Runtime.APIBaseURL,
		TokenPublicID:  authn.Identity.TokenPublicID,
		RequiredScope:  "api:billing:write",
		OperationKey:   "billing.purchases.create",
		Method:         http.MethodPost,
		Path:           "/v1/billing/purchases",
		EncoderVersion: operations.EncoderVersion,
		RequestBytes:   body,
		IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	projection := operations.Projection(prepared.Record)
	var p purchase
	var meta api.ResponseMeta
	if prepared.AlreadyAccepted && prepared.Record.AcceptedOperationID != "" {
		p, meta, err = fetchPurchase(cmd, authn.Client, prepared.Record.AcceptedOperationID)
		if err != nil {
			return apiCredentialError(err, authn.Cred)
		}
	} else {
		if err := store.ReplayablePrepared(prepared); err != nil {
			return err
		}
		result, postErr := authn.Client.DoRawWithHeaders(cmd.Context(), http.MethodPost, "billing/purchases", nil, api.ExactJSONBody(body), api.JournaledIdempotency(key, prepared.Existing), header)
		if postErr != nil {
			if operations.DefinitiveAdmissionDenialForSubmission(postErr, prepared.Existing) {
				if markErr := store.MarkDenied(prepared.Record.ID, postErr); markErr != nil {
					return fmt.Errorf("purchase denied and local denial could not be persisted: %w (persistence: %v)", apiCredentialError(postErr, authn.Cred), markErr)
				}
				return apiCredentialError(postErr, authn.Cred)
			}
			if markErr := store.MarkUnknown(prepared.Record.ID, postErr); markErr != nil {
				return fmt.Errorf("purchase outcome uncertain and local recovery could not be persisted: %w (persistence: %v)", apiCredentialError(postErr, authn.Cred), markErr)
			}
			return &purchaseRecoveryError{Err: apiCredentialError(postErr, authn.Cred), ActionID: prepared.Record.ID, ExplicitKey: explicit}
		}
		meta = result.Meta
		if err := json.Unmarshal(result.Data, &p); err != nil {
			_ = store.MarkUnknown(prepared.Record.ID, err)
			return &api.ProtocolError{Detail: "purchase response data does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: err, Meta: meta}
		}
		recordPayload := operations.OperationPayload{ID: p.ID, OperationKey: "billing.purchases.create", Family: "billing", Status: p.State}
		var updated operations.ActionRecord
		if purchaseTerminal(p.State) {
			updated, err = store.MarkCompleted(prepared.Record.ID, recordPayload, meta)
		} else {
			updated, err = store.MarkAccepted(prepared.Record.ID, recordPayload, meta)
		}
		if err != nil {
			return fmt.Errorf("purchase %s was accepted but local action state could not be saved: %w", p.ID, err)
		}
		projection = operations.Projection(updated)
	}
	if flags.Wait && p.ID != "" && !purchaseTerminal(p.State) {
		p, meta, err = waitPurchase(cmd, f, authn.Client, p, meta)
		if err != nil {
			return apiCredentialError(err, authn.Cred)
		}
		if prepared.Record.ID != "" && purchaseTerminal(p.State) {
			updated, persistErr := store.MarkCompleted(prepared.Record.ID, operations.OperationPayload{ID: p.ID, OperationKey: "billing.purchases.create", Family: "billing", Status: p.State}, meta)
			if persistErr != nil {
				return fmt.Errorf("purchase %s reached %s but local action state could not be saved: %w", p.ID, p.State, persistErr)
			}
			projection = operations.Projection(updated)
		}
	}
	return writePurchase(cmd, f, purchaseOutput{Action: &projection, Purchase: p, RequestID: meta.RequestID, ResponseMeta: meta})
}

func newPurchaseShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <purchase-id>",
		Short: "Show a credit purchase",
		Long: `Show one credit purchase by its opaque pur_... id.

Polling reads require billing read scope independently of the original purchase
mutation scope.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default human summary, --plain, --json, --jq, and --template.

Related commands:
  chab billing purchases create
  chab billing purchases wait`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab billing purchases show pur_example\n  chab billing purchases show pur_example --json\n  chab billing purchases show pur_example --jq .purchase.state\n  chab billing purchases show pur_example --template '{{.purchase.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAuth(cmd, f, false, false)
			if err != nil {
				return err
			}
			p, meta, err := fetchPurchase(cmd, authn.Client, args[0])
			if err != nil {
				return apiCredentialError(err, authn.Cred)
			}
			return writePurchase(cmd, f, purchaseOutput{Purchase: p, RequestID: meta.RequestID, ResponseMeta: meta})
		},
	}
}

func newPurchaseWaitCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "wait <purchase-id>",
		Short: "Wait for a credit purchase",
		Long: `Wait for a purchase to leave pending or pending_reconciliation.

The command follows the supplied purchase id with GET
/v1/billing/purchases/{purchase_id}. It does not create another purchase.
Retry-After is honored when present; otherwise next_check_at is used.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default human summary, --plain, --json, --jq, and --template.

Related commands:
  chab billing purchases show
  chab credits balance`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab billing purchases wait pur_example\n  chab billing purchases wait pur_example --json\n  chab billing purchases wait pur_example --jq .purchase.state\n  chab billing purchases wait pur_example --template '{{.purchase.id}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAuth(cmd, f, false, false)
			if err != nil {
				return err
			}
			p, meta, err := fetchPurchase(cmd, authn.Client, args[0])
			if err != nil {
				return apiCredentialError(err, authn.Cred)
			}
			if !purchaseTerminal(p.State) {
				p, meta, err = waitPurchase(cmd, f, authn.Client, p, meta)
				if err != nil {
					return apiCredentialError(err, authn.Cred)
				}
			}
			return writePurchase(cmd, f, purchaseOutput{Purchase: p, RequestID: meta.RequestID, ResponseMeta: meta})
		},
	}
}

func fetchPurchase(cmd *cobra.Command, client *api.Client, id string) (purchase, api.ResponseMeta, error) {
	var p purchase
	meta, err := client.Get(cmd.Context(), api.Path("billing", "purchases", id), nil, &p)
	return p, meta, err
}

func waitPurchase(cmd *cobra.Command, f *cmdutil.Factory, client *api.Client, p purchase, meta api.ResponseMeta) (purchase, api.ResponseMeta, error) {
	deadline := f.Clock()().Add(10 * time.Minute)
	for !purchaseTerminal(p.State) {
		if p.State != "pending" && p.State != "pending_reconciliation" {
			return p, meta, nil
		}
		next := ""
		if p.NextCheckAt != nil {
			next = *p.NextCheckAt
		}
		sleep := waitFromMetaOrTimestamp(f.Clock(), meta, next, 5*time.Second)
		if remaining := deadline.Sub(f.Clock()()); remaining <= 0 {
			return p, meta, &usageError{detail: "timed out waiting for purchase " + p.ID}
		} else if sleep > remaining {
			sleep = remaining
		}
		if sleep > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "purchase %s is %s; waiting %s\n", p.ID, p.State, sleep)
			if err := f.Sleep(cmd.Context(), sleep); err != nil {
				return p, meta, err
			}
		}
		var err error
		p, meta, err = fetchPurchase(cmd, client, p.ID)
		if err != nil {
			return p, meta, err
		}
	}
	return p, meta, nil
}

func purchaseTerminal(state string) bool {
	return state == "fulfilled" || state == "failed"
}

type purchaseRecoveryError struct {
	Err         error
	ActionID    string
	ExplicitKey bool
}

func (e *purchaseRecoveryError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "purchase outcome is unknown"
}

func (e *purchaseRecoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *purchaseRecoveryError) APIErrorNotes() []string {
	if e == nil {
		return nil
	}
	notes := []string{"Recovery: purchase outcome is unknown."}
	if e.ActionID != "" {
		notes = append(notes, "Local action: "+e.ActionID)
		notes = append(notes, "Resume: "+e.resumeHint())
	}
	if e.ExplicitKey {
		notes = append(notes, "Retry: the stored action reuses the same explicit idempotency key before the replay window expires.")
	} else {
		notes = append(notes, "Retry: the stored action reuses the generated idempotency key; do not rerun purchase create as a new command before recovery.")
	}
	return notes
}

func (e *purchaseRecoveryError) APIErrorRecovery() *output.ErrorRecovery {
	if e == nil {
		return nil
	}
	return &output.ErrorRecovery{
		State:       "unknown",
		CanResume:   e.ActionID != "",
		ResumeHint:  e.resumeHint(),
		ActionID:    e.ActionID,
		KnownRemote: false,
	}
}

func (e *purchaseRecoveryError) resumeHint() string {
	if e == nil || e.ActionID == "" {
		return ""
	}
	return "chab operations resume " + e.ActionID + " --input @request.json"
}

func (e *purchaseRecoveryError) ExitCode() int {
	var coder interface{ ExitCode() int }
	if errors.As(e.Err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

func writePurchase(cmd *cobra.Command, f *cmdutil.Factory, value purchaseOutput) error {
	return writeTypedValue(cmd, f, value, value.ResponseMeta, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			nodes := []output.Node{
				output.Field("id", value.Purchase.ID),
				output.Field("package_id", value.Purchase.PackageID),
				output.Field("state", value.Purchase.State),
			}
			if value.Purchase.NextCheckAt != nil {
				nodes = append(nodes, output.Field("next_check_at", *value.Purchase.NextCheckAt))
			}
			if value.Purchase.CreatedAt != "" {
				nodes = append(nodes, output.Field("created_at", value.Purchase.CreatedAt))
			}
			if value.Action != nil {
				nodes = append(nodes, output.Field("action_id", value.Action.ID))
			}
			if value.RequestID != "" {
				nodes = append(nodes, output.Field("request_id", value.RequestID))
			}
			output.Detail{Nodes: nodes}.Render(w)
		},
		Plain: func(data, prose io.Writer) {
			fmt.Fprintln(data, value.Purchase.ID)
		},
	})
}
