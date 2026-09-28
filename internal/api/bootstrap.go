package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/redact"
)

const DeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"
const ChabOAuthClientID = "chab-cli"
const DevelopmentClientVersion = "1.0.0"

var semverRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

// EffectiveClientVersion returns the protocol version sent to Chab OAuth and
// compatibility endpoints. Local development builds display "dev" but must
// still send a supported SemVer to the server.
func EffectiveClientVersion(displayVersion string) string {
	if semverRE.MatchString(displayVersion) {
		return displayVersion
	}
	return DevelopmentClientVersion
}

type CompatibilityData struct {
	SchemaVersion         string                `json:"schema_version"`
	APIMajor              int                   `json:"api_major"`
	MinimumVersion        string                `json:"minimum_version"`
	RecommendedVersion    string                `json:"recommended_version"`
	CatalogVersion        string                `json:"catalog_version"`
	BlockedRanges         []BlockedRange        `json:"blocked_ranges"`
	Notices               []CompatibilityNotice `json:"notices"`
	URLs                  map[string]string     `json:"urls"`
	LocalMCP              json.RawMessage       `json:"local_mcp,omitempty"`
	HostedMCP             json.RawMessage       `json:"hosted_mcp,omitempty"`
	GuestTrialCredentials json.RawMessage       `json:"guest_trial_credentials,omitempty"`
}

// GuestLocalMCP is the small validated portion of the compatibility document
// needed to constrain local guest tools. The raw document above retains
// unknown fields so newer server metadata survives cache round trips.
type GuestLocalMCP struct {
	Accepted                    bool        `json:"accepted"`
	Enabled                     bool        `json:"enabled"`
	SpendEnabled                bool        `json:"spend_enabled"`
	Tools                       []GuestTool `json:"tools"`
	SupportedRouteOperationKeys []string    `json:"supported_route_operation_keys"`
}

type GuestTool struct {
	ToolName            string `json:"tool_name"`
	OperationKey        string `json:"operation_key"`
	Scope               string `json:"scope"`
	Method              string `json:"method"`
	Path                string `json:"path"`
	IdempotencyRequired bool   `json:"idempotency_required"`
	FundingMode         string `json:"funding_mode"`
	CreditSource        string `json:"credit_source"`
}

// HostedMCPInfo reports the current backend rollout state. It does not turn
// a REST guest bearer into a hosted OAuth credential.
type HostedMCPInfo struct {
	RequiresVerifiedAccount  bool     `json:"requires_verified_account"`
	GuestCredentialsAccepted bool     `json:"guest_credentials_accepted"`
	OperationTransport       bool     `json:"operation_transport"`
	AdvertisedOperationKeys  []string `json:"advertised_operation_keys"`
}

func (c CompatibilityData) HostedMCPInfo() (HostedMCPInfo, error) {
	if len(c.HostedMCP) == 0 {
		return HostedMCPInfo{}, fmt.Errorf("hosted MCP compatibility metadata is missing")
	}
	var hosted HostedMCPInfo
	if err := json.Unmarshal(c.HostedMCP, &hosted); err != nil {
		return HostedMCPInfo{}, fmt.Errorf("invalid hosted MCP compatibility metadata: %w", err)
	}
	return hosted, nil
}

func (c CompatibilityData) GuestLocalMCP() (GuestLocalMCP, error) {
	var local struct {
		Transport             string          `json:"transport"`
		APITransport          string          `json:"api_transport"`
		GuestTrialCredentials json.RawMessage `json:"guest_trial_credentials"`
	}
	if err := json.Unmarshal(c.LocalMCP, &local); err != nil {
		return GuestLocalMCP{}, fmt.Errorf("invalid local MCP compatibility metadata: %w", err)
	}
	if local.Transport != "stdio" || local.APITransport != "rest" || len(local.GuestTrialCredentials) == 0 {
		return GuestLocalMCP{}, fmt.Errorf("guest local MCP is not advertised as stdio over REST")
	}
	var guest GuestLocalMCP
	if err := json.Unmarshal(local.GuestTrialCredentials, &guest); err != nil {
		return GuestLocalMCP{}, fmt.Errorf("invalid guest local MCP compatibility metadata: %w", err)
	}
	// Accepted/enabled govern new credential issuance, not authorization of an
	// already-issued guest bearer. Route and spend flags remain authoritative.
	return guest, nil
}

