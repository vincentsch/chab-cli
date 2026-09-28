package cli

import (
	"sort"
	"strings"

	"github.com/vincentsch/chab-cli/internal/chabcontract"
)

// CommandStatus describes whether a catalog entry is executable today.
type CommandStatus string

const (
	StatusFunctional CommandStatus = "functional"
	StatusReserved   CommandStatus = "reserved"
	StatusAlias      CommandStatus = "alias"
)

// OutputMode names successful or documentation-only output surfaces.
type OutputMode string

const (
	OutputHuman       OutputMode = "human"
	OutputJSON        OutputMode = "json"
	OutputPlain       OutputMode = "plain"
	OutputJQ          OutputMode = "jq"
	OutputTemplate    OutputMode = "template"
	OutputHelp        OutputMode = "help"
	OutputShellScript OutputMode = "shell-script"
	OutputMCPStdio    OutputMode = "mcp-stdio"
)

// HelpRequirement names extra help content promised by a command.
type HelpRequirement string

const (
	HelpAPIKeySetup  HelpRequirement = "api-key-setup"
	HelpPagination   HelpRequirement = "pagination"
	HelpDryRun       HelpRequirement = "dry-run"
	HelpConfirmation HelpRequirement = "confirmation"
	HelpIdempotency  HelpRequirement = "idempotency"
	HelpMetadata     HelpRequirement = "metadata"
	HelpRawEnvelope  HelpRequirement = "raw-envelope"
	HelpUpdateCheck  HelpRequirement = "update-check"
	HelpPreview      HelpRequirement = "preview"
)

// CommandSpec holds generic command metadata for the catalog.
type CommandSpec struct {
	Path         string
	Summary      string
	GroupID      string
	OutputModes  []string
	RequiresAuth bool
	SupportsMeta bool
	Mutates      bool
	Destructive  bool
	Examples     []string
	Related      []string
	Extensions   map[string]map[string]any
}

// Sidecar holds Chab-specific catalog metadata.
type Sidecar struct {
	Owner                string
	Status               CommandStatus
	DocPath              string
	AuthNote             string
	DocOnlyOutputModes   []OutputMode
	FrameworkOwned       bool
	SupportsDryRun       bool
	RequiresConfirmation bool
	HelpRequirements     []HelpRequirement
}

// Entry pairs generic command metadata with Chab-specific sidecar metadata.
type Entry struct {
	Spec    CommandSpec
	Sidecar Sidecar
}

