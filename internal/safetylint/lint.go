// Package safetylint owns the dependency-neutral safety policy shared by
// production checks and test assertions.
package safetylint

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// These patterns define the shared safety bar for help text, generated
	// docs, checked examples, and golden files. They intentionally catch broad
	// leak shapes rather than trying to prove a value is a real secret or real
	// local path.
	TicketIDPattern   = regexp.MustCompile(`\bp\d{2}-\d{3}\b`)
	LocalPathPattern  = regexp.MustCompile(`(/home/|/Users/|C:\\)`)
	SecretPairPattern = regexp.MustCompile(`[A-Za-z0-9_-]{6,}\|\S+`)
	// Cobra prints string defaults as (default "value"), so a closing quote is
	// a URL boundary for generated help text rather than part of the URL.
	URLPattern = regexp.MustCompile("https?://[^\\s)\"`]+")
)

// Violations returns one message per safety rule broken by text: leaked
// planning ids, local machine paths, key-shaped tokens, or non-allowlisted URL
// hosts or shapes.
func Violations(text string) []string {
	var violations []string
	if TicketIDPattern.MatchString(text) {
		violations = append(violations, "leaks ticket id")
	}
	if LocalPathPattern.MatchString(text) {
		violations = append(violations, "leaks local machine path")
	}
	if SecretPairPattern.MatchString(text) {
		violations = append(violations, "leaks key-shaped token")
	}

	for _, rawURL := range URLPattern.FindAllString(text, -1) {
		if !urlAllowed(rawURL) {
			violations = append(violations, fmt.Sprintf("contains disallowed URL %q", rawURL))
		}
	}
	return violations
}

// urlAllowed accepts local/example hosts and the small set of repository
// release and companion-documentation links used by repository documentation.
func urlAllowed(rawURL string) bool {
	candidate := strings.TrimRight(rawURL, ".,")
	switch candidate {
	case "https://github.com/vincentsch/chab-cli",
		"https://raw.githubusercontent.com/vincentsch/chab-cli/v0.1.0/scripts/install.sh",
		"https://raw.githubusercontent.com/vincentsch/chab-cli/v0.1.1/scripts/install.sh",
		"https://github.com/vincentsch/chab-saas",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md#hosted-mcp-planned",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access-launch.md",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access-launch.md#5-deploy-hosted-mcp-to-staging",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/hosted-mcp.md":
		return true
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	switch {
	case host == "www.chab.ai":
		if parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		return parsed.Path == "" || parsed.Path == "/" || parsed.Path == "/v1" || parsed.Path == "/mcp"
	case host == "localhost" || host == "example.test" || strings.HasSuffix(host, ".example.test"):
		return true
	case host == "github.com":
		// Release docs need public GitHub links, but only to this repository's
		// release listing, tag pages, or concrete release assets.
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
		if len(segments) < 3 || segments[0] != "vincentsch" || segments[1] != "chab-cli" || segments[2] != "releases" {
			return false
		}
		switch len(segments) {
		case 3:
			return true
		case 5:
			return segments[3] == "tag" && segments[4] != ""
		case 6:
			return segments[3] == "download" && segments[4] != "" && segments[5] != ""
		default:
			return false
		}
	default:
		return false
	}
}
