package management

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/localfile"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/output"
)

const approvalHeader = "X-Chab-Management-Approval"

type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "management command error"
	}
	return "management command error: " + e.detail
}

func (e *usageError) ExitCode() int { return 1 }

type authContext struct {
	Runtime  config.Runtime
	Cred     auth.Credential
	Client   *api.Client
	Identity api.WhoamiData
}

func family(use, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
}

func resolveAuth(cmd *cobra.Command, f *cmdutil.Factory, withIdentity bool, noRetry bool, secretValues ...string) (authContext, error) {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return authContext{}, err
	}
	cred, findings, err := f.Credential(rt)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return authContext{}, fmt.Errorf("%w; run \"chab login\" or set CHAB_API_KEY", err)
	}
	client, err := f.APIClientConfigured(rt, cred, cmd, func(opts *api.Options) {
		if noRetry {
			opts.MaxAttempts = 1
			opts.DisableTransportReuse = true
		}
		opts.SecretValues = append(opts.SecretValues, secretValues...)
	})
	if err != nil {
		return authContext{}, err
	}
	authn := authContext{Runtime: rt, Cred: cred, Client: client}
	if withIdentity {
		identity, _, err := client.Whoami(cmd.Context())
		if err != nil {
			return authContext{}, output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
		}
		authn.Identity = identity
	}
	return authn, nil
}

func apiCredentialError(err error, cred auth.Credential) error {
	return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
}

func writeRawJSONValue(cmd *cobra.Command, f *cmdutil.Factory, value json.RawMessage, meta api.ResponseMeta) error {
	return f.WriteResultWithMeta(cmd, value, meta, true, cmdutil.HumanOutput{
		Render: func(w io.Writer) { output.RawValueHuman(w, value) },
		Plain:  func(data, prose io.Writer) { output.RawValuePlain(data, prose, value) },
	})
}

func writeTypedValue(cmd *cobra.Command, f *cmdutil.Factory, value any, meta api.ResponseMeta, human cmdutil.HumanOutput) error {
	return f.WriteResultWithMeta(cmd, value, meta, true, human)
}

func readOnlyRaw(cmd *cobra.Command, f *cmdutil.Factory, path string, query url.Values) error {
	authn, err := resolveAuth(cmd, f, false, false)
	if err != nil {
		return err
	}
	result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, path, query, nil, api.IdempotencyNone)
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	return writeRawJSONValue(cmd, f, result.Data, result.Meta)
}

func cursorListRaw(cmd *cobra.Command, f *cmdutil.Factory, path string, baseQuery url.Values, flags *cmdutil.CursorPaginationFlags) error {
	plan, err := cmdutil.ResolveCursorListPlan(cmd, flags)
	if err != nil {
		return &usageError{detail: strings.TrimPrefix(err.Error(), "invalid list options: ")}
	}
	authn, err := resolveAuth(cmd, f, false, false)
	if err != nil {
		return err
	}
	outcome, err := cmdutil.FetchCursorPages[json.RawMessage](plan, func(cursor string, limit int) ([]json.RawMessage, api.ResponseMeta, error) {
		q := cloneValues(baseQuery)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		var rows []json.RawMessage
		meta, err := authn.Client.Get(cmd.Context(), path, q, &rows)
		return rows, meta, err
	})
	if err != nil {
		return apiCredentialError(err, authn.Cred)
	}
	data, err := json.Marshal(outcome.Rows)
	if err != nil {
		return err
	}
	return writeTypedValue(cmd, f, json.RawMessage(data), outcome.Meta, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			output.RawValueHuman(w, json.RawMessage(data))
			if hint := cmdutil.CursorPaginationHint(plan, len(outcome.Rows), outcome.Meta.CursorPagination); hint != "" {
				fmt.Fprintln(w, hint)
			}
		},
		Plain: func(dataOut, prose io.Writer) {
			output.RawValuePlain(dataOut, prose, json.RawMessage(data))
			if hint := cmdutil.CursorPaginationHint(plan, len(outcome.Rows), outcome.Meta.CursorPagination); hint != "" {
				fmt.Fprintln(prose, hint)
			}
		},
	})
}

func registerCursorFlags(cmd *cobra.Command, flags *cmdutil.CursorPaginationFlags) {
	cmdutil.RegisterCursorPaginationFlags(cmd, flags)
}

func registerBodyFlags(cmd *cobra.Command, body *string, bodyFile *string) {
	cmd.Flags().StringVar(body, "body", "", "JSON request body")
	cmd.Flags().StringVar(bodyFile, "body-file", "", "read the JSON request body from a file, or - for stdin")
}

