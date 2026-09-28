package auth_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/config"
)

func TestLoadMissingAuthAndMissingCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	file, findings, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
	if len(file.Profiles) != 0 {
		t.Fatalf("Profiles = %#v, want empty", file.Profiles)
	}
	if file.Exists() {
		t.Fatalf("missing auth Exists() = true")
	}

	_, findings, err = auth.LookupCredential(config.Runtime{Profile: "local", AuthPath: path}, auth.Options{LookupEnv: fakeEnv(nil)})
	if len(findings) != 0 {
		t.Fatalf("credential findings = %#v, want none", findings)
	}
	requireAuthKind(t, err, auth.ErrMissingCredential, 3)
}

func TestAuthFileExistsAccessorAfterWriteAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	file := &auth.File{}
	if file.Exists() {
		t.Fatalf("zero file Exists() = true")
	}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_exists|"+"secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !file.Exists() {
		t.Fatalf("written auth Exists() = false")
	}
	loaded, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load(existing) error = %v", err)
	}
	if !loaded.Exists() {
		t.Fatalf("loaded auth Exists() = false")
	}
}

func TestStoredAuthOnlyProfileDoesNotInheritProductionDestination(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	writeAuth(t, authPath, `{"version": 1, "profiles": {"local": {"api_key": "ak_legacy|secret"}}}`)
	rt, err := config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			ConfigPath: config.OverrideString{Value: filepath.Join(dir, "missing-config.yml"), Set: true},
			AuthPath:   config.OverrideString{Value: authPath, Set: true},
		},
		LookupEnv: fakeEnv(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, _, err := auth.LookupCredential(rt, auth.Options{LookupEnv: fakeEnv(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if credential.Source != auth.SourceAuthFile || rt.BaseURL != config.LegacyImplicitBaseURL || rt.APIBaseURL != "http://localhost/v1" {
		t.Fatalf("stored credential destination changed: runtime=%#v source=%q", rt, credential.Source)
	}
}

func TestLookupCredentialEnvWinsAndDoesNotOpenAuthFile(t *testing.T) {
	// These auth-file states would fail or warn if the file were opened. A
	// non-empty env credential must bypass that path completely.
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{name: "missing"},
		{name: "malformed", setup: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
		}},
		{name: "broad permissions", setup: func(t *testing.T, path string) {
			t.Helper()
			writeAuth(t, path, `{"version": 1, "profiles": {"local": {"api_key": "ak_file|file-secret"}}}`)
			if runtime.GOOS != "windows" {
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatalf("Chmod() error = %v", err)
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			if test.setup != nil {
				test.setup(t, path)
			}

			credential, findings, err := auth.LookupCredential(
				config.Runtime{Profile: "local", AuthPath: path},
				auth.Options{LookupEnv: fakeEnv(map[string]string{"CHAB_API_KEY": "ak_env|env-secret"})},
			)
			if err != nil {
				t.Fatalf("LookupCredential() error = %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("findings = %#v, want none for env credential", findings)
			}
			if credential.Source != auth.SourceEnv || credential.APIKey != "ak_env|env-secret" {
				t.Fatalf("credential = %#v", credential)
			}
			if strings.Contains(fmt.Sprint(credential), "env-secret") {
				t.Fatalf("Credential.String leaked API key: %s", credential)
			}
		})
	}
}

func TestLookupCredentialStoredProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	file := &auth.File{}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_local|stored-secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	credential, findings, err := auth.LookupCredential(
		config.Runtime{Profile: "local", AuthPath: path},
		auth.Options{LookupEnv: fakeEnv(map[string]string{"CHAB_API_KEY": ""})},
	)
	if err != nil {
		t.Fatalf("LookupCredential() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
	if credential.Source != auth.SourceAuthFile || credential.APIKey != "ak_local|stored-secret" || credential.DisplayID != "ak_display" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestAuthSchemaRoundTripAndUnknownPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	keyLastUsed := time.Date(2026, 5, 18, 9, 30, 0, 0, time.UTC)
	keyCreated := time.Date(2026, 5, 1, 8, 15, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	lastValidated := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	// The fixture mixes three cases that matter on rewrite: unknown
	// profile-level metadata that must survive, known fields that must be
	// refreshed from the typed record, and nested unknown keys inside known
	// fields that should be dropped with the old known-field value.
	writeAuth(t, path, `{
  "version": 1,
  "x_top": {"kept": true},
  "profiles": {
    "local": {
      "api_key": "ak_old|old-secret",
      "display_id": "ak_old_display",
      "key_name": "Old",
      "team_display_id": "team_old",
      "team_name": "Old Team",
      "plan": {"name": "basic", "label": "Basic", "status": "active", "api_access": true},
      "capabilities": [{"id": "old", "label": "Old", "x_capability": true}],
      "project_scope": {
        "mode": "selected_projects",
        "selected_count": 1,
        "selected_projects": {
          "selected_count": 1,
          "has_more": false,
          "data": [{"id": "old-project", "name": "Old Project", "status": "archived"}]
        },
        "x_scope": true
      },
      "expires_at": null,
      "expired": false,
      "revoked": false,
      "last_validated_at": "2026-05-20T10:00:00Z",
      "x_profile": {"kept": true},
      "team": {"id": 123, "display_id": "team_unknown"},
      "whoami": {"team": {"id": 456, "display_id": "team_nested"}, "kept": true}
    }
  }
}`)

	file, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	record := sampleRecord("ak_new|new-secret")
	record.KeyLastUsedAt = &keyLastUsed
	record.KeyCreatedAt = &keyCreated
	record.ExpiresAt = &expiresAt
	record.LastValidatedAt = &lastValidated
	if err := auth.PutProfile(file, "local", record); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load(after Write) error = %v", err)
	}
	got := loaded.Profiles["local"]
	if got.APIKey != "ak_new|new-secret" || got.Plan == nil || got.Plan.Name != "pro" {
		t.Fatalf("round-tripped record = %#v", got)
	}
	if got.KeyLastUsedAt == nil || !got.KeyLastUsedAt.Equal(keyLastUsed) {
		t.Fatalf("KeyLastUsedAt = %v", got.KeyLastUsedAt)
	}
	if got.KeyCreatedAt == nil || !got.KeyCreatedAt.Equal(keyCreated) {
		t.Fatalf("KeyCreatedAt = %v", got.KeyCreatedAt)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v", got.ExpiresAt)
	}
	if got.LastValidatedAt == nil || !got.LastValidatedAt.Equal(lastValidated) {
		t.Fatalf("LastValidatedAt = %v", got.LastValidatedAt)
	}
	if len(got.Capabilities) != 1 || got.ProjectScope == nil || got.ProjectScope.SelectedCount != 5 {
		t.Fatalf("metadata did not round-trip: %#v", got)
	}

	// Inspect the raw JSON too. The typed load path proves known values are
	// usable; the raw view proves exactly which unknown keys survived the write.
	var raw map[string]any
	if err := json.Unmarshal(readFile(t, path), &raw); err != nil {
		t.Fatalf("Unmarshal written JSON error = %v", err)
	}
	if _, ok := raw["x_top"]; !ok {
		t.Fatalf("top-level unknown key was not preserved: %#v", raw)
	}
	profiles := raw["profiles"].(map[string]any)
	local := profiles["local"].(map[string]any)
	if _, ok := local["x_profile"]; !ok {
		t.Fatalf("profile unknown key was not preserved: %#v", local)
	}
	for _, legacy := range []string{"expired", "revoked"} {
		if local[legacy] != false {
			t.Fatalf("profile-level unknown %s key was not preserved: %#v", legacy, local)
		}
	}
	for _, added := range []string{"key_preset", "key_expiration_state", "key_last_used_at", "key_created_at"} {
		if _, ok := local[added]; !ok {
			t.Fatalf("new %s key missing from rewritten auth: %#v", added, local)
		}
	}
	team := local["team"].(map[string]any)
	if team["id"] != float64(123) {
		t.Fatalf("numeric team.id unknown key was not preserved: %#v", team)
	}
	if team["display_id"] != "team_unknown" {
		t.Fatalf("unknown team display_id was not preserved: %#v", team)
	}
	whoami := local["whoami"].(map[string]any)
	nestedTeam := whoami["team"].(map[string]any)
	if nestedTeam["id"] != float64(456) {
		t.Fatalf("numeric whoami.team.id unknown key was not preserved: %#v", nestedTeam)
	}
	if nestedTeam["display_id"] != "team_nested" || whoami["kept"] != true {
		t.Fatalf("unknown whoami metadata was not preserved: %#v", whoami)
	}
	plan := local["plan"].(map[string]any)
	if plan["name"] != "pro" || plan["label"] != "Pro" || plan["api_access"] != true {
		t.Fatalf("plan known shape was not rewritten: %#v", plan)
	}
	if _, ok := plan["status"]; ok {
		t.Fatalf("plan nested unknown key survived rewrite: %#v", plan)
	}
	capability := local["capabilities"].([]any)[0].(map[string]any)
	if capability["id"] != "projects" || capability["read"] != true || capability["write"] != true {
		t.Fatalf("capability known shape was not rewritten: %#v", capability)
	}
	if _, ok := capability["x_capability"]; ok {
		t.Fatalf("capability row unknown key survived rewrite: %#v", capability)
	}
	scope := local["project_scope"].(map[string]any)
	if scope["mode"] != "selected_projects" || scope["selected_count"] != float64(5) {
		t.Fatalf("project scope known shape was not rewritten: %#v", scope)
	}
	if _, ok := scope["x_scope"]; ok {
		t.Fatalf("project scope unknown key survived rewrite: %#v", scope)
	}
	selected := scope["selected_projects"].(map[string]any)
	if _, ok := selected["selected_count"]; ok {
		t.Fatalf("selected_projects nested unknown key survived rewrite: %#v", selected)
	}
	project := selected["data"].([]any)[0].(map[string]any)
	if project["id"] != "01HYPROJECT" || project["name"] != "Demo" {
		t.Fatalf("selected project known shape was not rewritten: %#v", project)
	}
	if _, ok := project["status"]; ok {
		t.Fatalf("selected project unknown key survived rewrite: %#v", project)
	}
	if data := readFile(t, path); len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("written auth JSON is missing trailing newline: %q", data)
	}
}

func TestAuthKnownWritesDoNotEmitTeamObjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	file := &auth.File{}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_known|secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(readFile(t, path), &raw); err != nil {
		t.Fatalf("Unmarshal written JSON error = %v", err)
	}
	profile := raw["profiles"].(map[string]any)["local"].(map[string]any)
	for _, key := range []string{"team", "whoami"} {
		if _, ok := profile[key]; ok {
			t.Fatalf("known write emitted %s object: %#v", key, profile)
		}
	}
}

func TestAuthKnownNestedFieldsRewriteOlderCachedShapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	writeAuth(t, path, `{
  "version": 1,
  "profiles": {
    "local": {
      "api_key": "ak_old|old-secret",
      "capabilities": [
        {
          "group": "projects",
          "items": [
            {"id": "projects.read", "label": "Read projects", "read": true}
          ]
        }
      ],
      "project_scope": {
        "mode": "selected",
        "projects": [
          {"id": "01OLD", "name": "Old Project"}
        ]
      }
    }
  }
}`)

	file, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_new|new-secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(readFile(t, path), &raw); err != nil {
		t.Fatalf("Unmarshal written JSON error = %v", err)
	}
	profile := raw["profiles"].(map[string]any)["local"].(map[string]any)
	capability := profile["capabilities"].([]any)[0].(map[string]any)
	if _, ok := capability["group"]; ok {
		t.Fatalf("older grouped capability shape survived rewrite: %#v", capability)
	}
	if _, ok := capability["items"]; ok {
		t.Fatalf("older grouped capability items survived rewrite: %#v", capability)
	}
	if capability["id"] != "projects" || capability["read"] != true || capability["write"] != true {
		t.Fatalf("capability was not rewritten to known shape: %#v", capability)
	}

	scope := profile["project_scope"].(map[string]any)
	if scope["mode"] != "selected_projects" || scope["selected_count"] != float64(5) {
		t.Fatalf("project scope was not rewritten to known shape: %#v", scope)
	}
	if _, ok := scope["projects"]; ok {
		t.Fatalf("older selected project list survived rewrite: %#v", scope)
	}
	selected := scope["selected_projects"].(map[string]any)
	project := selected["data"].([]any)[0].(map[string]any)
	if project["id"] != "01HYPROJECT" || project["name"] != "Demo" {
		t.Fatalf("selected project was not rewritten to known shape: %#v", project)
	}
}

func TestAuthWritePermissionsAndUnsafeFinding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertions are not portable to Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "chab", "ak_path|perm-secret", "auth.json")
	file := &auth.File{}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_perm|secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	_, findings, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load() with broad permissions error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one", findings)
	}
	if findings[0].ActualMode != 0o644 || findings[0].ExpectedMode != 0o600 {
		t.Fatalf("finding modes = %#v", findings[0])
	}
	if strings.Contains(fmt.Sprintf("%#v", findings[0]), "secret") || strings.Contains(findings[0].Path, "perm-secret") {
		t.Fatalf("permission finding leaked secret: %#v", findings[0])
	}
	if !strings.Contains(findings[0].Path, "[REDACTED]") {
		t.Fatalf("permission finding path was not redacted: %#v", findings[0])
	}
	credential, lookupFindings, err := auth.LookupCredential(
		config.Runtime{Profile: "local", AuthPath: path},
		auth.Options{LookupEnv: fakeEnv(nil)},
	)
	if err != nil {
		t.Fatalf("LookupCredential() with broad permissions error = %v", err)
	}
	if credential.Source != auth.SourceAuthFile || credential.APIKey == "" {
		t.Fatalf("credential = %#v", credential)
	}
	if len(lookupFindings) != 1 {
		t.Fatalf("lookup findings = %#v, want one", lookupFindings)
	}

	loaded, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("Load() before rewrite error = %v", err)
	}
	if err := auth.PutProfile(loaded, "local", sampleRecord("ak_newperm|new-secret")); err != nil {
		t.Fatalf("PutProfile(rewrite) error = %v", err)
	}
	if err := auth.Write(path, loaded); err != nil {
		t.Fatalf("Write(rewrite) error = %v", err)
	}
	assertMode(t, path, 0o600)
	if strings.Contains(string(readFile(t, path)), "ak_perm|secret") {
		t.Fatalf("old secret still present after atomic replacement")
	}
}

func TestAuthWriteValidationBeforeCreatingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "auth.json")
	err := auth.Write(path, &auth.File{
		Version:  1,
		Profiles: map[string]auth.ProfileAuth{"Bad": sampleRecord("ak_bad|secret")},
	})
	requireAuthKind(t, err, auth.ErrInvalidProfileName, 1)
	if _, statErr := os.Stat(filepath.Dir(path)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Write created directory before validation failed: %v", statErr)
	}

	emptyKeyPath := filepath.Join(dir, "missing-empty", "ak_path|path-secret", "auth.json")
	err = auth.Write(emptyKeyPath, &auth.File{
		Version:  1,
		Profiles: map[string]auth.ProfileAuth{"local": sampleRecord("")},
	})
	requireAuthKind(t, err, auth.ErrMalformedConfig, 1)
	if _, statErr := os.Stat(filepath.Dir(emptyKeyPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Write created directory before empty-key validation failed: %v", statErr)
	}
	var authErr *auth.Error
	if !errors.As(err, &authErr) {
		t.Fatalf("error type = %T, want *auth.Error", err)
	}
	if strings.Contains(authErr.Path, "path-secret") || strings.Contains(err.Error(), "path-secret") {
		t.Fatalf("auth error context leaked secret-shaped path: field=%q error=%v", authErr.Path, err)
	}
}

func TestAuthWriteFailureCleansTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	file := &auth.File{}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_cleanup|secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	err := auth.Write(path, file)
	requireAuthKind(t, err, auth.ErrMalformedConfig, 1)
	matches, globErr := filepath.Glob(filepath.Join(dir, ".auth.json.tmp-*"))
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("Write left temp files after failed replacement: %#v", matches)
	}
}

func TestAuthMalformedAndUnsupportedVersionErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		kind auth.ErrorKind
	}{
		{name: "malformed json", body: "{", kind: auth.ErrMalformedConfig},
		{name: "empty file", body: " \n", kind: auth.ErrMalformedConfig},
		{name: "non object", body: `[]`, kind: auth.ErrMalformedConfig},
		{name: "missing version", body: `{"profiles": {}}`, kind: auth.ErrMalformedConfig},
		{name: "unsupported version", body: `{"version": 2, "profiles": {}}`, kind: auth.ErrUnsupportedVersion},
		{name: "non numeric version", body: `{"version": "1", "profiles": {}}`, kind: auth.ErrMalformedConfig},
		{name: "non object profiles", body: `{"version": 1, "profiles": []}`, kind: auth.ErrMalformedConfig},
		{name: "non object profile auth", body: `{"version": 1, "profiles": {"local": null}}`, kind: auth.ErrMalformedConfig},
		{name: "malformed known field", body: `{"version": 1, "profiles": {"local": {"api_key": "ak_local|secret", "plan": "pro"}}}`, kind: auth.ErrMalformedConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			writeAuth(t, path, test.body)
			_, _, err := auth.Load(path)
			requireAuthKind(t, err, test.kind, 1)
		})
	}
}

