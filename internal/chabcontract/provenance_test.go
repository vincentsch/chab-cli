package chabcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPinnedContractFixtureDigestsMatchProvenance(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "provenance.json"))
	if err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	var provenance struct {
		Fixtures []struct {
			File   string `json:"file"`
			SHA256 string `json:"sha256"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(raw, &provenance); err != nil {
		t.Fatalf("decode provenance: %v", err)
	}
	if len(provenance.Fixtures) == 0 {
		t.Fatal("provenance fixtures are empty")
	}
	for _, fixture := range provenance.Fixtures {
		t.Run(fixture.File, func(t *testing.T) {
			if fixture.File == "" || fixture.SHA256 == "" {
				t.Fatalf("incomplete provenance fixture: %#v", fixture)
			}
			bytes, err := os.ReadFile(filepath.Join("testdata", fixture.File))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			sum := sha256.Sum256(bytes)
			got := hex.EncodeToString(sum[:])
			if got != fixture.SHA256 {
				t.Fatalf("sha256 = %s, want %s", got, fixture.SHA256)
			}
		})
	}
}

func TestRegistryLoadsPinnedContractMetadata(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(registry.Operations()), 98; got != want {
		t.Fatalf("operation count = %d, want %d", got, want)
	}
	if got, want := registry.PathCount(), 84; got != want {
		t.Fatalf("path count = %d, want %d", got, want)
	}
	for _, key := range []string{
		"operations.estimate",
		"search.web",
		"operations.result",
		"operations.artifact",
		"llm.generate",
		"research.deep",
	} {
		op, ok := registry.Find(key)
		if !ok {
			t.Fatalf("operation %q missing", key)
		}
		if op.Availability == "" || op.Behavior == "" || op.OutputKind == "" {
			t.Fatalf("operation %q has incomplete metadata: %#v", key, op)
		}
	}

	search, _ := registry.Find("search.web")
	if search.Method != "POST" ||
		search.Path != "/v1/search/web" ||
		search.Availability != "preview" ||
		search.Behavior != "async" ||
		search.OutputKind != "inline" ||
		search.RequiredScope != "api:search:read" ||
		!search.IdempotencyRequired ||
		search.DryRunSupported ||
		!search.RequiresPaidPlan ||
		search.NotBillable ||
		search.ResultSchemaRef != "#/components/schemas/SearchWebResultData" ||
		len(search.RequestSchema) == 0 ||
		len(search.ResultSchema) == 0 {
		t.Fatalf("search.web metadata drifted: %#v", search)
	}
}

func TestRegistryOperationSetMatchesFixture(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := make([]string, 0, len(registry.Operations()))
	for _, op := range registry.Operations() {
		got = append(got, op.ID)
	}
	sort.Strings(got)

	raw, err := os.ReadFile(filepath.Join("testdata", "chab-v1.yaml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var doc openAPIDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	want := []string{}
	for _, methods := range doc.Paths {
		for method, op := range methods {
			if isHTTPMethod(method) && op.OperationID != "" {
				want = append(want, op.OperationID)
			}
		}
	}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("operation set length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("operation set[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
