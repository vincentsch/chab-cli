package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestFilesUploadCreatesActionAndStreamsMultipart(t *testing.T) {
	state, key := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("stored file bytes\n"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s, want POST /v1/files", r.Method, r.URL.Path)
			}
			entries, err := os.ReadDir(actionDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("action record before file POST = %d, %v", len(entries), err)
			}
			if r.URL.RawQuery != "" {
				t.Fatalf("file upload query = %q, want empty", r.URL.RawQuery)
			}
			form := parseMultipartRequest(t, r)
			defer form.RemoveAll()
			assertFormValue(t, form, "project_id", "42")
			assertFormValue(t, form, "retention_hours", "96")
			assertFormValue(t, form, "filename", "source.txt")
			files := form.File["file"]
			if len(files) != 1 {
				t.Fatalf("multipart file count = %d, want 1", len(files))
			}
			if files[0].Filename != "source.txt" {
				t.Fatalf("multipart filename = %q, want source.txt", files[0].Filename)
			}
			part, err := files[0].Open()
			if err != nil {
				t.Fatalf("open multipart file: %v", err)
			}
			defer part.Close()
			got, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read multipart file: %v", err)
			}
			if string(got) != "stored file bytes\n" {
				t.Fatalf("multipart file bytes = %q", got)
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"file": map[string]any{
						"id":           "fil_123",
						"state":        "scan_pending",
						"download_url": "/v1/files/fil_123/download",
					},
				},
				"request_id": "req-file-upload",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	result := runConnectedData(t, state, server,
		"files", "upload", uploadPath,
		"--project-id", "42",
		"--retention-hours", "96",
		"--filename", "source.txt",
		"--idempotency-key", "idem-file-upload",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, result)
	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKey(t, 1, "idem-file-upload")
	server.AssertNoIdempotencyKeyLeak(t, 1, result.Stdout)

	var out struct {
		Action *struct {
			OperationKey string `json:"operation_key"`
			State        string `json:"state"`
		} `json:"action"`
		FileID        string `json:"file_id"`
		LocalRecovery struct {
			State       string `json:"state"`
			CanResume   bool   `json:"can_resume"`
			KnownRemote bool   `json:"known_remote"`
		} `json:"local_recovery"`
		RequestID string `json:"request_id"`
		Server    struct {
			File struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"file"`
		} `json:"server"`
	}
	mustUnmarshal(t, result.Stdout, &out)
	if out.Action == nil || out.Action.OperationKey != "files.create" || out.Action.State != "completed" {
		t.Fatalf("file upload action = %#v", out.Action)
	}
	if out.FileID != "fil_123" || out.Server.File.ID != "fil_123" || out.Server.File.State != "scan_pending" {
		t.Fatalf("file upload output = %#v", out)
	}
	if out.LocalRecovery.State != "completed" || out.LocalRecovery.CanResume || !out.LocalRecovery.KnownRemote || out.RequestID != "req-file-upload" {
		t.Fatalf("file upload recovery = %#v request=%q", out.LocalRecovery, out.RequestID)
	}
	records, err := os.ReadDir(actionDir)
	if err != nil || len(records) != 1 {
		t.Fatalf("action records after upload = %d, %v", len(records), err)
	}
	recordBytes, err := os.ReadFile(filepath.Join(actionDir, records[0].Name()))
	if err != nil {
		t.Fatalf("read action record: %v", err)
	}
	if strings.Contains(string(recordBytes), "stored file bytes") {
		t.Fatalf("action record stored upload bytes")
	}
}

func TestFilesUploadMarkedDenialCannotResume(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "denied.txt")
	if err := os.WriteFile(uploadPath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files":
			starts++
			w.Header().Set("X-Chab-Idempotency-Outcome", "released")
			w.Header().Set("Retry-After", "1")
			testutil.WriteJSON(t, w, http.StatusTooManyRequests, testutil.ErrorEnvelope("rate_limited", "denied after claim", "req-file-denied", nil))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	first := runConnectedData(t, state, server, "files", "upload", uploadPath, "--idempotency-key", "idem-file-denied", "--yes", "--json")
	if first.ExitCode == cli.ExitSuccess || starts != 1 || !strings.Contains(first.Stdout, `"state": "denied"`) || !strings.Contains(first.Stdout, `"can_resume": false`) {
		t.Fatalf("first file denial = %#v, starts=%d", first, starts)
	}
	actionID := singleActionID(t, state)
	resumed := runConnectedData(t, state, server, "files", "resume", actionID, uploadPath, "--yes", "--json")
	if resumed.ExitCode == cli.ExitSuccess || starts != 1 {
		t.Fatalf("denied file upload replayed: %#v, starts=%d", resumed, starts)
	}
}

func TestFilesUploadUnknownReplayUnmarkedDenialStaysUnknown(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "uncertain.txt")
	if err := os.WriteFile(uploadPath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files":
			starts++
			if starts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
			} else {
				testutil.WriteJSON(t, w, http.StatusForbidden, testutil.ErrorEnvelope("api_scope_missing", "scope changed", "req-replay", nil))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	first := runConnectedData(t, state, server, "files", "upload", uploadPath, "--idempotency-key", "idem-file-unknown-replay", "--yes", "--json")
	actionID := singleActionID(t, state)
	second := runConnectedData(t, state, server, "files", "resume", actionID, uploadPath, "--yes", "--json")
	if first.ExitCode == cli.ExitSuccess || second.ExitCode == cli.ExitSuccess || starts != 2 || !strings.Contains(second.Stdout, `"state": "unknown"`) || !strings.Contains(second.Stdout, `"can_resume": true`) {
		t.Fatalf("file unknown replay = first %#v, second %#v, starts=%d", first, second, starts)
	}
}

func TestFilesUploadMalformedReceiptRemainsRecoverable(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("stored file bytes\n"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s, want POST /v1/files", r.Method, r.URL.Path)
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data":       map[string]any{},
				"request_id": "req-file-upload-malformed",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	result := runConnectedData(t, state, server,
		"files", "upload", uploadPath,
		"--idempotency-key", "idem-file-upload-malformed",
		"--yes",
		"--json",
	)
	if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "missing file id or state") {
		t.Fatalf("malformed upload receipt result = %#v", result)
	}
	var out struct {
		Action *struct {
			State string `json:"state"`
		} `json:"action"`
		LocalRecovery struct {
			State     string `json:"state"`
			CanResume bool   `json:"can_resume"`
		} `json:"local_recovery"`
		Server map[string]any `json:"server"`
	}
	mustUnmarshal(t, result.Stdout, &out)
	if out.Action == nil || out.Action.State != "unknown" || out.LocalRecovery.State != "unknown" || !out.LocalRecovery.CanResume {
		t.Fatalf("malformed upload recovery = %#v", out)
	}
	if out.Server == nil {
		t.Fatalf("malformed upload output did not preserve server data: %#v", out)
	}
}

func TestFilesResumeWaitPollsAcceptedUploadWithoutReupload(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("stored file bytes\n"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	var uploadRequests int
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			uploadRequests++
			if uploadRequests != 1 {
				t.Fatalf("resume re-uploaded bytes")
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"file": map[string]any{
						"id":    "fil_scan",
						"state": "scan_pending",
					},
				},
				"request_id": "req-file-upload-scan",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/fil_scan":
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"file": map[string]any{
						"id":           "fil_scan",
						"state":        "available",
						"download_url": "/v1/files/fil_scan/download",
					},
				},
				"request_id": "req-file-ready",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	first := runConnectedData(t, state, server,
		"files", "upload", uploadPath,
		"--idempotency-key", "idem-file-upload-wait",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, first)
	actionID := singleActionID(t, state)

	second := runConnectedData(t, state, server,
		"files", "resume", actionID, uploadPath,
		"--wait",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, second)
	var out struct {
		Action *struct {
			State      string `json:"state"`
			LastStatus string `json:"last_status"`
		} `json:"action"`
		FileID string `json:"file_id"`
		Server struct {
			File struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"file"`
		} `json:"server"`
	}
	mustUnmarshal(t, second.Stdout, &out)
	if out.Action == nil || out.Action.State != "completed" || out.Action.LastStatus != "available" ||
		out.FileID != "fil_scan" || out.Server.File.ID != "fil_scan" || out.Server.File.State != "available" {
		t.Fatalf("resume wait output = %#v", out)
	}
	requests := server.Requests()
	if len(requests) != 4 {
		t.Fatalf("resume wait request count = %d, want 4", len(requests))
	}
	if requests[1].Method != http.MethodPost || requests[1].Path != "/v1/files" ||
		requests[3].Method != http.MethodGet || requests[3].Path != "/v1/files/fil_scan" {
		t.Fatalf("resume wait requests = %#v", requests)
	}
}