func TestAuthErrorsRedactSecretShapedProfileAndField(t *testing.T) {
	err := auth.PutProfile(&auth.File{}, "ak_bad|super-secret", sampleRecord("ak_record|record-secret"))
	requireAuthKind(t, err, auth.ErrInvalidProfileName, 1)
	assertRedactedError(t, err, "super-secret")

	path := filepath.Join(t.TempDir(), "auth.json")
	writeAuth(t, path, `{
  "version": 1,
  "profiles": {
    "ak_bad|file-secret": null
  }
}`)
	_, _, err = auth.Load(path)
	requireAuthKind(t, err, auth.ErrMalformedConfig, 1)
	assertRedactedError(t, err, "file-secret")
}

func TestCredentialFormattingRedactsSecretShapedMetadata(t *testing.T) {
	credential := auth.Credential{
		APIKey:    "ak_key|api-secret",
		Source:    auth.SourceAuthFile,
		Profile:   "ak_profile|profile-secret",
		DisplayID: "ak_display",
		KeyName:   "ak_name|name-secret",
	}
	rendered := fmt.Sprintf("%s %#v", credential, credential)
	for _, leaked := range []string{"api-secret", "profile-secret", "name-secret"} {
		if strings.Contains(rendered, leaked) {
			t.Fatalf("credential formatting leaked %q in %q", leaked, rendered)
		}
	}
	if !strings.Contains(rendered, "ak_profile|[REDACTED]") || !strings.Contains(rendered, "ak_name|[REDACTED]") {
		t.Fatalf("credential formatting did not redact secret-shaped metadata: %q", rendered)
	}
}

