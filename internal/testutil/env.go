package testutil

import "os"

// HermeticEnv returns a LookupEnv that resolves only the supplied values and
// never consults the process environment. A nil map behaves like a fully empty
// environment.
func HermeticEnv(values map[string]string) func(string) (string, bool) {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return func(name string) (string, bool) {
		value, ok := copied[name]
		return value, ok
	}
}

// ViltEnvNames returns the canonical CHAB_* auth/config variable names that can
// make command behavior depend on a developer machine. ClearViltEnv and the
// no-secret release smoke script both clear exactly these.
func ViltEnvNames() []string {
	return []string{
		"CHAB_API_KEY",
		"CHAB_PROFILE",
		"CHAB_CONFIG",
		"CHAB_AUTH_FILE",
		"CHAB_BASE_URL",
		"CHAB_API_BASE_URL",
		"CHAB_LOCALE",
		"CHAB_PAGER",
	}
}

// ClearViltEnv removes host CHAB_* values that can make command tests depend
// on a developer machine. Individual tests can still opt into env behavior with
// t.Setenv or HermeticEnv.
func ClearViltEnv() {
	for _, name := range ViltEnvNames() {
		_ = os.Unsetenv(name)
	}
}
