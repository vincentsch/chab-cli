package config

import (
	"fmt"

	"github.com/vincentsch/chab-cli/internal/localfile"
	"gopkg.in/yaml.v3"
)

// RepairSelection describes the resolved profile settings that must remain
// reproducible from config and built-in defaults after an invocation ends.
type RepairSelection struct {
	Profile          string
	BaseURL          string
	APIBaseURL       string
	APIBaseURLSource RuntimeValueSource
	Locale           string
}

// RepairResolvedSelection minimally updates the prepared YAML tree, known-field
// view, and record of which fields were present. It leaves unrelated nodes and
// omitted defaults untouched. If it returns an error after inspecting existing
// fields, callers must discard the prepared File rather than attempting to
// write it.
func RepairResolvedSelection(file *File, desired RepairSelection) (bool, error) {
	if file == nil {
		return false, newError(ErrMalformedConfig, "", desired.Profile, "file", "must not be nil", nil)
	}
	normalized, err := normalizeRepairSelection(desired)
	if err != nil {
		return false, withConfigContext(err, "", desired.Profile)
	}
	ensureFileMaps(file)

	if file.root == nil {
		buildMinimalRoot(file, normalized)
		return true, nil
	}
	if file.root.Kind != yaml.MappingNode {
		return false, newError(ErrMalformedConfig, "", normalized.Profile, "file", "prepared YAML root must be a mapping", nil)
	}

	changed := false
	// Selection is independent from profile contents, so changing it alone must
	// not rebuild or normalize the target profile mapping.
	if file.CurrentProfile != normalized.Profile {
		setScalar(file.root, "current_profile", normalized.Profile, "!!str")
		file.CurrentProfile = normalized.Profile
		changed = true
	}

	profilesNode := mappingValue(file.root, "profiles")
	if profilesNode == nil {
		profilesNode = ensureMapping(file.root, "profiles")
		changed = true
	}
	if profilesNode.Kind != yaml.MappingNode {
		return false, newError(ErrMalformedConfig, "", normalized.Profile, "profiles", "must be a mapping", nil)
	}

	profileNode := mappingValue(profilesNode, normalized.Profile)
	if profileNode == nil {
		// New profiles intentionally contain only the fields needed to recreate
		// the resolved connection; unrelated defaults stay omitted.
		appendMinimalProfile(file, profilesNode, normalized)
		return true, nil
	}
	if profileNode.Kind != yaml.MappingNode {
		return false, newError(ErrMalformedConfig, "", normalized.Profile, "profiles."+normalized.Profile, "profile must be a mapping", nil)
	}

	profile := file.Profiles[normalized.Profile]
	meta := file.profileMeta[normalized.Profile]

	// Compare effective file/default behavior, not raw field presence. An
	// omitted built-in default is already durable and should remain omitted.
	effectiveBase := LegacyImplicitBaseURL
	if meta.BaseURLSet {
		effectiveBase, err = normalizeURL("base_url", profile.BaseURL)
		if err != nil {
			return false, withConfigContext(err, "", normalized.Profile)
		}
	}
	if effectiveBase != normalized.BaseURL {
		setScalar(profileNode, "base_url", normalized.BaseURL, "!!str")
		profile.BaseURL = normalized.BaseURL
		meta.BaseURLSet = true
		changed = true
	}

	switch normalized.APIBaseURLSource {
	case RuntimeValueSourceDerived:
		// A derived API URL normally remains omitted. Remove only an explicit
		// value that would override the newly derived destination.
		if meta.APIBaseURLSet {
			existingAPI, normalizeErr := normalizeURL("api_base_url", profile.APIBaseURL)
			if normalizeErr != nil {
				return false, withConfigContext(normalizeErr, "", normalized.Profile)
			}
			if existingAPI != normalized.APIBaseURL {
				apiNode := mappingValue(profileNode, "api_base_url")
				if nodeHasAliasReference(file.root, apiNode) {
					// Retain a referenced anchor while making the explicit value
					// equivalent to derivation. Removing it would leave aliases
					// in unknown extension fields dangling.
					setScalar(profileNode, "api_base_url", normalized.APIBaseURL, "!!str")
					profile.APIBaseURL = normalized.APIBaseURL
				} else {
					removeMappingPair(profileNode, "api_base_url")
					profile.APIBaseURL = ""
					meta.APIBaseURLSet = false
				}
				changed = true
			}
		}
	default:
		// Environment and flag API URLs must become explicit file values when
		// the current profile would not reproduce them.
		effectiveAPI := deriveAPIBaseURL(normalized.BaseURL)
		if meta.APIBaseURLSet {
			effectiveAPI, err = normalizeURL("api_base_url", profile.APIBaseURL)
			if err != nil {
				return false, withConfigContext(err, "", normalized.Profile)
			}
		}
		if effectiveAPI != normalized.APIBaseURL {
			setScalar(profileNode, "api_base_url", normalized.APIBaseURL, "!!str")
			profile.APIBaseURL = normalized.APIBaseURL
			meta.APIBaseURLSet = true
			changed = true
		}
	}

	effectiveLocale := ""
	if meta.LocaleSet {
		effectiveLocale = profile.Locale
	}
	// Presence matters for locale: an explicit empty scalar is preserved, while
	// an omitted empty locale remains omitted.
	if effectiveLocale != normalized.Locale {
		setScalar(profileNode, "locale", normalized.Locale, "!!str")
		profile.Locale = normalized.Locale
		meta.LocaleSet = true
		changed = true
	}

	file.Profiles[normalized.Profile] = profile
	file.profileMeta[normalized.Profile] = meta
	return changed, nil
}

