package testutil

import (
	"net/url"
	"strings"
	"testing"
)

func TestSecretValueMismatchDiagnosticsContainOnlyFingerprints(t *testing.T) {
	secret := "ak_diagnostic|quote\"slash\\control\x01雪<&tail"
	diagnostic := compareSecretValues("path segment", []string{secret}, []string{secret + "-different"})
	if diagnostic == "" {
		t.Fatal("compareSecretValues() returned no mismatch")
	}
	forms := secretValueForms(secret)
	doubleEncoded := url.QueryEscape(url.QueryEscape(secret))
	lowerDoubleEncoded := strings.ReplaceAll(doubleEncoded, "%257C", "%257c")
	for _, want := range []string{doubleEncoded, lowerDoubleEncoded} {
		found := false
		for _, form := range forms {
			if form == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("secretValueForms() missing double-encoded form")
		}
	}
	for _, form := range forms {
		if form != "" && strings.Contains(diagnostic, form) {
			t.Fatalf("diagnostic contains secret form; diagnostic=%q", diagnostic)
		}
	}
	for _, want := range []string{"path segment 0 mismatch", "length=", "sha256="} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostic %q missing %q", diagnostic, want)
		}
	}
}