var shellCatalog = []Entry{
	functional("completion", "Generate the autocompletion script for the specified shell", nil, []OutputMode{OutputShellScript}),
	functionalWith("api", "Call public API endpoints directly", nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupAPI
	}),
	functionalWith("api delete", "Send a DELETE request to an API path", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Spec.Mutates = true
		entry.Spec.Destructive = true
		entry.Sidecar.SupportsDryRun = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpRawEnvelope, HelpDryRun, HelpIdempotency, HelpMetadata}
	}),
	functionalWith("api get", "Send a GET request to an API path", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpPagination, HelpRawEnvelope, HelpMetadata}
	}),
	functionalWith("api patch", "Send a PATCH request to an API path", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Spec.Mutates = true
		entry.Sidecar.SupportsDryRun = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpRawEnvelope, HelpDryRun, HelpIdempotency, HelpMetadata}
	}),
	functionalWith("api post", "Send a POST request to an API path", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Spec.Mutates = true
		entry.Sidecar.SupportsDryRun = true
		entry.Sidecar.RequiresConfirmation = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpRawEnvelope, HelpDryRun, HelpIdempotency, HelpMetadata, HelpConfirmation}
	}),
	functionalWith("auth", "Inspect and manage API authentication", nil, nil, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup}
	}),
	functionalWith("auth env", "Report the effective authentication environment", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup}
	}),
	functionalWith("auth login", "Authorize and store an API or guest trial key", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup, HelpConfirmation}
	}),
	functionalWith("auth logout", "Remove the stored credential for the active profile", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup}
	}),
	functionalWith("auth status", "Report credential and key state", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup}
	}),
	functionalWith("config", "Inspect and edit non-secret CLI configuration", nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupConfig
	}),
	functional("config get", "Print one configuration value", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functional("config list", "List configuration values", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functional("config path", "Print the config and auth file paths", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functionalWith("config set", "Set one configuration value", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.Mutates = true
	}),
	functionalWith("credits", "Inspect team credits", nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupProjects
	}),
	functionalWith("credits balance", "Show the available credit balance", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpMetadata}
	}),
	functionalWith("credits transactions", "List credit transactions", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpPagination, HelpMetadata}
	}),
	connectedFamilyCatalog("drive", "Work with connected drive files"),
	connectedRead("drive connections", "List drive connections", true),
	connectedFamilyCatalog("drive folders", "Work with drive folders"),
	connectedMutation("drive folders create", "Create a drive folder", true, false, false),
	connectedFamilyCatalog("drive items", "Work with drive items"),
	connectedRead("drive items list", "List drive items", true),
	connectedRead("drive items show", "Show drive item metadata", false),
	connectedDownload("drive items download", "Download drive item bytes"),
	connectedMutation("drive items upload", "Upload a stored source to drive", true, false, false),
	connectedMutation("drive items update", "Replace drive item bytes", true, true, false),
	connectedMutation("drive items export", "Export a drive-native document", false, false, false),
	connectedMutation("drive items move", "Move or rename a drive item", true, true, false),
	connectedMutation("drive items trash", "Move a drive item to trash", true, true, true),
	connectedRead("drive search", "Search drive item names", false),
	connectedFamilyCatalog("drive permissions", "Work with drive permissions"),
	connectedMutation("drive permissions update", "Update drive item permissions", true, true, false),
	connectedFamilyCatalog("files", "Work with stored files"),
	connectedRead("files list", "List stored files", true),
	connectedRead("files show", "Show stored file metadata", false),
	connectedMutation("files upload", "Upload a stored file", false, true, false),
	connectedMutation("files resume", "Resume a stored-file upload", false, true, false),
	connectedRead("files wait", "Wait for stored file readiness", false),
	connectedDownload("files download", "Download stored file bytes"),
	connectedMutation("files delete", "Delete a stored file", false, true, true),
	connectedFamilyCatalog("mail", "Work with connected mail"),
	connectedRead("mail connections", "List mail connections", true),
	connectedRead("mail folders", "List mail folders", true),
	connectedRead("mail personas", "List mail personas", true),
	connectedFamilyCatalog("mail threads", "Work with mail threads"),
	connectedRead("mail threads list", "List mail threads", true),
	connectedRead("mail threads show", "Show a mail thread", true),
	connectedRead("mail search", "Search connected mail", false),
	connectedFamilyCatalog("mail drafts", "Work with API-owned mail drafts"),
	connectedMutation("mail drafts create", "Create a mail draft", true, false, false),
	connectedRead("mail drafts show", "Show a mail draft", false),
	connectedMutation("mail drafts update", "Update a mail draft", true, false, false),
	connectedMutation("mail drafts discard", "Discard a mail draft", true, false, false),
	connectedMutation("mail drafts send", "Send a mail draft", true, true, false),
	connectedFamilyCatalog("mail drafts attachments", "Work with draft attachments"),
	connectedMutation("mail drafts attachments add", "Add a draft attachment", true, false, false),
	connectedMutation("mail drafts attachments remove", "Remove a draft attachment", true, false, false),
	connectedFamilyCatalog("mail messages", "Work with mail messages"),
	connectedRead("mail messages body", "Read a mail message body", false),
	connectedMutation("mail messages state", "Update mail message state", true, false, false),
	connectedFamilyCatalog("mail attachments", "Work with message attachments"),
	connectedDownload("mail attachments download", "Download a mail attachment"),
	managementFamily("project", "Manage projects"),
	managementRead("project list", "List projects", true),
	managementRead("project show", "Show a project", false),
	managementMutation("project create", "Create a project", false, false),
	managementMutation("project update", "Update a project", false, false),
	managementMutation("project pause", "Pause a project", false, false),
	managementMutation("project resume", "Resume a project", false, false),
	managementMutation("project archive", "Archive a project", false, false),
	managementMutation("project delete", "Delete a project", true, true),
	managementRead("usage", "Show team API usage", false),
	managementFamily("billing", "Manage billing settings"),
	managementRead("billing show", "Show billing context", false),
	managementRead("billing packages", "List billing packages", false),
	managementRead("billing reconciliation", "Show billing reconciliation", false),
	managementFamily("billing auto-recharge", "Manage auto-recharge"),
	managementRead("billing auto-recharge show", "Show auto-recharge", false),
	managementMutation("billing auto-recharge update", "Update auto-recharge", true, false),
	managementFamily("billing purchases", "Manage billing purchases"),
	managementMutation("billing purchases create", "Create a credit purchase", true, false),
	managementRead("billing purchases show", "Show a credit purchase", false),
	managementRead("billing purchases wait", "Wait for a credit purchase", false),
	managementFamily("tokens", "Manage team API tokens"),
	managementRead("tokens list", "List API tokens", true),
	managementRead("tokens show", "Show API token metadata", false),
	managementMutation("tokens create", "Create an API token", false, false),
	managementMutation("tokens update", "Update an API token", true, false),
	managementMutation("tokens revoke", "Revoke an API token", true, true),
	managementFamily("tokens approvals", "Manage approval challenges"),
	managementMutation("tokens approvals create", "Create an approval challenge", false, false),
	managementRead("tokens approvals wait", "Wait for an approval proof", false),
	managementFamily("webhooks", "Manage webhook endpoints and replays"),
	managementFamily("webhooks endpoints", "Manage webhook endpoints"),
	managementRead("webhooks endpoints list", "List webhook endpoints", true),
	managementRead("webhooks endpoints show", "Show a webhook endpoint", false),
	managementMutation("webhooks endpoints create", "Create a webhook endpoint", false, false),
	managementMutation("webhooks endpoints update", "Update a webhook endpoint", false, false),
	managementMutation("webhooks endpoints delete", "Delete a webhook endpoint", true, true),
	managementMutation("webhooks endpoints rotate-secret", "Rotate a webhook secret", true, false),
	managementFamily("webhooks deliveries", "Manage webhook deliveries"),
	managementRead("webhooks deliveries list", "List webhook deliveries", true),
	managementRead("webhooks deliveries show", "Show a webhook delivery", false),
	managementMutation("webhooks deliveries replay", "Replay a webhook delivery", true, false),
	managementFamily("webhooks replays", "Manage webhook replay windows"),
	managementMutation("webhooks replays create", "Create a webhook replay window", true, false),
	operationFamily("operations", "Inspect and recover Chab operations"),
	operationReadMeta("operations list", "List remote operations"),
	operationReadMeta("operations show", "Show operation status"),
	operationRead("operations estimate", "Estimate an operation request"),
	operationStart("operations start", "Start a supported operation by key", true),
	operationReadMeta("operations wait", "Wait for an operation to finish"),
	operationReadMeta("operations result", "Fetch operation result metadata"),
	operationReadMeta("operations artifact", "Fetch operation artifact metadata"),
	operationReadMeta("operations artifact download", "Download operation artifact bytes"),
	operationMutation("operations cancel", "Cancel an operation", true, true),
	operationMutation("operations bulk-cancel", "Cancel operations by filter", true, true),
	operationLocal("operations schema", "Show an embedded operation schema"),
	operationRead("operations resume", "Resume a local action"),
	operationFamily("operations actions", "Inspect local action records"),
	operationLocal("operations actions list", "List local action records"),
	operationLocal("operations actions show", "Show a local action record"),
	operationFamily("examples", "Run example operations"),
	operationStart("examples echo", "Echo a setup payload", false),
	operationFamily("search", "Run search operations"),
	operationStart("search web", "Start a web search operation (preview)", true),
	operationStart("search serp", "Start a SERP search operation (preview)", true),
	operationFamily("seo", "Run SEO operations"),
	operationFamily("seo keywords", "Run keyword operations"),
	operationStart("seo keywords ideas", "Start a keyword ideas operation (preview)", true),
	operationStart("seo keywords metrics", "Start a keyword metrics operation (preview)", true),
	operationFamily("seo domains", "Run domain SEO operations"),
	operationStart("seo domains overview", "Start a domain overview operation (preview)", true),
	operationStart("seo domains backlinks", "Start a backlinks operation (preview)", true),
	operationFamily("business", "Run business-data operations"),
	operationStart("business search", "Start a business search operation (preview)", true),
	operationStart("business details", "Start a business details operation (preview)", true),
	operationFamily("contacts", "Run contact discovery operations"),
	operationStart("contacts domain-search", "Start a domain contact search (preview)", true),
	operationStart("contacts email-finder", "Start an email finder operation (preview)", true),
	operationStart("contacts email-verify", "Start an email verification operation (preview)", true),
	operationFamily("scrape", "Run scrape operations"),
	operationStart("scrape markdown", "Start a markdown scrape operation (preview)", true),
	operationStart("scrape dom", "Start a DOM scrape operation (preview)", true),
	operationFamily("screenshots", "Run screenshot operations"),
	operationStart("screenshots url", "Start a URL screenshot operation (preview)", true),
	operationFamily("convert", "Run conversion operations"),
	operationStart("convert file", "Start a file conversion operation (preview)", true),
	operationFamily("translate", "Run translation operations"),
	operationStart("translate text-or-document", "Start a text or document translation (preview)", true),
	operationFamily("llm", "Run model operations"),
	operationRead("llm models", "List available LLM models (preview)"),
	operationStart("llm generate", "Start an LLM generation operation (preview)", true),
	operationStart("llm embeddings", "Start an embeddings operation (preview)", true),
	operationFamily("research", "Run research operations"),
	operationStart("research deep", "Start a deep research operation (preview)", true),
	functional("doctor", "Check local setup and API readiness", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functional("errors", "List public API error codes", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functional("health", "Show public API health", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functional("help", "Help about any command", nil, []OutputMode{OutputHelp}),
	alias("login", "Authorize and store an API or guest trial key", []OutputMode{OutputHuman, OutputPlain}, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup, HelpConfirmation}
	}),
	alias("logout", "Remove the stored credential for the active profile", []OutputMode{OutputHuman, OutputPlain}, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup}
	}),
	functionalWith("mcp", "Run local MCP integrations", nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupUtilities
		entry.Sidecar.AuthNote = mcpAuthNote
	}),
	functionalWith("mcp serve", "Serve Chab tools over MCP stdio", nil, []OutputMode{OutputMCPStdio}, func(entry *Entry) {
		entry.Spec.GroupID = groupUtilities
		entry.Sidecar.AuthNote = mcpAuthNote
	}),
	functionalWith("profile", "Manage connection profiles", nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupConfig
	}),
	functionalWith("profile create", "Create a profile", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.Mutates = true
	}),
	functionalWith("profile delete", "Delete a profile", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.Mutates = true
		entry.Spec.Destructive = true
		entry.Sidecar.RequiresConfirmation = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpConfirmation}
	}),
	functional("profile list", "List configured profiles", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functional("profile show", "Show a profile's settings", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functionalWith("profile use", "Switch the current profile", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.Mutates = true
	}),
	functionalWith("setup", "Set up a verified API or guest trial profile", []OutputMode{OutputHuman, OutputPlain}, nil, func(entry *Entry) {
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup, HelpConfirmation}
	}),
	functional("version", "Print build metadata", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil),
	functionalWith("whoami", "Show the authenticated team or guest principal", []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.RequiresAuth = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpAPIKeySetup}
	}),
}