// ValidatePreservingShape validates a tree prepared by RepairResolvedSelection
// without normalizing omitted fields. Setup calls this even for an exact no-op
// so an invalid known field elsewhere in the file cannot be bypassed merely
// because the selected profile already has its desired values.
func ValidatePreservingShape(path string, file *File) error {
	if err := validatePreparedFile(file); err != nil {
		return withConfigContext(err, path, "")
	}
	return nil
}

// WritePreservingShape validates and serializes a tree prepared by
// RepairResolvedSelection without normalizing omitted fields. Exact no-op
// callers must skip this function because YAML serialization may reindent an
// otherwise unchanged document.
func WritePreservingShape(path string, file *File) error {
	if path == "" {
		return newError(ErrMalformedConfig, path, "", "path", "must not be empty", nil)
	}
	if err := ValidatePreservingShape(path, file); err != nil {
		return err
	}

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{file.root}}
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

// normalizeRepairSelection validates every desired value before tree mutation
// and verifies that a claimed derived API URL actually matches its base URL.
func normalizeRepairSelection(desired RepairSelection) (RepairSelection, error) {
	if err := validateProfileName(desired.Profile); err != nil {
		return RepairSelection{}, err
	}
	baseURL, err := normalizeURL("base_url", desired.BaseURL)
	if err != nil {
		return RepairSelection{}, err
	}
	apiBaseURL, err := normalizeURL("api_base_url", desired.APIBaseURL)
	if err != nil {
		return RepairSelection{}, err
	}
	if err := validateLocale(desired.Locale); err != nil {
		return RepairSelection{}, err
	}
	switch desired.APIBaseURLSource {
	case RuntimeValueSourceDerived:
		if apiBaseURL != deriveAPIBaseURL(baseURL) {
			return RepairSelection{}, newError(ErrInvalidURL, "", desired.Profile, "api_base_url", "derived API URL does not match base_url", nil)
		}
	case RuntimeValueSourceFile, RuntimeValueSourceEnv, RuntimeValueSourceFlag:
	default:
		return RepairSelection{}, newError(ErrInvalidURL, "", desired.Profile, "api_base_url", "has invalid runtime provenance", nil)
	}
	desired.BaseURL = baseURL
	desired.APIBaseURL = apiBaseURL
	return desired, nil
}

