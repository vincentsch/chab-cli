package config

import "gopkg.in/yaml.v3"

const (
	DefaultProfile    = "local"
	DefaultBaseURL    = "https://www.chab.ai"
	DefaultAPIBaseURL = "https://www.chab.ai/v1"
	// LegacyImplicitBaseURL preserves the effective destination of profiles
	// created before the production default existed but saved without a URL.
	LegacyImplicitBaseURL   = "http://localhost"
	DefaultOutput           = "table"
	DefaultProjectListLimit = 30
	fileVersion             = 1
)

// OverrideString carries a parsed string flag value and whether the user
// supplied it explicitly. An explicitly supplied blank value is still Set.
type OverrideString struct {
	Value string
	Set   bool
}

// FlagOverrides contains the persistent flags owned by this runtime layer.
type FlagOverrides struct {
	Profile    OverrideString
	ConfigPath OverrideString
	AuthPath   OverrideString
	BaseURL    OverrideString
	APIBaseURL OverrideString
	Locale     OverrideString
}

// ResolveMode controls whether a missing selected profile is a local error.
type ResolveMode int

const (
	ResolveStrict ResolveMode = iota
	ResolveForWrite
)

// ProfileSource reports which precedence tier selected the runtime profile.
// Login uses this to distinguish exact user targets from automatic defaults.
type ProfileSource int

const (
	// Default/current profiles are automatic login inputs.
	ProfileSourceDefault ProfileSource = iota
	ProfileSourceCurrent
	// Env/flag profiles are exact login write targets.
	ProfileSourceEnv
	ProfileSourceFlag
)

// RuntimeValueSource reports which precedence tier supplied a resolved runtime
// value. Commands use it when the final value is not enough to distinguish a
// built-in default from an explicit flag, environment, or profile setting.
type RuntimeValueSource int

const (
	RuntimeValueSourceDefault RuntimeValueSource = iota
	RuntimeValueSourceFile
	RuntimeValueSourceEnv
	RuntimeValueSourceFlag
	RuntimeValueSourceDerived
)

// Options injects all process-dependent inputs for runtime resolution.
type Options struct {
	Flags         FlagOverrides
	LookupEnv     func(string) (string, bool)
	UserConfigDir func() (string, error)
	Mode          ResolveMode
}

// Runtime is the fully resolved non-secret runtime state used by later command
// and API layers.
type Runtime struct {
	Profile       string
	ProfileSource ProfileSource
	ConfigPath    string
	AuthPath      string
	BaseURL       string
	// BaseURLSource lets interactive commands decide whether the base URL still
	// needs confirmation or was already supplied by the user or config file.
	BaseURLSource RuntimeValueSource
	APIBaseURL    string
	// APIBaseURLSource distinguishes an explicit API URL from one derived from
	// the resolved product base URL.
	APIBaseURLSource RuntimeValueSource
	Locale           string
	DefaultOutput    string
	ProjectListLimit int
}

// File is the public, known-field view of config.yml. Unknown YAML keys are
// retained internally after Load and preserved by Write.
type File struct {
	Version        int
	CurrentProfile string
	Profiles       map[string]Profile

	root   *yaml.Node
	exists bool
	// legacyAuthProfiles holds only names, never keys. A stored credential for
	// a profile without a saved URL retains that profile's localhost destination,
	// even after another profile creates config.yml.
	legacyAuthProfiles map[string]bool
	profileMeta        map[string]profileMetadata
}

// Exists reports whether Load read an existing config file. Missing files
// produce a valid in-memory default file and return false here.
func (f *File) Exists() bool {
	return f != nil && f.exists
}

// Profile contains one connection profile's known non-secret settings.
type Profile struct {
	BaseURL       string
	APIBaseURL    string
	Locale        string
	DefaultOutput string
	Defaults      Defaults
}

// Defaults contains per-profile default command behavior carried by the
// runtime, even before the commands that consume it are implemented.
type Defaults struct {
	ProjectListLimit int
}

// profileMetadata records whether known YAML fields were present in the
// loaded file. Profile uses Go zero values, so resolution and writes need this
// side channel to tell "omitted" from "explicitly set to empty or zero".
type profileMetadata struct {
	BaseURLSet          bool
	APIBaseURLSet       bool
	LocaleSet           bool
	DefaultOutputSet    bool
	DefaultsSet         bool
	ProjectListLimitSet bool
}