func TestFilesResumeReportsAcceptedReceiptWhenPersistenceFails(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("stored file bytes\n"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	var uploadRequests int
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			uploadRequests++
			switch uploadRequests {
			case 1:
				writeAPIError(w, http.StatusInternalServerError, "internal_error", "uncertain submission")
			case 2:
				corruptSingleActionRecord(t, state)
				testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
					"data": map[string]any{
						"file": map[string]any{
							"id":    "fil_persisted_elsewhere",
							"state": "scan_pending",
						},
					},
					"request_id": "req-file-resume-accepted",
				})
			default:
				t.Fatalf("unexpected upload request count %d", uploadRequests)
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	failed := runConnectedData(t, state, server,
		"files", "upload", uploadPath,
		"--idempotency-key", "idem-file-upload-persist",
		"--yes",
		"--json",
	)
	if failed.ExitCode == cli.ExitSuccess {
		t.Fatalf("initial upload unexpectedly succeeded: %#v", failed)
	}
	actionID := singleActionID(t, state)

	result := runConnectedData(t, state, server,
		"files", "resume", actionID, uploadPath,
		"--yes",
		"--json",
	)
	if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "failed to persist local action state") {
		t.Fatalf("resume persistence result = %#v", result)
	}
	var out struct {
		FileID           string `json:"file_id"`
		LocalPersistence struct {
			State  string `json:"state"`
			Detail string `json:"detail"`
		} `json:"local_persistence"`
		Server struct {
			File struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"file"`
		} `json:"server"`
	}
	mustUnmarshal(t, result.Stdout, &out)
	if out.FileID != "fil_persisted_elsewhere" || out.Server.File.ID != "fil_persisted_elsewhere" ||
		out.Server.File.State != "scan_pending" || out.LocalPersistence.State != "failed" || out.LocalPersistence.Detail == "" {
		t.Fatalf("resume persistence output = %#v", out)
	}
}

func TestFilesResumeRejectsDifferentDestinationBeforeUpload(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("stored file bytes\n"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	first := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files":
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "uncertain submission")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	failed := runConnectedData(t, state, first,
		"files", "upload", uploadPath,
		"--idempotency-key", "idem-file-upload-destination",
		"--yes",
		"--json",
	)
	if failed.ExitCode == cli.ExitSuccess {
		t.Fatalf("initial upload unexpectedly succeeded: %#v", failed)
	}
	actionID := singleActionID(t, state)

	second := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files":
			t.Fatalf("resume sent upload bytes to a different destination")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	result := runConnectedData(t, state, second,
		"files", "resume", actionID, uploadPath,
		"--yes",
		"--json",
	)
	if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "credential context does not match") {
		t.Fatalf("cross-destination resume result = %#v", result)
	}
	if second.Count() != 1 {
		t.Fatalf("cross-destination resume request count = %d, want whoami only", second.Count())
	}
}

func TestFilesUploadRejectsOversizedInputBeforeHTTP(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	uploadPath := filepath.Join(state.Dir, "too-large.txt")
	if err := os.WriteFile(uploadPath, []byte("abc"), 0o600); err != nil {
		t.Fatalf("write oversized fixture: %v", err)
	}
	server := testutil.NewAPIServer(t, testutil.FailOnContact(t))

	result := runConnectedData(t, state, server,
		"files", "upload", uploadPath,
		"--max-bytes", "2",
		"--yes",
		"--json",
	)
	if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "exceeds configured byte limit") {
		t.Fatalf("oversized upload result = %#v", result)
	}
	if server.Count() != 0 {
		t.Fatalf("oversized upload contacted API")
	}
	assertNotExists(t, filepath.Join(filepath.Dir(state.ConfigPath), "actions"))
}

func TestFilesDeleteResumesPendingReceiptByPollingFileLifecycle(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/fil_pending":
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"file": map[string]any{
						"id":    "fil_pending",
						"state": "deletion_pending",
					},
				},
				"request_id": "req-file-delete",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/fil_pending":
			writeAPIError(w, http.StatusNotFound, "not_found", "file disappeared")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	first := runConnectedData(t, state, server,
		"files", "delete", "fil_pending",
		"--idempotency-key", "idem-file-delete",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, first)
	var firstOut struct {
		Action *struct {
			State string `json:"state"`
		} `json:"action"`
		FileID        string `json:"file_id"`
		LocalRecovery struct {
			State       string `json:"state"`
			CanResume   bool   `json:"can_resume"`
			KnownRemote bool   `json:"known_remote"`
		} `json:"local_recovery"`
	}
	mustUnmarshal(t, first.Stdout, &firstOut)
	if firstOut.Action == nil || firstOut.Action.State != "accepted" || firstOut.FileID != "fil_pending" ||
		firstOut.LocalRecovery.State != "accepted" || !firstOut.LocalRecovery.CanResume || !firstOut.LocalRecovery.KnownRemote {
		t.Fatalf("pending delete output = %#v", firstOut)
	}

	second := runConnectedData(t, state, server,
		"files", "delete", "fil_pending",
		"--idempotency-key", "idem-file-delete",
		"--yes",
		"--wait",
		"--json",
	)
	assertCommandSuccess(t, second)
	var secondOut struct {
		Action *struct {
			State      string `json:"state"`
			LastStatus string `json:"last_status"`
		} `json:"action"`
		Server struct {
			File struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"file"`
		} `json:"server"`
		LocalRecovery struct {
			State     string `json:"state"`
			CanResume bool   `json:"can_resume"`
		} `json:"local_recovery"`
	}
	mustUnmarshal(t, second.Stdout, &secondOut)
	if secondOut.Action == nil || secondOut.Action.State != "completed" || secondOut.Action.LastStatus != "deleted" ||
		secondOut.Server.File.ID != "fil_pending" || secondOut.Server.File.State != "deleted" ||
		secondOut.LocalRecovery.State != "completed" || secondOut.LocalRecovery.CanResume {
		t.Fatalf("resumed delete output = %#v", secondOut)
	}
	if server.Count() != 4 {
		t.Fatalf("delete replay request count = %d, want 4", server.Count())
	}
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKey(t, 1, "idem-file-delete")
	server.AssertNoIdempotencyKey(t, 2)
	server.AssertNoIdempotencyKey(t, 3)
	requests := server.Requests()
	if requests[1].Method != http.MethodDelete || requests[3].Method != http.MethodGet {
		t.Fatalf("delete replay requests = %#v", requests)
	}
}

