package operations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestJournalPreparesPrivateRecordAndProjectionOmitsRecoveryMaterial(t *testing.T) {
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: func() time.Time {
		return time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	}}
	prepared, err := store.Prepare(PrepareInput{
		Profile:        "local",
		Destination:    "http://localhost/v1",
		TokenPublicID:  "ak_test",
		RequiredScope:  "api:search:read",
		OperationKey:   "search.web",
		Method:         "POST",
		Path:           "/v1/search/web",
		RequestBytes:   []byte(`{"query":"example"}`),
		IdempotencyKey: "idem-key-123",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(store.root, prepared.Record.ID+".json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("record mode = %04o, want 0600", got)
	}
	projection := Projection(prepared.Record)
	if projection.ID == "" || projection.OperationKey != "search.web" {
		t.Fatalf("projection = %#v", projection)
	}
	data, _ := os.ReadFile(filepath.Join(store.root, prepared.Record.ID+".json"))
	if string(data) == "" || !contains(string(data), "idem-key-123") {
		t.Fatalf("private record did not contain recovery key")
	}
	publicBytes := mustJSON(t, projection)
	if contains(string(publicBytes), "idem-key-123") || contains(string(publicBytes), prepared.Record.RequestSHA256) {
		t.Fatalf("projection leaked private recovery material: %s", publicBytes)
	}
}

func TestGuestPreparedReceiptCannotReplayWithoutPersistedOutcome(t *testing.T) {
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: time.Now}
	input := PrepareInput{Profile: "local", Destination: "https://example.test/v1", PrincipalID: "guest_trial:one", OperationKey: "search.web", Method: "POST", Path: "/v1/search/web", RequestBytes: []byte(`{"query":"coffee"}`), IdempotencyKey: "idem-guest-one"}
	fresh, err := store.Prepare(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplayablePrepared(fresh); err != nil {
		t.Fatalf("fresh guest start: %v", err)
	}
	existing, err := store.Prepare(input)
	if err != nil {
		t.Fatal(err)
	}
	if !existing.Existing {
		t.Fatal("second prepare was not existing")
	}
	if err := store.ReplayablePrepared(existing); err == nil {
		t.Fatal("existing prepared guest receipt replayed")
	}
	if err := store.Replayable(existing.Record); err == nil {
		t.Fatal("guest resume replayed prepared receipt")
	}
	if err := store.MarkUnknown(fresh.Record.ID, nil); err != nil {
		t.Fatal(err)
	}
	unknown, err := store.Load(fresh.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Replayable(unknown); err != nil {
		t.Fatalf("persisted unknown outcome cannot reconcile: %v", err)
	}
}

func TestUnknownReplayIsDurablyReservedBeforeEffectfulRequest(t *testing.T) {
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: time.Now}
	input := PrepareInput{Profile: "local", Destination: "https://example.test/v1", PrincipalID: "guest_trial:one", OperationKey: "search.web", Method: "POST", Path: "/v1/search/web", RequestBytes: []byte(`{"query":"coffee"}`), IdempotencyKey: "idem-guest-replay"}
	fresh, err := store.Prepare(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkUnknown(fresh.Record.ID, errors.New("uncertain")); err != nil {
		t.Fatal(err)
	}
	existing, err := store.Prepare(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplayablePrepared(existing); err != nil {
		t.Fatal(err)
	}
	reserved, err := store.Load(fresh.Record.ID)
	if err != nil || reserved.State != ActionPrepared {
		t.Fatalf("reserved replay = %#v, error=%v", reserved, err)
	}
	if err := store.Replayable(reserved); err == nil {
		t.Fatal("reserved replay was reusable after a hypothetical persistence failure")
	}
	if err := store.MarkUnknown(reserved.ID, errors.New("still uncertain")); err != nil {
		t.Fatal(err)
	}
	unknown, err := store.Load(reserved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginReplay(unknown); err != nil {
		t.Fatal(err)
	}
}

func TestJournalExplicitKeyReuseKeepsCreatedAtAndRejectsChangedPayload(t *testing.T) {
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: func() time.Time { return now }}
	input := PrepareInput{
		Profile:        "local",
		Destination:    "http://localhost/v1",
		TokenPublicID:  "ak_test",
		OperationKey:   "examples.echo",
		Method:         "POST",
		Path:           "/v1/examples/echo",
		RequestBytes:   []byte(`{"data":{}}`),
		IdempotencyKey: "idem-key-123",
	}
	first, err := store.Prepare(input)
	if err != nil {
		t.Fatalf("first Prepare() error = %v", err)
	}
	now = now.Add(time.Hour)
	second, err := store.Prepare(input)
	if err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}
	if !second.Existing || !second.Record.CreatedAt.Equal(first.Record.CreatedAt) {
		t.Fatalf("second prepared = %#v, first created_at=%s", second, first.Record.CreatedAt)
	}
	input.RequestBytes = []byte(`{"data":{"changed":true}}`)
	if _, err := store.Prepare(input); err == nil {
		t.Fatalf("Prepare() with changed payload succeeded")
	}
}

func TestJournalExplicitKeyRejectsIncompatibleContext(t *testing.T) {
	base := PrepareInput{
		Profile:        "local",
		Destination:    "http://localhost/v1",
		TokenPublicID:  "ak_test",
		RequiredScope:  "api:examples:write",
		OperationKey:   "examples.echo",
		Method:         "POST",
		Path:           "/v1/examples/echo",
		RequestBytes:   []byte(`{"data":{}}`),
		IdempotencyKey: "idem-key-123",
	}
	tests := []struct {
		name   string
		mutate func(*PrepareInput)
	}{
		{name: "profile", mutate: func(input *PrepareInput) { input.Profile = "other" }},
		{name: "destination", mutate: func(input *PrepareInput) { input.Destination = "http://other.test/v1" }},
		{name: "token", mutate: func(input *PrepareInput) { input.TokenPublicID = "ak_other" }},
		{name: "scope", mutate: func(input *PrepareInput) { input.RequiredScope = "api:search:read" }},
		{name: "operation", mutate: func(input *PrepareInput) { input.OperationKey = "search.web" }},
		{name: "method", mutate: func(input *PrepareInput) { input.Method = "PATCH" }},
		{name: "path", mutate: func(input *PrepareInput) { input.Path = "/v1/search/web" }},
		{name: "encoder", mutate: func(input *PrepareInput) { input.EncoderVersion = RawEncoderVersion }},
		{name: "body", mutate: func(input *PrepareInput) { input.RequestBytes = []byte(`{"data":{"changed":true}}`) }},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := Store{root: filepath.Join(t.TempDir(), "actions"), now: func() time.Time {
				return time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
			}}
			first, err := store.Prepare(base)
			if err != nil {
				t.Fatalf("first Prepare() error = %v", err)
			}
			next := base
			tc.mutate(&next)
			second, err := store.Prepare(next)
			if err == nil {
				t.Fatalf("Prepare() with changed %s succeeded: %#v", tc.name, second)
			}
			entries, err := os.ReadDir(store.root)
			if err != nil {
				t.Fatalf("ReadDir() error = %v", err)
			}
			if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), first.Record.ID) {
				t.Fatalf("journal files = %v, want only %s", entryNames(entries), first.Record.ID)
			}
		})
	}
}

