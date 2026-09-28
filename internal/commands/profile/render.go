package profile

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// profileListTable maps each stable profile field to the public table layout.
// Keep the row values in the same order as Columns when adding or moving data.
func profileListTable(rows []profileJSON) output.Table {
	tableRows := make([][]string, 0, len(rows))
	for _, profile := range rows {
		current := ""
		if profile.Selected {
			current = "yes"
		}
		tableRows = append(tableRows, []string{
			profile.Name,
			current,
			yesNo(profile.Persisted),
			profile.BaseURL,
			profile.APIBaseURL,
			localeListCell(profile.Locale),
			profile.DefaultOutput,
			strconv.Itoa(profile.ProjectListLimit),
			keyCell(profile.StoredAuth),
			teamCell(profile.StoredAuth),
			lastValidatedCell(profile.StoredAuth),
		})
	}
	return output.Table{
		Columns: []string{"PROFILE", "CURRENT", "SAVED", "BASE URL", "API BASE URL", "LOCALE", "DEFAULT OUTPUT", "LIMIT", "AUTH", "TEAM", "LAST VALIDATED"},
		Rows:    tableRows,
		Empty:   "No profiles found.",
	}
}

func renderProfileListHuman(w io.Writer, result profileListJSON) {
	profileListContext(result).Render(w)
	fmt.Fprintln(w)
	profileListTable(result.Profiles).Render(w)
	if note := unsavedProfileNote(result.Profiles); note != "" {
		fmt.Fprintln(w, note)
	}
}

func renderProfileListPlain(data, prose io.Writer, result profileListJSON) {
	// Plain mode keeps machine-readable rows on stdout and explanatory context
	// on stderr, matching the list-command contract used elsewhere.
	profileListProseContext(result).RenderPlain(io.Discard, prose)
	if note := unsavedProfileNote(result.Profiles); note != "" {
		fmt.Fprintln(prose, note)
	}
	profileListTable(result.Profiles).RenderPlain(data, prose)
}

func profileListContext(result profileListJSON) output.Detail {
	return output.Detail{Nodes: []output.Node{
		output.Field("Current profile", result.CurrentProfile),
		output.Field("Config", result.ConfigPath),
	}}
}

func profileListProseContext(result profileListJSON) output.Detail {
	return output.Detail{Nodes: []output.Node{
		output.Guidance("Current profile: " + result.CurrentProfile),
		output.Guidance("Config: " + result.ConfigPath),
	}}
}

func profileDetail(profile profileJSON) output.Detail {
	nodes := []output.Node{
		output.Field("Profile", profile.Name),
		output.Field("Current", yesNo(profile.Selected)),
		output.Field("Saved", yesNo(profile.Persisted)),
		output.Field("Base URL", profile.BaseURL),
		output.Field("API base URL", profile.APIBaseURL),
		output.Field("Locale", localeDetailCell(profile.Locale)),
		output.Field("Default output", profile.DefaultOutput),
		output.Field("Project list limit", strconv.Itoa(profile.ProjectListLimit)),
		output.Field("Stored API key", storedAuthDetail(profile.StoredAuth)),
		output.Field("Stored team", storedTeamDetail(profile.StoredAuth)),
		output.Field("Last validated", lastValidatedDetail(profile.StoredAuth)),
	}
	if !profile.Persisted {
		nodes = append(nodes, output.Guidance("This profile is not saved; built-in defaults are shown. Save it with chab profile create or chab login --profile <name>."))
	}
	return output.Detail{Nodes: nodes}
}

func configPathDetail(paths configPathJSON) output.Detail {
	return output.Detail{Nodes: []output.Node{
		output.Field("Config path", paths.ConfigPath),
		output.Field("Auth path", paths.AuthPath),
	}}
}

func configListTable(values []configValueJSON) output.Table {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.Key, configValueCell(value.Value), value.Source})
	}
	return output.Table{
		Columns: []string{"KEY", "VALUE", "SOURCE"},
		Rows:    rows,
		Empty:   "No configuration values found.",
	}
}

func renderConfigListHuman(w io.Writer, result configListJSON) {
	configListContext(result).Render(w)
	fmt.Fprintln(w)
	configListTable(result.Values).Render(w)
}

func renderConfigListPlain(data, prose io.Writer, result configListJSON) {
	// The path is context, not row data, so keep it off stdout in plain mode.
	configListProseContext(result).RenderPlain(io.Discard, prose)
	configListTable(result.Values).RenderPlain(data, prose)
}