func TestOperationsResumePollsFileDeletionLifecycle(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/fil_pending":
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"file": map[string]any{
						"id":    "fil_pending",
						"state": "deletion_pending",
					},
				},
				"request_id": "req-file-delete-action",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/fil_pending":
			writeAPIError(w, http.StatusNotFound, "not_found", "file disappeared")
		case strings.HasPrefix(r.URL.Path, "/v1/operations/"):
			t.Fatalf("file deletion resume used operation polling path %s", r.URL.Path)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	first := runConnectedData(t, state, server,
		"files", "delete", "fil_pending",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, first)
	actionID := singleActionID(t, state)

	second := runConnectedData(t, state, server,
		"operations", "resume", actionID,
		"--json",
	)
	assertCommandSuccess(t, second)
	var out struct {
		Action *struct {
			State      string `json:"state"`
			LastStatus string `json:"last_status"`
		} `json:"action"`
		FileID string `json:"file_id"`
		Server struct {
			File struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"file"`
		} `json:"server"`
		LocalRecovery struct {
			State     string `json:"state"`
			CanResume bool   `json:"can_resume"`
		} `json:"local_recovery"`
	}
	mustUnmarshal(t, second.Stdout, &out)
	if out.Action == nil || out.Action.State != "completed" || out.Action.LastStatus != "deleted" ||
		out.FileID != "fil_pending" || out.Server.File.State != "deleted" ||
		out.LocalRecovery.State != "completed" || out.LocalRecovery.CanResume {
		t.Fatalf("operations resume file deletion output = %#v", out)
	}
	requests := server.Requests()
	if len(requests) != 4 {
		t.Fatalf("operations resume request count = %d, want 4", len(requests))
	}
	if requests[1].Method != http.MethodDelete || requests[3].Method != http.MethodGet || requests[3].Path != "/v1/files/fil_pending" {
		t.Fatalf("operations resume requests = %#v", requests)
	}
}

func TestFilesDownloadReportsUnavailableFileStates(t *testing.T) {
	for _, stateName := range []string{"scan_pending", "quarantined", "expired"} {
		t.Run(stateName, func(t *testing.T) {
			state, _ := configuredChabAuthState(t)
			requestID := "req-file-" + stateName
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/files/fil_"+stateName {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
					"data": map[string]any{
						"file": map[string]any{
							"id":           "fil_" + stateName,
							"state":        stateName,
							"download_url": nil,
						},
					},
					"request_id": requestID,
				})
			})
			out := filepath.Join(state.Dir, stateName+".bin")
			result := runConnectedData(t, state, server,
				"files", "download", "fil_"+stateName,
				"--output", out,
				"--json",
			)
			if result.ExitCode == cli.ExitSuccess ||
				!strings.Contains(result.Stderr, "not downloadable") ||
				!strings.Contains(result.Stderr, stateName) ||
				!strings.Contains(result.Stderr, requestID) ||
				strings.Contains(result.Stderr, "api_protocol_error") {
				t.Fatalf("unavailable download result = %#v", result)
			}
			assertNotExists(t, out)
			if server.Count() != 1 {
				t.Fatalf("unavailable download request count = %d, want 1", server.Count())
			}
		})
	}
}

func TestConnectedDownloadsUseExplicitOutputsAndChecksums(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	fileBytes := []byte("file download\n")
	artifactBytes := []byte("artifact download\n")
	driveBytes := []byte("drive download\n")
	mailBytes := []byte("mail attachment\n")
	fileChecksum := checksumString(fileBytes)
	artifactChecksum := checksumString(artifactBytes)
	var server *testutil.APIServer
	server = testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/fil_123":
			if r.URL.RawQuery != "" {
				t.Fatalf("file metadata query = %q, want empty", r.URL.RawQuery)
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"file": map[string]any{
						"id":           "fil_123",
						"state":        "available",
						"download_url": "/v1/files/fil_123/download",
						"checksum":     fileChecksum,
					},
				},
				"request_id": "req-file-metadata",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/fil_123/download":
			if r.URL.RawQuery != "" {
				t.Fatalf("file download query = %q, want empty", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fileBytes)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/operations/op_123/artifacts/art_123":
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"artifact": map[string]any{
						"id":           "art_123",
						"download_url": "/v1/operations/op_123/artifacts/art_123/download",
						"checksum":     artifactChecksum,
					},
				},
				"request_id": "req-artifact-metadata",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/operations/op_123/artifacts/art_123/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(artifactBytes)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/drive/items/dri_file/download":
			assertQueryValue(t, r, "project_id", "42")
			assertQueryValue(t, r, "connection_id", "7")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(driveBytes)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/mail/messages/84/attachments/att_123":
			assertQueryValue(t, r, "project_id", "42")
			assertQueryValue(t, r, "connection_id", "7")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mailBytes)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	fileOut := filepath.Join(state.Dir, "downloaded-file.txt")
	fileResult := runConnectedData(t, state, server, "files", "download", "fil_123", "--output", fileOut, "--json")
	assertCommandSuccess(t, fileResult)
	assertFileBytes(t, fileOut, fileBytes)

	artifactOut := filepath.Join(state.Dir, "downloaded-artifact.txt")
	artifactResult := runConnectedData(t, state, server, "operations", "artifact", "download", "op_123", "art_123", "--output", artifactOut, "--json")
	assertCommandSuccess(t, artifactResult)
	assertFileBytes(t, artifactOut, artifactBytes)

	driveOut := filepath.Join(state.Dir, "downloaded-drive.txt")
	driveResult := runConnectedData(t, state, server, "drive", "items", "download", "dri_file", "--project-id", "42", "--connection-id", "7", "--output", driveOut, "--json")
	assertCommandSuccess(t, driveResult)
	assertFileBytes(t, driveOut, driveBytes)

	mailOut := filepath.Join(state.Dir, "downloaded-mail.txt")
	mailResult := runConnectedData(t, state, server, "mail", "attachments", "download", "84", "att_123", "--project-id", "42", "--connection-id", "7", "--output", mailOut, "--json")
	assertCommandSuccess(t, mailResult)
	assertFileBytes(t, mailOut, mailBytes)

	if server.Count() != 6 {
		t.Fatalf("download request count = %d, want 6", server.Count())
	}
	for i := 0; i < server.Count(); i++ {
		server.AssertNoIdempotencyKey(t, i)
	}

	existing := filepath.Join(state.Dir, "existing.txt")
	if err := os.WriteFile(existing, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	overwrite := runConnectedData(t, state, server, "files", "download", "fil_123", "--output", existing, "--json")
	if overwrite.ExitCode == cli.ExitSuccess || !strings.Contains(overwrite.Stderr, "already exists") {
		t.Fatalf("overwrite result = %#v", overwrite)
	}
	assertFileBytes(t, existing, []byte("keep me"))
	if server.Count() != 7 {
		t.Fatalf("overwrite should fetch metadata only; request count = %d, want 7", server.Count())
	}
}

func TestOperationArtifactDownloadAcceptsSupportedProducerFixtures(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	fixtures := map[string]struct {
		OperationID  string
		ArtifactID   string
		OperationKey string
		Bytes        []byte
	}{
		"conversion": {
			OperationID:  "op_convert",
			ArtifactID:   "art_convert",
			OperationKey: "convert.file",
			Bytes:        []byte("converted artifact\n"),
		},
		"drive export": {
			OperationID:  "op_drive_export",
			ArtifactID:   "art_drive_export",
			OperationKey: "drive.items.export",
			Bytes:        []byte("drive export artifact\n"),
		},
		"research": {
			OperationID:  "op_research",
			ArtifactID:   "art_research",
			OperationKey: "research.deep",
			Bytes:        []byte("research artifact\n"),
		},
		"screenshot": {
			OperationID:  "op_screenshot",
			ArtifactID:   "art_screenshot",
			OperationKey: "screenshots.url",
			Bytes:        []byte("screenshot bytes\n"),
		},
		"scrape": {
			OperationID:  "op_scrape",
			ArtifactID:   "art_scrape",
			OperationKey: "scrape.markdown",
			Bytes:        []byte("scrape artifact\n"),
		},
		"backlink export": {
			OperationID:  "op_backlinks",
			ArtifactID:   "art_backlinks",
			OperationKey: "seo.domains.backlinks",
			Bytes:        []byte("backlinks export\n"),
		},
		"translation": {
			OperationID:  "op_translate",
			ArtifactID:   "art_translate",
			OperationKey: "translate.text_or_document",
			Bytes:        []byte("translated artifact\n"),
		},
	}
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		for _, fixture := range fixtures {
			metadataPath := "/v1/operations/" + fixture.OperationID + "/artifacts/" + fixture.ArtifactID
			switch {
			case r.Method == http.MethodGet && r.URL.Path == metadataPath:
				testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
					"data": map[string]any{
						"artifact": map[string]any{
							"id":            fixture.ArtifactID,
							"operation_id":  fixture.OperationID,
							"operation_key": fixture.OperationKey,
							"download_url":  metadataPath + "/download",
							"checksum":      checksumString(fixture.Bytes),
						},
					},
					"request_id": "req-artifact-" + fixture.ArtifactID,
				})
				return
			case r.Method == http.MethodGet && r.URL.Path == metadataPath+"/download":
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(fixture.Bytes)
				return
			}
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	})

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(state.Dir, strings.ReplaceAll(fixture.OperationKey, ".", "-")+".artifact")
			result := runConnectedData(t, state, server, "operations", "artifact", "download", fixture.OperationID, fixture.ArtifactID, "--output", out, "--json")
			assertCommandSuccess(t, result)
			assertFileBytes(t, out, fixture.Bytes)
		})
	}
	if got, want := server.Count(), len(fixtures)*2; got != want {
		t.Fatalf("artifact producer request count = %d, want %d", got, want)
	}
}

func TestMailThreadsShowKeepsThreadObjectAndPaginatesMessages(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/mail/threads/88" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		assertQueryValue(t, r, "project_id", "42")
		assertQueryValue(t, r, "connection_id", "7")
		switch r.URL.Query().Get("cursor") {
		case "":
			assertQueryValue(t, r, "limit", "1")
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"thread": map[string]any{"thread_id": 88, "subject": "Quarterly"},
					"messages": []any{
						map[string]any{"message_id": 84, "thread_id": 88},
					},
				},
				"meta": map[string]any{
					"request_id":  "req-mail-thread-page-1",
					"next_cursor": "next-page",
					"has_more":    true,
					"limit":       1,
				},
			})
		case "next-page":
			assertQueryValue(t, r, "limit", "1")
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"thread": map[string]any{"thread_id": 88, "subject": "Quarterly"},
					"messages": []any{
						map[string]any{"message_id": 85, "thread_id": 88},
					},
				},
				"meta": map[string]any{
					"request_id":  "req-mail-thread-page-2",
					"next_cursor": nil,
					"has_more":    false,
					"limit":       1,
				},
			})
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	})

	result := runConnectedData(t, state, server,
		"mail", "threads", "show", "88",
		"--project-id", "42",
		"--connection-id", "7",
		"--page-size", "1",
		"--all",
		"--json",
	)
	assertCommandSuccess(t, result)
	var out struct {
		Thread struct {
			ThreadID int64  `json:"thread_id"`
			Subject  string `json:"subject"`
		} `json:"thread"`
		Messages []struct {
			MessageID int64 `json:"message_id"`
			ThreadID  int64 `json:"thread_id"`
		} `json:"messages"`
	}
	mustUnmarshal(t, result.Stdout, &out)
	if out.Thread.ThreadID != 88 || out.Thread.Subject != "Quarterly" || len(out.Messages) != 2 ||
		out.Messages[0].MessageID != 84 || out.Messages[1].MessageID != 85 {
		t.Fatalf("mail thread show output = %#v", out)
	}
	if server.Count() != 2 {
		t.Fatalf("mail thread show request count = %d, want 2", server.Count())
	}
}

func TestMailSendReplayAfterPublicWindowContactsServer(t *testing.T) {
	state, key := configuredChabAuthState(t)
	now := commandNow.UTC()
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/mail/drafts/21/send":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s, want POST /v1/mail/drafts/21/send", r.Method, r.URL.Path)
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "project_id", float64(42))
			assertBodyField(t, body, "connection_id", float64(7))
			assertBodyField(t, body, "expected_draft_revision", float64(3))
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"draft_id":          21,
					"message_id":        84,
					"draft_revision":    3,
					"outbound_revision": 1,
					"status":            "queued",
					"accepted_at":       "2026-08-15T10:01:00+00:00",
				},
				"request_id": "req-mail-send-retry",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	request := mustCanonicalJSON(t, `{"project_id":42,"connection_id":7,"expected_draft_revision":3}`)
	op, ok := chabcontract.MustLoad().Find("mail.messages.send")
	if !ok {
		t.Fatal("mail.messages.send operation missing from fixture")
	}
	store := operations.StoreForRuntime(config.Runtime{Profile: "local", APIBaseURL: server.APIBaseURL(), ConfigPath: state.ConfigPath}, func() time.Time {
		return now
	})
	prepared, err := store.Prepare(operations.PrepareInput{
		Profile:        "local",
		Destination:    server.APIBaseURL(),
		TokenPublicID:  "ak_01HY0000000000000000000000",
		RequiredScope:  op.RequiredScope,
		OperationKey:   "mail.messages.send",
		Method:         http.MethodPost,
		Path:           "/v1/mail/drafts/21/send",
		RequestBytes:   request,
		IdempotencyKey: "idem-mail-send-retry",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := store.MarkUnknown(prepared.Record.ID, fmt.Errorf("transport outcome unknown")); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}

	retryAt := now.Add(25 * time.Hour)
	result := runConnectedDataWithOptions(t, state, server, chabAuthOptionsAt(retryAt),
		"mail", "drafts", "send", "21",
		"--project-id", "42",
		"--connection-id", "7",
		"--expected-draft-revision", "3",
		"--idempotency-key", "idem-mail-send-retry",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, result)
	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKey(t, 1, "idem-mail-send-retry")
}

func TestMailSendRequiresConfirmationAndDryRunSkipsIdempotency(t *testing.T) {
	state, key := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/mail/drafts/21/send":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s, want POST /v1/mail/drafts/21/send", r.Method, r.URL.Path)
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "project_id", float64(42))
			assertBodyField(t, body, "connection_id", float64(7))
			assertBodyField(t, body, "expected_draft_revision", float64(3))
			if dryRun, _ := body["dry_run"].(bool); dryRun {
				testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
					"data": map[string]any{
						"draft_id":          21,
						"message_id":        84,
						"draft_revision":    3,
						"outbound_revision": 1,
						"status":            "queued",
						"accepted_at":       nil,
					},
					"meta": map[string]any{"request_id": "req-mail-send-dry", "dry_run": true, "live_requires_idempotency": true},
				})
				return
			}
			if _, ok := body["dry_run"]; ok {
				t.Fatalf("live mail send body kept dry_run=false")
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"draft_id":          21,
					"message_id":        84,
					"draft_revision":    3,
					"outbound_revision": 1,
					"status":            "queued",
					"accepted_at":       "2026-08-15T10:01:00+00:00",
				},
				"request_id": "req-mail-send-live",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	noConfirm := runConnectedData(t, state, server,
		"--no-prompt",
		"mail", "drafts", "send", "21",
		"--project-id", "42",
		"--connection-id", "7",
		"--expected-draft-revision", "3",
		"--idempotency-key", "idem-mail-send",
		"--json",
	)
	if noConfirm.ExitCode == cli.ExitSuccess || !strings.Contains(noConfirm.Stderr, "--yes") {
		t.Fatalf("mail send without confirmation = %#v", noConfirm)
	}
	if server.Count() != 0 {
		t.Fatalf("mail send without confirmation contacted API")
	}

	dryRun := runConnectedData(t, state, server,
		"mail", "drafts", "send", "21",
		"--project-id", "42",
		"--connection-id", "7",
		"--expected-draft-revision", "3",
		"--dry-run",
		"--json",
	)
	assertCommandSuccess(t, dryRun)
	server.AssertNoIdempotencyKey(t, 0)

	live := runConnectedData(t, state, server,
		"mail", "drafts", "send", "21",
		"--project-id", "42",
		"--connection-id", "7",
		"--expected-draft-revision", "3",
		"--idempotency-key", "idem-mail-send",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, live)
	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 1)
	server.AssertIdempotencyKey(t, 2, "idem-mail-send")
	server.AssertNoIdempotencyKeyLeak(t, 2, live.Stdout)

	var out struct {
		Action *struct {
			OperationKey string `json:"operation_key"`
			State        string `json:"state"`
		} `json:"action"`
		LocalRecovery struct {
			State string `json:"state"`
		} `json:"local_recovery"`
		Server struct {
			Status string `json:"status"`
		} `json:"server"`
	}
	mustUnmarshal(t, live.Stdout, &out)
	if out.Action == nil || out.Action.OperationKey != "mail.messages.send" || out.Action.State != "completed" || out.LocalRecovery.State != "completed" || out.Server.Status != "queued" {
		t.Fatalf("mail send output = %#v", out)
	}
}

func TestDriveMoveRequiresConfirmationAndDryRunSkipsIdempotency(t *testing.T) {
	state, key := configuredChabAuthState(t)
	targetParent := "dri_01JZ7H0A1B2C3D4E5F6G7H8J9K"
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/drive/items/dri_source/move":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s, want POST /v1/drive/items/dri_source/move", r.Method, r.URL.Path)
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "project_id", float64(42))
			assertBodyField(t, body, "connection_id", float64(7))
			assertBodyField(t, body, "target_parent_id", targetParent)
			assertBodyField(t, body, "expected_revision", "rev_1")
			if dryRun, _ := body["dry_run"].(bool); dryRun {
				testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
					"data": map[string]any{
						"operation_key": "drive.items.move",
						"family":        "drive",
						"preview":       map[string]any{"changed": true},
						"live_request":  map[string]any{"requires_idempotency_key": true},
					},
					"meta": map[string]any{"request_id": "req-drive-move-dry", "dry_run": true, "live_requires_idempotency": true},
				})
				return
			}
			if _, ok := body["dry_run"]; ok {
				t.Fatalf("live drive move body kept dry_run=false")
			}
			entries, err := os.ReadDir(actionDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("action record before drive POST = %d, %v", len(entries), err)
			}
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"operation":{"id":"op_drive_move_123","status":"queued","operation_key":"drive.items.move","family":"drive","result_available":false},"meta":{"request_id":"req-drive-move-live","credits":{"estimated":0,"reserved":0,"currency":"credit"}}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	noConfirm := runConnectedData(t, state, server,
		"--no-prompt",
		"drive", "items", "move", "dri_source",
		"--project-id", "42",
		"--connection-id", "7",
		"--target-parent-id", targetParent,
		"--expected-revision", "rev_1",
		"--idempotency-key", "idem-drive-move",
		"--json",
	)
	if noConfirm.ExitCode == cli.ExitSuccess || !strings.Contains(noConfirm.Stderr, "--yes") {
		t.Fatalf("drive move without confirmation = %#v", noConfirm)
	}
	if server.Count() != 0 {
		t.Fatalf("drive move without confirmation contacted API")
	}

	dryRun := runConnectedData(t, state, server,
		"drive", "items", "move", "dri_source",
		"--project-id", "42",
		"--connection-id", "7",
		"--target-parent-id", targetParent,
		"--expected-revision", "rev_1",
		"--dry-run",
		"--json",
	)
	assertCommandSuccess(t, dryRun)
	server.AssertNoIdempotencyKey(t, 0)

	live := runConnectedData(t, state, server,
		"drive", "items", "move", "dri_source",
		"--project-id", "42",
		"--connection-id", "7",
		"--target-parent-id", targetParent,
		"--expected-revision", "rev_1",
		"--idempotency-key", "idem-drive-move",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, live)
	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 1)
	server.AssertIdempotencyKey(t, 2, "idem-drive-move")
	server.AssertNoIdempotencyKeyLeak(t, 2, live.Stdout)

	var out struct {
		Action *struct {
			OperationKey string `json:"operation_key"`
			State        string `json:"state"`
		} `json:"action"`
		OperationID   string `json:"operation_id"`
		LocalRecovery struct {
			State       string `json:"state"`
			CanResume   bool   `json:"can_resume"`
			KnownRemote bool   `json:"known_remote"`
		} `json:"local_recovery"`
	}
	mustUnmarshal(t, live.Stdout, &out)
	if out.Action == nil || out.Action.OperationKey != "drive.items.move" || out.Action.State != "accepted" || out.OperationID != "op_drive_move_123" {
		t.Fatalf("drive move output = %#v", out)
	}
	if out.LocalRecovery.State != "accepted" || !out.LocalRecovery.CanResume || !out.LocalRecovery.KnownRemote {
		t.Fatalf("drive move recovery = %#v", out.LocalRecovery)
	}
}

func runConnectedData(t *testing.T, state testutil.State, server *testutil.APIServer, args ...string) testutil.Result {
	t.Helper()
	return testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server, args...)...)
}

func runConnectedDataWithOptions(t *testing.T, state testutil.State, server *testutil.APIServer, opts testutil.Options, args ...string) testutil.Result {
	t.Helper()
	return testutil.RunCommandWith(t, opts, state.APIArgs(server, args...)...)
}

func chabAuthOptionsAt(now time.Time) testutil.Options {
	opts := chabAuthOptions()
	opts.Now = func() time.Time { return now }
	return opts
}

func mustCanonicalJSON(t *testing.T, raw string) []byte {
	t.Helper()
	data, err := operations.CanonicalizeJSON([]byte(raw))
	if err != nil {
		t.Fatalf("CanonicalizeJSON() error = %v", err)
	}
	return data
}

func parseMultipartRequest(t *testing.T, r *http.Request) *multipart.Form {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("multipart content type parse error: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		t.Fatalf("multipart boundary missing")
	}
	form, err := multipart.NewReader(r.Body, boundary).ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("multipart body parse error: %v", err)
	}
	return form
}

func assertFormValue(t *testing.T, form *multipart.Form, name, want string) {
	t.Helper()
	got := form.Value[name]
	if len(got) != 1 || got[0] != want {
		t.Fatalf("form value %q = %v, want [%q]", name, got, want)
	}
}

func assertQueryValue(t *testing.T, r *http.Request, name, want string) {
	t.Helper()
	got := r.URL.Query()[name]
	if len(got) != 1 || got[0] != want {
		t.Fatalf("query %q = %v, want [%q]", name, got, want)
	}
}

func checksumString(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s bytes = %q, want %q", path, got, want)
	}
}

func corruptSingleActionRecord(t *testing.T, state testutil.State) {
	t.Helper()
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	entries, err := os.ReadDir(actionDir)
	if err != nil {
		t.Fatalf("read action dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("action record count = %d, want 1", len(entries))
	}
	path := filepath.Join(actionDir, entries[0].Name())
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove action record: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace action record with directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(path)
	})
}
