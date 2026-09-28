package cli_test

import (
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cli"
)

func TestChabCatalogMatchesVisibleTree(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	entries := cli.Catalog()
	seen := map[string]bool{}
	for i, entry := range entries {
		if i > 0 && entries[i-1].Spec.Path >= entry.Spec.Path {
			t.Fatalf("catalog not sorted at %q then %q", entries[i-1].Spec.Path, entry.Spec.Path)
		}
		if seen[entry.Spec.Path] {
			t.Fatalf("duplicate catalog path %q", entry.Spec.Path)
		}
		seen[entry.Spec.Path] = true
		assertCatalogDocPath(t, entry)
		assertChabCatalogEntry(t, entry)
	}

	visible := map[string]*cobra.Command{}
	for _, node := range cli.VisibleCommands(root, false) {
		visible[node.Path] = node.Command
	}
	if !reflect.DeepEqual(sortedStringSet(seen), sortedStringSetFromCommands(visible)) {
		t.Fatalf("catalog paths = %v, visible paths = %v", sortedStringSet(seen), sortedStringSetFromCommands(visible))
	}
	for _, entry := range entries {
		cmd := visible[entry.Spec.Path]
		if cmd == nil {
			t.Fatalf("%s has no visible command", entry.Spec.Path)
		}
		if entry.Spec.Summary != cmd.Short {
			t.Fatalf("%s summary = %q, command short = %q", entry.Spec.Path, entry.Spec.Summary, cmd.Short)
		}
	}
}

func TestChabCatalogFamiliesAndLocalFlags(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	for _, test := range []struct {
		path     string
		groupID  string
		children []string
	}{
		{path: "api", groupID: "api", children: []string{"delete", "get", "patch", "post"}},
		{path: "auth", groupID: "authentication", children: []string{"env", "login", "logout", "status"}},
		{path: "billing", groupID: "projects", children: []string{"auto-recharge", "packages", "purchases", "reconciliation", "show"}},
		{path: "config", groupID: "configuration", children: []string{"get", "list", "path", "set"}},
		{path: "credits", groupID: "projects", children: []string{"balance", "transactions"}},
		{path: "drive", groupID: "connected-data", children: []string{"connections", "folders", "items", "permissions", "search"}},
		{path: "files", groupID: "connected-data", children: []string{"delete", "download", "list", "resume", "show", "upload", "wait"}},
		{path: "mail", groupID: "connected-data", children: []string{"attachments", "connections", "drafts", "folders", "messages", "personas", "search", "threads"}},
		{path: "project", groupID: "projects", children: []string{"archive", "create", "delete", "list", "pause", "resume", "show", "update"}},
		{path: "operations", groupID: "operations", children: []string{"actions", "artifact", "bulk-cancel", "cancel", "estimate", "list", "result", "resume", "schema", "show", "start", "wait"}},
		{path: "search", groupID: "operations", children: []string{"serp", "web"}},
		{path: "seo", groupID: "operations", children: []string{"domains", "keywords"}},
		{path: "llm", groupID: "operations", children: []string{"embeddings", "generate", "models"}},
		{path: "mcp", groupID: "utilities", children: []string{"serve"}},
		{path: "profile", groupID: "configuration", children: []string{"create", "delete", "list", "show", "use"}},
		{path: "tokens", groupID: "projects", children: []string{"approvals", "create", "list", "revoke", "show", "update"}},
		{path: "usage", groupID: "projects", children: nil},
		{path: "webhooks", groupID: "projects", children: []string{"deliveries", "endpoints", "replays"}},
	} {
		t.Run(test.path, func(t *testing.T) {
			cmd, remaining, err := root.Find(strings.Fields(test.path))
			if err != nil || len(remaining) != 0 || cmd == nil {
				t.Fatalf("Find(%s) = %v, %v, %v", test.path, cmd, remaining, err)
			}
			if cmd.GroupID != test.groupID {
				t.Fatalf("%s GroupID = %q, want %q", test.path, cmd.GroupID, test.groupID)
			}
			var children []string
			for _, child := range cmd.Commands() {
				if child.IsAvailableCommand() {
					children = append(children, child.Name())
				}
			}
			sort.Strings(children)
			if !reflect.DeepEqual(children, test.children) {
				t.Fatalf("%s children = %v, want %v", test.path, children, test.children)
			}
		})
	}

	entries := map[string]cli.Entry{}
	for _, entry := range cli.Catalog() {
		entries[entry.Spec.Path] = entry
	}
	for _, node := range cli.VisibleCommands(root, false) {
		entry := entries[node.Path]
		if entry.Sidecar.SupportsDryRun {
			if node.Command.LocalNonPersistentFlags().Lookup("dry-run") == nil {
				t.Fatalf("%s missing local --dry-run", node.Path)
			}
			if entry.Spec.Mutates && node.Command.LocalNonPersistentFlags().Lookup("idempotency-key") == nil {
				t.Fatalf("%s missing local --idempotency-key", node.Path)
			}
			continue
		}
		assertNoFlagNamed(t, node.Path, node.Command, "dry-run")
	}
}

