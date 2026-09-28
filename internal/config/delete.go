package config

import "gopkg.in/yaml.v3"

// ProfileDisposition tells callers how to persist a profile delete.
type ProfileDisposition int

const (
	// DispositionWrite means at least one profile remains, so the caller should
	// write the updated config file.
	DispositionWrite ProfileDisposition = iota
	// DispositionRemoveFile means the final profile was deleted from a file that
	// contains only CLI-owned data, so removing config.yml is safe.
	DispositionRemoveFile
	// DispositionKeepFinalProfile means deleting the final profile would discard
	// comments or unknown top-level keys; callers should fail before writing.
	DispositionKeepFinalProfile
)

// DeleteProfileResult describes the in-memory result of deleting a profile.
// Callers use it to decide whether to write, remove, or keep the file and what
// user-facing notes to print.
type DeleteProfileResult struct {
	Existed                bool
	WasCurrent             bool
	PreviousCurrentMissing bool
	NewCurrent             string
	CurrentChanged         bool
	RemainingCount         int
	Disposition            ProfileDisposition
}

// DeleteProfile removes a profile from the in-memory file and decides how the
// caller should persist the change. It does not touch disk.
func DeleteProfile(file *File, name string) (DeleteProfileResult, error) {
	if file == nil {
		return DeleteProfileResult{}, newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	if err := validateProfileName(name); err != nil {
		return DeleteProfileResult{}, err
	}

	_, existed := file.Profiles[name]
	res := DeleteProfileResult{Existed: existed, RemainingCount: len(file.Profiles)}
	if !existed {
		return res, nil
	}

	old := file.CurrentProfile
	if old == "" {
		old = DefaultProfile
	}
	res.WasCurrent = old == name

	delete(file.Profiles, name)
	delete(file.profileMeta, name)
	res.RemainingCount = len(file.Profiles)

	if res.RemainingCount > 0 {
		_, currentStillExists := file.Profiles[old]
		if res.WasCurrent || !currentStillExists {
			// Keep current_profile pointing at a saved profile. This also repairs
			// configs that were already dangling before this delete.
			res.PreviousCurrentMissing = !res.WasCurrent
			res.NewCurrent = sortedProfileNames(file)[0]
			file.CurrentProfile = res.NewCurrent
			res.CurrentChanged = true
		}
		res.Disposition = DispositionWrite
		return res, nil
	}

	if fileHasOnlyKnownData(file) {
		res.Disposition = DispositionRemoveFile
	} else {
		res.Disposition = DispositionKeepFinalProfile
	}
	return res, nil
}

func fileHasOnlyKnownData(file *File) bool {
	if file == nil {
		return true
	}
	root := file.root
	if root == nil || root.Kind != yaml.MappingNode {
		return true
	}
	if nodeHasComments(root) {
		return false
	}

	// Removing the final profile deletes the whole file. Only do that when the
	// document has no user-owned data; comments anywhere in the node tree or
	// unknown top-level keys are treated as data to preserve.
	known := map[string]bool{
		"version":         true,
		"current_profile": true,
		"profiles":        true,
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if !known[key.Value] {
			return false
		}
	}
	return true
}

func nodeHasComments(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.HeadComment != "" || node.LineComment != "" || node.FootComment != "" {
		return true
	}
	for _, child := range node.Content {
		if nodeHasComments(child) {
			return true
		}
	}
	return false
}
