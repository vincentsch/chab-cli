package docscheck

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/vincentsch/rungrad/scaffold"
)

// viltScaffoldOptions are the Chab-like product-profile inputs the scaffold
// proof drives. RungradReplace is intentionally empty so the assertions cover
// the real, shippable scaffold content. Service URLs must be https under the
// reserved .invalid TLD with no userinfo, so the api service uses api.chab.invalid.
func viltScaffoldOptions() scaffold.Options {
	return scaffold.Options{
		Name:              "chab",
		Module:            "github.com/vincentsch/chab-cli",
		ProductProfile:    true,
		EnvPrefix:         "CHAB",
		ProductName:       "Chab SaaS CLI",
		Description:       "Chab SaaS command-line interface",
		DocsLabel:         "Chab SaaS CLI",
		Services:          []string{"api=https://api.chab.invalid"},
		MetadataNamespace: "chab/saas-cli",
		Surface:           "host",
	}
}

// TestRungradScaffoldGeneratesViltLikeProduct proves rungrad's product scaffold
// is a usable comparison target for a Chab-derived branch: the Chab-like inputs
// flow into the generated main.go and the product profile emits exactly four
// files. Substring assertions on resolved values keep the proof stable as the
// templates evolve.
func TestRungradScaffoldGeneratesViltLikeProduct(t *testing.T) {
	files, err := scaffold.Generate(viltScaffoldOptions())
	if err != nil {
		t.Fatalf("scaffold.Generate: %v", err)
	}

	got := make([]string, 0, len(files))
	for name := range files {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"README.md", "go.mod", "main.go", "main_test.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scaffold file set = %v, want %v", got, want)
	}

	main := files["main.go"]
	// Probe concrete rendered values, not Options field names or broad product
	// labels, so the test proves each product-profile input reached the template.
	for _, probe := range []string{
		`EnvVar:  "CHAB_TOKEN"`,          // env prefix rendered into auth env var
		`ProfileEnvVar:  "CHAB_PROFILE"`, // env prefix rendered into profile env var
		"chab/saas-cli",                  // manifest metadata namespace
		"SurfaceHostOwned",               // host-owned global-flag surface
		"rungrad.Service{",               // services rendered as rungrad services
		"Flag:",                          // each service binds a public flag
		"AppConfig",                      // rungrad.New(rungrad.AppConfig{...})
		"chab.invalid",                   // the api service host
		"Extension",                      // manifest extension example
	} {
		if !strings.Contains(main, probe) {
			t.Errorf("generated main.go missing %q", probe)
		}
	}
}

// TestRungradScaffoldWriteProducesFourFiles covers the scaffold.Write entry
// point and pins that generation lands only under t.TempDir(), never in the repo.
func TestRungradScaffoldWriteProducesFourFiles(t *testing.T) {
	tmp := t.TempDir()
	root, err := scaffold.Write(tmp, viltScaffoldOptions())
	if err != nil {
		t.Fatalf("scaffold.Write: %v", err)
	}
	if got, want := filepath.Clean(filepath.Dir(root)), filepath.Clean(tmp); got != want {
		t.Fatalf("scaffold.Write root parent = %q, want temp dir %q", got, want)
	}
	if filepath.Base(root) != "chab" {
		t.Fatalf("scaffold.Write root = %q, want a chab/ directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	want := []string{"README.md", "go.mod", "main.go", "main_test.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scaffold.Write files = %v, want %v", got, want)
	}
}

// TestRungradScaffoldRejectsProductFieldWithoutProfile pins the product-profile
// contract: a product field set with ProductProfile:false is a ValidationError.
func TestRungradScaffoldRejectsProductFieldWithoutProfile(t *testing.T) {
	_, err := scaffold.Generate(scaffold.Options{
		Name:           "chab",
		EnvPrefix:      "CHAB",
		ProductProfile: false,
	})
	var verr *scaffold.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want *scaffold.ValidationError, got %T: %v", err, err)
	}
	if got, want := verr.ExitCode(), 1; got != want {
		t.Fatalf("ValidationError ExitCode() = %d, want %d", got, want)
	}
}
