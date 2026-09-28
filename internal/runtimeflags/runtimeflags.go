// Package runtimeflags converts parsed Cobra flags into runtime config
// overrides without resolving files, environment variables, or credentials.
package runtimeflags

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/config"
)

// Overrides converts already-parsed persistent flags into config resolution
// inputs. Cobra's Changed bit preserves explicitly supplied empty strings.
func Overrides(cmd *cobra.Command) config.FlagOverrides {
	if cmd == nil || cmd.Root() == nil {
		return config.FlagOverrides{}
	}
	// Persistent flags live on the root command even when a leaf command is
	// executed, so read from the root instead of the command that received Run.
	return config.FlagOverrides{
		Profile:    stringOverride(cmd, "profile"),
		ConfigPath: stringOverride(cmd, "config"),
		AuthPath:   stringOverride(cmd, "auth-file"),
		BaseURL:    stringOverride(cmd, "base-url"),
		APIBaseURL: stringOverride(cmd, "api-base-url"),
		Locale:     stringOverride(cmd, "locale"),
	}
}

func stringOverride(cmd *cobra.Command, name string) config.OverrideString {
	flag := cmd.Root().PersistentFlags().Lookup(name)
	if flag == nil {
		return config.OverrideString{}
	}
	// Value alone is not enough: an explicitly supplied empty string must stay
	// different from an unset flag so config validation can reject it.
	return config.OverrideString{
		Value: flag.Value.String(),
		Set:   flag.Changed,
	}
}
