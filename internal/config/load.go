package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads config.yml. A missing file is valid and returns an empty v1 file.
func Load(path string) (*File, error) {
	if path == "" {
		return nil, newError(ErrMalformedConfig, path, "", "path", "must not be empty", nil)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newFile(false), nil
		}
		return nil, newError(ErrMalformedConfig, path, "", "", "could not read config file", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, newError(ErrMalformedConfig, path, "", "", "config file is empty", nil)
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, newError(ErrMalformedConfig, path, "", "", "could not parse YAML", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return nil, newError(ErrMalformedConfig, path, "", "", "config file must contain exactly one YAML document", nil)
	} else if !errors.Is(err, io.EOF) {
		return nil, newError(ErrMalformedConfig, path, "", "", "could not parse YAML", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, newError(ErrMalformedConfig, path, "", "", "top-level YAML document must be a mapping", nil)
	}
	// Keep the original root node so Write can update known keys in place while
	// preserving unknown keys and the existing mapping order where possible.
	root := doc.Content[0]
	if err := validateUniqueMappingKeys(root); err != nil {
		return nil, newError(ErrMalformedConfig, path, "", "", "YAML mapping keys must be unique", err)
	}

	versionNode := mappingValue(root, "version")
	if versionNode == nil {
		return nil, newError(ErrMalformedConfig, path, "", "version", "is required", nil)
	}
	version, err := scalarInt(versionNode)
	if err != nil {
		return nil, newError(ErrMalformedConfig, path, "", "version", "must be numeric", err)
	}
	if version != fileVersion {
		return nil, newError(ErrUnsupportedVersion, path, "", "version", "only version 1 is supported", nil)
	}

	file := newFile(true)
	file.Version = version
	file.root = root

	if node := mappingValue(root, "current_profile"); node != nil {
		current, err := scalarString(node)
		if err != nil {
			return nil, newError(ErrMalformedConfig, path, "", "current_profile", "must be a string", err)
		}
		file.CurrentProfile = current
	}

	if node := mappingValue(root, "profiles"); node != nil {
		if node.Kind != yaml.MappingNode {
			return nil, newError(ErrMalformedConfig, path, "", "profiles", "must be a mapping", nil)
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				return nil, newError(ErrMalformedConfig, path, "", "profiles", "profile name keys must be scalar values", nil)
			}
			name := key.Value
			if value.Kind != yaml.MappingNode {
				return nil, newError(ErrMalformedConfig, path, name, "profiles."+name, "profile must be a mapping", nil)
			}
			profile, meta, err := parseProfileNode(path, name, value)
			if err != nil {
				return nil, err
			}
			// Presence metadata is stored beside the public profile because empty
			// strings and zero integers can mean either "unset" or "invalid value".
			file.Profiles[name] = profile
			file.profileMeta[name] = meta
		}
	}

	return file, nil
}

// LoadWithAuthFallback keeps a legacy credential whose profile has no saved
// destination on its old implicit localhost destination. This also applies
// after another profile creates config.yml. Parsed keys are never retained:
// only profile names with nonempty credentials enter the returned File.
func LoadWithAuthFallback(configPath, authPath string) (*File, error) {
	file, err := Load(configPath)
	if err != nil {
		return file, err
	}
	data, err := os.ReadFile(authPath)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		// Auth lookup remains the authority for auth-file errors; runtime URL
		// discovery cannot block an environment-supplied credential on this
		// optional legacy hint. Without a usable stored key, no old key can be
		// redirected by falling back to the fresh default.
		return file, nil
	}
	var saved struct {
		Version  int `json:"version"`
		Profiles map[string]struct {
			APIKey string `json:"api_key"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &saved); err != nil || saved.Version != 1 {
		// This optional destination hint must not change the auth error/warning
		// ordering of callers that subsequently load auth.json. A malformed
		// credential cannot be used for an authenticated request anyway.
		return file, nil
	}
	for name, profile := range saved.Profiles {
		if profile.APIKey != "" {
			if file.legacyAuthProfiles == nil {
				file.legacyAuthProfiles = make(map[string]bool)
			}
			file.legacyAuthProfiles[name] = true
		}
	}
	return file, nil
}

func validateUniqueMappingKeys(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]*yaml.Node, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind == yaml.ScalarNode {
				identity := key.Tag + "\x00" + key.Value
				if first, ok := seen[identity]; ok {
					return fmt.Errorf("mapping key %q at line %d duplicates line %d", key.Value, key.Line, first.Line)
				}
				seen[identity] = key
			}
		}
	}
	for _, child := range node.Content {
		if err := validateUniqueMappingKeys(child); err != nil {
			return err
		}
	}
	return nil
}

func newFile(exists bool) *File {
	return &File{
		Version:     fileVersion,
		Profiles:    map[string]Profile{},
		exists:      exists,
		profileMeta: map[string]profileMetadata{},
	}
}

func parseProfileNode(path, name string, node *yaml.Node) (Profile, profileMetadata, error) {
	var profile Profile
	var meta profileMetadata

	if value, set, err := stringField(node, "base_url"); err != nil {
		return profile, meta, newError(ErrMalformedConfig, path, name, "base_url", "must be a string", err)
	} else if set {
		profile.BaseURL = value
		meta.BaseURLSet = true
	}

	if value, set, err := stringField(node, "api_base_url"); err != nil {
		return profile, meta, newError(ErrMalformedConfig, path, name, "api_base_url", "must be a string", err)
	} else if set {
		profile.APIBaseURL = value
		meta.APIBaseURLSet = true
	}

	if value, set, err := stringField(node, "locale"); err != nil {
		return profile, meta, newError(ErrMalformedConfig, path, name, "locale", "must be a string", err)
	} else if set {
		profile.Locale = value
		meta.LocaleSet = true
	}

	if value, set, err := stringField(node, "default_output"); err != nil {
		return profile, meta, newError(ErrMalformedConfig, path, name, "default_output", "must be a string", err)
	} else if set {
		profile.DefaultOutput = value
		meta.DefaultOutputSet = true
	}

	defaults := mappingValue(node, "defaults")
	if defaults != nil {
		if defaults.Kind != yaml.MappingNode {
			return profile, meta, newError(ErrMalformedConfig, path, name, "defaults", "must be a mapping", nil)
		}
		meta.DefaultsSet = true
		if limitNode := mappingValue(defaults, "project_list_limit"); limitNode != nil {
			limit, err := scalarInt(limitNode)
			if err != nil {
				return profile, meta, newError(ErrMalformedConfig, path, name, "defaults.project_list_limit", "must be numeric", err)
			}
			profile.Defaults.ProjectListLimit = limit
			meta.ProjectListLimitSet = true
		}
	}

	return profile, meta, nil
}

func stringField(node *yaml.Node, key string) (string, bool, error) {
	value := mappingValue(node, key)
	if value == nil {
		return "", false, nil
	}
	s, err := scalarString(value)
	return s, true, err
}

func scalarString(node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return "", errWrongType
	}
	if node.Tag != "" && node.Tag != "!!str" {
		return "", errWrongType
	}
	return node.Value, nil
}

func scalarInt(node *yaml.Node) (int, error) {
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return 0, errWrongType
	}
	// Use the scalar text so currently accepted numeric-looking YAML scalars
	// keep their existing behavior.
	return strconv.Atoi(node.Value)
}

var errWrongType = errors.New("wrong YAML scalar type")

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
