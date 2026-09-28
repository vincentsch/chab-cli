package management

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/localfile"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewTokensCommand builds chab tokens.
func NewTokensCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("tokens", "Manage team API tokens", `Manage team API tokens.

Token write operations use server-owned policy revisions and management
approval where required. Browser device login currently grants only
read-oriented management scopes; use a manually created team API key when the
server rejects token write scopes.

One-time token secrets and approval proofs are never printed. Use --secret-out
or --proof-out to save them to private files.

Related commands:
  chab whoami
  chab tokens approvals create`)
	cmd.AddCommand(
		newTokenListCommand(f),
		newTokenShowCommand(f),
		newTokenCreateCommand(f),
		newTokenUpdateCommand(f),
		newTokenRevokeCommand(f),
		newTokenApprovalsCommand(f),
	)
	return cmd
}

func newTokenListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &cmdutil.CursorPaginationFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API tokens",
		Long: `List team API tokens visible to the active API key.

The list is cursor-paginated. Use --limit to cap returned items, --cursor to
resume from an API cursor, --page-size for per-request size, or --all to fetch
every page.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab tokens show
  chab whoami`,
		Args:    cobra.NoArgs,
		Example: "  chab tokens list\n  chab tokens list --limit 20 --json\n  chab tokens list --all --json --include-meta\n  chab tokens list --jq '.[].token_public_id'\n  chab tokens list --template '{{range .}}{{.token_public_id}}{{\"\\n\"}}{{end}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cursorListRaw(cmd, f, "tokens", nil, flags)
		},
	}
	registerCursorFlags(cmd, flags)
	return cmd
}

func newTokenShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "show <token-public-id>",
		Short: "Show API token metadata",
		Long: `Show safe metadata for one API token.

The token public id is opaque and sent as one path segment. The plaintext token
secret is never available from this endpoint.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab tokens list
  chab tokens update`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab tokens show <token-public-id>\n  chab tokens show <token-public-id> --json\n  chab tokens show <token-public-id> --jq .token.policy_revision\n  chab tokens show <token-public-id> --template '{{.token.policy_revision}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOnlyRaw(cmd, f, api.Path("tokens", args[0]), nil)
		},
	}
}

type tokenMutationFlags struct {
	Name               string
	Permissions        []string
	FeatureMode        string
	FeatureScopes      []string
	SpendingMode       string
	AllowanceCredits   int64
	AllowanceNull      bool
	StartNewAllowance  bool
	ProjectMode        string
	ProjectIDs         []string
	IPRules            []string
	ExpiresAt          string
	ExpiresAtNull      bool
	Disabled           bool
	Revision           int64
	ProofFile          string
	ProofPrompt        bool
	SecretOut          string
	Body               string
	BodyFile           string
	IdempotencyKey     string
	changedAuthority   bool
	changedOnlyDisable bool
}

func newTokenCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &tokenMutationFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API token",
		Long: `Create a bounded child API token after management approval.

The plaintext access token is returned once by the API and must be written to
--secret-out. The CLI reserves that private path before the HTTP request and
prints only a safe receipt. Live create also requires --approval-proof-file or
--approval-proof-prompt; the proof is sent only in the X-Chab-Management-Approval
header.

Build the request from flags or provide the exact JSON object with --body or
--body-file. Flag mode requires --name and at least one --permission. Optional
authority fields include --feature-mode, --feature-scope, --spending-mode,
--allowance-credits, --allowance-credits-null, --start-new-allowance,
--project-mode, --project-id, --ip-rule and --expires-at.

Use --dry-run to preview the request without reserving --secret-out, resolving
credentials, reading proof input, or contacting the API. Mutating requests send
an idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
  chab tokens approvals create
  chab tokens approvals wait`,
		Args:    cobra.NoArgs,
		Example: "  chab tokens create --name worker --permission api:projects:read --spending-mode none --project-mode none --approval-proof-file proof.json --secret-out token.json\n  chab tokens create --body-file token.json --approval-proof-file proof.json --secret-out secret.json --json\n  chab tokens create --name worker --permission api:projects:read --dry-run\n  chab tokens create --name worker --permission api:projects:read --approval-proof-file proof.json --secret-out token.json --jq .secret_out\n  chab tokens create --name worker --permission api:projects:read --approval-proof-file proof.json --secret-out token.json --template '{{.secret_out}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenCreate(cmd, f, flags)
		},
	}
	registerTokenMutationFlags(cmd, flags, true)
	cmd.Flags().StringVar(&flags.SecretOut, "secret-out", "", "private path for the one-time plaintext token")
	return cmd
}

func newTokenUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &tokenMutationFlags{}
	cmd := &cobra.Command{
		Use:   "update <token-public-id>",
		Short: "Update an API token",
		Long: `Update API token policy or lifecycle metadata.

Flag mode requires --revision and at least one change. Use --approval-proof-file
or --approval-proof-prompt for cross-token or authority-widening mutations that
the server protects with management approval. The proof is sent only in
X-Chab-Management-Approval.

Authority-related changes require confirmation; --yes confirms only the risky
mutation and does not supply a proof, revision, budget, project grant or body
field. In --no-prompt or non-interactive mode, pass --yes after providing all
required inputs. Use --dry-run to preview the request without credentials,
proof input, confirmation or HTTP. Mutating requests send an idempotency key;
pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab tokens show
  chab tokens approvals create`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab tokens update <token-public-id> --revision 3 --disabled=true\n  chab tokens update <token-public-id> --revision 3 --spending-mode capped --allowance-credits 100 --approval-proof-file proof.json --yes\n  chab tokens update <token-public-id> --body-file mutation.json --approval-proof-file proof.json --yes --json\n  chab tokens update <token-public-id> --revision 3 --name worker --jq .token.name\n  chab tokens update <token-public-id> --revision 3 --name worker --template '{{.token.name}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenUpdate(cmd, f, flags, args[0])
		},
	}
	registerTokenMutationFlags(cmd, flags, false)
	return cmd
}

type tokenRevokeFlags struct {
	Revision       int64
	ProofFile      string
	ProofPrompt    bool
	IdempotencyKey string
}

func newTokenRevokeCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &tokenRevokeFlags{}
	cmd := &cobra.Command{
		Use:   "revoke <token-public-id>",
		Short: "Revoke an API token",
		Long: `Revoke an API token by public id and expected policy revision.

Revocation is permanent and requires confirmation. Cross-token revocation
requires an exact management approval proof; self-revocation is still confirmed
locally. --yes confirms only the revocation prompt and never supplies the
required revision or approval proof.
In --no-prompt or non-interactive mode, pass --yes after providing all required
inputs.

Use --dry-run to preview the request without credentials, proof input,
confirmation or HTTP. Mutating requests send an idempotency key; pass
--idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab tokens approvals create
  chab tokens approvals wait`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab tokens revoke <token-public-id> --revision 3 --approval-proof-file proof.json --yes\n  chab tokens revoke <token-public-id> --revision 3 --yes --json\n  chab tokens revoke <token-public-id> --revision 3 --dry-run\n  chab tokens revoke <token-public-id> --revision 3 --yes --jq .token.revoked\n  chab tokens revoke <token-public-id> --revision 3 --yes --template '{{.token.revoked}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenRevoke(cmd, f, flags, args[0])
		},
	}
	cmd.Flags().Int64Var(&flags.Revision, "revision", 0, "expected token policy revision")
	cmd.Flags().StringVar(&flags.ProofFile, "approval-proof-file", "", "private approval proof JSON file")
	cmd.Flags().BoolVar(&flags.ProofPrompt, "approval-proof-prompt", false, "read approval proof from a hidden prompt")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

