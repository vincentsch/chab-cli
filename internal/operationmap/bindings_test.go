package operationmap

import (
	"testing"

	"github.com/vincentsch/chab-cli/internal/chabcontract"
)

func TestBindingsCoverRegistry(t *testing.T) {
	registry, err := chabcontract.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	bindings, err := Bindings(registry)
	if err != nil {
		t.Fatalf("Bindings() error = %v", err)
	}
	if got, want := len(bindings), len(registry.Operations()); got != want {
		t.Fatalf("binding count = %d, want %d", got, want)
	}
	bindingByID := func(id string) Binding {
		t.Helper()
		for _, binding := range bindings {
			if binding.OperationID == id {
				return binding
			}
		}
		t.Fatalf("binding %s missing", id)
		return Binding{}
	}
	assertCommand := func(id, command string) {
		t.Helper()
		binding := bindingByID(id)
		if binding.CLI.Command != command || binding.CLI.State != StateImplemented {
			t.Fatalf("%s CLI binding = %#v, want implemented %q", id, binding.CLI, command)
		}
	}
	assertTool := func(id, tool string) {
		t.Helper()
		binding := bindingByID(id)
		if binding.MCP.Tool != tool || binding.MCP.State != StateImplemented {
			t.Fatalf("%s MCP binding = %#v, want implemented %q", id, binding.MCP, tool)
		}
	}
	assertExcluded := func(id string) {
		t.Helper()
		binding := bindingByID(id)
		if binding.MCP.State != StateExcluded || binding.MCP.Reason == "" {
			t.Fatalf("%s MCP binding = %#v, want excluded with reason", id, binding.MCP)
		}
	}
	assertCommand("search.web", "search web")
	assertCommand("operations.cancel", "operations cancel")
	assertCommand("operations.artifact", "operations artifact")
	assertCommand("operations.artifact_download", "operations artifact download")
	assertCommand("files.create", "files upload")
	assertCommand("mail.messages.send", "mail drafts send")
	assertCommand("drive.items.move", "drive items move")
	assertCommand("research.deep", "research deep")
	assertTool("auth.me", "chab_auth_me")
	assertTool("credits.get", "chab_credits_get")
	assertTool("search.web", "chab_search_web")
	assertTool("operations.artifact_download", "chab_operations_artifact_download")
	assertTool("files.download", "chab_files_download")
	assertTool("drive.items.download", "chab_drive_items_download")
	assertTool("mail.messages.send", "chab_mail_messages_send")
	assertExcluded("tokens.create")
	assertExcluded("tokens.management_approvals.get")
	assertExcluded("webhooks.endpoints.rotate_secret")
}