// buildMinimalRoot constructs only the required version, selection, and profile
// mapping for a previously missing file.
func buildMinimalRoot(file *File, desired RepairSelection) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	setScalar(root, "version", fmt.Sprintf("%d", fileVersion), "!!int")
	setScalar(root, "current_profile", desired.Profile, "!!str")
	profilesNode := ensureMapping(root, "profiles")
	file.root = root
	file.Version = fileVersion
	file.CurrentProfile = desired.Profile
	appendMinimalProfile(file, profilesNode, desired)
}

// appendMinimalProfile adds one profile without materializing optional command
// defaults or a derived API URL.
func appendMinimalProfile(file *File, profilesNode *yaml.Node, desired RepairSelection) {
	profileNode := &yaml.Node{Kind: yaml.MappingNode}
	setScalar(profileNode, "base_url", desired.BaseURL, "!!str")
	profile := Profile{BaseURL: desired.BaseURL}
	meta := profileMetadata{BaseURLSet: true}
	if desired.APIBaseURLSource == RuntimeValueSourceEnv || desired.APIBaseURLSource == RuntimeValueSourceFlag {
		setScalar(profileNode, "api_base_url", desired.APIBaseURL, "!!str")
		profile.APIBaseURL = desired.APIBaseURL
		meta.APIBaseURLSet = true
	}
	if desired.Locale != "" {
		setScalar(profileNode, "locale", desired.Locale, "!!str")
		profile.Locale = desired.Locale
		meta.LocaleSet = true
	}
	profilesNode.Content = append(profilesNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: desired.Profile},
		profileNode,
	)
	file.Profiles[desired.Profile] = profile
	file.profileMeta[desired.Profile] = meta
	file.CurrentProfile = desired.Profile
}

// removeMappingPair removes exactly one key/value pair and leaves surrounding
// YAML nodes in their existing order.
func removeMappingPair(mapping *yaml.Node, key string) bool {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		keyNode := mapping.Content[index]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return true
		}
	}
	return false
}

// nodeHasAliasReference reports whether removing target would strand an alias
// elsewhere in the prepared document.
func nodeHasAliasReference(root, target *yaml.Node) bool {
	if root == nil || target == nil || target.Anchor == "" {
		return false
	}
	if root.Kind == yaml.AliasNode &&
		(root.Alias == target || root.Value == target.Anchor) {
		return true
	}
	for _, child := range root.Content {
		if nodeHasAliasReference(child, target) {
			return true
		}
	}
	return false
}