func registerTokenMutationFlags(cmd *cobra.Command, flags *tokenMutationFlags, create bool) {
	cmd.Flags().StringVar(&flags.Name, "name", "", "token name")
	cmd.Flags().StringArrayVar(&flags.Permissions, "permission", nil, "token permission scope (repeatable)")
	cmd.Flags().StringVar(&flags.FeatureMode, "feature-mode", "", "feature access mode: none, selected, or all")
	cmd.Flags().StringArrayVar(&flags.FeatureScopes, "feature-scope", nil, "feature scope (repeatable)")
	cmd.Flags().StringVar(&flags.SpendingMode, "spending-mode", "", "spending mode: none, capped, or all")
	cmd.Flags().Int64Var(&flags.AllowanceCredits, "allowance-credits", 0, "spending allowance credits")
	cmd.Flags().BoolVar(&flags.AllowanceNull, "allowance-credits-null", false, "send allowance_credits as null")
	cmd.Flags().BoolVar(&flags.StartNewAllowance, "start-new-allowance", false, "start a new allowance version")
	cmd.Flags().StringVar(&flags.ProjectMode, "project-mode", "", "project access mode: none, selected, or all")
	cmd.Flags().StringArrayVar(&flags.ProjectIDs, "project-id", nil, "numeric project id grant (repeatable)")
	cmd.Flags().StringArrayVar(&flags.IPRules, "ip-rule", nil, "IP rule allow=<address-or-cidr> or deny=<address-or-cidr> (repeatable)")
	cmd.Flags().StringVar(&flags.ExpiresAt, "expires-at", "", "expiration date-time")
	cmd.Flags().BoolVar(&flags.ExpiresAtNull, "expires-at-null", false, "send expires_at as null")
	if !create {
		cmd.Flags().Int64Var(&flags.Revision, "revision", 0, "expected token policy revision")
		cmd.Flags().BoolVar(&flags.Disabled, "disabled", false, "enable or disable the token")
	}
	cmd.Flags().StringVar(&flags.ProofFile, "approval-proof-file", "", "private approval proof JSON file")
	cmd.Flags().BoolVar(&flags.ProofPrompt, "approval-proof-prompt", false, "read approval proof from a hidden prompt")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	registerBodyFlags(cmd, &flags.Body, &flags.BodyFile)
}

func runTokenCreate(cmd *cobra.Command, f *cmdutil.Factory, flags *tokenMutationFlags) error {
	body, err := tokenBody(cmd, f, flags, true)
	if err != nil {
		return err
	}
	if err := validateRequest("tokens.create", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPost, "/tokens", body, explicit)
	}
	if flags.SecretOut == "" {
		return &usageError{detail: "token create requires --secret-out"}
	}
	header, proof, err := mutationHeader(flags.ProofFile, flags.ProofPrompt, cmd, f)
	if err != nil {
		return err
	}
	if proof == "" {
		return &usageError{detail: "token create requires --approval-proof-file or --approval-proof-prompt"}
	}
	reserved, err := reservePrivateOutput(flags.SecretOut)
	if err != nil {
		return err
	}
	authn, err := oneShotSecretClient(cmd, f, proof)
	if err != nil {
		_ = reserved.Abort(true)
		return err
	}
	result, err := authn.Client.DoRawWithHeaders(cmd.Context(), http.MethodPost, "tokens", nil, api.ExactJSONBody(body), idem, header)
	if err != nil {
		_ = reserved.Abort(false)
		return apiCredentialError(err, authn.Cred)
	}
	var decoded struct {
		Token       tokenResource `json:"token"`
		AccessToken string        `json:"access_token"`
	}
	if err := json.Unmarshal(result.Data, &decoded); err != nil || decoded.AccessToken == "" {
		_ = reserved.Abort(false)
		if err == nil {
			err = fmt.Errorf("missing access_token")
		}
		return &api.ProtocolError{Detail: "token create response data does not match the expected shape", Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Err: err, Meta: result.Meta}
	}
	f.RegisterSecret(decoded.AccessToken)
	secretPayload := map[string]any{
		"operation":       "tokens.create",
		"token_public_id": decoded.Token.TokenPublicID,
		"access_token":    decoded.AccessToken,
		"request_id":      result.Meta.RequestID,
	}
	if err := commitSecretJSON(reserved, secretPayload); err != nil {
		return &usageError{detail: "token secret may have been consumed; saving private output failed: " + err.Error()}
	}
	receipt := secretReceipt{Operation: "tokens.create", ResourceID: decoded.Token.TokenPublicID, SecretOut: reserved.Path(), RequestID: result.Meta.RequestID, Status: "saved"}
	return writeSecretReceipt(cmd, f, receipt, result.Meta)
}

