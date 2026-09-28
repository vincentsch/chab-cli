package releasecheck

import (
	"strings"
	"testing"
)

// TestLiveSmokeRequiresOptIn proves that the live read-only script exits before
// it can invoke chab unless the CHAB_LIVE_* opt-in contract is satisfied.
func TestLiveSmokeRequiresOptIn(t *testing.T) {
	skipWindows(t)
	// No opt-in at all: exit 1 plus the shared guard message, before any request.
	res := runScript(t, "scripts/smoke-release-live.sh", cleanEnv(nil))
	if res.exitCode != 1 {
		t.Fatalf("exit %d, want 1; stderr=%s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "live smoke is opt-in") {
		t.Fatalf("stderr missing guard message: %s", res.stderr)
	}
	// Ordinary CHAB_* values must never trigger a live call.
	res = runScript(t, "scripts/smoke-release-live.sh", cleanEnv(map[string]string{
		"CHAB_API_KEY":  "unused-fake",
		"CHAB_BASE_URL": "http://localhost:9",
	}))
	if res.exitCode != 1 {
		t.Fatalf("exit %d, want 1; stderr=%s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "live smoke is opt-in") {
		t.Fatalf("stderr missing guard message with ordinary CHAB_* vars: %s", res.stderr)
	}
	// CHAB_LIVE_SMOKE=1 but missing base URL: nonzero before any request.
	res = runScript(t, "scripts/smoke-release-live.sh", cleanEnv(map[string]string{"CHAB_LIVE_SMOKE": "1"}))
	if res.exitCode == 0 {
		t.Fatalf("exit 0 with CHAB_LIVE_BASE_URL unset; stderr=%s", res.stderr)
	}
	if !strings.Contains(res.stderr, "CHAB_LIVE_BASE_URL") {
		t.Fatalf("stderr missing CHAB_LIVE_BASE_URL guidance: %s", res.stderr)
	}
}

// TestDisposableSmokeRequiresOptIn proves that the destructive smoke needs both
// opt-ins before it can create, update, or delete a project.
func TestDisposableSmokeRequiresOptIn(t *testing.T) {
	skipWindows(t)
	// Nothing set: exit 1 plus disposable guard.
	res := runScript(t, "scripts/smoke-release-disposable-project.sh", cleanEnv(nil))
	if res.exitCode != 1 {
		t.Fatalf("exit %d, want 1; stderr=%s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "disposable-project smoke is opt-in") {
		t.Fatalf("stderr missing disposable guard: %s", res.stderr)
	}
	// Disposable opt-in alone is not enough; the live smoke opt-in is also
	// required before any request can be made.
	res = runScript(t, "scripts/smoke-release-disposable-project.sh", cleanEnv(map[string]string{
		"CHAB_LIVE_DISPOSABLE_PROJECT": "1",
	}))
	if res.exitCode != 1 {
		t.Fatalf("exit %d, want 1; stderr=%s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "disposable-project smoke is opt-in") {
		t.Fatalf("stderr missing disposable guard with only disposable opt-in: %s", res.stderr)
	}
	// Live vars present but disposable opt-in missing: still guarded before any
	// request. The fake key is never used because the guard exits first.
	res = runScript(t, "scripts/smoke-release-disposable-project.sh", cleanEnv(map[string]string{
		"CHAB_LIVE_SMOKE":    "1",
		"CHAB_LIVE_BASE_URL": "http://localhost:9",
		"CHAB_LIVE_API_KEY":  "unused-fake",
	}))
	if res.exitCode != 1 {
		t.Fatalf("exit %d, want 1; stderr=%s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "disposable-project smoke is opt-in") {
		t.Fatalf("stderr missing disposable guard: %s", res.stderr)
	}
}
