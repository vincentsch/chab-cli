package commandmeta

import (
	"testing"

	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/rungrad/manifest"
)

func TestSidecarExtensionObjectStoresStatusAsString(t *testing.T) {
	sidecar := Sidecar{
		Owner:   "raw api",
		Status:  StatusFunctional,
		DocPath: "manual/commands/chab-api-get.md",
	}
	obj := SidecarExtensionObject(sidecar)

	status, ok := obj["status"].(string)
	if !ok {
		t.Fatalf("status type = %T, want string", obj["status"])
	}
	if status != string(sidecar.Status) {
		t.Fatalf("status = %q, want %q", status, sidecar.Status)
	}
	if got := obj["owner"]; got != sidecar.Owner {
		t.Fatalf("owner = %#v, want %q", got, sidecar.Owner)
	}
	if got := obj["docs_path"]; got != sidecar.DocPath {
		t.Fatalf("docs_path = %#v, want %q", got, sidecar.DocPath)
	}
}

func TestCloneRungradSpecIsolatesExtensions(t *testing.T) {
	original := rungrad.CommandSpec{
		Path: "api get",
		Extensions: manifest.ExtensionSet{
			ExtensionNamespace: {
				"owner":     "raw api",
				"status":    "functional",
				"docs_path": "manual/commands/chab-api-get.md",
			},
		},
	}

	clone := CloneRungradSpec(original)
	clone.Extensions[ExtensionNamespace]["owner"] = "mutated"
	clone.Extensions["acme/widget"] = manifest.ExtensionObject{"id": "x"}

	if got := original.Extensions[ExtensionNamespace]["owner"]; got != "raw api" {
		t.Fatalf("original owner = %#v, want raw api", got)
	}
	if got := original.Extensions[ExtensionNamespace]["status"]; got != "functional" {
		t.Fatalf("original status = %#v, want functional", got)
	}
	if got := original.Extensions[ExtensionNamespace]["docs_path"]; got != "manual/commands/chab-api-get.md" {
		t.Fatalf("original docs_path = %#v, want manual path", got)
	}
	if len(original.Extensions) != 1 {
		t.Fatalf("original namespace count = %d, want 1: %#v", len(original.Extensions), original.Extensions)
	}
	if len(original.Extensions[ExtensionNamespace]) != 3 {
		t.Fatalf("original object field count = %d, want 3: %#v", len(original.Extensions[ExtensionNamespace]), original.Extensions[ExtensionNamespace])
	}
	if _, ok := original.Extensions["acme/widget"]; ok {
		t.Fatalf("original gained foreign namespace: %#v", original.Extensions)
	}

	entry := Entry{Spec: original}
	entryClone := CloneEntry(entry)
	entryClone.Spec.Extensions[ExtensionNamespace]["docs_path"] = "mutated"
	if got := entry.Spec.Extensions[ExtensionNamespace]["docs_path"]; got != "manual/commands/chab-api-get.md" {
		t.Fatalf("entry original docs_path = %#v, want manual path", got)
	}

	if got := CloneExtensions(nil); got != nil {
		t.Fatalf("CloneExtensions(nil) = %#v, want nil", got)
	}
}