func runTokenUpdate(cmd *cobra.Command, f *cmdutil.Factory, flags *tokenMutationFlags, id string) error {
	body, err := tokenBody(cmd, f, flags, false)
	if err != nil {
		return err
	}
	if err := validateRequest("tokens.update", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPatch, "/"+api.Path("tokens", id), body, explicit)
	}
	header, proof, err := mutationHeader(flags.ProofFile, flags.ProofPrompt, cmd, f)
	if err != nil {
		return err
	}
	if tokenUpdateNeedsConfirmation(body) {
		if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Update token authority for "+id+"?"); err != nil {
			return err
		}
	}
	authn, err := resolveAuth(cmd, f, false, false, proof)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRawWithHeaders(cmd.Context(), http.MethodPatch, api.Path("tokens", id), nil, api.ExactJSONBody(body), idem, header)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func runTokenRevoke(cmd *cobra.Command, f *cmdutil.Factory, flags *tokenRevokeFlags, id string) error {
	if !cmd.Flags().Changed("revision") || flags.Revision < 1 {
		return &usageError{detail: "token revoke requires --revision"}
	}
	body, err := canonicalObject(map[string]any{"expected_policy_revision": flags.Revision})
	if err != nil {
		return err
	}
	if err := validateRequest("tokens.revoke", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPost, "/"+api.Path("tokens", id, "revoke"), body, explicit)
	}
	header, proof, err := mutationHeader(flags.ProofFile, flags.ProofPrompt, cmd, f)
	if err != nil {
		return err
	}
	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), "Revoke token "+id+"? This cannot be undone."); err != nil {
		return err
	}
	authn, err := resolveAuth(cmd, f, false, false, proof)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRawWithHeaders(cmd.Context(), http.MethodPost, api.Path("tokens", id, "revoke"), nil, api.ExactJSONBody(body), idem, header)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func tokenBody(cmd *cobra.Command, f *cmdutil.Factory, flags *tokenMutationFlags, create bool) (json.RawMessage, error) {
	fieldFlags := []string{
		"name",
		"permission",
		"feature-mode",
		"feature-scope",
		"spending-mode",
		"allowance-credits",
		"allowance-credits-null",
		"start-new-allowance",
		"project-mode",
		"project-id",
		"ip-rule",
		"expires-at",
		"expires-at-null",
	}
	if !create {
		fieldFlags = append(fieldFlags, "revision", "disabled")
	}
	body, bodyMode, err := readExclusiveBodyMode(cmd, f, flags.Body, flags.BodyFile, fieldFlags...)
	if err != nil || bodyMode {
		return body, err
	}
	fields := map[string]any{}
	if cmd.Flags().Changed("name") {
		fields["name"] = flags.Name
	}
	if cmd.Flags().Changed("permission") {
		fields["permissions"] = flags.Permissions
	}
	if cmd.Flags().Changed("feature-mode") {
		fields["feature_mode"] = flags.FeatureMode
	}
	if cmd.Flags().Changed("feature-scope") {
		fields["feature_scopes"] = flags.FeatureScopes
	}
	if cmd.Flags().Changed("spending-mode") {
		fields["spending_mode"] = flags.SpendingMode
	}
	if cmd.Flags().Changed("allowance-credits") && flags.AllowanceNull {
		return nil, &usageError{detail: "--allowance-credits and --allowance-credits-null cannot be combined"}
	}
	if flags.AllowanceNull {
		fields["allowance_credits"] = nil
	} else if cmd.Flags().Changed("allowance-credits") {
		fields["allowance_credits"] = flags.AllowanceCredits
	}
	if cmd.Flags().Changed("start-new-allowance") {
		fields["start_new_allowance"] = flags.StartNewAllowance
	}
	if cmd.Flags().Changed("project-mode") {
		fields["project_mode"] = flags.ProjectMode
	}
	if cmd.Flags().Changed("project-id") {
		ids, err := parseInt64List(flags.ProjectIDs, "project-id")
		if err != nil {
			return nil, err
		}
		fields["project_ids"] = ids
	}
	if cmd.Flags().Changed("ip-rule") {
		rules, err := parseIPRules(flags.IPRules)
		if err != nil {
			return nil, err
		}
		fields["ip_rules"] = rules
	}
	if cmd.Flags().Changed("expires-at") && flags.ExpiresAtNull {
		return nil, &usageError{detail: "--expires-at and --expires-at-null cannot be combined"}
	}
	if flags.ExpiresAtNull {
		fields["expires_at"] = nil
	} else if cmd.Flags().Changed("expires-at") {
		fields["expires_at"] = flags.ExpiresAt
	}
	if !create {
		if !cmd.Flags().Changed("revision") || flags.Revision < 1 {
			return nil, &usageError{detail: "token update requires --revision"}
		}
		fields["expected_policy_revision"] = flags.Revision
		if cmd.Flags().Changed("disabled") {
			fields["disabled"] = flags.Disabled
		}
		if len(fields) == 1 {
			return nil, &usageError{detail: "token update requires at least one change"}
		}
	} else {
		if !cmd.Flags().Changed("name") {
			return nil, &usageError{detail: "token create requires --name or a JSON body"}
		}
		if !cmd.Flags().Changed("permission") {
			return nil, &usageError{detail: "token create requires at least one --permission or a JSON body"}
		}
	}
	return canonicalObject(fields)
}

