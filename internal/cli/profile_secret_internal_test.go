package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingSecretRegistry struct {
	values []string
}

func (r *recordingSecretRegistry) RegisterSecret(value string) {
	r.values = append(r.values, value)
}

func (r *recordingSecretRegistry) RedactJSON(data []byte) []byte {
	return append([]byte(nil), data...)
}

func (r *recordingSecretRegistry) RedactText(data []byte) []byte {
	return append([]byte(nil), data...)
}

func TestProfileInspectionUsesOnlyCommandLocalStoredKeyRedaction(t *testing.T) {
	key := strings.Join([]string{"opaque", "stored", "credential", "value"}, "-")
	for _, field := range []string{"display_id", "key_name", "team_display_id", "team_name"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yml")
			authPath := filepath.Join(dir, "auth.json")
			record := map[string]any{
				"api_key":         key,
				"display_id":      "",
				"key_name":        "",
				"team_display_id": "",
				"team_name":       "",
			}
			record[field] = key
			data, err := json.Marshal(map[string]any{
				"version":  1,
				"profiles": map[string]any{"local": record},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(authPath, append(data, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}

			baseModes := []struct {
				name string
				args []string
			}{
				{name: "human", args: []string{"profile", "show", "local"}},
				{name: "plain", args: []string{"profile", "show", "local", "--plain"}},
				{name: "json", args: []string{"profile", "show", "local", "--json"}},
				{name: "jq", args: []string{"profile", "show", "local", "--jq", ".stored_auth"}},
				{name: "template", args: []string{"profile", "show", "local", "--template", "{{.stored_auth.present}}"}},
			}
			tests := append([]struct {
				name string
				args []string
			}{}, baseModes...)
			for _, mode := range baseModes {
				tests = append(tests, struct {
					name string
					args []string
				}{
					name: mode.name + " debug",
					args: append(append([]string(nil), mode.args...), "--debug"),
				})
			}
			tests = append(tests,
				struct {
					name string
					args []string
				}{name: "list debug", args: []string{"profile", "list", "--debug", "--json"}},
				struct {
					name string
					args []string
				}{name: "transform error", args: []string{"profile", "show", "local", "--jq", `.stored_auth.display_id | error(.)`}},
				struct {
					name string
					args []string
				}{name: "transform error debug", args: []string{"profile", "show", "local", "--jq", `.stored_auth.display_id | error(.)`, "--debug"}},
			)
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					registry := &recordingSecretRegistry{}
					var stdout, stderr bytes.Buffer
					args := []string{"--config", configPath, "--auth-file", authPath}
					args = append(args, test.args...)
					code, _ := RunWith(args, &stdout, &stderr, Options{
						LookupEnv: func(string) (string, bool) { return "", false },
						secrets:   registry,
					})
					if strings.HasPrefix(test.name, "transform error") {
						if code != ExitUsage || stdout.Len() != 0 {
							t.Fatalf("%s returned code=%d stdout-bytes=%d", test.name, code, stdout.Len())
						}
					} else if code != ExitSuccess {
						t.Fatalf("%s returned code=%d", test.name, code)
					}
					if strings.Contains(stdout.String(), key) || strings.Contains(stderr.String(), key) {
						t.Fatalf("%s exposed the stored credential through %s", test.name, field)
					}
					for _, registered := range registry.values {
						if registered == key {
							t.Fatalf("%s registered stored credential in invocation-wide registry", test.name)
						}
					}
				})
			}
		})
	}
}

func TestProfileDeleteDoesNotExposeStoredCredentialMetadata(t *testing.T) {
	key := strings.Join([]string{"opaque", "delete", "credential", "value"}, "-")
	for _, test := range []struct {
		name     string
		input    string
		wantCode int
	}{
		{name: "accept", input: "y\n", wantCode: ExitSuccess},
		{name: "deny", input: "n\n", wantCode: ExitUsage},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yml")
			authPath := filepath.Join(dir, "auth.json")
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"profiles": map[string]any{
					"local": map[string]any{
						"api_key":         key,
						"display_id":      key,
						"key_name":        key,
						"team_display_id": key,
						"team_name":       key,
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(authPath, append(data, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}

			registry := &recordingSecretRegistry{}
			var stdout, stderr bytes.Buffer
			code, _ := RunWith([]string{
				"--config", configPath,
				"--auth-file", authPath,
				"--debug",
				"profile", "delete", "local",
			}, &stdout, &stderr, Options{
				LookupEnv:       func(string) (string, bool) { return "", false },
				Stdin:           strings.NewReader(test.input),
				StdinIsTerminal: func() bool { return true },
				secrets:         registry,
			})
			if code != test.wantCode {
				t.Fatalf("delete returned code=%d, want %d", code, test.wantCode)
			}
			if strings.Contains(stdout.String(), key) || strings.Contains(stderr.String(), key) {
				t.Fatal("delete exposed stored credential or credential-derived metadata")
			}
			for _, registered := range registry.values {
				if registered == key {
					t.Fatal("delete registered stored key in invocation-wide registry")
				}
			}
		})
	}
}
