package operations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/localfile"
)

const (
	actionDirName              = "actions"
	actionFileVersion          = 1
	actionReplayWindow         = 24 * time.Hour
	mailSendReplayWindow       = 30 * 24 * time.Hour
	mailSendReplayWindowReason = "30-day mail send reconciliation window"
)

type ActionState string

const (
	ActionPrepared  ActionState = "prepared"
	ActionAccepted  ActionState = "accepted"
	ActionUnknown   ActionState = "unknown"
	ActionDenied    ActionState = "denied"
	ActionCompleted ActionState = "completed"
)

// ActionRecord is the private recovery record. It intentionally stores no
// bearer token and no request body.
type ActionRecord struct {
	Version             int         `json:"version"`
	ID                  string      `json:"id"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	Profile             string      `json:"profile"`
	Destination         string      `json:"destination"`
	TokenPublicID       string      `json:"token_public_id"`
	PrincipalID         string      `json:"principal_id,omitempty"`
	RequiredScope       string      `json:"required_scope,omitempty"`
	OperationKey        string      `json:"operation_key"`
	Method              string      `json:"method"`
	Path                string      `json:"path"`
	EncoderVersion      string      `json:"encoder_version"`
	RequestSHA256       string      `json:"request_sha256"`
	IdempotencyKey      string      `json:"idempotency_key"`
	State               ActionState `json:"state"`
	AcceptedOperationID string      `json:"accepted_operation_id,omitempty"`
	RequestID           string      `json:"request_id,omitempty"`
	LastStatus          string      `json:"last_status,omitempty"`
	LastError           string      `json:"last_error,omitempty"`
}

// ActionProjection is the public action view rendered by commands and MCP.
type ActionProjection struct {
	ID                  string      `json:"id"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	Profile             string      `json:"profile"`
	Destination         string      `json:"destination"`
	TokenPublicID       string      `json:"token_public_id,omitempty"`
	PrincipalID         string      `json:"principal_id,omitempty"`
	RequiredScope       string      `json:"required_scope,omitempty"`
	OperationKey        string      `json:"operation_key"`
	Method              string      `json:"method"`
	Path                string      `json:"path"`
	EncoderVersion      string      `json:"encoder_version"`
	State               ActionState `json:"state"`
	AcceptedOperationID string      `json:"accepted_operation_id,omitempty"`
	RequestID           string      `json:"request_id,omitempty"`
	LastStatus          string      `json:"last_status,omitempty"`
	LastError           string      `json:"last_error,omitempty"`
}

// Store persists private action records below the Chab config root.
type Store struct {
	root string
	now  func() time.Time
}

// StoreForRuntime returns the action journal associated with a resolved runtime.
func StoreForRuntime(rt config.Runtime, now func() time.Time) Store {
	if now == nil {
		now = time.Now
	}
	return Store{root: filepath.Join(filepath.Dir(rt.ConfigPath), actionDirName), now: now}
}

func (s Store) Root() string {
	return s.root
}

// PrepareInput identifies a recoverable action before the first effectful HTTP request.
type PrepareInput struct {
	Profile        string
	Destination    string
	TokenPublicID  string
	PrincipalID    string
	RequiredScope  string
	OperationKey   string
	Method         string
	Path           string
	EncoderVersion string
	RequestBytes   []byte
	// RequestFingerprint overrides the request-byte hash for actions whose
	// replay identity is semantic rather than the literal transport body, such
	// as multipart uploads with randomized boundaries.
	RequestFingerprint string
	IdempotencyKey     string
}

type PreparedAction struct {
	Record           ActionRecord
	Existing         bool
	AlreadyAccepted  bool
	AlreadyCompleted bool
}

func (s Store) Prepare(input PrepareInput) (PreparedAction, error) {
	if err := validatePrepareInput(input); err != nil {
		return PreparedAction{}, err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return PreparedAction{}, fmt.Errorf("create action journal directory: %w", err)
	}
	_ = os.Chmod(s.root, 0o700)

	var prepared PreparedAction
	if err := s.withLock(func() error {
		var err error
		prepared, err = s.prepareLocked(input)
		return err
	}); err != nil {
		return PreparedAction{}, err
	}
	return prepared, nil
}