const mcpAuthNote = "Startup, discovery, `tools/list`, `chab_auth_env`, `chab_health`, and `chab_errors` do not require credentials. API-backed tools such as `chab_auth_me`, `chab_credits_get`, `chab_credits_transactions_list`, and `chab_search_web` require a stored login or `CHAB_API_KEY` inherited by the MCP server process. Confirmation-gated tools require `local.confirmation=true` and recoverable action tools can be inspected with `chab_action_list`, `chab_action_show`, and `chab_action_resume`. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process."

func functional(path, summary string, outputModes, docOnly []OutputMode) Entry {
	return entry(path, summary, StatusFunctional, outputModes, docOnly)
}

func functionalWith(path, summary string, outputModes, docOnly []OutputMode, mutate func(*Entry)) Entry {
	entry := functional(path, summary, outputModes, docOnly)
	if mutate != nil {
		mutate(&entry)
	}
	return entry
}

func alias(path, summary string, outputModes []OutputMode, mutate func(*Entry)) Entry {
	entry := entry(path, summary, StatusAlias, outputModes, nil)
	if mutate != nil {
		mutate(&entry)
	}
	return entry
}

func reserved(path, summary string) Entry {
	return entry(path, summary, StatusReserved, nil, nil)
}

func connectedFamilyCatalog(path, summary string) Entry {
	return functionalWith(path, summary, nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupConnected
	})
}