func TestChabCatalogHelpRequirementsAreInCommandProse(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	rules := map[cli.HelpRequirement][]string{
		cli.HelpAPIKeySetup: {
			"Team API keys are created and revoked in the product web app.",
		},
		cli.HelpPagination: {
			"--all",
			"--limit",
			"--cursor",
			"--page-size",
		},
		cli.HelpDryRun:      {"--dry-run"},
		cli.HelpIdempotency: {"--idempotency-key"},
		cli.HelpMetadata:    {"--include-meta", "--json", "--jq", "--template"},
		cli.HelpRawEnvelope: {"--raw"},
		cli.HelpPreview:     {"Preview:", "preview Chab API contract"},
		cli.HelpConfirmation: {
			"confirmation",
			"--yes",
			"--no-prompt",
		},
	}

	for _, entry := range cli.Catalog() {
		cmd, _, err := root.Find(strings.Fields(entry.Spec.Path))
		if err != nil || cmd == nil {
			t.Fatalf("Find(%q) = %v, %v", entry.Spec.Path, cmd, err)
		}
		for _, requirement := range entry.Sidecar.HelpRequirements {
			substrings, ok := rules[requirement]
			if !ok {
				t.Fatalf("%s has unenforced help requirement %q", entry.Spec.Path, requirement)
			}
			for _, want := range substrings {
				if !strings.Contains(cmd.Long, want) {
					t.Fatalf("%s Long missing %q for %s:\n%s", entry.Spec.Path, want, requirement, cmd.Long)
				}
			}
		}
	}
}

