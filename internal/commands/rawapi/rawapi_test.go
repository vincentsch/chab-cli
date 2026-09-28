package rawapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

func TestNormalizeRawPath(t *testing.T) {
	t.Parallel()

	valid := map[string][2]string{
		"projects":           {"projects", "/projects"},
		"/projects":          {"projects", "/projects"},
		"/projects/a:b":      {"projects/a:b", "/projects/a:b"},
		"projects/a:b":       {"projects/a:b", "/projects/a:b"},
		"/mailto:x":          {"mailto:x", "/mailto:x"},
		".":                  {"%2E", "/%2E"},
		"%2f":                {"%252f", "/%252f"},
		"/projects/opaque x": {"projects/opaque%20x", "/projects/opaque%20x"},
	}
	for input, want := range valid {
		input, want := input, want
		t.Run("valid "+input, func(t *testing.T) {
			t.Parallel()
			requestPath, displayPath, err := normalizeRawPath(input)
			if err != nil {
				t.Fatalf("normalizeRawPath(%q) error = %v", input, err)
			}
			if requestPath != want[0] || displayPath != want[1] {
				t.Fatalf("normalizeRawPath(%q) = %q, %q; want %q, %q", input, requestPath, displayPath, want[0], want[1])
			}
		})
	}

	for _, input := range []string{
		"", "/", "//host/path", "http://host/path", "HTTP:/host/path",
		"https:opaque", "mailto:x", "projects//id", "projects/", "projects?x=1",
		"projects#fragment",
	} {
		input := input
		t.Run("invalid "+input, func(t *testing.T) {
			t.Parallel()
			if _, _, err := normalizeRawPath(input); err == nil {
				t.Fatalf("normalizeRawPath(%q) error = nil", input)
			}
		})
	}
}

func TestOneTimeSecretRouteClassification(t *testing.T) {
	t.Parallel()

	matches := []string{
		"/tokens",
		"/webhooks/endpoints",
		"/webhooks/endpoints/wh_123/rotate-secret",
		"/webhooks/endpoints/opaque:id/rotate-secret",
	}
	for _, input := range matches {
		input := input
		t.Run("match "+input, func(t *testing.T) {
			t.Parallel()
			requestPath, _, err := normalizeRawPath(input)
			if err != nil {
				t.Fatalf("normalizeRawPath(%q) error = %v", input, err)
			}
			if !isOneTimeSecretRoute(http.MethodPost, requestPath) {
				t.Fatalf("isOneTimeSecretRoute(POST, %q) = false", requestPath)
			}
		})
	}

	for _, test := range []struct {
		name       string
		method     string
		input      string
		directPath string
	}{
		{name: "safe method", method: http.MethodGet, input: "/tokens"},
		{name: "method case lookalike", method: "post", input: "/tokens"},
		{name: "wrong unsafe method", method: http.MethodPatch, input: "/tokens"},
		{name: "prefix lookalike", method: http.MethodPost, input: "/prefix/tokens"},
		{name: "suffix lookalike", method: http.MethodPost, input: "/tokens-extra"},
		{name: "missing rotate literal", method: http.MethodPost, input: "/webhooks/endpoints/wh_123"},
		{name: "extra rotate segment", method: http.MethodPost, input: "/webhooks/endpoints/wh_123/rotate-secret/again"},
		{name: "escaped fixed literal", method: http.MethodPost, input: "/tok%65ns"},
		{name: "escaped rotate literal", method: http.MethodPost, input: "/webhooks/endpoints/wh_123/rotate%2Dsecret"},
		{name: "encoded separator wildcard", method: http.MethodPost, input: "/webhooks/endpoints/wh%2F123/rotate-secret"},
		{name: "dot segment wildcard", method: http.MethodPost, input: "/webhooks/endpoints/./rotate-secret"},
		{name: "encoded dot segment wildcard", method: http.MethodPost, input: "/webhooks/endpoints/%2E%2E/rotate-secret"},
		{name: "wrong webhook suffix", method: http.MethodPost, input: "/webhooks/endpoints/wh_123/rotate"},
		{name: "repeated segment", method: http.MethodPost, directPath: "webhooks/endpoints//rotate-secret"},
		{name: "caller supplied encoded separator", method: http.MethodPost, directPath: api.Path("webhooks", "endpoints", "wh_123/rotate-secret", "rotate-secret")},
	} {
		test := test
		t.Run("no match "+test.name, func(t *testing.T) {
			t.Parallel()
			requestPath := test.directPath
			if requestPath == "" {
				var err error
				requestPath, _, err = normalizeRawPath(test.input)
				if err != nil {
					t.Fatalf("normalizeRawPath(%q) error = %v", test.input, err)
				}
			}
			if isOneTimeSecretRoute(test.method, requestPath) {
				t.Fatalf("isOneTimeSecretRoute(%q, %q) = true", test.method, requestPath)
			}
		})
	}
}