func (s Store) prepareLocked(input PrepareInput) (PreparedAction, error) {
	requestHash := input.RequestFingerprint
	if requestHash == "" {
		requestHash = RequestSHA256(input.RequestBytes)
	}
	actionID := actionID(input)
	path := s.path(actionID)
	now := s.now().UTC()
	encoderVersion := input.EncoderVersion
	if encoderVersion == "" {
		encoderVersion = EncoderVersion
	}
	record := ActionRecord{
		Version:        actionFileVersion,
		ID:             actionID,
		CreatedAt:      now,
		UpdatedAt:      now,
		Profile:        input.Profile,
		Destination:    input.Destination,
		TokenPublicID:  input.TokenPublicID,
		PrincipalID:    input.PrincipalID,
		RequiredScope:  input.RequiredScope,
		OperationKey:   input.OperationKey,
		Method:         strings.ToUpper(input.Method),
		Path:           input.Path,
		EncoderVersion: encoderVersion,
		RequestSHA256:  requestHash,
		IdempotencyKey: input.IdempotencyKey,
		State:          ActionPrepared,
	}
	data, err := marshalRecord(record)
	if err != nil {
		return PreparedAction{}, err
	}
	err = localfile.CreateExclusive(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	if err == nil {
		return PreparedAction{Record: record}, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return PreparedAction{}, fmt.Errorf("create action record: %w", err)
	}
	existing, err := s.Load(actionID)
	if err != nil {
		return PreparedAction{}, err
	}
	if err := compatible(existing, record); err != nil {
		return PreparedAction{}, err
	}
	return PreparedAction{
		Record:           existing,
		Existing:         true,
		AlreadyAccepted:  existing.AcceptedOperationID != "",
		AlreadyCompleted: existing.State == ActionCompleted,
	}, nil
}

func (s Store) MarkAccepted(id string, payload OperationPayload, meta api.ResponseMeta) (ActionRecord, error) {
	var record ActionRecord
	err := s.withLock(func() error {
		var err error
		record, err = s.Load(id)
		if err != nil {
			return err
		}
		record.UpdatedAt = s.now().UTC()
		record.State = ActionAccepted
		record.AcceptedOperationID = payload.ID
		record.RequestID = meta.RequestID
		record.LastStatus = payload.Status
		return s.write(record)
	})
	return record, err
}

func (s Store) MarkCompleted(id string, payload OperationPayload, meta api.ResponseMeta) (ActionRecord, error) {
	var record ActionRecord
	err := s.withLock(func() error {
		var err error
		record, err = s.Load(id)
		if err != nil {
			return err
		}
		record.UpdatedAt = s.now().UTC()
		record.State = ActionCompleted
		if payload.ID != "" {
			record.AcceptedOperationID = payload.ID
		}
		record.RequestID = meta.RequestID
		record.LastStatus = payload.Status
		return s.write(record)
	})
	return record, err
}

func (s Store) MarkUnknown(id string, err error) error {
	return s.withLock(func() error {
		record, loadErr := s.Load(id)
		if loadErr != nil {
			return loadErr
		}
		record.UpdatedAt = s.now().UTC()
		record.State = ActionUnknown
		if err != nil {
			record.LastError = err.Error()
		}
		return s.write(record)
	})
}

// MarkDenied records a definitive API rejection. Such a request has no remote
// operation to reconcile; replaying its idempotency key could start new work.
func (s Store) MarkDenied(id string, err error) error {
	return s.withLock(func() error {
		record, loadErr := s.Load(id)
		if loadErr != nil {
			return loadErr
		}
		record.UpdatedAt = s.now().UTC()
		record.State = ActionDenied
		if err != nil {
			record.LastError = err.Error()
		}
		return s.write(record)
	})
}

func (s Store) Load(id string) (ActionRecord, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return ActionRecord{}, fmt.Errorf("action id must be a local action id")
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return ActionRecord{}, fmt.Errorf("read action record: %w", err)
	}
	var record ActionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return ActionRecord{}, fmt.Errorf("decode action record: %w", err)
	}
	if record.Version != actionFileVersion {
		return ActionRecord{}, fmt.Errorf("action %s uses unsupported journal version %d", id, record.Version)
	}
	return record, nil
}

func (s Store) List() ([]ActionRecord, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read action journal: %w", err)
	}
	var records []ActionRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func (s Store) Replayable(record ActionRecord) error {
	if record.State == ActionPrepared {
		return fmt.Errorf("action %s has no persisted request outcome and cannot be resumed", record.ID)
	}
	if record.State == ActionDenied {
		return fmt.Errorf("action %s was denied by the API and cannot be resumed", record.ID)
	}
	if record.EncoderVersion != EncoderVersion && record.EncoderVersion != RawEncoderVersion {
		return fmt.Errorf("action %s uses unsupported encoder version %q", record.ID, record.EncoderVersion)
	}
	if record.State == ActionCompleted {
		return nil
	}
	if record.AcceptedOperationID != "" {
		return nil
	}
	window := replayWindow(record)
	if s.now().UTC().Sub(record.CreatedAt) > window.duration {
		return fmt.Errorf("action %s is outside the %s", record.ID, window.reason)
	}
	return nil
}

// ReplayablePrepared permits a newly journaled first submission. An existing
// prepared receipt is ambiguous: a prior request may have been denied while
// persistence failed, so it must not become a second start.
func (s Store) ReplayablePrepared(prepared PreparedAction) error {
	if !prepared.Existing {
		firstSubmission := prepared.Record
		firstSubmission.State = ActionUnknown
		return s.Replayable(firstSubmission)
	}
	return s.BeginReplay(prepared.Record)
}