func TestChabCatalogOutputModeInvariants(t *testing.T) {
	var metadataPaths []string
	for _, entry := range cli.Catalog() {
		modes := modeSet(entry)
		if (modes["jq"] || modes["template"]) && !modes["json"] {
			t.Fatalf("%s claims jq/template without json: %v", entry.Spec.Path, entry.Spec.OutputModes)
		}
		if modes["plain"] && !modes["human"] {
			t.Fatalf("%s claims plain without human: %v", entry.Spec.Path, entry.Spec.OutputModes)
		}
		hasRequirement := containsRequirement(entry.Sidecar.HelpRequirements, cli.HelpMetadata)
		if entry.Spec.SupportsMeta != hasRequirement {
			t.Fatalf("%s metadata support = %t, help requirement = %t", entry.Spec.Path, entry.Spec.SupportsMeta, hasRequirement)
		}
		if entry.Spec.SupportsMeta {
			metadataPaths = append(metadataPaths, entry.Spec.Path)
		}
	}
	wantMetadataPaths := []string{
		"api delete",
		"api get",
		"api patch",
		"api post",
		"billing auto-recharge show",
		"billing auto-recharge update",
		"billing packages",
		"billing purchases create",
		"billing purchases show",
		"billing purchases wait",
		"billing reconciliation",
		"billing show",
		"credits balance",
		"credits transactions",
		"drive connections",
		"drive items download",
		"drive items list",
		"drive items show",
		"drive search",
		"files download",
		"files list",
		"files show",
		"files wait",
		"mail attachments download",
		"mail connections",
		"mail drafts show",
		"mail folders",
		"mail messages body",
		"mail personas",
		"mail search",
		"mail threads list",
		"mail threads show",
		"operations artifact",
		"operations artifact download",
		"operations list",
		"operations result",
		"operations show",
		"operations wait",
		"project archive",
		"project create",
		"project delete",
		"project list",
		"project pause",
		"project resume",
		"project show",
		"project update",
		"tokens approvals create",
		"tokens approvals wait",
		"tokens create",
		"tokens list",
		"tokens revoke",
		"tokens show",
		"tokens update",
		"usage",
		"webhooks deliveries list",
		"webhooks deliveries replay",
		"webhooks deliveries show",
		"webhooks endpoints create",
		"webhooks endpoints delete",
		"webhooks endpoints list",
		"webhooks endpoints rotate-secret",
		"webhooks endpoints show",
		"webhooks endpoints update",
		"webhooks replays create",
	}
	if !reflect.DeepEqual(metadataPaths, wantMetadataPaths) {
		t.Fatalf("metadata paths = %v, want %v", metadataPaths, wantMetadataPaths)
	}
}

func TestChabMCPAuthNotesKeepStartupUnauthenticated(t *testing.T) {
	entries := map[string]cli.Entry{}
	for _, entry := range cli.Catalog() {
		entries[entry.Spec.Path] = entry
		if !strings.HasPrefix(entry.Spec.Path, "mcp") && entry.Sidecar.AuthNote != "" {
			t.Fatalf("%s has unexpected auth note %q", entry.Spec.Path, entry.Sidecar.AuthNote)
		}
	}
	for _, path := range []string{"mcp", "mcp serve"} {
		entry := entries[path]
		if entry.Spec.RequiresAuth {
			t.Fatalf("%s should keep startup RequiresAuth=false: %#v", path, entry)
		}
		for _, want := range []string{
			"Startup, discovery, `tools/list`, `chab_auth_env`, `chab_health`, and `chab_errors` do not require credentials.",
			"`chab_auth_me`",
			"`chab_credits_get`",
			"`chab_credits_transactions_list`",
			"`chab_search_web`",
			"`chab_action_resume`",
			"Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.",
		} {
			if !strings.Contains(entry.Sidecar.AuthNote, want) {
				t.Fatalf("%s auth note missing %q:\n%s", path, want, entry.Sidecar.AuthNote)
			}
		}
	}
}

func TestChabCatalogReturnsCopies(t *testing.T) {
	first := cli.Catalog()
	first[0].Spec.Path = "mutated"
	first[0].Spec.OutputModes = append(first[0].Spec.OutputModes, "mutated")
	first[0].Spec.Examples = append(first[0].Spec.Examples, "mutated")
	first[0].Spec.Related = append(first[0].Spec.Related, "mutated")
	first[0].Spec.Extensions = map[string]map[string]any{"mutated": {"value": true}}
	first[0].Sidecar.DocOnlyOutputModes = append(first[0].Sidecar.DocOnlyOutputModes, cli.OutputPlain)
	first[0].Sidecar.HelpRequirements = append(first[0].Sidecar.HelpRequirements, cli.HelpDryRun)

	second := cli.Catalog()
	if second[0].Spec.Path == "mutated" ||
		contains(second[0].Spec.OutputModes, "mutated") ||
		contains(second[0].Spec.Examples, "mutated") ||
		contains(second[0].Spec.Related, "mutated") ||
		second[0].Spec.Extensions != nil ||
		containsOutput(second[0].Sidecar.DocOnlyOutputModes, cli.OutputPlain) ||
		containsRequirement(second[0].Sidecar.HelpRequirements, cli.HelpDryRun) {
		t.Fatalf("catalog data was mutated across calls: %#v", second[0])
	}
}