func tokenUpdateNeedsConfirmation(body json.RawMessage) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return true
	}
	for key, raw := range object {
		switch key {
		case "expected_policy_revision", "name":
			continue
		case "disabled":
			if string(raw) == "true" {
				continue
			}
		}
		return true
	}
	return false
}

type tokenResource struct {
	TokenPublicID  string   `json:"token_public_id"`
	Name           string   `json:"name"`
	Permissions    []string `json:"permissions"`
	PolicyRevision int64    `json:"policy_revision"`
	Disabled       bool     `json:"disabled"`
	Revoked        bool     `json:"revoked"`
	ExpiresAt      *string  `json:"expires_at"`
	LastUsedAt     *string  `json:"last_used_at"`
}

type secretReceipt struct {
	Operation  string `json:"operation"`
	ResourceID string `json:"resource_id,omitempty"`
	SecretOut  string `json:"secret_out"`
	RequestID  string `json:"request_id,omitempty"`
	Status     string `json:"status"`
}

func writeSecretReceipt(cmd *cobra.Command, f *cmdutil.Factory, receipt secretReceipt, meta api.ResponseMeta) error {
	return writeTypedValue(cmd, f, receipt, meta, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			output.Detail{Nodes: []output.Node{
				output.Field("operation", receipt.Operation),
				output.Field("resource_id", receipt.ResourceID),
				output.Field("secret_out", receipt.SecretOut),
				output.Field("request_id", receipt.RequestID),
				output.Field("status", receipt.Status),
			}}.Render(w)
		},
		Plain: func(data, prose io.Writer) {
			fmt.Fprintln(data, receipt.SecretOut)
		},
	})
}

type approvalCreateFlags struct {
	Action         string
	TargetToken    string
	Revision       int64
	Mutation       string
	IdempotencyKey string
}

type approvalWaitFlags struct {
	ProofOut string
}

func newTokenApprovalsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("approvals", "Manage approval challenges", `Manage human approval challenges for protected token and billing actions.

The CLI can create a challenge and poll for a one-time proof after a human
approves it in the product web app. It cannot approve its own request. Proof
polling uses one physical attempt per API call; ambiguous proof responses are
not retried automatically.

Related commands:
  chab tokens revoke
  chab billing purchases create`)
	cmd.AddCommand(newApprovalCreateCommand(f), newApprovalWaitCommand(f))
	return cmd
}

func newApprovalCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &approvalCreateFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an approval challenge",
		Long: `Create a management approval challenge.

Provide --action and --mutation. Use --target-token for token-targeted actions
such as tokens.update and tokens.revoke; when omitted, the current team is used
for tokens.create, billing.purchases.create, and billing.auto_recharge.update.
For token-targeted actions, --revision is sent as expected_policy_revision
outside mutation and must match the bound mutation request later.

The response includes a product-web verification URL resolved from the profile
base URL, not the API base URL. Complete that web approval, then run
chab tokens approvals wait <approval-id> --proof-out <path>.

Mutating requests send an idempotency key; pass --idempotency-key to provide
your own. Browser device login may not grant token or billing write scopes; use
a manually created team API key when needed.

Use --dry-run to preview the request without resolving credentials or
contacting the API. --include-meta adds safe transport context under meta when
--json, --jq, or --template is selected. It cannot be combined with --dry-run.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
  chab tokens approvals wait
  chab tokens revoke`,
		Args:    cobra.NoArgs,
		Example: "  chab tokens approvals create --action tokens.revoke --target-token <token-public-id> --revision 3 --mutation @empty.json\n  chab tokens approvals create --action billing.purchases.create --mutation '{\"package_id\":\"credits_1000\"}' --json\n  chab tokens approvals create --action tokens.create --mutation @token.json --idempotency-key <key>\n  chab tokens approvals create --action tokens.create --mutation @token.json --jq .approval_id\n  chab tokens approvals create --action tokens.create --mutation @token.json --template '{{.approval_id}}'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runApprovalCreate(cmd, f, flags)
		},
	}
	cmd.Flags().StringVar(&flags.Action, "action", "", "protected action, such as tokens.revoke")
	cmd.Flags().StringVar(&flags.TargetToken, "target-token", "", "target token public id for token-targeted actions")
	cmd.Flags().Int64Var(&flags.Revision, "revision", 0, "expected token policy revision for token-targeted actions")
	cmd.Flags().StringVar(&flags.Mutation, "mutation", "", "mutation JSON, @path, or @-")
	cmd.Flags().StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
	cmd.Flags().Bool("dry-run", false, "preview the request without resolving credentials or contacting the API")
	return cmd
}

