package operationscmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

type startSpec struct {
	OperationKey string
	Use          string
	Short        string
	Example      string
	Fields       []fieldFlag
}

func NewExamplesCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("examples", "Run example operations")
	cmd.AddCommand(newNativeStartCommand(f, startSpec{
		OperationKey: "examples.echo",
		Use:          "echo",
		Short:        "Echo a setup payload",
		Example:      "  chab examples echo --set data='{\"hello\":\"world\"}'\n  chab examples echo --simulate-async --set data='{\"hello\":\"world\"}' --json",
		Fields: []fieldFlag{
			{Name: "simulate-async", Field: "simulate_async", Kind: valueBool, Usage: "create an asynchronous echo operation"},
		},
	}))
	return cmd
}

func NewSearchCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("search", "Run search operations")
	cmd.AddCommand(
		newNativeStartCommand(f, startSpec{OperationKey: "search.web", Use: "web", Short: "Start a web search operation", Example: searchExample("web"), Fields: searchFields(false)}),
		newNativeStartCommand(f, startSpec{OperationKey: "search.serp", Use: "serp", Short: "Start a SERP search operation", Example: searchExample("serp"), Fields: searchFields(true)}),
	)
	return cmd
}

func NewSEOCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("seo", "Run SEO operations")
	keywords := family("keywords", "Run keyword operations")
	keywords.AddCommand(
		newNativeStartCommand(f, startSpec{OperationKey: "seo.keywords.ideas", Use: "ideas", Short: "Start a keyword ideas operation", Example: "  chab seo keywords ideas --seeds coffee --engine google --location '{\"country\":\"DE\"}' --language en --yes", Fields: []fieldFlag{
			{Name: "seeds", Field: "seeds", Kind: valueStringSlice, Usage: "keyword seed (repeatable)"},
			{Name: "engine", Field: "engine", Kind: valueString, Usage: "search engine"},
			{Name: "location", Field: "location", Kind: valueJSON, Usage: "location object as JSON"},
			{Name: "language", Field: "language", Kind: valueString, Usage: "language code"},
			{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
		}}),
		newNativeStartCommand(f, startSpec{OperationKey: "seo.keywords.metrics", Use: "metrics", Short: "Start a keyword metrics operation", Example: "  chab seo keywords metrics --keywords coffee --engine google --location '{\"country\":\"DE\"}' --language en --yes", Fields: []fieldFlag{
			{Name: "keywords", Field: "keywords", Kind: valueStringSlice, Usage: "keyword (repeatable)"},
			{Name: "engine", Field: "engine", Kind: valueString, Usage: "search engine"},
			{Name: "location", Field: "location", Kind: valueJSON, Usage: "location object as JSON"},
			{Name: "language", Field: "language", Kind: valueString, Usage: "language code"},
		}}),
	)
	domains := family("domains", "Run domain SEO operations")
	domains.AddCommand(
		newNativeStartCommand(f, startSpec{OperationKey: "seo.domains.overview", Use: "overview", Short: "Start a domain overview operation", Example: "  chab seo domains overview --domain example.com --location '{\"country\":\"DE\"}' --language en --yes", Fields: domainFields(false)}),
		newNativeStartCommand(f, startSpec{OperationKey: "seo.domains.backlinks", Use: "backlinks", Short: "Start a backlinks operation", Example: "  chab seo domains backlinks --domain example.com --limit 100 --yes", Fields: domainFields(true)}),
	)
	cmd.AddCommand(keywords, domains)
	return cmd
}

func NewBusinessCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("business", "Run business-data operations")
	cmd.AddCommand(
		newNativeStartCommand(f, startSpec{OperationKey: "business.search", Use: "search", Short: "Start a business search operation", Example: "  chab business search --query cafe --engine google --location '{\"country\":\"DE\"}' --language en --yes", Fields: []fieldFlag{
			{Name: "query", Field: "query", Kind: valueString, Usage: "business search query"},
			{Name: "category", Field: "category", Kind: valueString, Usage: "business category"},
			{Name: "engine", Field: "engine", Kind: valueString, Usage: "search engine"},
			{Name: "location", Field: "location", Kind: valueJSON, Usage: "location object as JSON"},
			{Name: "language", Field: "language", Kind: valueString, Usage: "language code"},
			{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
		}}),
		newNativeStartCommand(f, startSpec{OperationKey: "business.details", Use: "details", Short: "Start a business details operation", Example: "  chab business details --business-ref bref_EXAMPLE --engine google --language en --yes", Fields: []fieldFlag{
			{Name: "business-ref", Field: "business_ref", Kind: valueString, Usage: "business reference string"},
			{Name: "identity", Field: "identity", Kind: valueJSON, Usage: "business identity object as JSON"},
			{Name: "engine", Field: "engine", Kind: valueString, Usage: "search engine"},
			{Name: "language", Field: "language", Kind: valueString, Usage: "language code"},
		}}),
	)
	return cmd
}

func NewContactsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("contacts", "Run contact discovery operations")
	cmd.AddCommand(
		newNativeStartCommand(f, startSpec{OperationKey: "contacts.domain_search", Use: "domain-search", Short: "Start a domain contact search", Example: "  chab contacts domain-search --domain example.com --limit 10 --yes", Fields: []fieldFlag{
			{Name: "domain", Field: "domain", Kind: valueString, Usage: "domain to search"},
			{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
			{Name: "include-generic", Field: "include_generic", Kind: valueBool, Usage: "include generic role inboxes"},
		}}),
		newNativeStartCommand(f, startSpec{OperationKey: "contacts.email_finder", Use: "email-finder", Short: "Start an email finder operation", Example: "  chab contacts email-finder --full-name 'Ada Lovelace' --domain example.com --yes", Fields: []fieldFlag{
			{Name: "full-name", Field: "full_name", Kind: valueString, Usage: "person full name"},
			{Name: "first-name", Field: "first_name", Kind: valueString, Usage: "person first name"},
			{Name: "last-name", Field: "last_name", Kind: valueString, Usage: "person last name"},
			{Name: "domain", Field: "domain", Kind: valueString, Usage: "company domain"},
			{Name: "company", Field: "company", Kind: valueString, Usage: "company name"},
		}}),
		newNativeStartCommand(f, startSpec{OperationKey: "contacts.email_verify", Use: "email-verify", Short: "Start an email verification operation", Example: "  chab contacts email-verify --email person@example.com --yes", Fields: []fieldFlag{
			{Name: "email", Field: "email", Kind: valueString, Usage: "email address to verify"},
		}}),
	)
	return cmd
}

func NewScrapeCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("scrape", "Run scrape operations")
	cmd.AddCommand(
		newNativeStartCommand(f, startSpec{OperationKey: "scrape.markdown", Use: "markdown", Short: "Start a markdown scrape operation", Example: "  chab scrape markdown --url https://example.test --yes", Fields: scrapeFields(false)}),
		newNativeStartCommand(f, startSpec{OperationKey: "scrape.dom", Use: "dom", Short: "Start a DOM scrape operation", Example: "  chab scrape dom --url https://example.test --selectors '{\"title\":{\"selector\":\"h1\"}}' --yes", Fields: scrapeFields(true)}),
	)
	return cmd
}

func NewScreenshotsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("screenshots", "Run screenshot operations")
	cmd.AddCommand(newNativeStartCommand(f, startSpec{OperationKey: "screenshots.url", Use: "url", Short: "Start a URL screenshot operation", Example: "  chab screenshots url --url https://example.test --full-page --yes", Fields: []fieldFlag{
		{Name: "url", Field: "url", Kind: valueString, Usage: "URL to capture"},
		{Name: "viewport", Field: "viewport", Kind: valueJSON, Usage: "viewport object as JSON"},
		{Name: "full-page", Field: "full_page", Kind: valueBool, Usage: "capture the full page"},
		{Name: "element-selector", Field: "element_selector", Kind: valueString, Usage: "CSS selector to capture"},
		{Name: "format", Field: "format", Kind: valueString, Usage: "image format"},
		{Name: "quality", Field: "quality", Kind: valueInt, Usage: "image quality"},
	}}))
	return cmd
}

func NewConvertCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("convert", "Run conversion operations")
	cmd.AddCommand(newNativeStartCommand(f, startSpec{OperationKey: "convert.file", Use: "file", Short: "Start a file conversion operation", Example: "  chab convert file --source '{\"type\":\"file_id\",\"id\":\"fil_123\"}' --output-format markdown --yes", Fields: []fieldFlag{
		{Name: "source", Field: "source", Kind: valueJSON, Usage: "source object as JSON"},
		{Name: "output-format", Field: "output_format", Kind: valueString, Usage: "target output format"},
		{Name: "include-source-metadata", Field: "include_source_metadata", Kind: valueBool, Usage: "include source metadata in the result"},
	}}))
	return cmd
}

func NewTranslateCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("translate", "Run translation operations")
	cmd.AddCommand(newNativeStartCommand(f, startSpec{OperationKey: "translate.text_or_document", Use: "text-or-document", Short: "Start a text or document translation", Example: "  chab translate text-or-document --source '{\"type\":\"inline\",\"format\":\"text\",\"content\":\"Hallo\"}' --target-language en --max-credits 25 --yes", Fields: []fieldFlag{
		{Name: "source", Field: "source", Kind: valueJSON, Usage: "source object as JSON"},
		{Name: "target-language", Field: "target_language", Kind: valueString, Usage: "target language"},
		{Name: "source-language", Field: "source_language", Kind: valueString, Usage: "source language"},
		{Name: "style", Field: "style", Kind: valueString, Usage: "translation style"},
		{Name: "formality", Field: "formality", Kind: valueString, Usage: "translation formality"},
	}}))
	return cmd
}

func NewLLMCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("llm", "Run model operations")
	cmd.AddCommand(
		newModelsCommand(f),
		newNativeStartCommand(f, startSpec{OperationKey: "llm.generate", Use: "generate", Short: "Start an LLM generation operation", Example: "  chab llm generate --model fast-chat --prompt 'Write a haiku' --max-credits 20 --yes", Fields: []fieldFlag{
			{Name: "model", Field: "model", Kind: valueString, Usage: "model identifier"},
			{Name: "prompt", Field: "prompt", Kind: valueString, Usage: "prompt text"},
			{Name: "messages", Field: "messages", Kind: valueJSON, Usage: "messages array as JSON"},
			{Name: "system", Field: "system", Kind: valueString, Usage: "system instruction"},
			{Name: "temperature", Field: "temperature", Kind: valueJSON, Usage: "temperature JSON number"},
			{Name: "max-output-tokens", Field: "max_output_tokens", Kind: valueInt, Usage: "maximum output tokens"},
		}}),
		newNativeStartCommand(f, startSpec{OperationKey: "llm.embeddings", Use: "embeddings", Short: "Start an embeddings operation", Example: "  chab llm embeddings --model embedding-small --input-json '[\"hello\"]' --max-credits 10 --yes", Fields: []fieldFlag{
			{Name: "model", Field: "model", Kind: valueString, Usage: "model identifier"},
			{Name: "input-json", Field: "input", Kind: valueJSON, Usage: "embedding input as JSON"},
		}}),
	)
	return cmd
}

func NewResearchCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := family("research", "Run research operations")
	cmd.AddCommand(newNativeStartCommand(f, startSpec{OperationKey: "research.deep", Use: "deep", Short: "Start a deep research operation", Example: "  chab research deep --question 'Market size?' --mode standard --source-budget 8 --max-credits 100 --yes", Fields: []fieldFlag{
		{Name: "question", Field: "question", Kind: valueString, Usage: "research question"},
		{Name: "mode", Field: "mode", Kind: valueString, Usage: "research mode"},
		{Name: "source-budget", Field: "source_budget", Kind: valueInt, Usage: "source budget"},
		{Name: "report-format", Field: "report_format", Kind: valueString, Usage: "report format"},
	}}))
	return cmd
}

func family(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  short + ".\n\nRelated commands:\n  chab operations",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
}

func newNativeStartCommand(f *cmdutil.Factory, spec startSpec) *cobra.Command {
	flags := &requestFlags{}
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: withPreviewLabel(spec.OperationKey, spec.Short),
		Long: fmt.Sprintf(`%s.

This command resolves convenience flags and --input into one request body,
creates a private action record before live submission, and sends exactly one
idempotency key for the logical action. Use --dry-run for a local preview; to
request a server dry run, include "dry_run": true in the JSON body when the
operation supports it.

Paid live submissions require confirmation through --yes or an interactive
prompt. JSON output is one stable object with action, remote operation and
recovery fields. Use --plain to print the accepted operation id or action id.
In --no-prompt or non-interactive mode, use --yes to acknowledge paid live
work. When the operation schema supports dry_run, an input body with
"dry_run": true performs authenticated server validation without creating a
local action record or sending an idempotency key. Pass --idempotency-key to
supply a caller-owned replay key for live work.

Related commands:
  chab operations schema %s
  chab operations resume`, spec.Short, spec.OperationKey),
		Args:    cobra.NoArgs,
		Example: spec.Example,
		RunE: func(cmd *cobra.Command, _ []string) error {
			op, err := operationByKey(spec.OperationKey)
			if err != nil {
				return err
			}
			if err := ensureStartable(op); err != nil {
				return err
			}
			request, err := resolveRequest(cmd, f, flags, op.ID)
			if err != nil {
				return err
			}
			out, _, err := startOperation(cmd, f, op, request, flags)
			if flags.OfflinePreview {
				return err
			}
			if err != nil && !hasCommandOutput(out) {
				return err
			}
			if writeErr := writeCommandOutput(cmd, f, out); writeErr != nil {
				return writeErr
			}
			return err
		},
	}
	registerRequestFlags(cmd, flags, spec.Fields, true)
	return cmd
}

func newModelsCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: withPreviewLabel("llm.models", "List available LLM models"),
		Long: `List available LLM models.

JSON output is the API model catalog. Use --plain for raw catalog output.

Related commands:
  chab llm generate
  chab operations schema llm.models`,
		Args: cobra.NoArgs,
		Example: `  chab llm models
  chab llm models --json
  chab llm models --plain`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			authn, err := resolveAuthContext(cmd, f)
			if err != nil {
				return err
			}
			result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, "llm/models", nil, nil, api.IdempotencyNone)
			if err != nil {
				return err
			}
			return writeRawJSONValue(cmd, f, result.Data, result.Meta)
		},
	}
}

func withPreviewLabel(operationKey, short string) string {
	op, err := operationByKey(operationKey)
	if err == nil && op.Availability == "preview" {
		return short + " (preview)"
	}
	return short
}

func withPreviewNotice(operationKey, long string) string {
	op, err := operationByKey(operationKey)
	if err == nil && op.Availability == "preview" {
		return strings.TrimRight(long, "\n") + "\n\nPreview: this command uses a preview Chab API contract."
	}
	return long
}

func searchFields(includeBlocks bool) []fieldFlag {
	fields := []fieldFlag{
		{Name: "query", Field: "query", Kind: valueString, Usage: "search query"},
		{Name: "engine", Field: "engine", Kind: valueString, Usage: "search engine"},
		{Name: "location", Field: "location", Kind: valueJSON, Usage: "location object as JSON"},
		{Name: "language", Field: "language", Kind: valueString, Usage: "language code"},
		{Name: "device", Field: "device", Kind: valueString, Usage: "device type"},
		{Name: "safe-search", Field: "safe_search", Kind: valueString, Usage: "safe search setting"},
		{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
	}
	if includeBlocks {
		fields = append(fields, fieldFlag{Name: "blocks", Field: "blocks", Kind: valueJSON, Usage: "SERP block selection as JSON"})
	}
	return fields
}

func domainFields(includeBacklinks bool) []fieldFlag {
	fields := []fieldFlag{
		{Name: "domain", Field: "domain", Kind: valueString, Usage: "domain name"},
		{Name: "location", Field: "location", Kind: valueJSON, Usage: "location object as JSON"},
		{Name: "language", Field: "language", Kind: valueString, Usage: "language code"},
		{Name: "include-subdomains", Field: "include_subdomains", Kind: valueBool, Usage: "include subdomains"},
		{Name: "path-scope", Field: "path_scope", Kind: valueString, Usage: "path scope"},
	}
	if includeBacklinks {
		fields = append(fields,
			fieldFlag{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
			fieldFlag{Name: "output-format", Field: "output_format", Kind: valueString, Usage: "output format"},
		)
	}
	return fields
}

func scrapeFields(includeSelectors bool) []fieldFlag {
	fields := []fieldFlag{
		{Name: "url", Field: "url", Kind: valueString, Usage: "URL to scrape"},
		{Name: "rendering", Field: "rendering", Kind: valueString, Usage: "rendering mode"},
		{Name: "timeout-ms", Field: "timeout_ms", Kind: valueInt, Usage: "target timeout in milliseconds"},
		{Name: "wait-until", Field: "wait_until", Kind: valueString, Usage: "browser wait condition"},
		{Name: "wait-for-selector", Field: "wait_for_selector", Kind: valueString, Usage: "CSS selector to wait for"},
		{Name: "viewport", Field: "viewport", Kind: valueJSON, Usage: "viewport object as JSON"},
	}
	if includeSelectors {
		fields = append(fields, fieldFlag{Name: "selectors", Field: "selectors", Kind: valueJSON, Usage: "selector extraction object as JSON"})
	} else {
		fields = append(fields, fieldFlag{Name: "include-raw-html", Field: "include_raw_html", Kind: valueBool, Usage: "include raw HTML artifact"})
	}
	return fields
}

func searchExample(name string) string {
	return fmt.Sprintf("  chab search %s --query 'best coffee grinder' --engine google --location '{\"country\":\"US\"}' --language en --yes\n  chab search %s --input @request.json --yes --json", name, name)
}