type BlockedRange struct {
	Minimum string `json:"minimum,omitempty"`
	Maximum string `json:"maximum,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type CompatibilityNotice struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

// DeviceData is the pending device authorization returned before the user
// approves login in the browser. DeviceCode is secret; UserCode is display data.
type DeviceData struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenData is the one-time issued Chab API token.
type TokenData struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	ExpiresIn   *int   `json:"expires_in,omitempty"`
}

// BootstrapOptions configures the credential-free OAuth/bootstrap client.
type BootstrapOptions struct {
	AppBaseURL             string
	APIBaseURL             string
	Locale                 string
	UserAgentVersion       string
	HTTPClient             *http.Client
	Now                    func() time.Time
	DebugWriter            io.Writer
	CompatibilityCachePath string
}

// OptionsForBootstrap maps resolved runtime values into bootstrap options.
func OptionsForBootstrap(runtime config.Runtime, version string, debug bool, debugOutput io.Writer) BootstrapOptions {
	opts := BootstrapOptions{
		AppBaseURL:       runtime.BaseURL,
		APIBaseURL:       runtime.APIBaseURL,
		Locale:           runtime.Locale,
		UserAgentVersion: EffectiveClientVersion(version),
	}
	if runtime.ConfigPath != "" {
		opts.CompatibilityCachePath = filepath.Join(filepath.Dir(runtime.ConfigPath), "compatibility-cache.json")
	}
	if debug && debugOutput != nil {
		opts.DebugWriter = debugOutput
	}
	return opts
}

type BootstrapClient struct {
	appBase                *url.URL
	apiBase                *url.URL
	httpClient             *http.Client
	locale                 string
	userAgentVersion       string
	now                    func() time.Time
	debugWriter            io.Writer
	compatibilityCachePath string
	debugMu                sync.Mutex
	secretMu               sync.Mutex
	secretValues           []string
	replacer               atomic.Pointer[strings.Replacer]
}

func NewBootstrap(opts BootstrapOptions) (*BootstrapClient, error) {
	appBase, err := validateBaseURL(opts.AppBaseURL)
	if err != nil {
		return nil, &UsageError{Field: "base_url", Detail: "must be an absolute http or https URL without userinfo, query, or fragment", Err: err}
	}
	apiBase, err := validateBaseURL(opts.APIBaseURL)
	if err != nil {
		return nil, &UsageError{Field: "api_base_url", Detail: "must be an absolute http or https URL without userinfo, query, or fragment", Err: err}
	}
	version := opts.UserAgentVersion
	if version == "" {
		version = DevelopmentClientVersion
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	} else {
		cloned := *httpClient
		httpClient = &cloned
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &BootstrapClient{
		appBase:                appBase,
		apiBase:                apiBase,
		httpClient:             httpClient,
		locale:                 opts.Locale,
		userAgentVersion:       version,
		now:                    now,
		debugWriter:            opts.DebugWriter,
		compatibilityCachePath: opts.CompatibilityCachePath,
	}, nil
}

func (c *BootstrapClient) RegisterSecret(value string) {
	if c == nil || value == "" {
		return
	}
	c.secretMu.Lock()
	defer c.secretMu.Unlock()
	for _, existing := range c.secretValues {
		if existing == value {
			return
		}
	}
	c.secretValues = append(c.secretValues, value)
	c.replacer.Store(newSecretReplacer(c.secretValues))
}

func (c *BootstrapClient) DeviceEndpoint() string {
	return joinAppOriginPath(c.appBase, "/oauth/device/code", nil)
}

func (c *BootstrapClient) TokenEndpoint() string {
	return joinAppOriginPath(c.appBase, "/oauth/token", nil)
}

func (c *BootstrapClient) Compatibility(ctx context.Context, clientVersion string) (CompatibilityData, ResponseMeta, error) {
	key := compatibilityCacheKey(c.apiBase.String(), clientVersion)
	entry, hasEntry := loadCompatibilityCache(key, c.now, c.compatibilityCachePath)
	headers := http.Header{}
	if hasEntry && entry.etag != "" {
		headers.Set("If-None-Match", entry.etag)
	}
	var data CompatibilityData
	meta, etag, notModified, err := c.doEnvelope(ctx, http.MethodGet, joinAPIPath(c.apiBase, "cli/compatibility", nil), headers, nil, &data)
	if err != nil {
		return CompatibilityData{}, meta, err
	}
	if notModified {
		if !hasEntry {
			return CompatibilityData{}, meta, &ProtocolError{Detail: "compatibility cache revalidation returned 304 without cached metadata", Status: http.StatusNotModified, RequestID: meta.RequestID, Meta: meta}
		}
		data = entry.data
	} else {
		if err := validateCompatibility(data, clientVersion, meta); err != nil {
			return CompatibilityData{}, meta, err
		}
		storeCompatibilityCache(key, compatibilityCacheEntry{data: data, etag: etag, expiresAt: c.now().Add(5 * time.Minute)}, c.compatibilityCachePath)
		return data, meta, nil
	}
	if err := validateCompatibility(data, clientVersion, meta); err != nil {
		return CompatibilityData{}, meta, err
	}
	return data, meta, nil
}

func (c *BootstrapClient) CreateDevice(ctx context.Context, clientID, clientVersion string, scopes []string, deviceName string) (DeviceData, ResponseMeta, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_version", clientVersion)
	if len(scopes) > 0 {
		form.Set("scope", strings.Join(scopes, " "))
	}
	if deviceName != "" {
		form.Set("device_name", deviceName)
	}
	var data DeviceData
	meta, err := c.doOAuthForm(ctx, c.DeviceEndpoint(), form, &data)
	return data, meta, err
}

func (c *BootstrapClient) PollToken(ctx context.Context, deviceCode string) (TokenData, ResponseMeta, error) {
	form := url.Values{}
	form.Set("grant_type", DeviceCodeGrantType)
	form.Set("client_id", ChabOAuthClientID)
	form.Set("device_code", deviceCode)
	var data TokenData
	meta, err := c.doOAuthForm(ctx, c.TokenEndpoint(), form, &data)
	return data, meta, err
}

func (c *BootstrapClient) doOAuthForm(ctx context.Context, target string, form url.Values, out any) (ResponseMeta, error) {
	body := []byte(form.Encode())
	headers := http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}}
	meta, _, _, err := c.doBare(ctx, http.MethodPost, target, headers, body, out)
	return meta, err
}

func (c *BootstrapClient) doEnvelope(ctx context.Context, method, target string, headers http.Header, body []byte, out any) (ResponseMeta, string, bool, error) {
	meta, responseBody, respHeaders, err := c.send(ctx, method, target, headers, body)
	if err != nil {
		return meta, "", false, err
	}
	etag := respHeaders.Get("ETag")
	if meta.HTTPStatus == http.StatusNotModified {
		return meta, etag, true, nil
	}
	if meta.HTTPStatus < 200 || meta.HTTPStatus >= 300 {
		_, err := c.decodeEnvelopeError(meta.HTTPStatus, responseBody, meta)
		return meta, etag, false, err
	}
	meta, err = c.decodeEnvelopeSuccess(meta.HTTPStatus, responseBody, meta, out)
	return meta, etag, false, err
}

func (c *BootstrapClient) doBare(ctx context.Context, method, target string, headers http.Header, body []byte, out any) (ResponseMeta, []byte, http.Header, error) {
	meta, responseBody, respHeaders, err := c.send(ctx, method, target, headers, body)
	if err != nil {
		return meta, nil, nil, err
	}
	if meta.HTTPStatus >= 200 && meta.HTTPStatus < 300 {
		if out != nil {
			c.registerSuccessDataSecrets(responseBody)
			if err := json.Unmarshal(responseBody, out); err != nil {
				return meta, nil, nil, &ProtocolError{Detail: "response data does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: meta}
			}
		}
		return meta, responseBody, respHeaders, nil
	}
	return meta, nil, respHeaders, c.decodeOAuthError(meta.HTTPStatus, responseBody, meta)
}

func (c *BootstrapClient) send(ctx context.Context, method, target string, headers http.Header, body []byte) (ResponseMeta, []byte, http.Header, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return ResponseMeta{}, nil, nil, &UsageError{Field: "path", Detail: "could not build request URL", Err: err}
	}
	if len(body) > 0 {
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "chab/"+c.userAgentVersion)
	if c.locale != "" {
		req.Header.Set("Accept-Language", c.locale)
	}
	c.debugf("request %s %s attempt=1", method, target)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.debugf("transport error: %s", err)
		return ResponseMeta{}, nil, nil, &TransportError{Err: c.redactErr(err), Attempts: 1}
	}
	meta := captureResponseMeta(resp.StatusCode, resp.Header, c.now())
	meta.Attempts = 1
	c.debugf("response status=%d request_id=%s attempt=1", resp.StatusCode, meta.RequestID)
	c.debugMeta(meta)
	responseBody, readErr := readAndClose(resp.Body)
	if readErr != nil {
		return meta, nil, resp.Header.Clone(), &TransportError{Err: c.redactErr(readErr), Attempts: 1}
	}
	return meta, responseBody, resp.Header.Clone(), nil
}

func (c *BootstrapClient) decodeEnvelopeSuccess(status int, body []byte, meta ResponseMeta, out any) (ResponseMeta, error) {
	meta = c.redactMeta(meta)
	if len(bytes.TrimSpace(body)) == 0 {
		return ResponseMeta{}, &ProtocolError{Detail: "empty response body", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	var env successEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ResponseMeta{}, &ProtocolError{Detail: "malformed JSON response body", Status: status, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: meta}
	}
	if perr := c.applyEnvelopeMeta(&meta, env, status); perr != nil {
		return ResponseMeta{}, perr
	}
	if out != nil {
		payload := env.Data
		if payload == nil {
			payload = env.Operation
		}
		if payload == nil || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
			return ResponseMeta{}, &ProtocolError{Detail: "response envelope missing data", Status: status, RequestID: meta.RequestID, Meta: meta}
		}
		if err := json.Unmarshal(payload, out); err != nil {
			return ResponseMeta{}, &ProtocolError{Detail: "response data does not match the expected shape", Status: status, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: meta}
		}
	}
	return meta, nil
}

func (c *BootstrapClient) decodeEnvelopeError(status int, body []byte, meta ResponseMeta) (ResponseMeta, error) {
	meta = c.redactMeta(meta)
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ResponseMeta{}, &ProtocolError{Detail: "malformed JSON response body", Status: status, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: meta}
	}
	c.applyEnvelopeRequestID(&meta, env.RequestID)
	if env.Error == nil || env.Error.Code == "" || env.Error.Retryable == nil {
		return ResponseMeta{}, &ProtocolError{Detail: "error envelope missing required fields", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	return ResponseMeta{}, &Error{
		Code:           c.redactText(env.Error.Code),
		Message:        c.redactText(env.Error.Message),
		UserAction:     c.redactText(env.Error.UserAction),
		Retryable:      *env.Error.Retryable,
		Details:        c.redactDetails(decodeDetails(env.Error.Details)),
		DetailsValue:   redactStructuredDetails(decodeAnyDetails(env.Error.Details), c.redactText),
		DetailsPresent: env.Error.Details != nil,
		RawDetails:     redactRawDetails(env.Error.Details, c.redactText),
		RequestID:      meta.RequestID,
		Status:         status,
		Meta:           meta,
	}
}

func (c *BootstrapClient) decodeOAuthError(status int, body []byte, meta ResponseMeta) error {
	meta = c.redactMeta(meta)
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return &ProtocolError{Detail: "malformed OAuth error response body", Status: status, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: meta}
	}
	if payload.Error == "" {
		return &ProtocolError{Detail: "OAuth error response missing error", Status: status, RequestID: meta.RequestID, Meta: meta}
	}
	message := payload.ErrorDescription
	if message == "" {
		message = payload.Error
	}
	var details any
	if payload.ErrorDescription != "" {
		details = map[string]any{"error_description": c.redactText(payload.ErrorDescription)}
	}
	rawDetails := oauthErrorDetails(body)
	if value := redactStructuredDetails(decodeAnyDetails(rawDetails), c.redactText); value != nil {
		details = value
	}
	return &Error{
		Code:           c.redactText(payload.Error),
		Message:        c.redactText(message),
		Retryable:      payload.Error == "slow_down" || payload.Error == "authorization_pending" || payload.Error == "temporarily_unavailable",
		DetailsValue:   details,
		DetailsPresent: len(rawDetails) != 0,
		RawDetails:     redactRawDetails(rawDetails, c.redactText),
		RequestID:      meta.RequestID,
		Status:         status,
		Meta:           meta,
	}
}

func oauthErrorDetails(body []byte) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil
	}
	delete(fields, "error")
	if len(fields) == 0 {
		return nil
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return raw
}

func (c *BootstrapClient) applyEnvelopeMeta(meta *ResponseMeta, env successEnvelope, status int) *ProtocolError {
	if env.Meta == nil {
		return nil
	}
	if rawRequestID, ok := env.Meta["request_id"]; ok {
		var requestID string
		if err := json.Unmarshal(rawRequestID, &requestID); err != nil {
			return &ProtocolError{Detail: "malformed response metadata", Status: status, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: *meta}
		}
		c.applyEnvelopeRequestID(meta, requestID)
	}
	rawMeta, err := sanitizeRawMeta(cloneRawMeta(env.Meta), c.redactText)
	if err != nil {
		return &ProtocolError{Detail: "malformed response metadata", Status: status, RequestID: meta.RequestID, Err: c.redactErr(err), Meta: *meta}
	}
	meta.RawMeta = rawMeta
	return nil
}

func (c *BootstrapClient) applyEnvelopeRequestID(meta *ResponseMeta, envelopeID string) {
	envelopeID = c.redactText(envelopeID)
	meta.EnvelopeRequestID = envelopeID
	if envelopeID != "" {
		if meta.HeaderRequestID != "" && envelopeID != meta.HeaderRequestID {
			c.debugf("request-id mismatch header=%s envelope=%s", meta.HeaderRequestID, envelopeID)
		}
		meta.RequestID = envelopeID
		return
	}
	meta.RequestID = meta.HeaderRequestID
}

func validateCompatibility(data CompatibilityData, clientVersion string, meta ResponseMeta) error {
	if data.SchemaVersion != "1.0.0" {
		return &ProtocolError{Detail: "compatibility metadata schema_version is not supported", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	if data.APIMajor != 1 {
		return &ProtocolError{Detail: "compatibility metadata api_major is not supported", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	if !semverRE.MatchString(data.MinimumVersion) || !semverRE.MatchString(data.RecommendedVersion) || data.CatalogVersion == "" {
		return &ProtocolError{Detail: "compatibility metadata is incomplete", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	if compareSemver(clientVersion, data.MinimumVersion) < 0 {
		return &UsageError{Field: "client_version", Detail: "client version " + clientVersion + " is below the minimum supported Chab CLI version " + data.MinimumVersion}
	}
	for _, blocked := range data.BlockedRanges {
		if blocked.Minimum != "" && !semverRE.MatchString(blocked.Minimum) {
			return &ProtocolError{Detail: "compatibility metadata blocked range is malformed", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
		}
		if blocked.Maximum != "" && !semverRE.MatchString(blocked.Maximum) {
			return &ProtocolError{Detail: "compatibility metadata blocked range is malformed", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
		}
		if versionInRange(clientVersion, blocked.Minimum, blocked.Maximum) {
			reason := blocked.Reason
			if reason == "" {
				reason = "client version is blocked by server compatibility metadata"
			}
			return &UsageError{Field: "client_version", Detail: reason}
		}
	}
	return nil
}

func versionInRange(version, minVersion, maxVersion string) bool {
	if minVersion != "" && compareSemver(version, minVersion) < 0 {
		return false
	}
	if maxVersion != "" && compareSemver(version, maxVersion) > 0 {
		return false
	}
	return true
}

func compareSemver(a, b string) int {
	parse := func(v string) [3]int {
		var out [3]int
		parts := strings.Split(v, ".")
		for i := 0; i < len(parts) && i < 3; i++ {
			fmt.Sscanf(parts[i], "%d", &out[i])
		}
		return out
	}
	av, bv := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}

type compatibilityCacheEntry struct {
	data      CompatibilityData
	etag      string
	expiresAt time.Time
}

var compatibilityCache sync.Map

func compatibilityCacheKey(apiBase, clientVersion string) string {
	return apiBase + "|" + clientVersion
}

func loadCompatibilityCache(key string, now func() time.Time, path string) (compatibilityCacheEntry, bool) {
	value, ok := compatibilityCache.Load(key)
	if ok {
		entry, ok := value.(compatibilityCacheEntry)
		if ok && !now().After(entry.expiresAt) {
			return entry, true
		}
		compatibilityCache.Delete(key)
	}
	if path == "" {
		return compatibilityCacheEntry{}, false
	}
	entry, ok := loadCompatibilityDiskEntry(path, key, now)
	if ok {
		compatibilityCache.Store(key, entry)
		return entry, true
	}
	return compatibilityCacheEntry{}, false
}

func storeCompatibilityCache(key string, entry compatibilityCacheEntry, path string) {
	compatibilityCache.Store(key, entry)
	if path != "" {
		_ = storeCompatibilityDiskEntry(path, key, entry)
	}
}

type compatibilityDiskCache struct {
	Entries map[string]compatibilityDiskEntry `json:"entries"`
}

type compatibilityDiskEntry struct {
	Data      CompatibilityData `json:"data"`
	ETag      string            `json:"etag,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

func loadCompatibilityDiskEntry(path, key string, now func() time.Time) (compatibilityCacheEntry, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return compatibilityCacheEntry{}, false
	}
	var disk compatibilityDiskCache
	if err := json.Unmarshal(raw, &disk); err != nil || disk.Entries == nil {
		return compatibilityCacheEntry{}, false
	}
	entry, ok := disk.Entries[key]
	if !ok || now().After(entry.ExpiresAt) {
		return compatibilityCacheEntry{}, false
	}
	return compatibilityCacheEntry{data: entry.Data, etag: entry.ETag, expiresAt: entry.ExpiresAt}, true
}

