package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// DownloadOptions configures byte transfers that intentionally differ from
// normal API JSON calls. Redirect following is opt-in and uses a fresh request
// without Chab credentials or cookies.
type DownloadOptions struct {
	Query               url.Values
	MaxBytes            int64
	FollowHTTPSRedirect bool
	Accept              string
}

// Download streams one authenticated response, with no redirects or retries.
// A body failure must not append a second attempt to a partial file. The caller
// owns publishing/removing its partial output. Neither filenames nor bodies
// from the provider are written to diagnostics.
func (c *Client) Download(ctx context.Context, path string, query url.Values, dst io.Writer, maxBytes int64) (int64, ResponseMeta, error) {
	return c.DownloadWithOptions(ctx, path, dst, DownloadOptions{Query: query, MaxBytes: maxBytes})
}

// DownloadAPILink resolves a documented relative /v1 link from an API envelope
// and streams the target with the same safeguards as DownloadWithOptions.
func (c *Client) DownloadAPILink(ctx context.Context, rawLink string, dst io.Writer, opts DownloadOptions) (int64, ResponseMeta, error) {
	if len(opts.Query) > 0 {
		return 0, ResponseMeta{}, &UsageError{Field: "download", Detail: "API-link downloads cannot add query parameters"}
	}
	target, err := c.ResolveAPILink(rawLink)
	if err != nil {
		redactor := c.newRequestRedactor("")
		return 0, ResponseMeta{}, &UsageError{Field: "download_url", Detail: "could not resolve API download link", Err: redactor.redactErr(err)}
	}
	return c.downloadTarget(ctx, target, dst, opts)
}

// DownloadWithOptions streams one bounded download. When FollowHTTPSRedirect
// is true, one external HTTPS redirect is accepted after local target
// validation; authorization, idempotency and cookies are not forwarded.
func (c *Client) DownloadWithOptions(ctx context.Context, path string, dst io.Writer, opts DownloadOptions) (int64, ResponseMeta, error) {
	return c.downloadTarget(ctx, c.requestURL(path, opts.Query), dst, opts)
}

func (c *Client) downloadTarget(ctx context.Context, target string, dst io.Writer, opts DownloadOptions) (int64, ResponseMeta, error) {
	if dst == nil || opts.MaxBytes < 1 {
		return 0, ResponseMeta{}, &UsageError{Field: "download", Detail: "requires a writer and positive byte limit"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	redactor := c.newRequestRedactor("")
	req, err := c.newRequest(ctx, http.MethodGet, target, nil, false, "", nil)
	if err != nil {
		return 0, ResponseMeta{}, &UsageError{Field: "path", Detail: "could not build download URL", Err: redactor.redactErr(err)}
	}
	accept := opts.Accept
	if accept == "" {
		accept = "application/octet-stream, message/rfc822, application/json"
	}
	req.Header.Set("Accept", accept)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, ResponseMeta{}, c.transportError(err, 1, nil, redactor)
	}
	defer resp.Body.Close()
	meta := redactor.redactMeta(captureResponseMeta(resp.StatusCode, resp.Header, c.now()))
	meta.Attempts = 1
	c.debugMetaWith(redactor, meta)
	if isDownloadRedirect(resp.StatusCode) && opts.FollowHTTPSRedirect {
		location := resp.Header.Get("Location")
		target, validateErr := validateDownloadRedirect(location)
		if validateErr != nil {
			return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: validateErr.Error(), Meta: meta}
		}
		return c.downloadRedirect(ctx, target, dst, opts, meta, redactor)
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 65537))
		if readErr != nil {
			return 0, meta, c.transportError(readErr, 1, nil, redactor)
		}
		if len(body) > 65536 {
			return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: "download error response exceeds limit", Meta: meta}
		}
		decoded, decodeErr := c.decodeError(resp.StatusCode, body, meta, redactor)
		return 0, decoded, decodeErr
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") && !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
		return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: "unexpected HTML page instead of authenticated download", Meta: meta}
	}
	if resp.ContentLength > opts.MaxBytes {
		return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: "download exceeds configured byte limit", Meta: meta}
	}
	count, err := io.Copy(dst, io.LimitReader(resp.Body, opts.MaxBytes))
	if err != nil {
		return count, meta, c.transportError(err, 1, nil, redactor)
	}
	var extra [1]byte
	n, err := io.ReadFull(resp.Body, extra[:])
	if n > 0 {
		return count, meta, &ProtocolError{Status: resp.StatusCode, Detail: "download exceeds configured byte limit", Meta: meta}
	}
	if err != nil && err != io.EOF {
		return count, meta, c.transportError(err, 1, nil, redactor)
	}
	return count, meta, nil
}

func (c *Client) downloadRedirect(ctx context.Context, target string, dst io.Writer, opts DownloadOptions, meta ResponseMeta, redactor requestRedactor) (int64, ResponseMeta, error) {
	redirectRedactor := c.newRequestRedactor(target)
	redirectClient := *c.httpClient
	redirectClient.Jar = nil
	redirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, meta, &UsageError{Field: "redirect", Detail: "could not build download redirect request", Err: redirectRedactor.redactErr(err)}
	}
	accept := opts.Accept
	if accept == "" {
		accept = "application/octet-stream"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "chab/"+c.userAgentVersion)
	c.debugfWith(redactor, "following validated HTTPS download redirect")
	resp, err := redirectClient.Do(req)
	if err != nil {
		return 0, meta, c.transportError(err, 1, nil, redirectRedactor)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: fmt.Sprintf("redirect download returned HTTP status %d", resp.StatusCode), Meta: meta}
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") && !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
		return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: "unexpected HTML page instead of redirected download", Meta: meta}
	}
	if resp.ContentLength > opts.MaxBytes {
		return 0, meta, &ProtocolError{Status: resp.StatusCode, Detail: "download exceeds configured byte limit", Meta: meta}
	}
	count, err := io.Copy(dst, io.LimitReader(resp.Body, opts.MaxBytes))
	if err != nil {
		return count, meta, c.transportError(err, 1, nil, redirectRedactor)
	}
	var extra [1]byte
	n, err := io.ReadFull(resp.Body, extra[:])
	if n > 0 {
		return count, meta, &ProtocolError{Status: resp.StatusCode, Detail: "download exceeds configured byte limit", Meta: meta}
	}
	if err != nil && err != io.EOF {
		return count, meta, c.transportError(err, 1, nil, redirectRedactor)
	}
	return count, meta, nil
}

func isDownloadRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func validateDownloadRedirect(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) != raw {
		return "", fmt.Errorf("download redirect target is missing or malformed")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("download redirect target is malformed")
	}
	if !parsed.IsAbs() || strings.ToLower(parsed.Scheme) != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("download redirect target must be an absolute HTTPS URL")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("download redirect target must not contain a fragment")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", fmt.Errorf("download redirect target host is not allowed")
	}
	if ip := parseIPHost(host); ip.IsValid() {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return "", fmt.Errorf("download redirect target host is not allowed")
		}
	}
	return parsed.String(), nil
}

func parseIPHost(host string) netip.Addr {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr
	}
	if parsed := net.ParseIP(host); parsed != nil {
		if addr, ok := netip.AddrFromSlice(parsed); ok {
			return addr
		}
	}
	return netip.Addr{}
}
