package api

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Path joins segments into a request path, escaping each segment so opaque ids
// cannot add separators, queries, or fragments. Empty segments are omitted.
func Path(segments ...string) string {
	var escaped []string
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		escaped = append(escaped, escapePathSegment(segment))
	}
	return strings.Join(escaped, "/")
}

// escapePathSegment treats each caller value as an opaque segment. PathEscape
// leaves "." and ".." unchanged, so handle those cases before they can be
// interpreted as relative path segments by routing layers.
func escapePathSegment(segment string) string {
	switch segment {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	default:
		return url.PathEscape(segment)
	}
}

// requestURL joins the configured API base path and caller path without
// url.ResolveReference, which would normalize escaped or dot-like segments.
func (c *Client) requestURL(path string, query url.Values) string {
	return joinAPIPath(c.base, path, query)
}

// ResolveAPILink maps a documented /v1 resource link from an API envelope onto
// this client's configured API base. Absolute, off-base, traversal, and
// fragment-bearing links are refused locally.
func (c *Client) ResolveAPILink(raw string) (string, error) {
	return resolveCanonicalAPILink(c.base, raw)
}

// joinAPIPath appends an API-relative path to an already-validated base URL
// without resolving dot segments or letting caller input replace the base path.
func joinAPIPath(base *url.URL, path string, query url.Values) string {
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	callerPath := strings.TrimLeft(path, "/")

	joinedPath := basePath
	if callerPath != "" {
		if joinedPath == "" {
			joinedPath = "/" + callerPath
		} else {
			joinedPath += "/" + callerPath
		}
	}

	target := base.Scheme + "://" + base.Host + joinedPath
	if encodedQuery := query.Encode(); encodedQuery != "" {
		target += "?" + encodedQuery
	}
	return target
}

func joinAppOriginPath(base *url.URL, path string, query url.Values) string {
	callerPath := strings.TrimLeft(path, "/")
	target := base.Scheme + "://" + base.Host
	if callerPath != "" {
		target += "/" + callerPath
	}
	if encodedQuery := query.Encode(); encodedQuery != "" {
		target += "?" + encodedQuery
	}
	return target
}

func resolveCanonicalAPILink(base *url.URL, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("API link must not be empty")
	}
	if strings.TrimSpace(raw) != raw {
		return "", fmt.Errorf("API link must not include leading or trailing whitespace")
	}
	if strings.Contains(raw, "#") {
		return "", fmt.Errorf("API link fragment is not allowed")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(raw, "//") {
		return "", fmt.Errorf("API link must be relative to /v1")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("API link fragment is not allowed")
	}
	if parsed.EscapedPath() == "/v1" {
		return joinAPIPath(base, "", parsed.Query()), nil
	}
	if !strings.HasPrefix(parsed.EscapedPath(), "/v1/") {
		return "", fmt.Errorf("API link must start with /v1/")
	}
	rel := strings.TrimPrefix(parsed.EscapedPath(), "/v1/")
	if rel == "" || strings.HasSuffix(rel, "/") || strings.Contains(rel, "//") {
		return "", fmt.Errorf("API link path is invalid")
	}
	parts := strings.Split(rel, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("API link path contains an empty segment")
		}
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return "", err
		}
		if decoded == "." || decoded == ".." || strings.Contains(decoded, "/") {
			return "", fmt.Errorf("API link traversal is not allowed")
		}
		segments = append(segments, decoded)
	}
	return joinAPIPath(base, Path(segments...), parsed.Query()), nil
}

// NormalizeURLForMatch canonicalizes API-owned URLs for exact deployment
// matching. It rejects ambiguous pieces and compares hosts literally so aliases
// such as localhost and 127.0.0.1 remain distinct.
func NormalizeURLForMatch(raw string) (string, error) {
	parsed, err := validateBaseURL(raw)
	if err != nil {
		return "", err
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	path := parsed.EscapedPath()
	if strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return parsed.Scheme + "://" + host + path, nil
}

// validateBaseURL accepts only an absolute API origin and optional base path.
// Query markers, fragments, and userinfo would make later request joining or
// exact deployment matching ambiguous.
func validateBaseURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("URL must not be empty")
	}
	if strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("URL must not include leading or trailing whitespace")
	}
	if strings.Contains(raw, "#") {
		return nil, fmt.Errorf("fragment is not allowed")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("URL must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("userinfo is not allowed")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, fmt.Errorf("query is not allowed")
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("fragment is not allowed")
	}
	copy := *parsed
	return &copy, nil
}

// parseBaseURL keeps older internal tests pointed at the same validator used by
// client construction.
func parseBaseURL(raw string) (*url.URL, error) {
	return validateBaseURL(raw)
}
