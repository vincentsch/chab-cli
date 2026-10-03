//go:build windows

package operations

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/vincentsch/chab-cli/internal/auth"
	"golang.org/x/sys/windows"
)

func TestJournalAndAuthRemainPrivateUnderBroadWindowsParent(t *testing.T) {
	dir := t.TempDir()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	userSID := user.User.Sid.String()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + userSID + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GR;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}

	store := Store{root: filepath.Join(dir, "actions"), now: time.Now}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(store.root, "inherited-control")
	if err := os.WriteFile(control, []byte("plain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sids, _ := windowsFileACL(t, control); !sids["S-1-5-32-545"] {
		t.Fatalf("control did not inherit BUILTIN\\Users: %#v", sids)
	}

	prepared, err := store.Prepare(PrepareInput{
		Profile: "local", Destination: "https://example.test/v1", TokenPublicID: "ak_test",
		OperationKey: "search.web", Method: "POST", Path: "/v1/search/web",
		RequestBytes: []byte(`{"query":"example"}`), IdempotencyKey: "idem-windows-acl",
	})
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(store.root, prepared.Record.ID+".json")
	assertWindowsPrivateFile(t, recordPath, userSID)
	if err := store.MarkUnknown(prepared.Record.ID, nil); err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivateFile(t, recordPath, userSID)

	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte("broad prior file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sids, _ := windowsFileACL(t, authPath); !sids["S-1-5-32-545"] {
		t.Fatalf("auth control did not inherit BUILTIN\\Users: %#v", sids)
	}
	file := &auth.File{Version: 1, Profiles: map[string]auth.ProfileAuth{"local": {APIKey: "ak_test|synthetic-secret"}}}
	if err := auth.Write(authPath, file); err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivateFile(t, authPath, userSID)
}

func windowsFileACL(t *testing.T, path string) (map[string]bool, bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("DACL for %s: %v", path, err)
	}
	sids := make(map[string]bool)
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("unexpected ACE type %d in %s", ace.Header.AceType, path)
		}
		sids[(*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()] = true
	}
	return sids, control&windows.SE_DACL_PROTECTED != 0
}

func assertWindowsPrivateFile(t *testing.T, path, userSID string) {
	t.Helper()
	sids, protected := windowsFileACL(t, path)
	want := map[string]bool{userSID: true, "S-1-5-18": true, "S-1-5-32-544": true}
	if !protected || len(sids) != len(want) {
		t.Fatalf("file %s DACL = %#v protected=%t, want %#v", path, sids, protected, want)
	}
	for sid := range want {
		if !sids[sid] {
			t.Fatalf("file %s DACL lacks %s: %#v", path, sid, sids)
		}
	}
}
