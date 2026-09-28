package auth

import (
	"encoding/json"
	"regexp"
	"sort"

	"github.com/vincentsch/chab-cli/internal/localfile"
)

var profileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// PutProfile validates and stores one profile auth record. The API key is
// assumed to have been validated by the caller before this write path.
func PutProfile(file *File, profile string, record ProfileAuth) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", profile, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	if err := validateProfileName(profile); err != nil {
		return err
	}
	if err := validateRecord(profile, record); err != nil {
		return err
	}
	file.Profiles[profile] = record
	if file.profileRaw[profile] == nil {
		file.profileRaw[profile] = map[string]json.RawMessage{}
	}
	if file.Version == 0 {
		file.Version = fileVersion
	}
	return nil
}

// DeleteProfile removes a profile from both the known-field view and the raw
// preservation map so a later Write drops the record completely.
func DeleteProfile(file *File, profile string) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", profile, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	if err := validateProfileName(profile); err != nil {
		return err
	}
	delete(file.Profiles, profile)
	delete(file.profileRaw, profile)
	if file.Version == 0 {
		file.Version = fileVersion
	}
	return nil
}

// Write validates and atomically replaces auth.json. Unknown JSON object keys
// from a loaded file are preserved.
func Write(path string, file *File) error {
	if path == "" {
		return newError(ErrMalformedConfig, path, "", "path", "must not be empty", nil)
	}
	if err := validateFileForWrite(file); err != nil {
		return withAuthContext(err, path, "")
	}

	output := copyRawMap(file.raw)
	output["version"] = mustRaw(fileVersion)
	output["profiles"] = buildProfilesRaw(file)

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return newError(ErrMalformedConfig, path, "", "", "could not marshal auth JSON", err)
	}
	data = append(data, '\n')

	if err := localfile.AtomicWrite(path, data, 0o700, 0o600); err != nil {
		return newError(ErrMalformedConfig, path, "", "", "could not write auth file", err)
	}
	file.exists = true
	return nil
}

func validateFileForWrite(file *File) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", "", "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	if file.Version == 0 {
		file.Version = fileVersion
	}
	if file.Version != fileVersion {
		return newError(ErrUnsupportedVersion, "", "", "version", "only version 1 is supported", nil)
	}
	names := make([]string, 0, len(file.Profiles))
	for name := range file.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// Writes validate the complete known-field view so any auth file this
		// package emits has valid profile names and usable credentials.
		if err := validateProfileName(name); err != nil {
			return err
		}
		if err := validateRecord(name, file.Profiles[name]); err != nil {
			return err
		}
	}
	return nil
}

func validateRecord(profile string, record ProfileAuth) error {
	if record.APIKey == "" {
		// An empty key in a record being written is malformed local state.
		// Missing-credential errors are reserved for lookup paths that cannot
		// find a usable selected credential.
		return newError(ErrMalformedConfig, "", profile, "api_key", "must not be empty", nil)
	}
	return nil
}

func validateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return newError(ErrInvalidProfileName, "", name, "profile", "must match ^[a-z0-9][a-z0-9._-]{0,62}$", nil)
	}
	return nil
}

func ensureFileMaps(file *File) {
	if file.Profiles == nil {
		file.Profiles = map[string]ProfileAuth{}
	}
	if file.raw == nil {
		file.raw = map[string]json.RawMessage{}
	}
	if file.profileRaw == nil {
		file.profileRaw = map[string]map[string]json.RawMessage{}
	}
}

// buildProfilesRaw merges preserved unknown profile keys with refreshed known
// fields. Stable ordering keeps the marshaled output predictable for tests and
// manual diffs.
func buildProfilesRaw(file *File) json.RawMessage {
	profiles := map[string]json.RawMessage{}
	names := make([]string, 0, len(file.Profiles))
	for name := range file.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := copyRawMap(file.profileRaw[name])
		setKnownProfileFields(raw, file.Profiles[name])
		profiles[name] = mustRaw(raw)
	}
	return mustRaw(profiles)
}

// setKnownProfileFields rewrites every field owned by this CLI. Nil pointer
// fields intentionally marshal as JSON null to match the documented auth shape.
func setKnownProfileFields(raw map[string]json.RawMessage, record ProfileAuth) {
	raw["api_key"] = mustRaw(record.APIKey)
	raw["principal_type"] = mustRaw(record.PrincipalType)
	raw["principal_id"] = mustRaw(record.PrincipalID)
	raw["team_id"] = mustRaw(record.TeamID)
	raw["token_id"] = mustRaw(record.TokenID)
	raw["token_public_id"] = mustRaw(record.TokenPublicID)
	if record.Scopes == nil {
		raw["scopes"] = mustRaw([]string{})
	} else {
		raw["scopes"] = mustRaw(record.Scopes)
	}
	raw["token_controls"] = mustRaw(record.TokenControls)
	raw["display_id"] = mustRaw(record.DisplayID)
	raw["key_name"] = mustRaw(record.KeyName)
	raw["key_preset"] = mustRaw(record.KeyPreset)
	raw["key_expiration_state"] = mustRaw(record.KeyExpirationState)
	raw["key_last_used_at"] = mustRaw(record.KeyLastUsedAt)
	raw["key_created_at"] = mustRaw(record.KeyCreatedAt)
	raw["team_display_id"] = mustRaw(record.TeamDisplayID)
	raw["team_name"] = mustRaw(record.TeamName)
	raw["plan"] = mustRaw(record.Plan)
	if record.Capabilities == nil {
		raw["capabilities"] = mustRaw([]Capability{})
	} else {
		raw["capabilities"] = mustRaw(record.Capabilities)
	}
	raw["project_scope"] = mustRaw(record.ProjectScope)
	raw["expires_at"] = mustRaw(record.ExpiresAt)
	raw["last_validated_at"] = mustRaw(record.LastValidatedAt)
}

func copyRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for key, value := range in {
		copied := make([]byte, len(value))
		copy(copied, value)
		out[key] = json.RawMessage(copied)
	}
	return out
}

// mustRaw is used only for values with a fixed JSON shape owned by this
// package. A marshal failure here is a programmer error, not user input.
func mustRaw(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(data)
}