// BeginReplay durably reserves an unknown action before another effectful
// request. If recording its later response fails, the prepared state blocks a
// further replay rather than silently resending a released idempotency key.
func (s Store) BeginReplay(record ActionRecord) error {
	if err := s.Replayable(record); err != nil {
		return err
	}
	if record.State != ActionUnknown || record.AcceptedOperationID != "" {
		return nil
	}
	return s.withLock(func() error {
		latest, err := s.Load(record.ID)
		if err != nil {
			return err
		}
		if err := s.Replayable(latest); err != nil {
			return err
		}
		if latest.State != ActionUnknown || latest.AcceptedOperationID != "" {
			return fmt.Errorf("action %s changed before recovery submission", record.ID)
		}
		latest.State = ActionPrepared
		latest.UpdatedAt = s.now().UTC()
		return s.write(latest)
	})
}

type replayWindowInfo struct {
	duration time.Duration
	reason   string
}

func replayWindow(record ActionRecord) replayWindowInfo {
	if record.OperationKey == "mail.messages.send" {
		return replayWindowInfo{duration: mailSendReplayWindow, reason: mailSendReplayWindowReason}
	}
	return replayWindowInfo{duration: actionReplayWindow, reason: "24-hour idempotency replay window"}
}

func (s Store) VerifyInput(record ActionRecord, requestBytes []byte) error {
	if record.RequestSHA256 != RequestSHA256(requestBytes) {
		return fmt.Errorf("action %s input does not match the original request bytes", record.ID)
	}
	return nil
}

func (s Store) write(record ActionRecord) error {
	data, err := marshalRecord(record)
	if err != nil {
		return err
	}
	return localfile.AtomicWrite(s.path(record.ID), data, 0o700, 0o600)
}

func (s Store) withLock(fn func() error) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create action journal directory: %w", err)
	}
	_ = os.Chmod(s.root, 0o700)
	lock, err := os.OpenFile(s.lockPath(), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open action journal lock: %w", err)
	}
	if err := lock.Chmod(0o600); err != nil {
		_ = lock.Close()
		return fmt.Errorf("protect action journal lock: %w", err)
	}
	if err := lockFile(lock); err != nil {
		_ = lock.Close()
		return fmt.Errorf("lock action journal: %w", err)
	}
	defer func() {
		_ = unlockFile(lock)
		_ = lock.Close()
	}()
	return fn()
}

func (s Store) path(id string) string {
	return filepath.Join(s.root, id+".json")
}

func (s Store) lockPath() string {
	return filepath.Join(filepath.Dir(s.root), "."+filepath.Base(s.root)+".lock")
}

func Projection(record ActionRecord) ActionProjection {
	return ActionProjection{
		ID:                  record.ID,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
		Profile:             record.Profile,
		Destination:         record.Destination,
		TokenPublicID:       record.TokenPublicID,
		PrincipalID:         record.PrincipalID,
		RequiredScope:       record.RequiredScope,
		OperationKey:        record.OperationKey,
		Method:              record.Method,
		Path:                record.Path,
		EncoderVersion:      record.EncoderVersion,
		State:               record.State,
		AcceptedOperationID: record.AcceptedOperationID,
		RequestID:           record.RequestID,
		LastStatus:          record.LastStatus,
		LastError:           record.LastError,
	}
}

func Projections(records []ActionRecord) []ActionProjection {
	out := make([]ActionProjection, 0, len(records))
	for _, record := range records {
		out = append(out, Projection(record))
	}
	return out
}

func validatePrepareInput(input PrepareInput) error {
	switch {
	case input.Profile == "":
		return fmt.Errorf("action profile must not be empty")
	case input.Destination == "":
		return fmt.Errorf("action destination must not be empty")
	case input.OperationKey == "":
		return fmt.Errorf("action operation key must not be empty")
	case input.Method == "":
		return fmt.Errorf("action method must not be empty")
	case input.Path == "":
		return fmt.Errorf("action path must not be empty")
	case input.IdempotencyKey == "":
		return fmt.Errorf("action idempotency key must not be empty")
	}
	return nil
}

func compatible(existing, next ActionRecord) error {
	checks := []struct {
		name string
		a    string
		b    string
	}{
		{"profile", existing.Profile, next.Profile},
		{"destination", existing.Destination, next.Destination},
		{"token public id", existing.TokenPublicID, next.TokenPublicID},
		{"principal id", existing.PrincipalID, next.PrincipalID},
		{"required scope", existing.RequiredScope, next.RequiredScope},
		{"operation key", existing.OperationKey, next.OperationKey},
		{"method", existing.Method, next.Method},
		{"path", existing.Path, next.Path},
		{"encoder version", existing.EncoderVersion, next.EncoderVersion},
		{"request bytes", existing.RequestSHA256, next.RequestSHA256},
		{"idempotency key", existing.IdempotencyKey, next.IdempotencyKey},
	}
	for _, check := range checks {
		if check.a != check.b {
			return fmt.Errorf("idempotency key belongs to an incompatible local action (%s differs)", check.name)
		}
	}
	return nil
}

func actionID(input PrepareInput) string {
	sum := sha256.Sum256([]byte(input.IdempotencyKey))
	return "act_" + hex.EncodeToString(sum[:])[:32]
}

func RequestSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func marshalRecord(record ActionRecord) ([]byte, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