func assertChabCatalogEntry(t *testing.T, entry cli.Entry) {
	t.Helper()
	fiveMode := []string{"human", "json", "plain", "jq", "template"}
	if isOperationCatalogPath(entry.Spec.Path) {
		assertOperationCatalogEntry(t, entry)
	} else if isManagementCatalogPath(entry.Spec.Path) {
		assertManagementCatalogEntry(t, entry)
	} else if isConnectedCatalogPath(entry.Spec.Path) {
		assertConnectedCatalogEntry(t, entry)
	} else {
		switch entry.Spec.Path {
		case "api", "auth", "config", "credits", "mcp", "profile":
			assertOutputs(t, entry, nil, nil)
			assertStatus(t, entry, cli.StatusFunctional)
		case "completion":
			assertOutputs(t, entry, nil, []cli.OutputMode{cli.OutputShellScript})
			assertStatus(t, entry, cli.StatusFunctional)
			if !entry.Sidecar.FrameworkOwned {
				t.Fatalf("completion should be framework-owned: %#v", entry.Sidecar)
			}
		case "help":
			assertOutputs(t, entry, nil, []cli.OutputMode{cli.OutputHelp})
			assertStatus(t, entry, cli.StatusFunctional)
			if !entry.Sidecar.FrameworkOwned {
				t.Fatalf("help should be framework-owned: %#v", entry.Sidecar)
			}
		case "mcp serve":
			assertOutputs(t, entry, nil, []cli.OutputMode{cli.OutputMCPStdio})
			assertStatus(t, entry, cli.StatusFunctional)
		case "auth login", "auth logout", "login", "logout", "profile create", "profile delete", "profile use", "config set", "setup":
			assertOutputs(t, entry, []string{"human", "plain"}, nil)
			if entry.Spec.Path == "login" || entry.Spec.Path == "logout" {
				assertStatus(t, entry, cli.StatusAlias)
			} else {
				assertStatus(t, entry, cli.StatusFunctional)
			}
		case "api delete", "api get", "api patch", "api post", "auth env", "auth status", "config get", "config list", "config path", "credits balance", "credits transactions", "doctor", "errors", "health", "profile list", "profile show", "version", "whoami":
			assertOutputs(t, entry, fiveMode, nil)
			assertStatus(t, entry, cli.StatusFunctional)
		default:
			t.Fatalf("unexpected catalog path %q", entry.Spec.Path)
		}
	}

	if entry.Sidecar.Status == cli.StatusReserved {
		t.Fatalf("%s remains reserved", entry.Spec.Path)
	}
	if entry.Spec.Path == "api get" || entry.Spec.Path == "credits transactions" {
		if !containsRequirement(entry.Sidecar.HelpRequirements, cli.HelpPagination) {
			t.Fatalf("%s missing pagination help requirement", entry.Spec.Path)
		}
	}
	if strings.HasPrefix(entry.Spec.Path, "api ") || strings.HasPrefix(entry.Spec.Path, "credits ") {
		if !entry.Spec.SupportsMeta || !entry.Spec.RequiresAuth {
			t.Fatalf("%s should require auth and support metadata: %#v", entry.Spec.Path, entry)
		}
	}
	if entry.Spec.Path == "health" || entry.Spec.Path == "errors" {
		if entry.Spec.RequiresAuth || entry.Spec.SupportsMeta {
			t.Fatalf("%s should be public without metadata opt-in: %#v", entry.Spec.Path, entry)
		}
	}
}

