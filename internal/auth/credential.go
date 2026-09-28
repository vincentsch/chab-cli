package auth

import (
	"fmt"
	"os"

	"github.com/vincentsch/chab-cli/internal/config"
)

// LookupCredential selects the API key for the resolved runtime. CHAB_API_KEY
// wins and avoids reading auth.json; otherwise the selected profile is loaded
// from the auth file and any permission findings are returned to the caller.
func LookupCredential(runtime config.Runtime, opts Options) (Credential, []PermissionFinding, error) {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	if value, ok := lookupNonBlank(lookup, "CHAB_API_KEY"); ok {
		return Credential{
			APIKey:        value,
			Source:        SourceEnv,
			Profile:       runtime.Profile,
			PrincipalType: credentialPrincipalType(value, ""),
		}, nil, nil
	}

	file, findings, err := Load(runtime.AuthPath)
	if err != nil {
		return Credential{}, findings, err
	}
	record, ok := file.Profiles[runtime.Profile]
	if !ok || record.APIKey == "" {
		return Credential{}, findings, newError(ErrMissingCredential, runtime.AuthPath, runtime.Profile, "api_key", "no API key found for selected profile", nil)
	}
	return Credential{
		APIKey:        record.APIKey,
		Source:        SourceAuthFile,
		Profile:       runtime.Profile,
		DisplayID:     record.DisplayID,
		KeyName:       record.KeyName,
		PrincipalType: credentialPrincipalType(record.APIKey, record.PrincipalType),
	}, findings, nil
}

func credentialPrincipalType(key, cached string) string {
	if len(key) >= len("chab_guest_") && key[:len("chab_guest_")] == "chab_guest_" {
		return "guest_trial"
	}
	return cached
}

// String omits APIKey so accidental fmt usage cannot expose the secret. The
// remaining fields are file-controlled metadata, so redact them before
// rendering too.
func (c Credential) String() string {
	return fmt.Sprintf("credential source=%s profile=%q display_id=%q key_name=%q",
		c.Source,
		RedactString(c.Profile),
		RedactString(c.DisplayID),
		RedactString(c.KeyName),
	)
}

// GoString matches String for %#v, which is a common debug-printing path.
func (c Credential) GoString() string {
	return c.String()
}

func lookupNonBlank(lookup func(string) (string, bool), name string) (string, bool) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}
