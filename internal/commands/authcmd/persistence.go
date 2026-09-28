package authcmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// configPreparation changes an in-memory config and reports whether it needs
// to be written. Login and setup deliberately use different preparation rules.
type configPreparation func(*config.File) (bool, error)

// configWriter lets the shared credential transaction use either the normal
// config serializer or setup's shape-preserving serializer.
type configWriter func(string, *config.File) error

// loginConfigPreparation retains login's normalizing profile write behavior.
func loginConfigPreparation(rt config.Runtime, profileName string) configPreparation {
	return func(file *config.File) (bool, error) {
		target := file.Profiles[profileName]
		target.BaseURL = rt.BaseURL
		target.APIBaseURL = rt.APIBaseURL
		target.Locale = rt.Locale
		if target.DefaultOutput == "" {
			target.DefaultOutput = rt.DefaultOutput
		}
		if target.Defaults.ProjectListLimit == 0 {
			target.Defaults.ProjectListLimit = rt.ProjectListLimit
		}
		if err := config.UpsertProfile(file, profileName, target); err != nil {
			return false, err
		}
		if err := config.SetCurrentProfile(file, profileName); err != nil {
			return false, err
		}
		return true, nil
	}
}

// setupConfigPreparation persists only the resolved fields needed to reproduce
// this setup invocation later.
func setupConfigPreparation(rt config.Runtime) configPreparation {
	return func(file *config.File) (bool, error) {
		changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
			Profile:          rt.Profile,
			BaseURL:          rt.BaseURL,
			APIBaseURL:       rt.APIBaseURL,
			APIBaseURLSource: rt.APIBaseURLSource,
			Locale:           rt.Locale,
		})
		if err != nil {
			return false, err
		}
		if err := config.ValidatePreservingShape(rt.ConfigPath, file); err != nil {
			return false, err
		}
		return changed, nil
	}
}

// persistValidatedCredential writes config before auth, then restores the exact
// previous config bytes if any auth-side step fails. The caller supplies the
// config strategy so login can normalize while setup can preserve YAML shape.
func persistValidatedCredential(
	cmd *cobra.Command,
	f *cmdutil.Factory,
	rt config.Runtime,
	profileName string,
	data api.WhoamiData,
	key string,
	authFindingsWarned bool,
	operation string,
	prepare configPreparation,
	writer configWriter,
) error {
	file, err := config.Load(rt.ConfigPath)
	if err != nil {
		return err
	}
	// Capture disk state before the preparation callback mutates its in-memory
	// view. The raw snapshot also represents a previously missing file.
	snapshot, err := config.CaptureRollbackSnapshot(rt.ConfigPath, file)
	if err != nil {
		return err
	}
	changed, err := prepare(file)
	if err != nil {
		return err
	}
	wroteConfig := false
	if changed {
		if err := writer(rt.ConfigPath, file); err != nil {
			return err
		}
		wroteConfig = true
	}

	authFile, findings, err := auth.Load(rt.AuthPath)
	if err == nil {
		if record, ok := authFile.Profiles[profileName]; ok && record.APIKey != "" {
			// Register the prior credential before displaying permission
			// findings whose path may contain user-controlled text.
			f.RegisterSecret(record.APIKey)
		}
	}
	if !authFindingsWarned {
		// The earlier overwrite/setup gate may already have displayed these
		// findings; do not repeat them during the persistence reload.
		cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), presentationPermissionFindings(f, findings))
	}
	if err != nil {
		rollbackCredentialConfig(cmd, f, rt, profileName, operation, snapshot, wroteConfig)
		return err
	}

	validatedAt := f.Clock()().UTC().Truncate(time.Second)
	record := profileAuthFromWhoami(data, key, validatedAt)
	if err := auth.PutProfile(authFile, profileName, record); err != nil {
		rollbackCredentialConfig(cmd, f, rt, profileName, operation, snapshot, wroteConfig)
		return err
	}
	if err := auth.Write(rt.AuthPath, authFile); err != nil {
		rollbackCredentialConfig(cmd, f, rt, profileName, operation, snapshot, wroteConfig)
		return err
	}
	return nil
}

// repairSetupConfig applies the preserving writer after stored-key validation.
// Skipping the writer on an exact no-op is what keeps the original bytes.
func repairSetupConfig(rt config.Runtime) error {
	file, err := config.Load(rt.ConfigPath)
	if err != nil {
		return err
	}
	changed, err := setupConfigPreparation(rt)(file)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return config.WritePreservingShape(rt.ConfigPath, file)
}

// rollbackCredentialConfig is best effort and intentionally keeps the auth
// failure as the command's primary error. A rollback failure is additional,
// redacted context written as a warning.
func rollbackCredentialConfig(
	cmd *cobra.Command,
	f *cmdutil.Factory,
	rt config.Runtime,
	profileName string,
	operation string,
	snapshot config.RollbackSnapshot,
	wroteConfig bool,
) {
	if !wroteConfig {
		return
	}
	if err := config.RestoreRollbackSnapshot(snapshot); err != nil {
		fmt.Fprintf(
			cmd.ErrOrStderr(),
			"Warning: %s updated config for profile %q at %s, but auth persistence failed and rollback also failed: %s\n",
			semanticString(f, operation),
			semanticString(f, profileName),
			semanticString(f, rt.ConfigPath),
			semanticString(f, err.Error()),
		)
	}
}

// rollbackLoginConfig retains the focused compatibility seam used by package
// tests while real command paths provide their invocation redaction registry.
func rollbackLoginConfig(cmd *cobra.Command, rt config.Runtime, profileName string, snapshot config.RollbackSnapshot) {
	rollbackCredentialConfig(cmd, nil, rt, profileName, "login", snapshot, true)
}