type approvalChallenge struct {
	ApprovalID      string `json:"approval_id"`
	VerificationURI string `json:"verification_uri"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt       string `json:"expires_at"`
	NextCheckAt     string `json:"next_check_at"`
}

type approvalRecord struct {
	ApprovalID             string          `json:"approval_id"`
	Action                 string          `json:"action"`
	TargetType             string          `json:"target_type"`
	TargetID               string          `json:"target_id,omitempty"`
	ExpectedPolicyRevision *int64          `json:"expected_policy_revision,omitempty"`
	Mutation               json.RawMessage `json:"mutation"`
	IssuingTokenPublicID   string          `json:"issuing_token_public_id"`
	VerificationURL        string          `json:"verification_url"`
	ExpiresAt              string          `json:"expires_at"`
	NextCheckAt            string          `json:"next_check_at"`
}

func runApprovalCreate(cmd *cobra.Command, f *cmdutil.Factory, flags *approvalCreateFlags) error {
	if flags.Action == "" {
		return &usageError{detail: "approval create requires --action"}
	}
	if flags.Mutation == "" {
		return &usageError{detail: "approval create requires --mutation"}
	}
	mutation, err := parseJSONArgument(cmd, f, "mutation", flags.Mutation)
	if err != nil {
		return err
	}
	body, targetType, targetID, revision, err := approvalCreateBody(cmd, flags, mutation, 0)
	if err != nil {
		return err
	}
	if err := validateRequest("tokens.management_approvals.create", body); err != nil {
		return err
	}
	idem, explicit, err := idemFromFlags(cmd, f, flags.IdempotencyKey)
	if err != nil {
		return err
	}
	if dryRunEnabled(cmd) {
		return writeDryRun(cmd, f, http.MethodPost, "/management-approvals", body, explicit)
	}
	authn, err := resolveAuth(cmd, f, true, false)
	if err != nil {
		return err
	}
	body, targetType, targetID, revision, err = approvalCreateBody(cmd, flags, mutation, authn.Identity.TeamID)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodPost, "management-approvals", nil, api.ExactJSONBody(body), idem)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	var challenge approvalChallenge
	if err := json.Unmarshal(result.Data, &challenge); err != nil || challenge.ApprovalID == "" {
		if err == nil {
			err = fmt.Errorf("missing approval_id")
		}
		return &api.ProtocolError{Detail: "approval create response data does not match the expected shape", Status: result.Meta.HTTPStatus, RequestID: result.Meta.RequestID, Err: err, Meta: result.Meta}
	}
	challenge.VerificationURL = resolveAppURL(authn.Runtime.BaseURL, challenge.VerificationURI)
	record := approvalRecord{
		ApprovalID:             challenge.ApprovalID,
		Action:                 flags.Action,
		TargetType:             targetType,
		TargetID:               targetID,
		ExpectedPolicyRevision: revision,
		Mutation:               mutation,
		IssuingTokenPublicID:   authn.Identity.TokenPublicID,
		VerificationURL:        challenge.VerificationURL,
		ExpiresAt:              challenge.ExpiresAt,
		NextCheckAt:            challenge.NextCheckAt,
	}
	if err := saveApprovalRecord(pathForApprovalRecords(authn.Runtime), record); err != nil {
		return fmt.Errorf("approval %s was created but local context could not be saved: %w", challenge.ApprovalID, err)
	}
	return writeApprovalChallenge(cmd, f, challenge, result.Meta)
}

func approvalCreateBody(cmd *cobra.Command, flags *approvalCreateFlags, mutation json.RawMessage, teamID int64) (json.RawMessage, string, string, *int64, error) {
	var mutationObject map[string]any
	if err := json.Unmarshal(mutation, &mutationObject); err != nil {
		return nil, "", "", nil, &usageError{detail: "--mutation must be a JSON object"}
	}
	targetType := "team"
	targetID := ""
	if teamID > 0 {
		targetID = strconv.FormatInt(teamID, 10)
	} else {
		targetID = "current-team"
	}
	var revision *int64
	if flags.TargetToken != "" {
		targetType = "token"
		targetID = flags.TargetToken
		if !cmd.Flags().Changed("revision") || flags.Revision < 1 {
			return nil, "", "", nil, &usageError{detail: "token-targeted approval requires --revision"}
		}
		rev := flags.Revision
		revision = &rev
	} else if cmd.Flags().Changed("revision") {
		return nil, "", "", nil, &usageError{detail: "--revision is only valid with --target-token"}
	}
	fields := map[string]any{
		"action":      flags.Action,
		"target_type": targetType,
		"target_id":   targetID,
		"mutation":    mutationObject,
	}
	if revision != nil {
		fields["expected_policy_revision"] = *revision
	}
	body, err := canonicalObject(fields)
	return body, targetType, targetID, revision, err
}

func newApprovalWaitCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &approvalWaitFlags{}
	cmd := &cobra.Command{
		Use:   "wait <approval-id>",
		Short: "Wait for an approval proof",
		Long: `Wait for a management approval proof and write it to a private file.

The CLI polls the approval endpoint with a no-retry API client because the
proof is returned once. Pending responses follow Retry-After when present and
otherwise use the server's next_check_at or a short fallback. If the server says
proof_issued without returning a proof, the one-time value is no longer
recoverable; create a fresh approval instead of polling indefinitely.

The proof file is JSON and includes the approval id, action, target, revision
where applicable, issuing token context, normalized mutation and proof value.
The proof itself is never printed.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
  chab tokens revoke
  chab billing purchases create`,
		Args:    cobra.ExactArgs(1),
		Example: "  chab tokens approvals wait <approval-id> --proof-out proof.json\n  chab tokens approvals wait <approval-id> --proof-out proof.json --json\n  chab tokens approvals wait <approval-id> --proof-out proof.json --jq .secret_out\n  chab tokens approvals wait <approval-id> --proof-out proof.json --template '{{.secret_out}}'",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApprovalWait(cmd, f, flags, args[0])
		},
	}
	cmd.Flags().StringVar(&flags.ProofOut, "proof-out", "", "private path for the one-time approval proof")
	return cmd
}

type approvalPoll struct {
	ApprovalID  string  `json:"approval_id"`
	State       string  `json:"state"`
	ExpiresAt   string  `json:"expires_at"`
	NextCheckAt string  `json:"next_check_at,omitempty"`
	Proof       *string `json:"proof,omitempty"`
}

func runApprovalWait(cmd *cobra.Command, f *cmdutil.Factory, flags *approvalWaitFlags, approvalID string) error {
	if flags.ProofOut == "" {
		return &usageError{detail: "approval wait requires --proof-out"}
	}
	reserved, err := reservePrivateOutput(flags.ProofOut)
	if err != nil {
		return err
	}
	authn, err := resolveAuth(cmd, f, true, true)
	if err != nil {
		_ = reserved.Abort(true)
		return err
	}
	record, err := loadApprovalRecord(pathForApprovalRecords(authn.Runtime), approvalID)
	if err != nil || record.ApprovalID != approvalID || record.Action == "" || record.TargetType == "" {
		_ = reserved.Abort(true)
		return &usageError{detail: "local approval context is missing; create a fresh approval challenge before waiting for its proof"}
	}
	deadline := f.Clock()().Add(6 * time.Minute)
	var meta api.ResponseMeta
	for {
		var poll approvalPoll
		meta, err = authn.Client.Get(cmd.Context(), api.Path("management-approvals", approvalID), nil, &poll)
		if err != nil {
			_ = reserved.Abort(false)
			return apiCredentialError(err, authn.Cred)
		}
		if poll.Proof != nil && *poll.Proof != "" {
			f.RegisterSecret(*poll.Proof)
			payload := proofFile{
				ApprovalID:             approvalID,
				Action:                 record.Action,
				TargetType:             record.TargetType,
				TargetID:               record.TargetID,
				ExpectedPolicyRevision: record.ExpectedPolicyRevision,
				IssuingTokenPublicID:   record.IssuingTokenPublicID,
				Mutation:               record.Mutation,
				Proof:                  *poll.Proof,
			}
			if payload.IssuingTokenPublicID == "" {
				payload.IssuingTokenPublicID = authn.Identity.TokenPublicID
			}
			if err := commitSecretJSON(reserved, payload); err != nil {
				return &usageError{detail: "approval proof may have been consumed; saving private proof failed: " + err.Error()}
			}
			receipt := secretReceipt{Operation: "tokens.management_approvals.get", ResourceID: approvalID, SecretOut: reserved.Path(), RequestID: meta.RequestID, Status: "saved"}
			return writeSecretReceipt(cmd, f, receipt, meta)
		}
		switch poll.State {
		case "pending", "approved":
			if f.Clock()().After(deadline) {
				_ = reserved.Abort(true)
				return &usageError{detail: "timed out waiting for approval " + approvalID}
			}
			sleep := waitFromMetaOrTimestamp(f.Clock(), meta, poll.NextCheckAt, 2*time.Second)
			if sleep > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "approval %s is %s; waiting %s\n", approvalID, poll.State, sleep)
				if err := f.Sleep(cmd.Context(), sleep); err != nil {
					_ = reserved.Abort(false)
					return err
				}
			}
		case "proof_issued":
			_ = reserved.Abort(true)
			return &usageError{detail: "approval proof was already issued and cannot be recovered; create a fresh approval"}
		case "denied", "expired", "consumed":
			_ = reserved.Abort(true)
			return &usageError{detail: "approval " + approvalID + " reached terminal state " + poll.State}
		default:
			_ = reserved.Abort(true)
			return &usageError{detail: "approval " + approvalID + " returned unsupported state " + poll.State}
		}
	}
}

func writeApprovalChallenge(cmd *cobra.Command, f *cmdutil.Factory, challenge approvalChallenge, meta api.ResponseMeta) error {
	return writeTypedValue(cmd, f, challenge, meta, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			output.Detail{Nodes: []output.Node{
				output.Field("approval_id", challenge.ApprovalID),
				output.Field("verification_url", challenge.VerificationURL),
				output.Field("expires_at", challenge.ExpiresAt),
				output.Field("next_check_at", challenge.NextCheckAt),
			}}.Render(w)
		},
		Plain: func(data, prose io.Writer) {
			fmt.Fprintln(data, challenge.ApprovalID)
			if challenge.VerificationURL != "" {
				fmt.Fprintln(prose, challenge.VerificationURL)
			}
		},
	})
}

func saveApprovalRecord(root string, record approvalRecord) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return localfile.AtomicWrite(filepath.Join(root, record.ApprovalID+".json"), data, 0o700, 0o600)
}

func loadApprovalRecord(root, id string) (approvalRecord, error) {
	var record approvalRecord
	data, err := os.ReadFile(filepath.Join(root, id+".json"))
	if err != nil {
		return record, err
	}
	err = json.Unmarshal(data, &record)
	return record, err
}

func resolveAppURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(refURL).String()
}