func readBodyMode(cmd *cobra.Command, f *cmdutil.Factory, body, bodyFile string) (json.RawMessage, bool, error) {
	bodySet := cmd.Flags().Changed("body")
	bodyFileSet := cmd.Flags().Changed("body-file")
	if bodySet && bodyFileSet {
		return nil, false, &usageError{detail: "--body and --body-file cannot be combined"}
	}
	if !bodySet && !bodyFileSet {
		return nil, false, nil
	}
	var raw []byte
	if bodySet {
		raw = []byte(body)
	} else if bodyFile == "-" {
		text, err := f.Prompt(cmd).ReadPiped()
		if err != nil {
			return nil, true, &usageError{detail: "could not read request body from stdin"}
		}
		raw = []byte(text)
	} else {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, true, &usageError{detail: "could not read request body file"}
		}
		raw = data
	}
	canon, err := operations.CanonicalizeJSON(raw)
	if err != nil {
		return nil, true, &usageError{detail: err.Error()}
	}
	if bytes.TrimSpace(canon)[0] != '{' {
		return nil, true, &usageError{detail: "request body must be a JSON object"}
	}
	return json.RawMessage(canon), true, nil
}

func readExclusiveBodyMode(cmd *cobra.Command, f *cmdutil.Factory, body, bodyFile string, fieldFlags ...string) (json.RawMessage, bool, error) {
	bodyValue, bodyMode, err := readBodyMode(cmd, f, body, bodyFile)
	if err != nil || !bodyMode {
		return bodyValue, bodyMode, err
	}
	for _, name := range fieldFlags {
		if cmd.Flags().Changed(name) {
			return nil, true, &usageError{detail: "--body/--body-file cannot be combined with --" + name}
		}
	}
	return bodyValue, true, nil
}

func canonicalObject(value map[string]any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	canon, err := operations.CanonicalizeJSON(data)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canon), nil
}

func validateRequest(operationKey string, body json.RawMessage) error {
	registry, err := chabcontract.Load()
	if err != nil {
		return err
	}
	if err := registry.ValidateRequest(operationKey, body); err != nil {
		return &usageError{detail: err.Error()}
	}
	return nil
}

func idemFromFlags(cmd *cobra.Command, f *cmdutil.Factory, key string) (api.Idempotency, bool, error) {
	explicit := cmd.Flags().Changed("idempotency-key")
	if explicit {
		if err := api.ValidateIdempotencyKey(key); err != nil {
			return api.IdempotencyNone, false, err
		}
		f.RegisterSecret(key)
		return api.ExplicitIdempotency(key), true, nil
	}
	return api.AutoIdempotency(), false, nil
}

func dryRunEnabled(cmd *cobra.Command) bool {
	if cmd == nil || cmd.LocalNonPersistentFlags().Lookup("dry-run") == nil {
		return false
	}
	enabled, err := cmd.LocalNonPersistentFlags().GetBool("dry-run")
	return err == nil && enabled
}

func dryRunIdempotency(explicit bool) *output.DryRunIdempotency {
	if explicit {
		return &output.DryRunIdempotency{Source: "explicit"}
	}
	return &output.DryRunIdempotency{Source: "generated"}
}

func writeDryRun(cmd *cobra.Command, f *cmdutil.Factory, method, path string, body json.RawMessage, explicitKey bool) error {
	preview := output.DryRunPreview{
		Method:      method,
		Path:        f.RedactValue(path),
		Body:        dryRunBody(body),
		Idempotency: dryRunIdempotency(explicitKey),
	}
	return f.WriteResult(cmd, preview, cmdutil.HumanOutput{
		Render: func(w io.Writer) { preview.Render(w) },
		Plain:  preview.RenderPlain,
	})
}

func dryRunBody(body json.RawMessage) []output.DryRunValue {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return []output.DryRunValue{{Name: "body", Value: string(body)}}
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sortStrings(keys)
	nodes := make([]output.DryRunValue, 0, len(keys))
	for _, key := range keys {
		nodes = append(nodes, output.DryRunValue{Name: key, Value: string(object[key])})
	}
	return nodes
}

func reservePrivateOutput(path string) (*localfile.ReservedPrivateFile, error) {
	reserved, err := localfile.ReservePrivate(path)
	if err != nil {
		return nil, &usageError{detail: "could not reserve private output: " + err.Error()}
	}
	return reserved, nil
}

func commitSecretJSON(reserved *localfile.ReservedPrivateFile, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return reserved.Commit(data)
}

type proofFile struct {
	ApprovalID             string          `json:"approval_id"`
	Action                 string          `json:"action"`
	TargetType             string          `json:"target_type"`
	TargetID               string          `json:"target_id,omitempty"`
	ExpectedPolicyRevision *int64          `json:"expected_policy_revision,omitempty"`
	IssuingTokenPublicID   string          `json:"issuing_token_public_id"`
	Mutation               json.RawMessage `json:"mutation,omitempty"`
	Proof                  string          `json:"proof"`
}

