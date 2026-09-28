package redact_test

import (
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/redact"
)

func TestExactRegistryRedactsRawEncodedAndJSONForms(t *testing.T) {
	secret := `ordinary/key?quote"slash\<value>`
	registry := redact.NewRegistry()
	registry.RegisterSecret(secret)

	encodedJSON, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	forms := []string{
		secret,
		url.QueryEscape(secret),
		url.PathEscape(secret),
		url.QueryEscape(url.QueryEscape(secret)),
		string(encodedJSON),
		string(encodedJSON[1 : len(encodedJSON)-1]),
	}
	for _, form := range forms {
		redacted := string(registry.RedactText([]byte("before " + form + " after")))
		if strings.Contains(redacted, form) || !strings.Contains(redacted, "[REDACTED]") {
			t.Fatal("form survived exact redaction")
		}
	}

	document := []byte(`{"value":` + string(encodedJSON) + `}`)
	redactedJSON := registry.RedactJSON(document)
	var decoded map[string]string
	if err := json.Unmarshal(redactedJSON, &decoded); err != nil {
		t.Fatal("redacted JSON is invalid")
	}
	if decoded["value"] != "[REDACTED]" {
		t.Fatalf("redacted JSON value = %q", decoded["value"])
	}

	numeric := redact.NewRegistry()
	numeric.RegisterSecret("101")
	redactedNumber := numeric.RedactJSON([]byte(`{"limit":101,"name":"101"}`))
	var numericDocument map[string]any
	if err := json.Unmarshal(redactedNumber, &numericDocument); err != nil {
		t.Fatal("numeric overlap produced invalid JSON")
	}
	if numericDocument["limit"] != float64(101) || numericDocument["name"] != "[REDACTED]" {
		t.Fatalf("numeric overlap = %#v", numericDocument)
	}
}

func TestExactRegistryPreservesJSONStructureAndStreams(t *testing.T) {
	registry := redact.NewRegistry()
	registry.RegisterSecret("name")

	document := registry.RedactJSON([]byte(`{"name":"name","nested":{"name":"prefix-name"},"limit":101}`))
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal("structural overlap produced invalid JSON:", err)
	}
	if decoded["name"] != "[REDACTED]" || decoded["limit"] != float64(101) {
		t.Fatalf("document = %#v", decoded)
	}
	nested, ok := decoded["nested"].(map[string]any)
	if !ok || nested["name"] != "prefix-[REDACTED]" {
		t.Fatalf("nested = %#v", decoded["nested"])
	}

	punctuation := redact.NewRegistry()
	punctuation.RegisterSecret(":")
	stream := punctuation.RedactJSON([]byte("{\"method\":\"POST\"}\n\"POST\"\n101\n"))
	decoder := json.NewDecoder(strings.NewReader(string(stream)))
	var values []any
	for {
		var value any
		err := decoder.Decode(&value)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("punctuation overlap produced invalid JSON stream:", err)
		}
		values = append(values, value)
	}
	if len(values) != 3 {
		t.Fatalf("decoded stream = %#v", values)
	}
	object, ok := values[0].(map[string]any)
	if !ok || object["method"] != "POST" || values[1] != "POST" || values[2] != float64(101) {
		t.Fatalf("decoded stream = %#v", values)
	}

	quote := redact.NewRegistry()
	quote.RegisterSecret(`"`)
	if got := quote.RedactJSON([]byte(`"POST"`)); !json.Valid(got) || string(got) != `"POST"` {
		t.Fatalf("quote overlap = %q", got)
	}
}

func TestExactRegistryRemovesStaticRemainderOfSecretShapedValue(t *testing.T) {
	registry := redact.NewRegistry()
	registry.RegisterSecret("ak_exact|ordinary-secret")
	redacted := string(registry.RedactText([]byte("ak_exact|ordinary-secret")))
	if redacted != "[REDACTED]" {
		t.Fatalf("redacted value = %q", redacted)
	}
}