func connectedRead(path, summary string, paginated bool) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupConnected
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpMetadata}
		if paginated {
			entry.Sidecar.HelpRequirements = append([]HelpRequirement{HelpPagination}, entry.Sidecar.HelpRequirements...)
		}
		applyConnectedContractMetadata(entry)
	})
}

func connectedDownload(path, summary string) Entry {
	return connectedRead(path, summary, false)
}

func connectedMutation(path, summary string, dryRun, confirmation, destructive bool) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupConnected
		entry.Spec.RequiresAuth = true
		entry.Spec.Mutates = true
		entry.Spec.Destructive = destructive
		entry.Sidecar.SupportsDryRun = dryRun
		entry.Sidecar.RequiresConfirmation = confirmation
		entry.Sidecar.HelpRequirements = []HelpRequirement{}
		if dryRun {
			entry.Sidecar.HelpRequirements = append(entry.Sidecar.HelpRequirements, HelpDryRun)
		}
		if path != "files resume" {
			entry.Sidecar.HelpRequirements = append(entry.Sidecar.HelpRequirements, HelpIdempotency)
		}
		if confirmation {
			entry.Sidecar.HelpRequirements = append(entry.Sidecar.HelpRequirements, HelpConfirmation)
		}
		applyConnectedContractMetadata(entry)
	})
}

