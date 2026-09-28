package config

import (
	"fmt"
	"sort"

	"github.com/vincentsch/chab-cli/internal/localfile"
	"gopkg.in/yaml.v3"
)

// UpsertProfile validates and stores a profile in the known-field view. Unknown
// keys from a loaded profile with the same name are preserved by Write.
func UpsertProfile(file *File, name string, profile Profile) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	if err := validateProfileName(name); err != nil {
		return err
	}
	normalized, err := normalizeProfileForWrite(profile, profileMetadata{})
	if err != nil {
		return withConfigContext(err, "", name)
	}
	file.Profiles[name] = normalized
	file.profileMeta[name] = allProfileFieldsSet()
	if file.Version == 0 {
		file.Version = fileVersion
	}
	return nil
}

// SetCurrentProfile validates and selects the default profile for later
// invocations. It does not require the profile to exist so login can call it
// after validation in whichever order is convenient.
func SetCurrentProfile(file *File, name string) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	if err := validateProfileName(name); err != nil {
		return err
	}
	file.CurrentProfile = name
	if file.Version == 0 {
		file.Version = fileVersion
	}
	return nil
}

// Write validates and atomically replaces config.yml. Unknown YAML keys from a
// loaded file are preserved.
func Write(path string, file *File) error {
	if path == "" {
		return newError(ErrMalformedConfig, path, "", "path", "must not be empty", nil)
	}
	if err := normalizeFileForWrite(file); err != nil {
		return withConfigContext(err, path, "")
	}

	// Start from the loaded YAML tree when one exists. That lets this package
	// own the documented keys without deleting extensions written by other tools.
	root := file.root
	if root == nil || root.Kind != yaml.MappingNode {
		root = &yaml.Node{Kind: yaml.MappingNode}
		file.root = root
	}

	setScalar(root, "version", fmt.Sprintf("%d", fileVersion), "!!int")
	setScalar(root, "current_profile", file.CurrentProfile, "!!str")
	profilesNode := ensureMapping(root, "profiles")
	syncProfileNodes(profilesNode, file)

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return newError(ErrMalformedConfig, path, "", "", "could not marshal config YAML", err)
	}

	if err := localfile.AtomicWrite(path, data, 0o700, 0o644); err != nil {
		return newError(ErrMalformedConfig, path, "", "", "could not write config file", err)
	}
	file.exists = true
	return nil
}

func normalizeFileForWrite(file *File) error {
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
	if file.CurrentProfile == "" {
		file.CurrentProfile = DefaultProfile
	}
	if err := validateProfileName(file.CurrentProfile); err != nil {
		return err
	}

	names := make([]string, 0, len(file.Profiles))
	for name := range file.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// Writes validate the complete known-field view so any file emitted by
		// this package is internally consistent.
		if err := validateProfileName(name); err != nil {
			return err
		}
		meta := file.profileMeta[name]
		profile := file.Profiles[name]
		if file.exists && !meta.BaseURLSet {
			profile.BaseURL = LegacyImplicitBaseURL
		}
		normalized, err := normalizeProfileForWrite(profile, meta)
		if err != nil {
			return withConfigContext(err, "", name)
		}
		file.Profiles[name] = normalized
		file.profileMeta[name] = allProfileFieldsSet()
	}
	return nil
}

func normalizeProfileForWrite(profile Profile, meta profileMetadata) (Profile, error) {
	// Code-created profiles use zero values to mean "fill the default". Loaded
	// profiles use metadata so explicit empty/zero values are still validated.
	if profile.BaseURL == "" && !meta.BaseURLSet {
		profile.BaseURL = DefaultBaseURL
	}
	baseURL, err := normalizeURL("base_url", profile.BaseURL)
	if err != nil {
		return Profile{}, err
	}
	profile.BaseURL = baseURL

	if profile.APIBaseURL == "" && !meta.APIBaseURLSet {
		profile.APIBaseURL = deriveAPIBaseURL(profile.BaseURL)
	}
	apiBaseURL, err := normalizeURL("api_base_url", profile.APIBaseURL)
	if err != nil {
		return Profile{}, err
	}
	profile.APIBaseURL = apiBaseURL

	if err := validateLocale(profile.Locale); err != nil {
		return Profile{}, err
	}

	if profile.DefaultOutput == "" && !meta.DefaultOutputSet {
		profile.DefaultOutput = DefaultOutput
	}
	if err := validateDefaultOutput(profile.DefaultOutput); err != nil {
		return Profile{}, err
	}

	if profile.Defaults.ProjectListLimit == 0 && !meta.ProjectListLimitSet {
		profile.Defaults.ProjectListLimit = DefaultProjectListLimit
	}
	if err := validateProjectListLimit(profile.Defaults.ProjectListLimit); err != nil {
		return Profile{}, err
	}

	return profile, nil
}

func ensureFileMaps(file *File) {
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}
	if file.profileMeta == nil {
		file.profileMeta = map[string]profileMetadata{}
	}
}

func allProfileFieldsSet() profileMetadata {
	return profileMetadata{
		BaseURLSet:          true,
		APIBaseURLSet:       true,
		LocaleSet:           true,
		DefaultOutputSet:    true,
		DefaultsSet:         true,
		ProjectListLimitSet: true,
	}
}

// syncProfileNodes mirrors File.Profiles into the YAML profiles mapping.
// Existing profile nodes are reused to preserve unknown keys; newly added
// profiles are appended in a stable order.
func syncProfileNodes(profilesNode *yaml.Node, file *File) {
	present := map[string]bool{}
	for name := range file.Profiles {
		present[name] = true
	}

	filtered := profilesNode.Content[:0]
	for i := 0; i < len(profilesNode.Content); i += 2 {
		key := profilesNode.Content[i]
		value := profilesNode.Content[i+1]
		if present[key.Value] {
			filtered = append(filtered, key, value)
			setProfileNode(value, file.Profiles[key.Value])
			delete(present, key.Value)
		}
	}
	profilesNode.Content = filtered

	names := make([]string, 0, len(present))
	for name := range present {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profileNode := &yaml.Node{Kind: yaml.MappingNode}
		setProfileNode(profileNode, file.Profiles[name])
		profilesNode.Content = append(profilesNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			profileNode,
		)
	}
}

func setProfileNode(node *yaml.Node, profile Profile) {
	if node.Kind != yaml.MappingNode {
		node.Kind = yaml.MappingNode
		node.Tag = ""
		node.Value = ""
		node.Content = nil
	}
	setScalar(node, "base_url", profile.BaseURL, "!!str")
	setScalar(node, "api_base_url", profile.APIBaseURL, "!!str")
	setScalar(node, "locale", profile.Locale, "!!str")
	setScalar(node, "default_output", profile.DefaultOutput, "!!str")
	defaultsNode := ensureMapping(node, "defaults")
	setScalar(defaultsNode, "project_list_limit", fmt.Sprintf("%d", profile.Defaults.ProjectListLimit), "!!int")
}

func ensureMapping(mapping *yaml.Node, key string) *yaml.Node {
	value := mappingValue(mapping, key)
	if value != nil {
		if value.Kind != yaml.MappingNode {
			value.Kind = yaml.MappingNode
			value.Tag = ""
			value.Value = ""
			value.Content = nil
		}
		return value
	}
	value = &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
	return value
}

func setScalar(mapping *yaml.Node, key, value, tag string) {
	node := mappingValue(mapping, key)
	if node == nil {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
		)
		return
	}
	node.Kind = yaml.ScalarNode
	node.Tag = tag
	node.Value = value
	node.Content = nil
}