func storeCompatibilityDiskEntry(path, key string, entry compatibilityCacheEntry) error {
	disk := compatibilityDiskCache{Entries: map[string]compatibilityDiskEntry{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &disk)
		if disk.Entries == nil {
			disk.Entries = map[string]compatibilityDiskEntry{}
		}
	}
	disk.Entries[key] = compatibilityDiskEntry{Data: entry.data, ETag: entry.etag, ExpiresAt: entry.expiresAt}
	raw, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func (c *BootstrapClient) registerSuccessDataSecrets(raw json.RawMessage) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return
	}
	for _, name := range []string{"device_code", "access_token"} {
		value, ok := stringField(fields, name)
		if ok {
			c.RegisterSecret(value)
		}
	}
}

func stringField(fields map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := fields[name]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", false
	}
	return value, true
}

func (c *BootstrapClient) redactMeta(meta ResponseMeta) ResponseMeta {
	return redactResponseMeta(meta, c.redactText)
}

func (c *BootstrapClient) redactDetails(details map[string][]string) map[string][]string {
	if len(details) == 0 {
		return details
	}
	out := make(map[string][]string, len(details))
	for key, values := range details {
		redactedKey := c.redactText(key)
		for _, value := range values {
			out[redactedKey] = append(out[redactedKey], c.redactText(value))
		}
	}
	return out
}

