package operationmap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vincentsch/chab-cli/internal/chabcontract"
)

// Binding records implementation placement separately from contract metadata.
type Binding struct {
	OperationID string
	Owner       string
	CLI         Surface
	MCP         Surface
}

// Surface records an interface decision for an operation.
type Surface struct {
	Command string
	Tool    string
	State   string
	Reason  string
}

const (
	StateImplemented = "implemented"
	StatePlanned     = "planned"
	StateExcluded    = "excluded"
)

const (
	ownerRuntime    = "runtime foundation"
	ownerOperations = "operation workflows"
	ownerManagement = "management workflows"
	ownerFiles      = "file and connected-data workflows"
	ownerAgent      = "agent packaging workflows"
)

// Bindings returns interface decisions for every known public operation.
func Bindings(registry chabcontract.Registry) ([]Binding, error) {
	out := make([]Binding, 0, len(registry.Operations()))
	for _, op := range registry.Operations() {
		out = append(out, bindingFor(op))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	if err := validateBindings(registry, out); err != nil {
		return nil, err
	}
	return out, nil
}

func bindingFor(op chabcontract.Operation) Binding {
	b := Binding{
		OperationID: op.ID,
		Owner:       ownerFor(op.ID),
		CLI:         planned("command in later workflow"),
		MCP:         planned("local MCP adapter in later workflow"),
	}
	switch op.ID {
	case "system.health":
		b.CLI = implementedCommand("health")
		b.MCP = implementedTool("chab_health")
	case "errors.list":
		b.CLI = implementedCommand("errors")
		b.MCP = implementedTool("chab_errors")
	case "auth.me":
		b.CLI = implementedCommand("whoami")
		b.MCP = implementedTool(operationToolName(op.ID))
	case "cli.compatibility":
		b.CLI = implementedCommand("auth login")
		b.MCP = excluded("browser/device setup is a deliberate CLI flow")
	case "credits.get":
		b.CLI = implementedCommand("credits balance")
		b.MCP = implementedTool(operationToolName(op.ID))
	case "credits.transactions.list":
		b.CLI = implementedCommand("credits transactions")
		b.MCP = implementedTool(operationToolName(op.ID))
	case "projects.list":
		b.CLI = implementedCommand("project list")
	case "projects.get":
		b.CLI = implementedCommand("project show")
	case "projects.create":
		b.CLI = implementedCommand("project create")
	case "projects.update":
		b.CLI = implementedCommand("project update")
	case "projects.delete":
		b.CLI = implementedCommand("project delete")
	case "usage.get":
		b.CLI = implementedCommand("usage")
	case "billing.get":
		b.CLI = implementedCommand("billing show")
	case "billing.packages.list":
		b.CLI = implementedCommand("billing packages")
	case "billing.reconciliation.get":
		b.CLI = implementedCommand("billing reconciliation")
	case "billing.auto_recharge.get":
		b.CLI = implementedCommand("billing auto-recharge show")
	case "billing.auto_recharge.update":
		b.CLI = implementedCommand("billing auto-recharge update")
	case "billing.purchases.create":
		b.CLI = implementedCommand("billing purchases create")
	case "billing.purchases.get":
		b.CLI = implementedCommand("billing purchases show")
	case "tokens.list":
		b.CLI = implementedCommand("tokens list")
	case "tokens.get":
		b.CLI = implementedCommand("tokens show")
	case "tokens.create":
		b.CLI = implementedCommand("tokens create")
	case "tokens.update":
		b.CLI = implementedCommand("tokens update")
	case "tokens.revoke":
		b.CLI = implementedCommand("tokens revoke")
	case "tokens.management_approvals.create":
		b.CLI = implementedCommand("tokens approvals create")
	case "tokens.management_approvals.get":
		b.CLI = implementedCommand("tokens approvals wait")
	case "webhooks.endpoints.list":
		b.CLI = implementedCommand("webhooks endpoints list")
	case "webhooks.endpoints.get":
		b.CLI = implementedCommand("webhooks endpoints show")
	case "webhooks.endpoints.create":
		b.CLI = implementedCommand("webhooks endpoints create")
	case "webhooks.endpoints.update":
		b.CLI = implementedCommand("webhooks endpoints update")
	case "webhooks.endpoints.delete":
		b.CLI = implementedCommand("webhooks endpoints delete")
	case "webhooks.endpoints.rotate_secret":
		b.CLI = implementedCommand("webhooks endpoints rotate-secret")
	case "webhooks.deliveries.list":
		b.CLI = implementedCommand("webhooks deliveries list")
	case "webhooks.deliveries.get":
		b.CLI = implementedCommand("webhooks deliveries show")
	case "webhooks.deliveries.replay":
		b.CLI = implementedCommand("webhooks deliveries replay")
	case "webhooks.replays.create":
		b.CLI = implementedCommand("webhooks replays create")
	case "operations.list":
		b.CLI = implementedCommand("operations list")
	case "operations.get":
		b.CLI = implementedCommand("operations show")
	case "operations.estimate":
		b.CLI = implementedCommand("operations estimate")
	case "operations.result":
		b.CLI = implementedCommand("operations result")
	case "operations.artifact":
		b.CLI = implementedCommand("operations artifact")
	case "operations.cancel":
		b.CLI = implementedCommand("operations cancel")
	case "operations.bulk_cancel":
		b.CLI = implementedCommand("operations bulk-cancel")
	case "operations.artifact_download":
		b.CLI = implementedCommand("operations artifact download")
		b.MCP = planned("artifact byte transfer requires explicit local path safeguards")
	case "files.list":
		b.CLI = implementedCommand("files list")
	case "files.create":
		b.CLI = implementedCommand("files upload")
	case "files.get":
		b.CLI = implementedCommand("files show")
	case "files.delete":
		b.CLI = implementedCommand("files delete")
	case "files.download":
		b.CLI = implementedCommand("files download")
	case "mail.connections.list":
		b.CLI = implementedCommand("mail connections")
	case "mail.drafts.create":
		b.CLI = implementedCommand("mail drafts create")
	case "mail.drafts.read":
		b.CLI = implementedCommand("mail drafts show")
	case "mail.drafts.update":
		b.CLI = implementedCommand("mail drafts update")
	case "mail.draft_attachments.add":
		b.CLI = implementedCommand("mail drafts attachments add")
	case "mail.draft_attachments.remove":
		b.CLI = implementedCommand("mail drafts attachments remove")
	case "mail.drafts.discard":
		b.CLI = implementedCommand("mail drafts discard")
	case "mail.messages.send":
		b.CLI = implementedCommand("mail drafts send")
	case "mail.folders.list":
		b.CLI = implementedCommand("mail folders")
	case "mail.attachments.download":
		b.CLI = implementedCommand("mail attachments download")
	case "mail.messages.body.read":
		b.CLI = implementedCommand("mail messages body")
	case "mail.messages.state.update":
		b.CLI = implementedCommand("mail messages state")
	case "mail.personas.list":
		b.CLI = implementedCommand("mail personas")
	case "mail.search":
		b.CLI = implementedCommand("mail search")
	case "mail.threads.list":
		b.CLI = implementedCommand("mail threads list")
	case "mail.threads.read":
		b.CLI = implementedCommand("mail threads show")
	case "drive.connections.list":
		b.CLI = implementedCommand("drive connections")
	case "drive.folders.create":
		b.CLI = implementedCommand("drive folders create")
	case "drive.items.list":
		b.CLI = implementedCommand("drive items list")
	case "drive.items.upload":
		b.CLI = implementedCommand("drive items upload")
	case "drive.items.read":
		b.CLI = implementedCommand("drive items show")
	case "drive.items.update":
		b.CLI = implementedCommand("drive items update")
	case "drive.items.download":
		b.CLI = implementedCommand("drive items download")
	case "drive.items.export":
		b.CLI = implementedCommand("drive items export")
	case "drive.items.move":
		b.CLI = implementedCommand("drive items move")
	case "drive.permissions.update":
		b.CLI = implementedCommand("drive permissions update")
	case "drive.items.trash":
		b.CLI = implementedCommand("drive items trash")
	case "drive.search":
		b.CLI = implementedCommand("drive search")
	case "examples.echo":
		b.CLI = implementedCommand("examples echo")
	case "search.web":
		b.CLI = implementedCommand("search web")
	case "search.serp":
		b.CLI = implementedCommand("search serp")
	case "seo.keywords.ideas":
		b.CLI = implementedCommand("seo keywords ideas")
	case "seo.keywords.metrics":
		b.CLI = implementedCommand("seo keywords metrics")
	case "seo.domains.overview":
		b.CLI = implementedCommand("seo domains overview")
	case "seo.domains.backlinks":
		b.CLI = implementedCommand("seo domains backlinks")
	case "business.search":
		b.CLI = implementedCommand("business search")
	case "business.details":
		b.CLI = implementedCommand("business details")
	case "contacts.domain_search":
		b.CLI = implementedCommand("contacts domain-search")
	case "contacts.email_finder":
		b.CLI = implementedCommand("contacts email-finder")
	case "contacts.email_verify":
		b.CLI = implementedCommand("contacts email-verify")
	case "scrape.markdown":
		b.CLI = implementedCommand("scrape markdown")
	case "scrape.dom":
		b.CLI = implementedCommand("scrape dom")
	case "screenshots.url":
		b.CLI = implementedCommand("screenshots url")
	case "convert.file":
		b.CLI = implementedCommand("convert file")
	case "translate.text_or_document":
		b.CLI = implementedCommand("translate text-or-document")
	case "llm.models":
		b.CLI = implementedCommand("llm models")
	case "llm.generate":
		b.CLI = implementedCommand("llm generate")
	case "llm.embeddings":
		b.CLI = implementedCommand("llm embeddings")
	case "research.deep":
		b.CLI = implementedCommand("research deep")
	}
	switch op.ID {
	case "billing.auto_recharge.update":
		b.MCP = excluded("billing setting mutation requires deliberate human approval and confirmation")
	case "billing.purchases.create":
		b.MCP = excluded("purchase creation can trigger billing effects and requires deliberate human approval")
	case "tokens.create":
		b.MCP = excluded("returns one-time plaintext API token secret")
	case "tokens.management_approvals.get":
		b.MCP = excluded("returns one-time management approval proof")
	case "webhooks.endpoints.create", "webhooks.endpoints.rotate_secret":
		b.MCP = excluded("returns one-time webhook signing secret")
	}
	if b.MCP.State == StatePlanned {
		b.MCP = implementedTool(operationToolName(op.ID))
	}
	return b
}

func operationToolName(operationID string) string {
	var b strings.Builder
	b.WriteString("chab_")
	lastUnderscore := false
	for _, r := range operationID {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

func ownerFor(operationID string) string {
	switch {
	case operationID == "system.health" ||
		operationID == "errors.list" ||
		operationID == "auth.me" ||
		operationID == "cli.compatibility" ||
		strings.HasPrefix(operationID, "credits."):
		return ownerRuntime
	case strings.HasPrefix(operationID, "operations.") ||
		operationID == "examples.echo" ||
		strings.HasPrefix(operationID, "search.") ||
		strings.HasPrefix(operationID, "seo.") ||
		strings.HasPrefix(operationID, "business.") ||
		strings.HasPrefix(operationID, "contacts.") ||
		strings.HasPrefix(operationID, "scrape.") ||
		strings.HasPrefix(operationID, "screenshots.") ||
		strings.HasPrefix(operationID, "convert.") ||
		strings.HasPrefix(operationID, "translate.") ||
		strings.HasPrefix(operationID, "llm.") ||
		strings.HasPrefix(operationID, "research."):
		return ownerOperations
	case strings.HasPrefix(operationID, "projects.") ||
		strings.HasPrefix(operationID, "usage.") ||
		strings.HasPrefix(operationID, "billing.") ||
		strings.HasPrefix(operationID, "tokens.") ||
		strings.HasPrefix(operationID, "webhooks."):
		return ownerManagement
	case strings.HasPrefix(operationID, "files.") ||
		strings.HasPrefix(operationID, "mail.") ||
		strings.HasPrefix(operationID, "drive."):
		return ownerFiles
	default:
		return ownerAgent
	}
}

func implementedCommand(path string) Surface {
	return Surface{Command: path, State: StateImplemented}
}

func implementedTool(name string) Surface {
	return Surface{Tool: name, State: StateImplemented}
}

func planned(reason string) Surface {
	return Surface{State: StatePlanned, Reason: reason}
}

func excluded(reason string) Surface {
	return Surface{State: StateExcluded, Reason: reason}
}

func validateBindings(registry chabcontract.Registry, bindings []Binding) error {
	seen := map[string]bool{}
	for _, binding := range bindings {
		if binding.OperationID == "" {
			return fmt.Errorf("binding has empty operation id")
		}
		if seen[binding.OperationID] {
			return fmt.Errorf("duplicate binding for %s", binding.OperationID)
		}
		seen[binding.OperationID] = true
		if _, ok := registry.Find(binding.OperationID); !ok {
			return fmt.Errorf("binding references unknown operation %s", binding.OperationID)
		}
		if binding.Owner == "" {
			return fmt.Errorf("binding %s has no owner", binding.OperationID)
		}
		if err := validateSurface("CLI", binding.OperationID, binding.CLI); err != nil {
			return err
		}
		if err := validateSurface("MCP", binding.OperationID, binding.MCP); err != nil {
			return err
		}
	}
	if len(seen) != len(registry.Operations()) {
		return fmt.Errorf("binding count = %d, want %d", len(seen), len(registry.Operations()))
	}
	return nil
}

func validateSurface(name, operationID string, surface Surface) error {
	switch surface.State {
	case StateImplemented:
		if surface.Command == "" && surface.Tool == "" {
			return fmt.Errorf("%s binding %s is implemented without a command or tool", name, operationID)
		}
	case StatePlanned, StateExcluded:
		if surface.Reason == "" {
			return fmt.Errorf("%s binding %s has %s state without a reason", name, operationID, surface.State)
		}
	default:
		return fmt.Errorf("%s binding %s has unknown state %q", name, operationID, surface.State)
	}
	return nil
}

// RenderSurface converts one binding surface to the generated table cell.
func RenderSurface(surface Surface) string {
	switch surface.State {
	case StateImplemented:
		if surface.Command != "" {
			return "`chab " + surface.Command + "`"
		}
		return "`" + surface.Tool + "`"
	case StateExcluded:
		return "excluded: " + surface.Reason
	default:
		return "planned: " + surface.Reason
	}
}