func assertManagementCatalogEntry(t *testing.T, entry cli.Entry) {
	t.Helper()
	fiveMode := []string{"human", "json", "plain", "jq", "template"}
	path := entry.Spec.Path
	switch {
	case contains(managementFamilyPaths(), path):
		assertOutputs(t, entry, nil, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if entry.Spec.RequiresAuth || entry.Spec.Mutates || entry.Spec.SupportsMeta {
			t.Fatalf("%s family should not claim auth, mutation, or metadata: %#v", path, entry.Spec)
		}
	case contains(managementReadPaths(), path):
		assertOutputs(t, entry, fiveMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if !entry.Spec.RequiresAuth || entry.Spec.Mutates || !entry.Spec.SupportsMeta {
			t.Fatalf("%s read command should require auth and metadata without mutation: %#v", path, entry.Spec)
		}
	case contains(managementMutationPaths(), path):
		assertOutputs(t, entry, fiveMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if !entry.Spec.RequiresAuth || !entry.Spec.Mutates || !entry.Spec.SupportsMeta || !entry.Sidecar.SupportsDryRun {
			t.Fatalf("%s mutation should require auth, mutate, support metadata, and support dry-run: %#v", path, entry)
		}
		if entry.Spec.Destructive != contains(managementDestructivePaths(), path) {
			t.Fatalf("%s destructive flag = %t", path, entry.Spec.Destructive)
		}
		if entry.Sidecar.RequiresConfirmation != contains(managementConfirmationPaths(), path) {
			t.Fatalf("%s confirmation flag = %t", path, entry.Sidecar.RequiresConfirmation)
		}
	default:
		t.Fatalf("unexpected management catalog path %q", path)
	}
}

func assertOperationCatalogEntry(t *testing.T, entry cli.Entry) {
	t.Helper()
	threeMode := []string{"human", "json", "plain"}
	switch path := entry.Spec.Path; {
	case contains(operationFamilyPaths(), path):
		assertOutputs(t, entry, nil, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if entry.Spec.RequiresAuth || entry.Spec.Mutates || entry.Spec.SupportsMeta {
			t.Fatalf("%s family should not claim auth, mutation, or metadata: %#v", path, entry.Spec)
		}
	case contains(operationLocalPaths(), path):
		assertOutputs(t, entry, threeMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if entry.Spec.RequiresAuth || entry.Spec.Mutates || entry.Spec.SupportsMeta {
			t.Fatalf("%s local command should not claim auth, mutation, or metadata: %#v", path, entry.Spec)
		}
	case contains(operationReadPaths(), path):
		assertOutputs(t, entry, threeMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if !entry.Spec.RequiresAuth || entry.Spec.Mutates || entry.Spec.SupportsMeta {
			t.Fatalf("%s read command should require auth without mutation or metadata: %#v", path, entry.Spec)
		}
	case contains(operationReadMetaPaths(), path):
		assertOutputs(t, entry, []string{"human", "json", "plain", "jq", "template"}, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if !entry.Spec.RequiresAuth || entry.Spec.Mutates || !entry.Spec.SupportsMeta {
			t.Fatalf("%s metadata read command should require auth and metadata without mutation: %#v", path, entry.Spec)
		}
	case contains(operationStartPaths(), path):
		assertOutputs(t, entry, threeMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if !entry.Spec.RequiresAuth || !entry.Spec.Mutates || entry.Spec.SupportsMeta || !entry.Sidecar.SupportsDryRun {
			t.Fatalf("%s start command should require auth, mutate, and support dry-run only: %#v", path, entry)
		}
	case contains(operationMutationPaths(), path):
		assertOutputs(t, entry, threeMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if !entry.Spec.RequiresAuth || !entry.Spec.Mutates || !entry.Spec.Destructive || entry.Spec.SupportsMeta {
			t.Fatalf("%s mutation command should require auth and be destructive without metadata: %#v", path, entry.Spec)
		}
	default:
		t.Fatalf("unexpected operation catalog path %q", path)
	}
}

func assertConnectedCatalogEntry(t *testing.T, entry cli.Entry) {
	t.Helper()
	threeMode := []string{"human", "json", "plain"}
	fiveMode := []string{"human", "json", "plain", "jq", "template"}
	path := entry.Spec.Path
	switch {
	case contains(connectedFamilyPaths(), path):
		assertOutputs(t, entry, nil, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		if entry.Spec.RequiresAuth || entry.Spec.Mutates || entry.Spec.SupportsMeta {
			t.Fatalf("%s family should not claim auth, mutation, or metadata: %#v", path, entry.Spec)
		}
	case contains(connectedReadPaths(), path):
		assertOutputs(t, entry, fiveMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		assertConnectedPreviewMetadata(t, entry)
		if !entry.Spec.RequiresAuth || entry.Spec.Mutates || !entry.Spec.SupportsMeta {
			t.Fatalf("%s read command should require auth and metadata without mutation: %#v", path, entry.Spec)
		}
	case contains(connectedMutationPaths(), path):
		assertOutputs(t, entry, threeMode, nil)
		assertStatus(t, entry, cli.StatusFunctional)
		assertConnectedPreviewMetadata(t, entry)
		if !entry.Spec.RequiresAuth || !entry.Spec.Mutates || entry.Spec.SupportsMeta {
			t.Fatalf("%s mutation should require auth and not support metadata opt-in: %#v", path, entry.Spec)
		}
		if entry.Spec.Destructive != contains(connectedDestructivePaths(), path) {
			t.Fatalf("%s destructive flag = %t", path, entry.Spec.Destructive)
		}
		if entry.Sidecar.RequiresConfirmation != contains(connectedConfirmationPaths(), path) {
			t.Fatalf("%s confirmation flag = %t", path, entry.Sidecar.RequiresConfirmation)
		}
		if entry.Sidecar.SupportsDryRun != contains(connectedDryRunPaths(), path) {
			t.Fatalf("%s dry-run flag = %t", path, entry.Sidecar.SupportsDryRun)
		}
	default:
		t.Fatalf("unexpected connected catalog path %q", path)
	}
}

func assertConnectedPreviewMetadata(t *testing.T, entry cli.Entry) {
	t.Helper()
	ext := entry.Spec.Extensions["chab/api-contract"]
	if ext == nil {
		t.Fatalf("%s missing contract extension", entry.Spec.Path)
	}
	operationID, _ := ext["operation_id"].(string)
	availability, _ := ext["availability"].(string)
	if operationID == "" || availability != "preview" {
		t.Fatalf("%s contract extension = %#v", entry.Spec.Path, ext)
	}
	op, ok := chabcontract.MustLoad().Find(operationID)
	if !ok {
		t.Fatalf("%s references unknown operation %q", entry.Spec.Path, operationID)
	}
	if op.Availability != availability {
		t.Fatalf("%s availability = %q, contract = %q", entry.Spec.Path, availability, op.Availability)
	}
	if !strings.Contains(entry.Spec.Summary, "(preview)") {
		t.Fatalf("%s summary missing preview label: %q", entry.Spec.Path, entry.Spec.Summary)
	}
	if !containsRequirement(entry.Sidecar.HelpRequirements, cli.HelpPreview) {
		t.Fatalf("%s missing preview help requirement", entry.Spec.Path)
	}
}

func isOperationCatalogPath(path string) bool {
	for _, prefix := range []string{
		"operations",
		"examples",
		"search",
		"seo",
		"business",
		"contacts",
		"scrape",
		"screenshots",
		"convert",
		"translate",
		"llm",
		"research",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+" ") {
			return true
		}
	}
	return false
}

func isConnectedCatalogPath(path string) bool {
	for _, prefix := range []string{"files", "mail", "drive"} {
		if path == prefix || strings.HasPrefix(path, prefix+" ") {
			return true
		}
	}
	return false
}

func isManagementCatalogPath(path string) bool {
	for _, prefix := range []string{"project", "usage", "billing", "tokens", "webhooks"} {
		if path == prefix || strings.HasPrefix(path, prefix+" ") {
			return true
		}
	}
	return false
}

func connectedFamilyPaths() []string {
	return []string{
		"files",
		"mail",
		"mail attachments",
		"mail drafts",
		"mail drafts attachments",
		"mail messages",
		"mail threads",
		"drive",
		"drive folders",
		"drive items",
		"drive permissions",
	}
}

func connectedReadPaths() []string {
	return []string{
		"files list",
		"files show",
		"files wait",
		"files download",
		"mail connections",
		"mail folders",
		"mail personas",
		"mail threads list",
		"mail threads show",
		"mail search",
		"mail drafts show",
		"mail messages body",
		"mail attachments download",
		"drive connections",
		"drive items list",
		"drive items show",
		"drive items download",
		"drive search",
	}
}

func connectedMutationPaths() []string {
	return []string{
		"files upload",
		"files resume",
		"files delete",
		"mail drafts create",
		"mail drafts update",
		"mail drafts discard",
		"mail drafts send",
		"mail drafts attachments add",
		"mail drafts attachments remove",
		"mail messages state",
		"drive folders create",
		"drive items upload",
		"drive items update",
		"drive items export",
		"drive items move",
		"drive items trash",
		"drive permissions update",
	}
}

func connectedDestructivePaths() []string {
	return []string{"files delete", "drive items trash"}
}

func connectedConfirmationPaths() []string {
	return []string{
		"files upload",
		"files resume",
		"files delete",
		"mail drafts send",
		"drive items update",
		"drive items move",
		"drive items trash",
		"drive permissions update",
	}
}

func connectedDryRunPaths() []string {
	return []string{
		"mail drafts create",
		"mail drafts update",
		"mail drafts discard",
		"mail drafts send",
		"mail drafts attachments add",
		"mail drafts attachments remove",
		"mail messages state",
		"drive folders create",
		"drive items upload",
		"drive items update",
		"drive items move",
		"drive items trash",
		"drive permissions update",
	}
}

func managementFamilyPaths() []string {
	return []string{
		"project",
		"billing",
		"billing auto-recharge",
		"billing purchases",
		"tokens",
		"tokens approvals",
		"webhooks",
		"webhooks endpoints",
		"webhooks deliveries",
		"webhooks replays",
	}
}

func managementReadPaths() []string {
	return []string{
		"project list",
		"project show",
		"usage",
		"billing show",
		"billing packages",
		"billing reconciliation",
		"billing auto-recharge show",
		"billing purchases show",
		"billing purchases wait",
		"tokens list",
		"tokens show",
		"tokens approvals wait",
		"webhooks endpoints list",
		"webhooks endpoints show",
		"webhooks deliveries list",
		"webhooks deliveries show",
	}
}

func managementMutationPaths() []string {
	return []string{
		"project create",
		"project update",
		"project pause",
		"project resume",
		"project archive",
		"project delete",
		"billing auto-recharge update",
		"billing purchases create",
		"tokens create",
		"tokens update",
		"tokens revoke",
		"tokens approvals create",
		"webhooks endpoints create",
		"webhooks endpoints update",
		"webhooks endpoints delete",
		"webhooks endpoints rotate-secret",
		"webhooks deliveries replay",
		"webhooks replays create",
	}
}

func managementDestructivePaths() []string {
	return []string{"project delete", "tokens revoke", "webhooks endpoints delete"}
}

func managementConfirmationPaths() []string {
	return []string{
		"project delete",
		"billing auto-recharge update",
		"billing purchases create",
		"tokens update",
		"tokens revoke",
		"webhooks endpoints delete",
		"webhooks endpoints rotate-secret",
		"webhooks deliveries replay",
		"webhooks replays create",
	}
}

func operationFamilyPaths() []string {
	return []string{
		"operations",
		"operations actions",
		"examples",
		"search",
		"seo",
		"seo keywords",
		"seo domains",
		"business",
		"contacts",
		"scrape",
		"screenshots",
		"convert",
		"translate",
		"llm",
		"research",
	}
}

func operationLocalPaths() []string {
	return []string{
		"operations schema",
		"operations actions list",
		"operations actions show",
	}
}

func operationReadPaths() []string {
	return []string{
		"operations estimate",
		"operations resume",
		"llm models",
	}
}

func operationReadMetaPaths() []string {
	return []string{
		"operations artifact",
		"operations artifact download",
		"operations list",
		"operations result",
		"operations show",
		"operations wait",
	}
}

func operationStartPaths() []string {
	return []string{
		"operations start",
		"examples echo",
		"search web",
		"search serp",
		"seo keywords ideas",
		"seo keywords metrics",
		"seo domains overview",
		"seo domains backlinks",
		"business search",
		"business details",
		"contacts domain-search",
		"contacts email-finder",
		"contacts email-verify",
		"scrape markdown",
		"scrape dom",
		"screenshots url",
		"convert file",
		"translate text-or-document",
		"llm generate",
		"llm embeddings",
		"research deep",
	}
}

func operationMutationPaths() []string {
	return []string{
		"operations cancel",
		"operations bulk-cancel",
	}
}

func assertCatalogDocPath(t *testing.T, entry cli.Entry) {
	t.Helper()
	want := "manual/commands/chab-" + strings.ReplaceAll(entry.Spec.Path, " ", "-") + ".md"
	if entry.Sidecar.DocPath != want {
		t.Fatalf("%s doc path = %q, want %q", entry.Spec.Path, entry.Sidecar.DocPath, want)
	}
	if strings.Contains(entry.Sidecar.DocPath, " ") || !strings.HasSuffix(entry.Sidecar.DocPath, ".md") {
		t.Fatalf("%s has invalid doc path %q", entry.Spec.Path, entry.Sidecar.DocPath)
	}
}

func assertStatus(t *testing.T, entry cli.Entry, want cli.CommandStatus) {
	t.Helper()
	if entry.Sidecar.Status != want {
		t.Fatalf("%s status = %q, want %q", entry.Spec.Path, entry.Sidecar.Status, want)
	}
}

func assertOutputs(t *testing.T, entry cli.Entry, wantSpec []string, wantDoc []cli.OutputMode) {
	t.Helper()
	if !reflect.DeepEqual(entry.Spec.OutputModes, wantSpec) {
		t.Fatalf("%s output modes = %v, want %v", entry.Spec.Path, entry.Spec.OutputModes, wantSpec)
	}
	if !reflect.DeepEqual(entry.Sidecar.DocOnlyOutputModes, wantDoc) {
		t.Fatalf("%s doc-only output modes = %v, want %v", entry.Spec.Path, entry.Sidecar.DocOnlyOutputModes, wantDoc)
	}
}

func assertNoFlagNamed(t *testing.T, path string, cmd *cobra.Command, name string) {
	t.Helper()
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.LocalFlags(), cmd.InheritedFlags(), cmd.PersistentFlags()} {
		if set != nil && set.Lookup(name) != nil {
			t.Fatalf("%s exposes --%s", path, name)
		}
	}
}

func modeSet(entry cli.Entry) map[string]bool {
	modes := make(map[string]bool, len(entry.Spec.OutputModes))
	for _, mode := range entry.Spec.OutputModes {
		modes[mode] = true
	}
	return modes
}

func sortedStringSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedStringSetFromCommands(values map[string]*cobra.Command) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsOutput(values []cli.OutputMode, want cli.OutputMode) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRequirement(values []cli.HelpRequirement, want cli.HelpRequirement) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