func (c *BootstrapClient) redactDynamic(s string) string {
	if c == nil {
		return s
	}
	replacer := c.replacer.Load()
	if replacer == nil {
		return s
	}
	return replacer.Replace(s)
}

func (c *BootstrapClient) redactText(s string) string {
	return c.redactDynamic(redact.String(s))
}

func (c *BootstrapClient) redactErr(err error) error {
	if err == nil {
		return nil
	}
	redacted := c.redactText(err.Error())
	if redacted == err.Error() {
		return err
	}
	return &redactedError{msg: redacted, err: err}
}

func (c *BootstrapClient) debugf(format string, args ...any) {
	if c == nil || c.debugWriter == nil {
		return
	}
	c.debugMu.Lock()
	defer c.debugMu.Unlock()
	fmt.Fprintln(c.debugWriter, "debug: "+c.redactText(fmt.Sprintf(format, args...)))
}

func (c *BootstrapClient) debugMeta(meta ResponseMeta) {
	if c == nil || c.debugWriter == nil {
		return
	}
	if meta.RetryAfter.Raw != "" {
		c.debugf("response retry-after=%s", meta.RetryAfter.Raw)
	}
	if meta.RateLimit.RawLimit != "" || meta.RateLimit.RawRemaining != "" || meta.RateLimit.RawReset != "" {
		c.debugf("response rate-limit limit=%s remaining=%s reset=%s", meta.RateLimit.RawLimit, meta.RateLimit.RawRemaining, meta.RateLimit.RawReset)
	}
}