const contractExtensionNamespace = "chab/api-contract"

var connectedOperationIDs = map[string]string{
	"drive connections":              "drive.connections.list",
	"drive folders create":           "drive.folders.create",
	"drive items download":           "drive.items.download",
	"drive items export":             "drive.items.export",
	"drive items list":               "drive.items.list",
	"drive items move":               "drive.items.move",
	"drive items show":               "drive.items.read",
	"drive items trash":              "drive.items.trash",
	"drive items update":             "drive.items.update",
	"drive items upload":             "drive.items.upload",
	"drive permissions update":       "drive.permissions.update",
	"drive search":                   "drive.search",
	"files delete":                   "files.delete",
	"files download":                 "files.download",
	"files list":                     "files.list",
	"files resume":                   "files.create",
	"files show":                     "files.get",
	"files upload":                   "files.create",
	"files wait":                     "files.get",
	"mail attachments download":      "mail.attachments.download",
	"mail connections":               "mail.connections.list",
	"mail drafts attachments add":    "mail.draft_attachments.add",
	"mail drafts attachments remove": "mail.draft_attachments.remove",
	"mail drafts create":             "mail.drafts.create",
	"mail drafts discard":            "mail.drafts.discard",
	"mail drafts send":               "mail.messages.send",
	"mail drafts show":               "mail.drafts.read",
	"mail drafts update":             "mail.drafts.update",
	"mail folders":                   "mail.folders.list",
	"mail messages body":             "mail.messages.body.read",
	"mail messages state":            "mail.messages.state.update",
	"mail personas":                  "mail.personas.list",
	"mail search":                    "mail.search",
	"mail threads list":              "mail.threads.list",
	"mail threads show":              "mail.threads.read",
}

