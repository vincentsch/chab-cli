package operationscmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/operations"
)

type valueKind string

const (
	valueString      valueKind = "string"
	valueInt         valueKind = "int"
	valueBool        valueKind = "bool"
	valueJSON        valueKind = "json"
	valueStringSlice valueKind = "string-slice"
	valueArtifactID  valueKind = "artifact-id"
)

type fieldFlag struct {
	Name  string
	Field string
	Kind  valueKind
	Usage string
}

type requestFlags struct {
	Input          string
	Set            []string
	IdempotencyKey string
	Wait           bool
	OfflinePreview bool
	MaxCredits     int64
	fields         map[string]*fieldValue
}

type fieldValue struct {
	kind   valueKind
	field  string
	s      string
	i      int64
	b      bool
	values []string
}

func registerRequestFlags(cmd *cobra.Command, flags *requestFlags, fields []fieldFlag, includeStart bool) {
	flags.fields = map[string]*fieldValue{}
	f := cmd.Flags()
	f.StringVar(&flags.Input, "input", "", "JSON request body, @path, or @- for stdin")
	f.StringArrayVar(&flags.Set, "set", nil, "top-level request field as name=json (repeatable)")
	if includeStart {
		f.StringVar(&flags.IdempotencyKey, "idempotency-key", "", "explicit idempotency key; generated when omitted")
		f.BoolVar(&flags.Wait, "wait", false, "wait for an accepted operation to reach a terminal state")
		f.BoolVar(&flags.OfflinePreview, "dry-run", false, "preview the resolved request without resolving credentials or contacting the API")
		f.Int64Var(&flags.MaxCredits, "max-credits", 0, "set max_credit_budget for supported operations")
	}
	for _, spec := range fields {
		value := &fieldValue{kind: spec.Kind, field: spec.Field}
		flags.fields[spec.Name] = value
		switch spec.Kind {
		case valueString, valueJSON, valueArtifactID:
			f.StringVar(&value.s, spec.Name, "", spec.Usage)
		case valueInt:
			f.Int64Var(&value.i, spec.Name, 0, spec.Usage)
		case valueBool:
			f.BoolVar(&value.b, spec.Name, false, spec.Usage)
		case valueStringSlice:
			f.StringArrayVar(&value.values, spec.Name, nil, spec.Usage)
		}
	}
}

func resolveRequest(cmd *cobra.Command, f *cmdutil.Factory, flags *requestFlags, operationKey string) ([]byte, error) {
	object := map[string]json.RawMessage{}
	if flags.Input != "" {
		raw, err := readInput(cmd, f, flags.Input)
		if err != nil {
			return nil, err
		}
		canon, err := operations.CanonicalizeJSON(raw)
		if err != nil {
			return nil, &usageError{detail: err.Error()}
		}
		if err := json.Unmarshal(canon, &object); err != nil {
			return nil, &usageError{detail: "--input must be a JSON object"}
		}
	}
	for _, entry := range flags.Set {
		key, raw, err := parseSet(entry)
		if err != nil {
			return nil, err
		}
		if err := putField(object, key, raw); err != nil {
			return nil, err
		}
	}
	for name, field := range flags.fields {
		if !cmd.Flags().Changed(name) {
			continue
		}
		raw, err := field.raw()
		if err != nil {
			return nil, err
		}
		if err := putField(object, field.field, raw); err != nil {
			return nil, err
		}
	}
	if cmd.Flags().Changed("max-credits") {
		if !supportsMaxCredits(operationKey) {
			return nil, &usageError{detail: "--max-credits is supported only for llm.generate, llm.embeddings, translate.text_or_document, and research.deep"}
		}
		raw := json.RawMessage(strconv.FormatInt(flags.MaxCredits, 10))
		if err := putField(object, "max_credit_budget", raw); err != nil {
			return nil, err
		}
	}
	if _, ok := object["max_credit_budget"]; ok && !supportsMaxCredits(operationKey) {
		return nil, &usageError{detail: "max_credit_budget is supported only for llm.generate, llm.embeddings, translate.text_or_document, and research.deep"}
	}
	if operationKey == "research.deep" {
		for _, key := range []string{"question", "mode", "source_budget", "max_credit_budget"} {
			if _, ok := object[key]; !ok {
				return nil, &usageError{detail: "research.deep requires question, mode, source_budget, and max_credit_budget"}
			}
		}
	}
	request, err := marshalObject(object)
	if err != nil {
		return nil, err
	}
	registry, err := chabcontract.Load()
	if err != nil {
		return nil, err
	}
	serverDryRun, err := operations.RequestDryRun(request)
	if err != nil {
		return nil, &usageError{detail: err.Error()}
	}
	if op, ok := registry.Find(operationKey); ok && serverDryRun && !op.DryRunSupported {
		return request, nil
	}
	if err := registry.ValidateRequest(operationKey, request); err != nil {
		return nil, &usageError{detail: err.Error()}
	}
	return request, nil
}