// validatePreparedFile checks structural safety and agreement between the YAML
// tree and the known-field view immediately before serialization. It validates
// present known fields without filling omitted values.
func validatePreparedFile(file *File) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", "", "file", "must not be nil", nil)
	}
	if file.root == nil || file.root.Kind != yaml.MappingNode {
		return newError(ErrMalformedConfig, "", "", "file", "prepared YAML root must be a mapping", nil)
	}
	if err := validateUniqueMappingKeys(file.root); err != nil {
		return newError(ErrMalformedConfig, "", "", "", "YAML mapping keys must be unique", err)
	}
	if err := validateAliasReferences(file.root); err != nil {
		return newError(ErrMalformedConfig, "", "", "", "YAML aliases must reference anchors in the prepared tree", err)
	}
	versionNode := mappingValue(file.root, "version")
	if versionNode == nil {
		return newError(ErrMalformedConfig, "", "", "version", "is required", nil)
	}
	version, err := scalarInt(versionNode)
	if err != nil || version != fileVersion || file.Version != fileVersion {
		return newError(ErrUnsupportedVersion, "", "", "version", "only version 1 is supported", err)
	}
	currentNode := mappingValue(file.root, "current_profile")
	if currentNode == nil {
		return newError(ErrMalformedConfig, "", "", "current_profile", "is required", nil)
	}
	current, err := scalarString(currentNode)
	if err != nil || current != file.CurrentProfile {
		return newError(ErrMalformedConfig, "", "", "current_profile", "prepared YAML tree and known-field view differ", err)
	}
	if err := validateProfileName(current); err != nil {
		return err
	}
	profilesNode := mappingValue(file.root, "profiles")
	if profilesNode == nil || profilesNode.Kind != yaml.MappingNode {
		return newError(ErrMalformedConfig, "", "", "profiles", "must be a mapping", nil)
	}
	if len(file.Profiles) != len(profilesNode.Content)/2 || len(file.profileMeta) != len(file.Profiles) {
		return newError(ErrMalformedConfig, "", "", "profiles", "prepared YAML tree and known-field view differ", nil)
	}
	for index := 0; index < len(profilesNode.Content); index += 2 {
		nameNode := profilesNode.Content[index]
		profileNode := profilesNode.Content[index+1]
		if nameNode.Kind != yaml.ScalarNode {
			return newError(ErrMalformedConfig, "", "", "profiles", "profile name keys must be scalar values", nil)
		}
		name := nameNode.Value
		if err := validateProfileName(name); err != nil {
			return err
		}
		if profileNode.Kind != yaml.MappingNode {
			return newError(ErrMalformedConfig, "", name, "profiles."+name, "profile must be a mapping", nil)
		}
		profile, meta, err := parseProfileNode("", name, profileNode)
		if err != nil {
			return err
		}
		if err := validatePresentProfile(profile, meta); err != nil {
			return withConfigContext(err, "", name)
		}
		known, ok := file.Profiles[name]
		if !ok || known != profile || file.profileMeta[name] != meta {
			return newError(ErrMalformedConfig, "", name, "profiles."+name, "prepared YAML tree and known-field view differ", nil)
		}
	}
	return nil
}

// validateAliasReferences first records every node still reachable through the
// document tree, then verifies that each alias points to one of those nodes and
// names the same anchor. This catches targets removed during repair before the
// original file is replaced.
func validateAliasReferences(root *yaml.Node) error {
	reachable := map[*yaml.Node]struct{}{}
	var collect func(*yaml.Node)
	collect = func(node *yaml.Node) {
		if node == nil {
			return
		}
		if _, exists := reachable[node]; exists {
			return
		}
		reachable[node] = struct{}{}
		for _, child := range node.Content {
			collect(child)
		}
	}
	collect(root)

	var validate func(*yaml.Node) error
	validate = func(node *yaml.Node) error {
		if node == nil {
			return nil
		}
		if node.Kind == yaml.AliasNode {
			if node.Alias == nil {
				return fmt.Errorf("alias %q has no target", node.Value)
			}
			if _, exists := reachable[node.Alias]; !exists {
				return fmt.Errorf("alias %q targets a removed node", node.Value)
			}
			if node.Value == "" || node.Alias.Anchor != node.Value {
				return fmt.Errorf("alias %q does not match target anchor %q", node.Value, node.Alias.Anchor)
			}
		}
		for _, child := range node.Content {
			if err := validate(child); err != nil {
				return err
			}
		}
		return nil
	}
	return validate(root)
}

// validatePresentProfile validates only fields that were explicitly present in
// the prepared profile, preserving omission as a meaningful file shape.
func validatePresentProfile(profile Profile, meta profileMetadata) error {
	if meta.BaseURLSet {
		if _, err := normalizeURL("base_url", profile.BaseURL); err != nil {
			return err
		}
	}
	if meta.APIBaseURLSet {
		if _, err := normalizeURL("api_base_url", profile.APIBaseURL); err != nil {
			return err
		}
	}
	if meta.LocaleSet {
		if err := validateLocale(profile.Locale); err != nil {
			return err
		}
	}
	if meta.DefaultOutputSet {
		if err := validateDefaultOutput(profile.DefaultOutput); err != nil {
			return err
		}
	}
	if meta.ProjectListLimitSet {
		if err := validateProjectListLimit(profile.Defaults.ProjectListLimit); err != nil {
			return err
		}
	}
	return nil
}
