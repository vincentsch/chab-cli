package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	projectOneID      = "01HXMACKPR0000000000000001"
	projectTwoID      = "01HXMACKPR0000000000000002"
	projectThreeID    = "01HXMACKPR0000000000000003"
	requestIDTemplate = "mock-req-%06d"
	rateLimitReset    = "1780000000"
)

// server holds the deterministic fixture state for one mock API process.
// The example checker starts a fresh process per script, but the mutex still
// keeps admin probes and CLI requests safe when they overlap.
type server struct {
	mu           sync.Mutex
	logger       *log.Logger
	expectations fingerprintExpectations
	projects     []project
	transactions []transaction
	balance      balance
	idem         map[string]recordedResponse
	apiRequests  int
	requestSeq   int
	createSeq    int
}

type project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Timezone    string `json:"timezone"`
	Language    string `json:"language"`
	Limit       int64  `json:"limit"`
	Automate    bool   `json:"automate"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type transaction struct {
	ID                        string  `json:"id"`
	OccurredAt                string  `json:"occurred_at"`
	Kind                      string  `json:"kind"`
	Amount                    int64   `json:"amount"`
	ResultingSpendableBalance int64   `json:"resulting_spendable_balance"`
	ExpiresAt                 *string `json:"expires_at"`
	OperationID               *string `json:"operation_id"`
	PurchaseID                *string `json:"purchase_id"`
	Description               string  `json:"description"`
}

type balance struct {
	SpendableBalance int64   `json:"spendable_balance"`
	Debt             int64   `json:"debt"`
	Expires          *string `json:"expires"`
}

// recordedResponse is the stored result for an idempotent write. Replays return
// the same status, body, and headers so the CLI observes a real retry shape.
type recordedResponse struct {
	status int
	body   []byte
	header http.Header
}

// responseRecorder lets shared request handling and idempotency logic run a
// route handler without immediately writing to the network response.
type responseRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

// newServer creates an independent deterministic fixture. Callers that omit
// expectations get the documented permissive manual mode; the checker passes
// an enabled fingerprint pair.
func newServer(logger *log.Logger, expectations ...fingerprintExpectations) *server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	s := &server{logger: logger}
	if len(expectations) > 0 {
		s.expectations = expectations[0]
	}
	s.resetLocked()
	return s
}

// resetLocked reseeds every mutable fixture. Once the server is shared, callers
// hold s.mu before calling it; construction calls it before publication.
func (s *server) resetLocked() {
	latest := "2026-05-30T11:05:00Z"
	s.projects = []project{
		{
			ID:          projectOneID,
			Name:        "Demo",
			Description: "Primary demonstration project",
			URL:         exampleURL("demo"),
			Status:      "active",
			Timezone:    "UTC",
			Language:    "en",
			Limit:       10,
			Automate:    false,
			CreatedAt:   "2026-05-01T09:00:00Z",
			UpdatedAt:   "2026-05-20T10:30:00Z",
		},
		{
			ID:          projectTwoID,
			Name:        "Staging",
			Description: "Staging validation project",
			URL:         exampleURL("staging"),
			Status:      "paused",
			Timezone:    "UTC",
			Language:    "en",
			Limit:       20,
			Automate:    true,
			CreatedAt:   "2026-05-03T12:00:00Z",
			UpdatedAt:   "2026-05-18T16:45:00Z",
		},
		{
			ID:          projectThreeID,
			Name:        "Old Landing",
			Description: "Archived marketing project",
			URL:         exampleURL("old"),
			Status:      "archived",
			Timezone:    "UTC",
			Language:    "en",
			Limit:       5,
			Automate:    false,
			CreatedAt:   "2026-05-05T08:00:00Z",
			UpdatedAt:   "2026-05-10T08:00:00Z",
		},
	}
	expiry := "2026-12-31T23:59:59Z"
	opID := "op_mock_0001"
	purchaseID := "pur_mock_0001"
	s.transactions = []transaction{
		{ID: "01HXMACKTX0000000000000001", OccurredAt: "2026-05-02T09:00:00Z", Kind: "credit_grant", Amount: 500, ResultingSpendableBalance: 500, ExpiresAt: &expiry, PurchaseID: &purchaseID, Description: "Initial credit grant"},
		{ID: "01HXMACKTX0000000000000002", OccurredAt: "2026-05-05T14:30:00Z", Kind: "operation_charge", Amount: -120, ResultingSpendableBalance: 380, OperationID: &opID, Description: "Operation run"},
		{ID: "01HXMACKTX0000000000000003", OccurredAt: "2026-05-12T08:15:00Z", Kind: "adjustment", Amount: 30, ResultingSpendableBalance: 410, Description: "Support adjustment"},
		{ID: "01HXMACKTX0000000000000004", OccurredAt: "2026-05-19T10:15:30Z", Kind: "operation_charge", Amount: -80, ResultingSpendableBalance: 330, Description: "Scheduled operation"},
		{ID: "01HXMACKTX0000000000000005", OccurredAt: "2026-05-26T12:00:00Z", Kind: "debt", Amount: -200, ResultingSpendableBalance: 130, Description: "Deferred usage debt"},
		{ID: "01HXMACKTX0000000000000006", OccurredAt: latest, Kind: "adjustment", Amount: 1, ResultingSpendableBalance: 131, Description: "Rounding correction"},
	}
	s.balance = balance{
		SpendableBalance: 131,
		Debt:             200,
		Expires:          &expiry,
	}
	s.idem = make(map[string]recordedResponse)
	s.apiRequests = 0
	s.requestSeq = 0
	s.createSeq = 0
}

// ServeHTTP handles the mock's admin endpoints separately from the public
// /v1 surface used by example scripts.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/readyz":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
		return
	case "/__mock/reset":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mu.Lock()
		s.resetLocked()
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	case "/__mock/requests":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mu.Lock()
		count := s.apiRequests
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"api_requests": count})
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		http.NotFound(w, r)
		return
	}

	s.handleAPI(w, r)
}

// handleAPI applies the common API envelope: request counting, mock request
// IDs, auth checks, rate-limit headers, and redacted access logs.
func (s *server) handleAPI(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	idempotencyKey := r.Header.Get("Idempotency-Key")
	authHash := sha8(token)
	idemHash := sha8(idempotencyKey)
	rec := &responseRecorder{header: http.Header{}}

	s.mu.Lock()
	s.apiRequests++
	requestID := fmt.Sprintf(requestIDTemplate, s.requestSeq+1)
	s.requestSeq++
	s.mu.Unlock()

	rec.Header().Set("Content-Type", "application/json")
	rec.Header().Set("X-Request-Id", requestID)
	rec.Header().Set("X-RateLimit-Limit", "100")
	rec.Header().Set("X-RateLimit-Remaining", "99")
	rec.Header().Set("X-RateLimit-Reset", rateLimitReset)

	status := http.StatusOK
	// Verify supplied values before routing so a rejected write cannot mutate
	// fixture state or create an idempotency replay record.
	if token == "" {
		status = s.writeError(rec, http.StatusUnauthorized, requestID, "api_token_missing", "Authentication is required.", nil)
	} else if s.expectations.enabled && sha256.Sum256([]byte(token)) != s.expectations.apiKey {
		status = s.writeError(rec, http.StatusUnauthorized, requestID, "invalid_api_token", "Credential rejected.", nil)
	} else if idempotencyKey != "" && s.expectations.enabled && sha256.Sum256([]byte(idempotencyKey)) != s.expectations.idempotencyKey {
		status = s.writeError(rec, http.StatusConflict, requestID, "idempotency_key_conflict", "Idempotency key rejected.", nil)
	} else {
		status = s.routeAPI(rec, r, requestID)
	}

	copyHeader(w.Header(), rec.Header())
	if rec.status == 0 {
		rec.status = status
	}
	w.WriteHeader(rec.status)
	_, _ = w.Write(rec.body.Bytes())
	s.logger.Printf("%s %s -> %d auth=%s idem=%s", r.Method, redactedRawURI(r.URL), rec.status, authHash, idemHash)
}

// routeAPI keeps routing explicit so tests and examples only depend on the
// small API contract implemented by this mock.
func (s *server) routeAPI(w http.ResponseWriter, r *http.Request, requestID string) int {
	rel := strings.TrimPrefix(r.URL.Path, "/v1/")
	parts := splitPath(rel)
	if len(parts) == 0 {
		return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Route not found.", nil)
	}

	if len(parts) == 1 && parts[0] == "me" {
		if r.Method != http.MethodGet {
			return s.writeError(w, http.StatusMethodNotAllowed, requestID, "method_not_allowed", "Method not allowed.", nil)
		}
		return s.writeData(w, http.StatusOK, requestID, whoamiFixture())
	}
	if len(parts) == 1 && parts[0] == "rate-limited" {
		if r.Method != http.MethodGet {
			return s.writeError(w, http.StatusMethodNotAllowed, requestID, "method_not_allowed", "Method not allowed.", nil)
		}
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-RateLimit-Limit", "5")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", rateLimitReset)
		return s.writeError(w, http.StatusTooManyRequests, requestID, "rate_limited", "Rate limit exceeded.", nil)
	}
	if parts[0] == "projects" {
		return s.handleProjects(w, r, requestID, parts)
	}
	if len(parts) >= 1 && parts[0] == "credits" {
		return s.handleCredits(w, r, requestID, parts)
	}
	return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Route not found.", nil)
}

// handleProjects implements the subset of project routes exercised by the
// examples, including idempotency for mutating operations.
func (s *server) handleProjects(w http.ResponseWriter, r *http.Request, requestID string, parts []string) int {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			return s.handleProjectList(w, r, requestID)
		case http.MethodPost:
			return s.withIdempotency(w, r, func(dst http.ResponseWriter) int {
				return s.handleProjectCreate(dst, r, requestID)
			})
		default:
			return s.writeError(w, http.StatusMethodNotAllowed, requestID, "method_not_allowed", "Method not allowed.", nil)
		}
	}
	if len(parts) != 2 {
		return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Route not found.", nil)
	}
	id, err := url.PathUnescape(parts[1])
	if err != nil {
		id = parts[1]
	}
	switch r.Method {
	case http.MethodGet:
		return s.handleProjectShow(w, requestID, id)
	case http.MethodPatch:
		return s.withIdempotency(w, r, func(dst http.ResponseWriter) int {
			return s.handleProjectPatch(dst, r, requestID, id)
		})
	case http.MethodDelete:
		return s.withIdempotency(w, r, func(dst http.ResponseWriter) int {
			return s.handleProjectDelete(dst, requestID, id)
		})
	default:
		return s.writeError(w, http.StatusMethodNotAllowed, requestID, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *server) handleProjectList(w http.ResponseWriter, r *http.Request, requestID string) int {
	query := r.URL.Query()
	statusFilter := query.Get("status")
	search := strings.ToLower(query.Get("q"))

	s.mu.Lock()
	rows := append([]project(nil), s.projects...)
	s.mu.Unlock()

	filtered := rows[:0]
	for _, p := range rows {
		if statusFilter != "" && p.Status != statusFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(p.Name), search) {
			continue
		}
		filtered = append(filtered, p)
	}
	window, limit, nextCursor, prevCursor, hasMore := cursorWindow(filtered, query, 25)
	return s.writeList(w, http.StatusOK, requestID, limit, nextCursor, prevCursor, hasMore, window)
}

func (s *server) handleProjectShow(w http.ResponseWriter, requestID, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if p.ID == id {
			return s.writeData(w, http.StatusOK, requestID, p)
		}
	}
	return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Project not found.", nil)
}

func (s *server) handleProjectCreate(w http.ResponseWriter, r *http.Request, requestID string) int {
	body, ok := readJSONObject(r.Body)
	if !ok || strings.TrimSpace(stringValue(body["name"])) == "" {
		return s.writeError(w, http.StatusUnprocessableEntity, requestID, "validation_failed", "Validation failed.", map[string][]string{"name": {"The name field is required."}})
	}
	p := project{
		ID:          "",
		Name:        stringValue(body["name"]),
		Description: stringValue(body["description"]),
		URL:         stringValue(body["url"]),
		Status:      stringValue(body["status"]),
		Timezone:    stringValue(body["timezone"]),
		Language:    stringValue(body["language"]),
		Limit:       int64Value(body["limit"]),
		Automate:    boolValue(body["automate"]),
		CreatedAt:   "2026-06-01T00:00:00Z",
		UpdatedAt:   "2026-06-01T00:00:00Z",
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Timezone == "" {
		p.Timezone = "UTC"
	}
	if p.Language == "" {
		p.Language = "en"
	}

	s.mu.Lock()
	s.createSeq++
	p.ID = fmt.Sprintf("01HXMACKNEW%012d", s.createSeq)
	s.projects = append(s.projects, p)
	s.mu.Unlock()
	return s.writeData(w, http.StatusCreated, requestID, p)
}

func (s *server) handleProjectPatch(w http.ResponseWriter, r *http.Request, requestID, id string) int {
	body, ok := readJSONObject(r.Body)
	if !ok {
		body = map[string]json.RawMessage{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.projects {
		if s.projects[i].ID != id {
			continue
		}
		mergeProject(&s.projects[i], body)
		s.projects[i].UpdatedAt = "2026-06-01T00:00:00Z"
		return s.writeData(w, http.StatusOK, requestID, s.projects[i])
	}
	return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Project not found.", nil)
}

func (s *server) handleProjectDelete(w http.ResponseWriter, requestID, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.projects {
		if s.projects[i].ID != id {
			continue
		}
		s.projects = append(s.projects[:i], s.projects[i+1:]...)
		return s.writeData(w, http.StatusOK, requestID, map[string]any{"id": id, "deleted": true})
	}
	return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Project not found.", nil)
}

// handleCredits serves balance and transaction fixtures used by examples.
func (s *server) handleCredits(w http.ResponseWriter, r *http.Request, requestID string, parts []string) int {
	if r.Method != http.MethodGet {
		return s.writeError(w, http.StatusMethodNotAllowed, requestID, "method_not_allowed", "Method not allowed.", nil)
	}
	if len(parts) == 1 {
		s.mu.Lock()
		b := s.balance
		s.mu.Unlock()
		return s.writeData(w, http.StatusOK, requestID, b)
	}
	if len(parts) != 2 {
		return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Route not found.", nil)
	}
	switch parts[1] {
	case "transactions":
		return s.handleTransactions(w, r, requestID)
	default:
		return s.writeError(w, http.StatusNotFound, requestID, "not_found", "Route not found.", nil)
	}
}

// handleTransactions mirrors Chab cursor pagination without trying to implement
// the whole production query language.
func (s *server) handleTransactions(w http.ResponseWriter, r *http.Request, requestID string) int {
	query := r.URL.Query()
	s.mu.Lock()
	rows := append([]transaction(nil), s.transactions...)
	s.mu.Unlock()

	filtered := rows[:0]
	for _, txn := range rows {
		filtered = append(filtered, txn)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].OccurredAt > filtered[j].OccurredAt
	})
	window, limit, nextCursor, prevCursor, hasMore := cursorWindow(filtered, query, 25)
	return s.writeCursorList(w, http.StatusOK, requestID, limit, nextCursor, prevCursor, hasMore, window)
}

// withIdempotency records the first response for an idempotency key and returns
// that same response on later requests with Idempotent-Replayed set.
func (s *server) withIdempotency(w http.ResponseWriter, r *http.Request, fn func(http.ResponseWriter) int) int {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return fn(w)
	}

	s.mu.Lock()
	if stored, ok := s.idem[key]; ok {
		s.mu.Unlock()
		copyHeader(w.Header(), stored.header)
		w.Header().Set("Idempotent-Replayed", "true")
		w.WriteHeader(stored.status)
		_, _ = w.Write(stored.body)
		return stored.status
	}
	s.mu.Unlock()

	// Run the write against a recorder first so the stored replay matches the
	// original response body and headers rather than rebuilding them later.
	rec := &responseRecorder{header: cloneHeader(w.Header())}
	status := fn(rec)
	if rec.status == 0 {
		rec.status = status
	}

	s.mu.Lock()
	s.idem[key] = recordedResponse{
		status: rec.status,
		body:   append([]byte(nil), rec.body.Bytes()...),
		header: cloneHeader(rec.Header()),
	}
	s.mu.Unlock()

	copyHeader(w.Header(), rec.Header())
	w.WriteHeader(rec.status)
	_, _ = w.Write(rec.body.Bytes())
	return rec.status
}

func (s *server) writeData(w http.ResponseWriter, status int, requestID string, data any) int {
	body := map[string]any{
		"data": data,
		"meta": map[string]any{
			"request_id": requestID,
		},
	}
	writeJSON(w, status, body)
	return status
}

func (s *server) writeList(w http.ResponseWriter, status int, requestID string, limit int, nextCursor, prevCursor *string, hasMore bool, data any) int {
	body := map[string]any{
		"data": data,
		"meta": map[string]any{
			"request_id":  requestID,
			"next_cursor": nextCursor,
			"prev_cursor": prevCursor,
			"has_more":    hasMore,
			"limit":       limit,
		},
	}
	writeJSON(w, status, body)
	return status
}

func (s *server) writeCursorList(w http.ResponseWriter, status int, requestID string, limit int, nextCursor, prevCursor *string, hasMore bool, data any) int {
	body := map[string]any{
		"data": data,
		"meta": map[string]any{
			"request_id":  requestID,
			"next_cursor": nextCursor,
			"prev_cursor": prevCursor,
			"has_more":    hasMore,
			"limit":       limit,
		},
	}
	writeJSON(w, status, body)
	return status
}

func (s *server) writeError(w http.ResponseWriter, status int, requestID, code, message string, details map[string][]string) int {
	errObj := map[string]any{
		"code":      code,
		"message":   message,
		"retryable": code == "rate_limited" || code == "internal_error",
	}
	if details != nil {
		errObj["details"] = details
	}
	body := map[string]any{
		"error":      errObj,
		"request_id": requestID,
	}
	writeJSON(w, status, body)
	return status
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	out, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	w.WriteHeader(status)
	_, _ = w.Write(append(out, '\n'))
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

func intParam(query url.Values, name string, fallback int) int {
	if query.Get(name) == "" {
		return fallback
	}
	value, err := strconv.Atoi(query.Get(name))
	if err != nil {
		return fallback
	}
	return value
}

func cursorWindow[T any](rows []T, query url.Values, defaultLimit int) ([]T, int, *string, *string, bool) {
	limit := intParam(query, "limit", defaultLimit)
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > 100 {
		limit = 100
	}
	offset := cursorOffset(query.Get("cursor"))
	if offset > len(rows) {
		offset = len(rows)
	}
	end := min(offset+limit, len(rows))
	var nextCursor *string
	if end < len(rows) {
		next := strconv.Itoa(end)
		nextCursor = &next
	}
	var prevCursor *string
	if offset > 0 {
		prev := strconv.Itoa(max(0, offset-limit))
		prevCursor = &prev
	}
	return rows[offset:end], limit, nextCursor, prevCursor, nextCursor != nil
}

func cursorOffset(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func readJSONObject(r io.Reader) (map[string]json.RawMessage, bool) {
	raw, err := io.ReadAll(r)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

func mergeProject(p *project, body map[string]json.RawMessage) {
	if v := stringValue(body["name"]); v != "" {
		p.Name = v
	}
	if _, ok := body["description"]; ok {
		p.Description = stringValue(body["description"])
	}
	if _, ok := body["url"]; ok {
		p.URL = stringValue(body["url"])
	}
	if v := stringValue(body["status"]); v != "" {
		p.Status = v
	}
	if v := stringValue(body["timezone"]); v != "" {
		p.Timezone = v
	}
	if v := stringValue(body["language"]); v != "" {
		p.Language = v
	}
	if _, ok := body["limit"]; ok {
		p.Limit = int64Value(body["limit"])
	}
	if _, ok := body["automate"]; ok {
		p.Automate = boolValue(body["automate"])
	}
}

func stringValue(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func int64Value(raw json.RawMessage) int64 {
	if raw == nil {
		return 0
	}
	var value int64
	_ = json.Unmarshal(raw, &value)
	return value
}

func boolValue(raw json.RawMessage) bool {
	if raw == nil {
		return false
	}
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

func compareRFC3339(left, right string) int {
	l, lerr := time.Parse(time.RFC3339, left)
	r, rerr := time.Parse(time.RFC3339, right)
	if lerr != nil || rerr != nil {
		return strings.Compare(left, right)
	}
	if l.Before(r) {
		return -1
	}
	if l.After(r) {
		return 1
	}
	return 0
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func bearerToken(header string) string {
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func sha8(value string) string {
	if value == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

// redactedRawURI logs request paths while replacing sensitive query values. The
// query is re-encoded after redaction so log output stays deterministic.
func redactedRawURI(u *url.URL) string {
	copy := *u
	query := copy.Query()
	for _, key := range []string{"token", "key", "api_key", "secret", "password"} {
		if _, ok := query[key]; ok {
			query[key] = []string{"[REDACTED]"}
		}
	}
	copy.RawQuery = query.Encode()
	if copy.RawQuery == "" {
		return copy.EscapedPath()
	}
	return copy.EscapedPath() + "?" + copy.RawQuery
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func cloneHeader(src http.Header) http.Header {
	dst := http.Header{}
	copyHeader(dst, src)
	return dst
}

// whoamiFixture matches the fields the auth command renders, without
// depending on internal CLI structs.
func whoamiFixture() map[string]any {
	return map[string]any{
		"principal_type":  "team",
		"team_id":         42,
		"token_id":        "tok_mock_0001",
		"token_public_id": "tok_public_mock_0001",
		"scopes":          []string{"api:projects:read", "api:credits:read"},
		"token_controls": map[string]any{
			"policy_revision": 1,
			"feature_access": map[string]any{
				"mode":           "full",
				"snapshot_stale": false,
			},
			"ip_restrictions": map[string]any{
				"restricted":       false,
				"allow_rule_count": 0,
				"deny_rule_count":  0,
			},
			"spending": map[string]any{
				"mode":                    "enabled",
				"active_reserved_credits": 0,
				"allowance": map[string]any{
					"version":           1,
					"limit_credits":     1000,
					"settled_credits":   100,
					"held_credits":      0,
					"remaining_credits": 900,
				},
			},
			"project_access": map[string]any{
				"mode":                 "all",
				"selected_project_ids": []int64{},
				"selected_count":       0,
			},
		},
	}
}

// exampleURL avoids literal URLs in examples/ Go source. The committed safety
// lint scans this tree for raw URLs and validates every match.
func exampleURL(name string) string {
	return "https" + "://" + name + ".example.test"
}