func applyConnectedContractMetadata(entry *Entry) {
	operationID, ok := connectedOperationIDs[entry.Spec.Path]
	if !ok {
		return
	}
	op, ok := chabcontract.MustLoad().Find(operationID)
	if !ok || op.Availability == "" {
		return
	}
	if entry.Spec.Extensions == nil {
		entry.Spec.Extensions = map[string]map[string]any{}
	}
	entry.Spec.Extensions[contractExtensionNamespace] = map[string]any{
		"operation_id": op.ID,
		"availability": op.Availability,
	}
	if op.Availability == "preview" {
		entry.Spec.Summary = previewCatalogSummary(entry.Spec.Summary)
		entry.Sidecar.HelpRequirements = appendRequirement(entry.Sidecar.HelpRequirements, HelpPreview)
	}
}

func previewCatalogSummary(summary string) string {
	if strings.Contains(summary, "(preview)") {
		return summary
	}
	return summary + " (preview)"
}

func appendRequirement(requirements []HelpRequirement, want HelpRequirement) []HelpRequirement {
	for _, requirement := range requirements {
		if requirement == want {
			return requirements
		}
	}
	return append(requirements, want)
}

func operationFamily(path, summary string) Entry {
	return functionalWith(path, summary, nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupOps
	})
}

func managementFamily(path, summary string) Entry {
	return functionalWith(path, summary, nil, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupProjects
	})
}

func managementRead(path, summary string, paginated bool) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupProjects
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpMetadata}
		if paginated {
			entry.Sidecar.HelpRequirements = append([]HelpRequirement{HelpPagination}, entry.Sidecar.HelpRequirements...)
		}
	})
}

func managementMutation(path, summary string, confirmation, destructive bool) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupProjects
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Spec.Mutates = true
		entry.Spec.Destructive = destructive
		entry.Sidecar.SupportsDryRun = true
		entry.Sidecar.RequiresConfirmation = confirmation
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpDryRun, HelpIdempotency, HelpMetadata}
		if confirmation {
			entry.Sidecar.HelpRequirements = append(entry.Sidecar.HelpRequirements, HelpConfirmation)
		}
	})
}

func operationLocal(path, summary string) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupOps
	})
}

func operationRead(path, summary string) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupOps
		entry.Spec.RequiresAuth = true
	})
}

func operationReadMeta(path, summary string) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain, OutputJQ, OutputTemplate}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupOps
		entry.Spec.RequiresAuth = true
		entry.Spec.SupportsMeta = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpMetadata}
	})
}

func operationStart(path, summary string, confirmation bool) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupOps
		entry.Spec.RequiresAuth = true
		entry.Spec.Mutates = true
		entry.Sidecar.SupportsDryRun = true
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpDryRun, HelpIdempotency}
		if confirmation {
			entry.Sidecar.RequiresConfirmation = true
			entry.Sidecar.HelpRequirements = append(entry.Sidecar.HelpRequirements, HelpConfirmation)
		}
	})
}

func operationMutation(path, summary string, confirmation, destructive bool) Entry {
	return functionalWith(path, summary, []OutputMode{OutputHuman, OutputJSON, OutputPlain}, nil, func(entry *Entry) {
		entry.Spec.GroupID = groupOps
		entry.Spec.RequiresAuth = true
		entry.Spec.Mutates = true
		entry.Spec.Destructive = destructive
		entry.Sidecar.RequiresConfirmation = confirmation
		entry.Sidecar.HelpRequirements = []HelpRequirement{HelpIdempotency}
		if confirmation {
			entry.Sidecar.HelpRequirements = append(entry.Sidecar.HelpRequirements, HelpConfirmation)
		}
	})
}

// entry builds one catalog row without implying behavior the shell does not
// implement. Reserved rows pass nil output modes, so downstream docs and tests
// cannot accidentally claim JSON, auth, metadata, mutation, or safety support.
func entry(path, summary string, status CommandStatus, outputModes, docOnly []OutputMode) Entry {
	specModes := make([]string, 0, len(outputModes))
	for _, mode := range outputModes {
		specModes = append(specModes, string(mode))
	}
	return Entry{
		Spec: CommandSpec{
			Path:        path,
			Summary:     summary,
			OutputModes: specModes,
		},
		Sidecar: Sidecar{
			Owner:              ownerFor(path),
			Status:             status,
			DocPath:            docPathFor(path),
			DocOnlyOutputModes: append([]OutputMode(nil), docOnly...),
			FrameworkOwned:     path == "help" || path == "completion",
		},
	}
}

