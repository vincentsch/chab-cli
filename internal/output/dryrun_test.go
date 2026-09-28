package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
)

func TestDryRunPreviewHumanPlainAndJSONRedaction(t *testing.T) {
	const rawSecret = "super-secret-value"
	const rawAuthorization = "Bearer ak_authz|authorization-secret"
	preview := output.DryRunPreview{
		Method: "POST",
		Path:   "/projects",
		Query: []output.DryRunValue{
			{Name: "team", Value: "demo"},
			{Name: "debug", Value: "ak_test|leak"},
		},
		Body: []output.DryRunValue{
			{Name: "name", Value: "Demo"},
			{Name: "token", Value: rawSecret, Secret: true},
			{Name: "Authorization", Value: rawAuthorization, Secret: true},
		},
		Idempotency: &output.DryRunIdempotency{Source: "flag"},
	}

	var human bytes.Buffer
	if err := output.WriteHuman(&human, preview.Render); err != nil {
		t.Fatalf("WriteHuman() error = %v", err)
	}
	wantHuman := "" +
		"Method: POST\n" +
		"Path: /projects\n" +
		"Query:\n" +
		"  team: demo\n" +
		"  debug: ak_test|[REDACTED]\n" +
		"Body:\n" +
		"  name: Demo\n" +
		"  token: [REDACTED]\n" +
		"  Authorization: [REDACTED]\n" +
		"Idempotency: flag\n"
	if human.String() != wantHuman {
		t.Fatalf("human = %q, want %q", human.String(), wantHuman)
	}
	assertNoOutputControl(t, human.String())
	assertDryRunNoSecret(t, human.String(), rawSecret, "leak", rawAuthorization, "authorization-secret")

	var plainData, plainProse bytes.Buffer
	if err := output.WritePlain(&plainData, &plainProse, preview.RenderPlain); err != nil {
		t.Fatalf("WritePlain() error = %v", err)
	}
	wantPlain := "" +
		"Method\tPOST\n" +
		"Path\t/projects\n" +
		"Query.team\tdemo\n" +
		"Query.debug\tak_test|[REDACTED]\n" +
		"Body.name\tDemo\n" +
		"Body.token\t[REDACTED]\n" +
		"Body.Authorization\t[REDACTED]\n" +
		"Idempotency\tflag\n"
	if plainData.String() != wantPlain || plainProse.String() != "" {
		t.Fatalf("plain data=%q prose=%q", plainData.String(), plainProse.String())
	}
	assertNoOutputControl(t, plainData.String()+plainProse.String())
	assertDryRunNoSecret(t, plainData.String()+plainProse.String(), rawSecret, "leak", rawAuthorization, "authorization-secret")

	var jsonBuf bytes.Buffer
	if err := output.WriteJSON(&jsonBuf, preview); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if !strings.HasSuffix(jsonBuf.String(), "\n") || !strings.Contains(jsonBuf.String(), "\n  \"method\"") {
		t.Fatalf("JSON byte style not stable: %q", jsonBuf.String())
	}
	assertNoOutputControl(t, jsonBuf.String())
	assertDryRunNoSecret(t, jsonBuf.String(), rawSecret, "leak", rawAuthorization, "authorization-secret")
	var doc struct {
		Query []output.DryRunValue `json:"query"`
		Body  []output.DryRunValue `json:"body"`
	}
	if err := json.Unmarshal(jsonBuf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON parse error = %v; raw=%s", err, jsonBuf.String())
	}
	if doc.Query[1].Value != "ak_test|[REDACTED]" {
		t.Fatalf("query redaction = %#v", doc.Query[1])
	}
	if doc.Body[1].Value != "[REDACTED]" || !doc.Body[1].Secret {
		t.Fatalf("body secret = %#v", doc.Body[1])
	}
	if doc.Body[2].Name != "Authorization" || doc.Body[2].Value != "[REDACTED]" || !doc.Body[2].Secret {
		t.Fatalf("authorization secret = %#v", doc.Body[2])
	}
}

func TestDryRunPreviewGeneratedIdempotencyAndOmitEmpty(t *testing.T) {
	preview := output.DryRunPreview{
		Method:      "DELETE",
		Path:        "/projects/proj_123",
		Idempotency: &output.DryRunIdempotency{Source: "generated"},
	}
	var human bytes.Buffer
	if err := output.WriteHuman(&human, preview.Render); err != nil {
		t.Fatalf("WriteHuman() error = %v", err)
	}
	if !strings.Contains(human.String(), "Idempotency key: generated on send\n") {
		t.Fatalf("human missing generated idempotency: %q", human.String())
	}

	var doc map[string]any
	stable, err := output.StableJSONBytes(preview)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	if err := json.Unmarshal(stable, &doc); err != nil {
		t.Fatalf("JSON parse error = %v; raw=%s", err, string(stable))
	}
	if _, ok := doc["query"]; ok {
		t.Fatalf("query key present in %#v", doc)
	}
	if _, ok := doc["body"]; ok {
		t.Fatalf("body key present in %#v", doc)
	}
	idempotency := doc["idempotency"].(map[string]any)
	if idempotency["source"] != "generated" {
		t.Fatalf("idempotency = %#v", idempotency)
	}
	if _, ok := idempotency["key"]; ok {
		t.Fatalf("generated idempotency key present: %#v", idempotency)
	}

	empty := output.DryRunPreview{Method: "GET", Path: "/projects"}
	stable, err = output.StableJSONBytes(empty)
	if err != nil {
		t.Fatalf("StableJSONBytes(empty) error = %v", err)
	}
	doc = map[string]any{}
	if err := json.Unmarshal(stable, &doc); err != nil {
		t.Fatalf("empty JSON parse error = %v; raw=%s", err, string(stable))
	}
	for _, key := range []string{"query", "body", "idempotency"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("%s key present in %#v", key, doc)
		}
	}
}

