package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

const (
	FamilyReservedSentence = "The subcommands listed below are planned for a later release of chab and cannot be run yet."
	LeafReservedSentence   = "This command is planned for a later release of chab and cannot be run yet."
)

// newReservedFamily creates a command family that can show help today but has
// no feature behavior.
func newReservedFamily(use, short, long string, related []string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  reservedFamilyLong(long, related),
		Args:  cobra.ArbitraryArgs,
		RunE:  runFamilyHelpOrUnknown,
	}
}

// newReservedLeaf creates a planned leaf command. It accepts arbitrary args so
// the shell consistently returns the reserved-command error until the command
// defines real validation.
func newReservedLeaf(use, short, purpose string, related []string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  reservedLeafLong(purpose, related),
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return &ReservedCommandError{Path: cmd.CommandPath()}
		},
	}
}

func reservedLeafLong(purpose string, related []string) string {
	return reservedLong(purpose, LeafReservedSentence, related)
}

func reservedFamilyLong(purpose string, related []string) string {
	return reservedLong(purpose, FamilyReservedSentence, related)
}

// reservedLong keeps family and leaf help wording in the same shape: purpose,
// reserved-status sentence, then nearby commands users can run or inspect.
func reservedLong(purpose, sentence string, related []string) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(purpose))
	builder.WriteString("\n\n")
	builder.WriteString(sentence)
	if len(related) == 0 {
		return builder.String()
	}
	builder.WriteString("\n\nRelated commands:")
	for _, command := range related {
		builder.WriteString("\n  ")
		builder.WriteString(command)
	}
	return builder.String()
}