// Catalog returns the initial command catalog, sorted by command path. Each
// call returns independent copies.
func Catalog() []Entry {
	out := make([]Entry, len(shellCatalog))
	for i, entry := range shellCatalog {
		out[i] = cloneEntry(entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Spec.Path < out[j].Spec.Path
	})
	return out
}

// cloneEntry protects the package-level catalog from callers that mutate the
// returned slices or extension maps in tests and doc generators.
func cloneEntry(entry Entry) Entry {
	entry.Spec.OutputModes = append([]string(nil), entry.Spec.OutputModes...)
	entry.Spec.Examples = append([]string(nil), entry.Spec.Examples...)
	entry.Spec.Related = append([]string(nil), entry.Spec.Related...)
	if entry.Spec.Extensions != nil {
		entry.Spec.Extensions = cloneExtensions(entry.Spec.Extensions)
	}
	entry.Sidecar.DocOnlyOutputModes = append([]OutputMode(nil), entry.Sidecar.DocOnlyOutputModes...)
	entry.Sidecar.HelpRequirements = append([]HelpRequirement(nil), entry.Sidecar.HelpRequirements...)
	return entry
}

func cloneExtensions(in map[string]map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(in))
	for namespace, object := range in {
		copied := make(map[string]any, len(object))
		for key, value := range object {
			copied[key] = value
		}
		out[namespace] = copied
	}
	return out
}

func summaryFor(path string) string {
	for _, entry := range shellCatalog {
		if entry.Spec.Path == path {
			return entry.Spec.Summary
		}
	}
	panic("missing catalog summary for " + path)
}

func docPathFor(path string) string {
	return "manual/commands/chab-" + strings.ReplaceAll(path, " ", "-") + ".md"
}

func ownerFor(path string) string {
	switch {
	case path == "version" || path == "help" || path == "completion":
		return "catalog-shell-foundation"
	case path == "setup" || path == "login" || path == "logout" || path == "whoami" || strings.HasPrefix(path, "auth") || path == "doctor":
		return "catalog-auth-readiness"
	case strings.HasPrefix(path, "credits"):
		return "catalog-credits"
	case path == "project" || strings.HasPrefix(path, "project ") ||
		path == "usage" ||
		path == "billing" || strings.HasPrefix(path, "billing ") ||
		path == "tokens" || strings.HasPrefix(path, "tokens ") ||
		path == "webhooks" || strings.HasPrefix(path, "webhooks "):
		return "catalog-management"
	case strings.HasPrefix(path, "profile") || strings.HasPrefix(path, "config"):
		return "catalog-local-settings"
	case strings.HasPrefix(path, "api"):
		return "catalog-raw-api"
	case strings.HasPrefix(path, "operations") ||
		strings.HasPrefix(path, "examples") ||
		strings.HasPrefix(path, "search") ||
		strings.HasPrefix(path, "seo") ||
		strings.HasPrefix(path, "business") ||
		strings.HasPrefix(path, "contacts") ||
		strings.HasPrefix(path, "scrape") ||
		strings.HasPrefix(path, "screenshots") ||
		strings.HasPrefix(path, "convert") ||
		strings.HasPrefix(path, "translate") ||
		strings.HasPrefix(path, "llm") ||
		strings.HasPrefix(path, "research"):
		return "catalog-operations"
	case path == "files" || strings.HasPrefix(path, "files ") ||
		path == "mail" || strings.HasPrefix(path, "mail ") ||
		path == "drive" || strings.HasPrefix(path, "drive "):
		return "catalog-connected-data"
	case path == "health" || path == "errors":
		return "catalog-public-diagnostics"
	default:
		return "catalog-shell-foundation"
	}
}