func TestDryRunPreviewExplicitIdempotencySourceOnly(t *testing.T) {
	preview := output.DryRunPreview{
		Method:      "PATCH",
		Path:        "/projects/p-123",
		Idempotency: &output.DryRunIdempotency{Source: "explicit"},
	}

	var human bytes.Buffer
	if err := output.WriteHuman(&human, preview.Render); err != nil {
		t.Fatalf("WriteHuman() error = %v", err)
	}
	if !strings.Contains(human.String(), "Idempotency key: explicit\n") {
		t.Fatalf("human missing explicit idempotency source: %q", human.String())
	}

	var doc map[string]any
	stable, err := output.StableJSONBytes(preview)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	if err := json.Unmarshal(stable, &doc); err != nil {
		t.Fatalf("JSON parse error = %v; raw=%s", err, string(stable))
	}
	idempotency := doc["idempotency"].(map[string]any)
	if idempotency["source"] != "explicit" {
		t.Fatalf("idempotency = %#v", idempotency)
	}
	if _, ok := idempotency["key"]; ok {
		t.Fatalf("explicit idempotency key present: %#v", idempotency)
	}

	flagPreview := output.DryRunPreview{
		Method:      "POST",
		Path:        "/projects",
		Idempotency: &output.DryRunIdempotency{Source: "flag"},
	}
	human.Reset()
	if err := output.WriteHuman(&human, flagPreview.Render); err != nil {
		t.Fatalf("WriteHuman(flag) error = %v", err)
	}
	if !strings.Contains(human.String(), "Idempotency: flag\n") {
		t.Fatalf("unrecognized idempotency source rendering changed: %q", human.String())
	}
}

func TestDryRunPreviewSuppressedIdempotencyAndRetry(t *testing.T) {
	preview := output.DryRunPreview{
		Method:      "POST",
		Path:        "/api-keys",
		Idempotency: &output.DryRunIdempotency{Source: "suppressed"},
		Retry:       &output.DryRunRetry{Automatic: false, Source: "disabled_non_replayable"},
	}

	var human bytes.Buffer
	if err := output.WriteHuman(&human, preview.Render); err != nil {
		t.Fatalf("WriteHuman() error = %v", err)
	}
	wantHuman := "" +
		"Method: POST\n" +
		"Path: /api-keys\n" +
		"Idempotency key: not sent (route is not replayable)\n" +
		"Automatic retry: disabled (route is not replayable)\n"
	if human.String() != wantHuman {
		t.Fatalf("human = %q, want %q", human.String(), wantHuman)
	}

	var plainData, plainProse bytes.Buffer
	if err := output.WritePlain(&plainData, &plainProse, preview.RenderPlain); err != nil {
		t.Fatalf("WritePlain() error = %v", err)
	}
	wantPlain := "" +
		"Method\tPOST\n" +
		"Path\t/api-keys\n" +
		"Idempotency key\tnot sent (route is not replayable)\n" +
		"Automatic retry\tdisabled (route is not replayable)\n"
	if plainData.String() != wantPlain || plainProse.String() != "" {
		t.Fatalf("plain data=%q prose=%q", plainData.String(), plainProse.String())
	}

	stable, err := output.StableJSONBytes(preview)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	var doc struct {
		Idempotency struct {
			Source string `json:"source"`
		} `json:"idempotency"`
		Retry struct {
			Automatic bool   `json:"automatic"`
			Source    string `json:"source"`
		} `json:"retry"`
	}
	if err := json.Unmarshal(stable, &doc); err != nil {
		t.Fatalf("JSON parse error = %v; raw=%s", err, string(stable))
	}
	if doc.Idempotency.Source != "suppressed" ||
		doc.Retry.Automatic ||
		doc.Retry.Source != "disabled_non_replayable" {
		t.Fatalf("preview JSON policy = %#v", doc)
	}
}

func assertDryRunNoSecret(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			t.Fatalf("secret value %q leaked in %q", needle, haystack)
		}
	}
}

func assertNoOutputControl(t *testing.T, value string) {
	t.Helper()
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b == '\n' || b == '\t' {
			continue
		}
		if b == 0x1b || b < 0x20 || b == 0x7f {
			t.Fatalf("output contains control byte 0x%02x in %q", b, value)
		}
	}
}