func readInput(cmd *cobra.Command, f *cmdutil.Factory, source string) ([]byte, error) {
	if source == "@-" {
		text, err := f.Prompt(cmd).ReadPiped()
		if err != nil {
			return nil, &usageError{detail: "could not read request body from stdin"}
		}
		return []byte(text), nil
	}
	if path, ok := strings.CutPrefix(source, "@"); ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, &usageError{detail: "could not read request body file"}
		}
		return data, nil
	}
	return []byte(source), nil
}

func parseSet(entry string) (string, json.RawMessage, error) {
	key, value, ok := strings.Cut(entry, "=")
	if !ok || key == "" {
		return "", nil, &usageError{detail: "--set must be name=json"}
	}
	raw := []byte(value)
	if !json.Valid(raw) {
		encoded, _ := json.Marshal(value)
		raw = encoded
	}
	canon, err := operations.CanonicalizeJSON(raw)
	if err != nil {
		return "", nil, &usageError{detail: "--set value must be valid JSON or a string"}
	}
	return key, json.RawMessage(canon), nil
}

func (v *fieldValue) raw() (json.RawMessage, error) {
	switch v.kind {
	case valueString:
		data, _ := json.Marshal(v.s)
		return data, nil
	case valueInt:
		return json.RawMessage(strconv.FormatInt(v.i, 10)), nil
	case valueBool:
		if v.b {
			return json.RawMessage("true"), nil
		}
		return json.RawMessage("false"), nil
	case valueJSON:
		canon, err := operations.CanonicalizeJSON([]byte(v.s))
		if err != nil {
			return nil, &usageError{detail: fmt.Sprintf("--%s must be valid JSON", strings.ReplaceAll(v.field, "_", "-"))}
		}
		return json.RawMessage(canon), nil
	case valueArtifactID:
		data, _ := json.Marshal(map[string]string{"type": "artifact_id", "id": v.s})
		return data, nil
	case valueStringSlice:
		data, _ := json.Marshal(v.values)
		return data, nil
	default:
		return nil, &usageError{detail: "unknown request field type"}
	}
}

func putField(object map[string]json.RawMessage, key string, raw json.RawMessage) error {
	if existing, ok := object[key]; ok {
		left, err := operations.CanonicalizeJSON(existing)
		if err != nil {
			return &usageError{detail: "existing request field could not be canonicalized"}
		}
		right, err := operations.CanonicalizeJSON(raw)
		if err != nil {
			return &usageError{detail: "request field could not be canonicalized"}
		}
		if string(left) != string(right) {
			return &usageError{detail: fmt.Sprintf("request field %q was provided more than once with different values", key)}
		}
		return nil
	}
	object[key] = append(json.RawMessage(nil), raw...)
	return nil
}

func marshalObject(object map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	canon, err := operations.CanonicalizeJSON(data)
	if err != nil {
		return nil, err
	}
	return canon, nil
}

func supportsMaxCredits(operationKey string) bool {
	switch operationKey {
	case "llm.generate", "llm.embeddings", "translate.text_or_document", "research.deep":
		return true
	default:
		return false
	}
}