func TestParseQueryPreservesEntryAndRepeatedValueOrder(t *testing.T) {
	t.Parallel()

	entries := []string{"z=first", "a=", "z=second=part", "a=with,comma"}
	pairs, values, err := parseQuery(entries)
	if err != nil {
		t.Fatalf("parseQuery() error = %v", err)
	}
	wantPairs := []kv{
		{key: "z", value: "first"},
		{key: "a", value: ""},
		{key: "z", value: "second=part"},
		{key: "a", value: "with,comma"},
	}
	if !reflect.DeepEqual(pairs, wantPairs) {
		t.Fatalf("pairs = %#v, want %#v", pairs, wantPairs)
	}
	if got, want := values.Encode(), "a=&a=with%2Ccomma&z=first&z=second%3Dpart"; got != want {
		t.Fatalf("encoded query = %q, want %q", got, want)
	}
}

func TestRequestJSONPreservesDuplicatesNumbersAndMasksDecodedKeys(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"token":"first","tok\u0065n":{"escaped":"line\u0020value","nested":9007199254740993},"safe":[{"token":"third"}],"token":null}`)
	node, err := parseRequestJSON(raw)
	if err != nil {
		t.Fatalf("parseRequestJSON() error = %v", err)
	}
	masked, err := maskedRequestJSON(node, map[string]bool{"token": true})
	if err != nil {
		t.Fatalf("maskedRequestJSON() error = %v", err)
	}
	want := `{"token":"[REDACTED]","token":"[REDACTED]","safe":[{"token":"[REDACTED]"}],"token":"[REDACTED]"}`
	if masked != want {
		t.Fatalf("masked JSON = %s, want %s", masked, want)
	}

	var collected []string
	collectRequestJSONSecrets(node, map[string]bool{"token": true}, func(value string) {
		collected = append(collected, value)
	})
	for _, wantValue := range []string{
		"first",
		`{"escaped":"line\u0020value","nested":9007199254740993}`,
		"line value",
		"9007199254740993",
		"third",
		"null",
	} {
		if !containsString(collected, wantValue) {
			t.Fatalf("collected secrets %q missing %q", collected, wantValue)
		}
	}
}

func TestSecretFieldSetRejectsEmptyAndDeduplicatesNames(t *testing.T) {
	t.Parallel()

	got, err := secretFieldSet([]string{"token", "missing", "token"})
	if err != nil {
		t.Fatalf("secretFieldSet() error = %v", err)
	}
	want := map[string]bool{"token": true, "missing": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("secretFieldSet() = %#v, want %#v", got, want)
	}
	if _, err := secretFieldSet([]string{"token", ""}); err == nil {
		t.Fatal("secretFieldSet(empty) error = nil")
	}
}

func TestRequestJSONParserFailsOnMalformedAndTrailingInput(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{``, `{"broken":`, `{} []`} {
		if node, err := parseRequestJSON(json.RawMessage(raw)); err == nil || node != nil {
			t.Fatalf("parseRequestJSON(%q) = %#v, %v; want nil, error", raw, node, err)
		}
	}
}

func TestDryRunFlagIsLeafLocal(t *testing.T) {
	t.Parallel()

	factory := &cmdutil.Factory{}
	apiCommand := NewAPICommand(factory)
	for _, name := range []string{"post", "patch", "delete"} {
		command, _, err := apiCommand.Find([]string{name})
		if err != nil || command == nil {
			t.Fatalf("Find(%s) = %v, %v", name, command, err)
		}
		if command.LocalNonPersistentFlags().Lookup("dry-run") == nil {
			t.Fatalf("%s does not own --dry-run", name)
		}
		if err := command.LocalNonPersistentFlags().Set("dry-run", "true"); err != nil {
			t.Fatalf("set %s --dry-run: %v", name, err)
		}
		if !dryRunEnabled(command) {
			t.Fatalf("dryRunEnabled(%s) = false", name)
		}
	}
	get, _, err := apiCommand.Find([]string{"get"})
	if err != nil || get == nil {
		t.Fatalf("Find(get) = %v, %v", get, err)
	}
	if dryRunEnabled(get) || get.LocalNonPersistentFlags().Lookup("dry-run") != nil {
		t.Fatal("GET exposes dry-run")
	}
}

func TestRawCommandFlagsAreExact(t *testing.T) {
	t.Parallel()

	apiCommand := NewAPICommand(&cmdutil.Factory{})
	tests := map[string][]string{
		"get":    {"all", "cursor", "limit", "page-size", "query", "raw", "secret-field"},
		"post":   {"body", "body-file", "dry-run", "field", "idempotency-key", "query", "raw", "secret-field"},
		"patch":  {"body", "body-file", "dry-run", "field", "idempotency-key", "query", "raw", "secret-field"},
		"delete": {"body", "body-file", "dry-run", "field", "idempotency-key", "query", "raw", "secret-field"},
	}
	for name, want := range tests {
		command, _, err := apiCommand.Find([]string{name})
		if err != nil || command == nil {
			t.Fatalf("Find(%s) = %v, %v", name, command, err)
		}
		var got []string
		command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
			got = append(got, flag.Name)
		})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s local flags = %v, want %v", name, got, want)
		}
		var rendered bytes.Buffer
		command.SetOut(&rendered)
		command.SetErr(&rendered)
		if err := command.Help(); err != nil {
			t.Fatalf("%s help error = %v", name, err)
		}
		help := rendered.String()
		for _, flag := range want {
			if !strings.Contains(help, "--"+flag) {
				t.Fatalf("%s rendered help missing --%s:\n%s", name, flag, help)
			}
		}
		for _, want := range []string{"--include-meta", "safe transport context", "under data"} {
			if !strings.Contains(help, want) {
				t.Fatalf("%s help missing %q:\n%s", name, want, help)
			}
		}
		for _, forbidden := range []string{"planned for a later release", "not implemented yet"} {
			if strings.Contains(help, forbidden) {
				t.Fatalf("%s rendered help contains %q:\n%s", name, forbidden, help)
			}
		}
	}
}

func TestSanitizeRawSuccessTransformsRequestSecrets(t *testing.T) {
	t.Parallel()

	const credential = "opaque-credential"
	raw := `{"data":{"opaque-credential":"opaque-credential","same":"opaque-credential","same":"kept","number":9007199254740993,"bool":true,"null":null},"meta":{"request_id":"req-safe"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()
	client, err := api.New(api.Options{
		BaseURL:      server.URL + "/v1",
		APIKey:       "test-key",
		SecretValues: []string{credential},
	})
	if err != nil {
		t.Fatalf("api.New() error = %v", err)
	}
	result, err := client.DoRaw(context.Background(), http.MethodGet, "echo", nil, nil, api.IdempotencyNone)
	if err != nil {
		t.Fatalf("DoRaw() error = %v", err)
	}
	got, err := sanitizeRawSuccess(result.Data, result.Meta)
	if err != nil {
		t.Fatalf("sanitizeRawSuccess() error = %v", err)
	}
	want := `{"[REDACTED]":"[REDACTED]","same":"[REDACTED]","same~2":"kept","number":9007199254740993,"bool":true,"null":null}`
	if string(got) != want {
		t.Fatalf("sanitizeRawSuccess() = %s, want %s", got, want)
	}

	got, err = sanitizeRawSuccess(json.RawMessage(`{`), api.ResponseMeta{HTTPStatus: 502, RequestID: "req-bad"})
	if err == nil || got != nil {
		t.Fatalf("sanitizeRawSuccess(invalid) = %q, %v; want nil, error", got, err)
	}
	var protocol *api.ProtocolError
	if !errors.As(err, &protocol) || protocol.Status != 502 || protocol.RequestID != "req-bad" {
		t.Fatalf("sanitize error = %#v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
