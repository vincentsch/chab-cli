// Package rungradadapter contains retained adapters for packages that still
// compile against rungrad-shaped contracts.
package rungradadapter

import (
	"github.com/vincentsch/chab-cli/internal/config"
	rgconfig "github.com/vincentsch/rungrad/config"
)

// Config projects Chab's YAML config file into rungrad's config shape. Chab
// ResolveRuntime remains authoritative for final runtime precedence.
func Config(path string) (rgconfig.Config, error) {
	file, err := config.Load(path)
	if err != nil {
		return rgconfig.Config{}, err
	}
	out := rgconfig.Config{Version: 1, CurrentProfile: file.CurrentProfile}
	if len(file.Profiles) == 0 {
		return out, nil
	}
	out.Profiles = make(map[string]rgconfig.Profile, len(file.Profiles))
	for name, profile := range file.Profiles {
		rp := rgconfig.Profile{BaseURL: profile.BaseURL}
		apiBase := profile.APIBaseURL
		if apiBase == "" && profile.BaseURL != "" {
			apiBase = config.DeriveAPIBaseURL(profile.BaseURL)
		}
		if apiBase != "" {
			rp.Services = map[string]string{"api_base_url": apiBase}
		}
		out.Profiles[name] = rp
	}
	return out, nil
}

// ValidateServiceURL validates a resolved base/API URL the way Chab runtime
// resolution does, for use as a rungrad Service.Validate hook.
func ValidateServiceURL(raw string) error {
	return config.ValidateEndpointURL(raw)
}

// Resolved projects a fully resolved Chab Runtime into the rungrad shape.
func Resolved(rt config.Runtime) rgconfig.Resolved {
	return rgconfig.Resolved{
		Profile:      rt.Profile,
		ConfigPath:   rt.ConfigPath,
		AuthFilePath: rt.AuthPath,
		Services: map[string]rgconfig.ResolvedService{
			"base_url":     {Value: rt.BaseURL},
			"api_base_url": {Value: rt.APIBaseURL},
			"locale":       {Value: rt.Locale},
		},
	}
}