func TestJournalPrepareSerializesConcurrentSameAction(t *testing.T) {
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: func() time.Time {
		return time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	}}
	input := PrepareInput{
		Profile:        "local",
		Destination:    "http://localhost/v1",
		TokenPublicID:  "ak_test",
		OperationKey:   "examples.echo",
		Method:         "POST",
		Path:           "/v1/examples/echo",
		RequestBytes:   []byte(`{"data":{}}`),
		IdempotencyKey: "idem-key-123",
	}

	const workers = 16
	var wg sync.WaitGroup
	results := make(chan PreparedAction, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prepared, err := store.Prepare(input)
			if err != nil {
				errs <- err
				return
			}
			results <- prepared
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("Prepare() concurrent error = %v", err)
	}
	seenIDs := map[string]bool{}
	newRecords := 0
	existingRecords := 0
	for prepared := range results {
		seenIDs[prepared.Record.ID] = true
		if prepared.Existing {
			existingRecords++
		} else {
			newRecords++
		}
	}
	if len(seenIDs) != 1 || newRecords != 1 || existingRecords != workers-1 {
		t.Fatalf("concurrent Prepare() created new=%d existing=%d ids=%v", newRecords, existingRecords, seenIDs)
	}
	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	actionFiles := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			actionFiles++
		}
	}
	if actionFiles != 1 {
		t.Fatalf("action files = %d, want 1", actionFiles)
	}
}

func TestJournalReplayableRejectsExpiredUnknownAction(t *testing.T) {
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: func() time.Time { return now }}
	prepared, err := store.Prepare(PrepareInput{
		Profile:        "local",
		Destination:    "http://localhost/v1",
		TokenPublicID:  "ak_test",
		RequiredScope:  "api:examples:write",
		OperationKey:   "examples.echo",
		Method:         "POST",
		Path:           "/v1/examples/echo",
		RequestBytes:   []byte(`{"data":{}}`),
		IdempotencyKey: "idem-key-123",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := store.MarkUnknown(prepared.Record.ID, nil); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	now = now.Add(25 * time.Hour)
	record, err := store.Load(prepared.Record.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.Replayable(record); err == nil || !strings.Contains(err.Error(), "24-hour") {
		t.Fatalf("Replayable() error = %v, want 24-hour refusal", err)
	}
}

func TestJournalReplayableAllowsMailSendReconciliationWindow(t *testing.T) {
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	store := Store{root: filepath.Join(t.TempDir(), "actions"), now: func() time.Time { return now }}
	prepared, err := store.Prepare(PrepareInput{
		Profile:        "local",
		Destination:    "http://localhost/v1",
		TokenPublicID:  "ak_test",
		RequiredScope:  "api:mail:write",
		OperationKey:   "mail.messages.send",
		Method:         "POST",
		Path:           "/v1/mail/drafts/21/send",
		RequestBytes:   []byte(`{"connection_id":7,"expected_draft_revision":3,"project_id":42}`),
		IdempotencyKey: "idem-mail-send",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := store.MarkUnknown(prepared.Record.ID, nil); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	now = now.Add(25 * time.Hour)
	record, err := store.Load(prepared.Record.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.Replayable(record); err != nil {
		t.Fatalf("Replayable() after public window error = %v", err)
	}
	now = now.Add(31 * 24 * time.Hour)
	record, err = store.Load(prepared.Record.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.Replayable(record); err == nil || !strings.Contains(err.Error(), "30-day") {
		t.Fatalf("Replayable() error = %v, want 30-day refusal", err)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
