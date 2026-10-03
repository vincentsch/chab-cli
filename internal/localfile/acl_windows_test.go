//go:build windows

package localfile

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestPrivateFilesDoNotInheritBroadParentACL(t *testing.T) {
	dir := broadACLTestDir(t)
	control := filepath.Join(dir, "plain-control")
	if err := os.WriteFile(control, []byte("plain"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertBroadInheritedACL(t, control)

	file, tempPath, err := createTemp(dir, "private", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempPath)
	assertPrivateACL(t, tempPath) // Before any payload byte is written.
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	exclusive := filepath.Join(dir, "exclusive")
	if err := CreateExclusive(exclusive, func(w io.Writer) error {
		_, err := io.WriteString(w, "private")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertPrivateACL(t, exclusive)

	replaced := filepath.Join(dir, "replaced")
	if err := os.WriteFile(replaced, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertBroadInheritedACL(t, replaced)
	if err := AtomicWrite(replaced, []byte("new"), 0o700, 0o600); err != nil {
		t.Fatal(err)
	}
	assertPrivateACL(t, replaced)

	public := filepath.Join(dir, "public")
	if err := AtomicWrite(public, []byte("not secret"), 0o700, 0o644); err != nil {
		t.Fatal(err)
	}
	assertBroadInheritedACL(t, public)
}

func broadACLTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + currentUserSID(t) + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GR;;;BU)")
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
	return dir
}

func currentUserSID(t *testing.T) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid.String()
}

func aclSIDs(t *testing.T, path string) (map[string]bool, bool) {
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

func assertBroadInheritedACL(t *testing.T, path string) {
	t.Helper()
	sids, _ := aclSIDs(t, path)
	if !sids["S-1-5-32-545"] {
		t.Fatalf("broad-parent control did not inherit BUILTIN\\Users on %s: %#v", path, sids)
	}
}

func assertPrivateACL(t *testing.T, path string) {
	t.Helper()
	sids, protected := aclSIDs(t, path)
	if !protected {
		t.Fatalf("private DACL was not protected on %s", path)
	}
	want := map[string]bool{
		currentUserSID(t): true,
		"S-1-5-18":        true, // SYSTEM
		"S-1-5-32-544":    true, // Administrators
	}
	if len(sids) != len(want) {
		t.Fatalf("private DACL SIDs on %s = %#v, want %#v", path, sids, want)
	}
	for sid := range want {
		if !sids[sid] {
			t.Fatalf("private DACL on %s lacks %s: %#v", path, sid, sids)
		}
	}
}