func TestRedactionHelpers(t *testing.T) {
	apiKey := testAPIKey("ak_123", "super-secret")
	jsonKey := testAPIKey("ak_json", "json-secret")
	if got := auth.MaskAPIKey(apiKey); got != "ak_123|[REDACTED]" {
		t.Fatalf("MaskAPIKey() = %q", got)
	}
	redacted := auth.RedactString("Authorization: Bearer " + apiKey + "\n" +
		`{"api_key":"` + jsonKey + `","display_id":"ak_display"}`)
	for _, leaked := range []string{"super-secret", "json-secret"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("RedactString leaked %q in %q", leaked, redacted)
		}
	}
	if !strings.Contains(redacted, "ak_display") {
		t.Fatalf("RedactString removed display id: %q", redacted)
	}
	if got := auth.RedactString("profiles.ak_field|field-secret.api_key"); strings.Contains(got, "field-secret") {
		t.Fatalf("RedactString leaked dynamic field secret: %q", got)
	}
	if got := auth.RedactString("wrapped error: " + apiKey); strings.Contains(got, "super-secret") {
		t.Fatalf("RedactString leaked wrapped error secret: %q", got)
	}
	if got := auth.RedactHeader("authorization", "Bearer "+apiKey); got != "Bearer [REDACTED]" {
		t.Fatalf("RedactHeader() = %q", got)
	}
	if got := string(auth.RedactBytes([]byte(apiKey))); strings.Contains(got, "super-secret") {
		t.Fatalf("RedactBytes leaked secret: %q", got)
	}
}

func testAPIKey(id, secret string) string {
	return id + "|" + secret
}

func assertRedactedError(t *testing.T, err error, secret string) {
	t.Helper()
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret-shaped value: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error did not include redaction token: %v", err)
	}
}

func sampleRecord(key string) auth.ProfileAuth {
	lastValidated := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	return auth.ProfileAuth{
		APIKey:             key,
		DisplayID:          "ak_display",
		KeyName:            "Local CLI",
		KeyPreset:          "full_access",
		KeyExpirationState: "no_expiration",
		TeamDisplayID:      "team_display",
		TeamName:           "Acme Inc.",
		Plan:               &auth.Plan{Name: "pro", Label: "Pro", APIAccess: true},
		Capabilities: []auth.Capability{{
			ID:            "projects",
			Label:         "Projects",
			Description:   "...",
			Read:          true,
			Write:         true,
			ScopeType:     "project_backed",
			PlanAvailable: true,
		}},
		ProjectScope: &auth.ProjectScope{
			Mode:          "selected_projects",
			SelectedCount: 5,
			SelectedProjects: &auth.SelectedProjects{
				Data:    []auth.ProjectSummary{{ID: "01HYPROJECT", Name: "Demo"}},
				HasMore: true,
			},
			AppliesToTeamLevelGroups: false,
		},
		ExpiresAt:       nil,
		LastValidatedAt: &lastValidated,
	}
}

func writeAuth(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s = %04o, want %04o", path, got, want)
	}
}

func fakeEnv(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func requireAuthKind(t *testing.T, err error, kind auth.ErrorKind, exitCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want kind %s", kind)
	}
	var authErr *auth.Error
	if !errors.As(err, &authErr) {
		t.Fatalf("error type = %T, want *auth.Error: %v", err, err)
	}
	if authErr.Kind != kind {
		t.Fatalf("error kind = %s, want %s: %v", authErr.Kind, kind, err)
	}
	coder, ok := err.(interface{ ExitCode() int })
	if !ok {
		t.Fatalf("error does not implement ExitCode(): %T", err)
	}
	if got := coder.ExitCode(); got != exitCode {
		t.Fatalf("ExitCode() = %d, want %d", got, exitCode)
	}
}