func approvalProofHeader(cmd *cobra.Command, f *cmdutil.Factory, path string, prompt bool) (http.Header, string, error) {
	switch {
	case path != "" && prompt:
		return nil, "", &usageError{detail: "--approval-proof-file and --approval-proof-prompt cannot be combined"}
	case path != "":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", &usageError{detail: "could not read approval proof file"}
		}
		proof, err := decodeProof(data)
		if err != nil {
			return nil, "", err
		}
		f.RegisterSecret(proof)
		return http.Header{approvalHeader: []string{proof}}, proof, nil
	case prompt:
		if !f.Prompt(cmd).Interactive() {
			return nil, "", &usageError{detail: "approval proof prompt requires an interactive terminal"}
		}
		proof, err := f.Prompt(cmd).Secret("Approval proof")
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(proof) == "" {
			return nil, "", &usageError{detail: "approval proof must not be empty"}
		}
		f.RegisterSecret(proof)
		return http.Header{approvalHeader: []string{proof}}, proof, nil
	default:
		return nil, "", nil
	}
}

func decodeProof(data []byte) (string, error) {
	var structured proofFile
	if json.Unmarshal(data, &structured) == nil && structured.Proof != "" {
		return structured.Proof, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) == nil {
		if raw := object["proof"]; len(raw) > 0 {
			var proof string
			if json.Unmarshal(raw, &proof) == nil && proof != "" {
				return proof, nil
			}
		}
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n\t ") || strings.HasPrefix(trimmed, "{") {
		return "", &usageError{detail: "approval proof file must contain the private JSON proof format"}
	}
	return trimmed, nil
}

func parseJSONArgument(cmd *cobra.Command, f *cmdutil.Factory, flagName, value string) (json.RawMessage, error) {
	if strings.HasPrefix(value, "@") {
		source := strings.TrimPrefix(value, "@")
		if source == "-" {
			text, err := f.Prompt(cmd).ReadPiped()
			if err != nil {
				return nil, &usageError{detail: "could not read " + flagName + " from stdin"}
			}
			value = text
		} else {
			data, err := os.ReadFile(source)
			if err != nil {
				return nil, &usageError{detail: "could not read " + flagName + " file"}
			}
			value = string(data)
		}
	}
	canon, err := operations.CanonicalizeJSON([]byte(value))
	if err != nil {
		return nil, &usageError{detail: "--" + flagName + " must be valid JSON"}
	}
	return json.RawMessage(canon), nil
}

func parseInt64List(values []string, flag string) ([]int64, error) {
	out := make([]int64, 0, len(values))
	for _, value := range values {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1 {
			return nil, &usageError{detail: "--" + flag + " must be a positive integer"}
		}
		out = append(out, n)
	}
	return out, nil
}

func parseIPRules(values []string) ([]map[string]string, error) {
	out := make([]map[string]string, 0, len(values))
	for _, value := range values {
		typ, network, ok := strings.Cut(value, "=")
		if !ok || (typ != "allow" && typ != "deny") || network == "" {
			return nil, &usageError{detail: "--ip-rule must use allow=<address-or-cidr> or deny=<address-or-cidr>"}
		}
		out = append(out, map[string]string{"type": typ, "value": network})
	}
	return out, nil
}

func oneShotSecretClient(cmd *cobra.Command, f *cmdutil.Factory, proof string) (authContext, error) {
	var secrets []string
	if proof != "" {
		secrets = append(secrets, proof)
	}
	return resolveAuth(cmd, f, false, true, secrets...)
}

func mutationHeader(path string, prompt bool, cmd *cobra.Command, f *cmdutil.Factory) (http.Header, string, error) {
	header, proof, err := approvalProofHeader(cmd, f, path, prompt)
	if err != nil {
		return nil, "", err
	}
	return header, proof, nil
}

func pathForApprovalRecords(rt config.Runtime) string {
	return filepath.Join(filepath.Dir(rt.ConfigPath), "management-approvals")
}

func cloneValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, raw := range values {
		out[key] = append([]string(nil), raw...)
	}
	return out
}

func waitFromMetaOrTimestamp(now func() time.Time, meta api.ResponseMeta, next string, fallback time.Duration) time.Duration {
	if meta.RetryAfter.Wait != nil {
		return *meta.RetryAfter.Wait
	}
	if next != "" {
		if t, err := time.Parse(time.RFC3339, next); err == nil {
			wait := t.Sub(now())
			if wait > 0 {
				return wait
			}
			return 0
		}
	}
	return fallback
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}
