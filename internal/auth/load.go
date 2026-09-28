package auth

import (
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
)

// Load reads auth.json. A missing file is valid and returns an empty v1 file.
// Existing broad permissions are reported as findings, not load failures.
func Load(path string) (*File, []PermissionFinding, error) {
	if path == "" {
		return nil, nil, newError(ErrMalformedConfig, path, "", "path", "must not be empty", nil)
	}

	// Permission findings are collected before parsing so callers can warn
	// about broad auth-file modes even when the file later fails to decode.
	findings := permissionFindings(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newFile(false), nil, nil
		}
		return nil, findings, newError(ErrMalformedConfig, path, "", "", "could not read auth file", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, findings, newError(ErrMalformedConfig, path, "", "", "auth file is empty", nil)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, findings, newError(ErrMalformedConfig, path, "", "", "could not parse auth JSON", err)
	}
	if raw == nil {
		return nil, findings, newError(ErrMalformedConfig, path, "", "", "top-level JSON document must be an object", nil)
	}

	versionRaw, ok := raw["version"]
	if !ok {
		return nil, findings, newError(ErrMalformedConfig, path, "", "version", "is required", nil)
	}
	var version int
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		return nil, findings, newError(ErrMalformedConfig, path, "", "version", "must be numeric", err)
	}
	if version != fileVersion {
		return nil, findings, newError(ErrUnsupportedVersion, path, "", "version", "only version 1 is supported", nil)
	}

	file := newFile(true)
	file.Version = version
	file.raw = raw

	if profilesRaw, ok := raw["profiles"]; ok {
		var rawProfiles map[string]json.RawMessage
		if err := json.Unmarshal(profilesRaw, &rawProfiles); err != nil || rawProfiles == nil {
			return nil, findings, newError(ErrMalformedConfig, path, "", "profiles", "must be an object", err)
		}
		for name, profileRaw := range rawProfiles {
			var profileObject map[string]json.RawMessage
			if err := json.Unmarshal(profileRaw, &profileObject); err != nil || profileObject == nil {
				return nil, findings, newError(ErrMalformedConfig, path, name, "profiles."+name, "profile auth must be an object", err)
			}

			var record ProfileAuth
			if err := json.Unmarshal(profileRaw, &record); err != nil {
				return nil, findings, newError(ErrMalformedConfig, path, name, "profiles."+name, "profile auth has invalid known fields", err)
			}
			// Keep each raw profile object so Write can preserve keys this CLI
			// does not understand while still refreshing the known fields.
			file.Profiles[name] = record
			file.profileRaw[name] = profileObject
		}
	}

	return file, findings, nil
}

func newFile(exists bool) *File {
	return &File{
		Version:    fileVersion,
		Profiles:   map[string]ProfileAuth{},
		raw:        map[string]json.RawMessage{},
		profileRaw: map[string]map[string]json.RawMessage{},
		exists:     exists,
	}
}

// permissionFindings checks the auth file mode on platforms where POSIX
// permission bits are meaningful for this contract.
func permissionFindings(path string) []PermissionFinding {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	actual := info.Mode().Perm()
	expected := os.FileMode(0o600)
	if actual&0o077 == 0 {
		return nil
	}
	return []PermissionFinding{{
		Path:         RedactString(path),
		ActualMode:   actual,
		ExpectedMode: expected,
	}}
}