func configListContext(result configListJSON) output.Detail {
	return output.Detail{Nodes: []output.Node{
		output.Field("Config", result.ConfigPath),
	}}
}

func configListProseContext(result configListJSON) output.Detail {
	return output.Detail{Nodes: []output.Node{
		output.Guidance("Config: " + result.ConfigPath),
	}}
}

func configValueDetail(value configValueJSON) output.Detail {
	return output.Detail{Nodes: []output.Node{
		output.Field("key", value.Key),
		output.Field("value", configValueDetailCell(value.Value)),
		output.Field("source", value.Source),
	}}
}

func mutationHuman(summary output.MutationSummary) func(io.Writer) {
	return func(w io.Writer) {
		summary.Render(w)
	}
}

func mutationPlain(summary output.MutationSummary) func(io.Writer, io.Writer) {
	return summary.RenderPlain
}

func localeListCell(locale string) string {
	if locale == "" {
		return "-"
	}
	return locale
}

func localeDetailCell(locale string) string {
	if locale == "" {
		return "(unset)"
	}
	return locale
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func keyCell(stored storedAuthJSON) string {
	if !stored.Present {
		return "none"
	}
	switch {
	case stored.DisplayID != nil && *stored.DisplayID != "" && stored.KeyName != nil && *stored.KeyName != "":
		return fmt.Sprintf("%s (%s)", *stored.DisplayID, *stored.KeyName)
	case stored.DisplayID != nil && *stored.DisplayID != "":
		return *stored.DisplayID
	case stored.KeyName != nil && *stored.KeyName != "":
		return *stored.KeyName
	default:
		return "stored"
	}
}

func storedAuthDetail(stored storedAuthJSON) string {
	if !stored.Present {
		return "none"
	}
	switch {
	case stored.DisplayID != nil && *stored.DisplayID != "" && stored.KeyName != nil && *stored.KeyName != "":
		return fmt.Sprintf("%s (%s)", *stored.DisplayID, *stored.KeyName)
	case stored.DisplayID != nil && *stored.DisplayID != "":
		return *stored.DisplayID
	case stored.KeyName != nil && *stored.KeyName != "":
		return *stored.KeyName
	default:
		return "stored"
	}
}

func teamCell(stored storedAuthJSON) string {
	if !stored.Present {
		return "-"
	}
	return teamValue(stored, "-")
}

func storedTeamDetail(stored storedAuthJSON) string {
	if !stored.Present {
		return "none"
	}
	return teamValue(stored, "(unset)")
}

func teamValue(stored storedAuthJSON, fallback string) string {
	switch {
	case stored.TeamName != nil && *stored.TeamName != "" && stored.TeamDisplayID != nil && *stored.TeamDisplayID != "":
		return fmt.Sprintf("%s (%s)", *stored.TeamName, *stored.TeamDisplayID)
	case stored.TeamName != nil && *stored.TeamName != "":
		return *stored.TeamName
	case stored.TeamDisplayID != nil && *stored.TeamDisplayID != "":
		return *stored.TeamDisplayID
	default:
		return fallback
	}
}

func lastValidatedCell(stored storedAuthJSON) string {
	if stored.LastValidatedAt == nil {
		return "-"
	}
	return stored.LastValidatedAt.Format(time.RFC3339)
}

func lastValidatedDetail(stored storedAuthJSON) string {
	if !stored.Present {
		return "none"
	}
	if stored.LastValidatedAt == nil {
		return "(unset)"
	}
	return stored.LastValidatedAt.Format(time.RFC3339)
}

// unsavedProfileNote adds guidance for override-selected or mixed saved/unsaved
// results. The lone built-in local row needs no extra prose because its SAVED
// column already says "no".
func unsavedProfileNote(rows []profileJSON) string {
	for _, row := range rows {
		if row.Persisted {
			continue
		}
		if len(rows) == 1 && row.Name == config.DefaultProfile && row.Selected {
			continue
		}
		return "Some listed profiles are not saved; save one with chab profile create or chab login --profile <name>."
	}
	return ""
}

// configValueCell converts the small set of JSON-native config scalar types to
// plain table text. Nil stays empty so an unset locale remains script-friendly.
func configValueCell(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func configValueDetailCell(value any) string {
	if value == nil {
		return "(unset)"
	}
	return configValueCell(value)
}
