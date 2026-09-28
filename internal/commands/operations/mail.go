package operationscmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewMailCommand builds chab mail.
func NewMailCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("mail", "Work with connected mail")
	cmd.Long = `Work with connected mail.

Chab Mail commands expose the documented synchronous Mail API only. They do
not add inherited send-status, scheduling, browser-draft adoption, management
approval, arbitrary local attachment upload, or provider-native controls.
Request the required mail scopes during browser device login. The team must
also have mail access, an eligible connection and the required project grants.

Related commands:
  chab setup
  chab files
  chab operations actions`
	cmd.AddCommand(
		newMailConnectionsCommand(f),
		newMailFoldersCommand(f),
		newMailPersonasCommand(f),
		newMailThreadsCommand(f),
		newMailSearchCommand(f),
		newMailDraftsCommand(f),
		newMailMessagesCommand(f),
		newMailAttachmentsCommand(f),
	)
	return cmd
}

func newMailConnectionsCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID int64
		Page      cmdutil.CursorPaginationFlags
	}{}
	cmd := &cobra.Command{
		Use:   "connections",
		Short: withPreviewLabel("mail.connections.list", "List mail connections"),
		Long: withPreviewNotice("mail.connections.list", `List mail connections for one project.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail folders
  chab mail personas`),
		Args: cobra.NoArgs,
		Example: `  chab mail connections --project-id 42
  chab mail connections --project-id 42 --all --json --include-meta
  chab mail connections --project-id 42 --jq '.[0].connection_id'
  chab mail connections --project-id 42 --template '{{range .}}{{.connection_id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cursorListRaw(cmd, f, "mail/connections", projectQuery(flags.ProjectID), &flags.Page)
		},
	}
	addProjectFlag(cmd, &flags.ProjectID)
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newMailFoldersCommand(f *cmdutil.Factory) *cobra.Command {
	return newMailCollectionCommand(f, "folders", "List mail folders", "mail/folders", "chab mail threads")
}

func newMailPersonasCommand(f *cmdutil.Factory) *cobra.Command {
	return newMailCollectionCommand(f, "personas", "List mail personas", "mail/personas", "chab mail drafts send")
}

func mailCollectionIDField(use string) string {
	switch use {
	case "folders":
		return "folder_id"
	case "personas":
		return "persona_id"
	default:
		return "id"
	}
}

func mailCollectionOperationKey(use string) string {
	switch use {
	case "folders":
		return "mail.folders.list"
	case "personas":
		return "mail.personas.list"
	default:
		return ""
	}
}

func newMailCollectionCommand(f *cmdutil.Factory, use, short, path, related string) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
		Page         cmdutil.CursorPaginationFlags
	}{}
	operationKey := mailCollectionOperationKey(use)
	cmd := &cobra.Command{
		Use:   use,
		Short: withPreviewLabel(operationKey, short),
		Long: withPreviewNotice(operationKey, short+`.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  `+related+`
  chab mail connections`),
		Args: cobra.NoArgs,
		Example: `  chab mail ` + use + ` --project-id 42 --connection-id 7
  chab mail ` + use + ` --project-id 42 --connection-id 7 --json
  chab mail ` + use + ` --project-id 42 --connection-id 7 --jq '.[0].` + mailCollectionIDField(use) + `'
  chab mail ` + use + ` --project-id 42 --connection-id 7 --template '{{range .}}{{.` + mailCollectionIDField(use) + `}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cursorListRaw(cmd, f, path, connectionQuery(flags.ProjectID, flags.ConnectionID), &flags.Page)
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newMailThreadsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("threads", "Work with mail threads")
	cmd.Long = `Work with mail threads.

Thread list and show commands return draft-free Chab message projections for a
selected project and connection.

Related commands:
  chab mail search
  chab mail messages body`
	cmd.AddCommand(newMailThreadsListCommand(f), newMailThreadsShowCommand(f))
	return cmd
}

func newMailThreadsListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
		FolderID     int64
		Page         cmdutil.CursorPaginationFlags
	}{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: withPreviewLabel("mail.threads.list", "List mail threads"),
		Long: withPreviewNotice("mail.threads.list", `List mail threads in the persisted Inbox or one selected folder.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail threads show
  chab mail search`),
		Args: cobra.NoArgs,
		Example: `  chab mail threads list --project-id 42 --connection-id 7
  chab mail threads list --project-id 42 --connection-id 7 --folder-id 3 --json
  chab mail threads list --project-id 42 --connection-id 7 --jq '.[0].thread_id'
  chab mail threads list --project-id 42 --connection-id 7 --template '{{range .}}{{.thread_id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := connectionQuery(flags.ProjectID, flags.ConnectionID)
			setIntQuery(q, "folder_id", flags.FolderID, cmd.Flags().Changed("folder-id"))
			return cursorListRaw(cmd, f, "mail/threads", q, &flags.Page)
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	cmd.Flags().Int64Var(&flags.FolderID, "folder-id", 0, "numeric folder id")
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newMailThreadsShowCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
		Page         cmdutil.CursorPaginationFlags
	}{}
	cmd := &cobra.Command{
		Use:   "show <thread-id>",
		Short: withPreviewLabel("mail.threads.read", "Show a mail thread"),
		Long: withPreviewNotice("mail.threads.read", `Show a mail thread and its draft-free messages.

Pagination: --limit caps the total messages fetched, --cursor starts at an
opaque API cursor, --page-size sets the per-request page size, and --all
follows every cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail threads list
  chab mail messages body`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail threads show 88 --project-id 42 --connection-id 7
  chab mail threads show 88 --project-id 42 --connection-id 7 --json --include-meta
  chab mail threads show 88 --project-id 42 --connection-id 7 --jq .thread.thread_id
  chab mail threads show 88 --project-id 42 --connection-id 7 --template '{{.thread.thread_id}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mailThreadShowRaw(cmd, f, api.Path("mail", "threads", args[0]), connectionQuery(flags.ProjectID, flags.ConnectionID), &flags.Page)
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func mailThreadShowRaw(cmd *cobra.Command, f *cmdutil.Factory, path string, baseQuery url.Values, flags *cmdutil.CursorPaginationFlags) error {
	plan, err := cmdutil.ResolveCursorListPlan(cmd, flags)
	if err != nil {
		return &usageError{detail: strings.TrimPrefix(err.Error(), "invalid list options: ")}
	}
	authn, err := resolveAPIClient(cmd, f, false)
	if err != nil {
		return err
	}
	var thread json.RawMessage
	outcome, err := cmdutil.FetchCursorPages[json.RawMessage](plan, func(cursor string, limit int) ([]json.RawMessage, api.ResponseMeta, error) {
		q := cloneValues(baseQuery)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		result, err := authn.Client.DoRaw(cmd.Context(), http.MethodGet, path, q, nil, api.IdempotencyNone)
		if err != nil {
			return nil, api.ResponseMeta{}, err
		}
		pageThread, messages, err := decodeMailThreadPage(result.Data, result.Meta)
		if err != nil {
			return nil, result.Meta, err
		}
		if len(thread) == 0 {
			thread = append(json.RawMessage(nil), pageThread...)
		}
		return messages, result.Meta, nil
	})
	if err != nil {
		return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
	}
	data, err := json.Marshal(struct {
		Thread   json.RawMessage   `json:"thread"`
		Messages []json.RawMessage `json:"messages"`
	}{
		Thread:   thread,
		Messages: outcome.Rows,
	})
	if err != nil {
		return err
	}
	hint := cmdutil.CursorPaginationHint(plan, len(outcome.Rows), outcome.Meta.CursorPagination)
	raw := json.RawMessage(data)
	return f.WriteResultWithMeta(cmd, raw, outcome.Meta, true, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			output.RawValueHuman(w, raw)
			if hint != "" {
				fmt.Fprintln(w, hint)
			}
		},
		Plain: func(dataOut, prose io.Writer) {
			output.RawValuePlain(dataOut, prose, raw)
			if hint != "" {
				fmt.Fprintln(prose, hint)
			}
		},
	})
}

func decodeMailThreadPage(raw json.RawMessage, meta api.ResponseMeta) (json.RawMessage, []json.RawMessage, error) {
	var page struct {
		Thread   json.RawMessage   `json:"thread"`
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, nil, &api.ProtocolError{Detail: "mail thread response data does not match the expected shape", Status: meta.HTTPStatus, RequestID: meta.RequestID, Err: meta.RedactError(err), Meta: meta}
	}
	if len(page.Thread) == 0 || string(page.Thread) == "null" || page.Messages == nil {
		return nil, nil, &api.ProtocolError{Detail: "mail thread response missing thread or messages", Status: meta.HTTPStatus, RequestID: meta.RequestID, Meta: meta}
	}
	return page.Thread, page.Messages, nil
}

func newMailSearchCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "search",
		Short: withPreviewLabel("mail.search", "Search connected mail"),
		Long: withPreviewNotice("mail.search", `Search connected mail with Chab's closed JSON request shape.

Supply --query, --filters as JSON, --input, or --set values. Search is a
synchronous read and does not use --idempotency-key or --dry-run.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail threads show
  chab mail folders`),
		Args: cobra.NoArgs,
		Example: `  chab mail search --project-id 42 --connection-id 7 --query quarterly
  chab mail search --input @mail-search.json --json --include-meta
  chab mail search --project-id 42 --connection-id 7 --query quarterly --jq '.[0].thread.thread_id'
  chab mail search --project-id 42 --connection-id 7 --query quarterly --template '{{range .}}{{.thread.thread_id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "mail.search")
			if err != nil {
				return err
			}
			return runJSONRead(cmd, f, http.MethodPost, "mail/search", body)
		},
	}
	registerJSONRequestFlags(cmd, flags, []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric mail connection id"},
		{Name: "folder-id", Field: "folder_id", Kind: valueInt, Usage: "numeric folder id"},
		{Name: "query", Field: "query", Kind: valueString, Usage: "literal search query"},
		{Name: "filters", Field: "filters", Kind: valueJSON, Usage: "filter object as JSON"},
		{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
		{Name: "cursor", Field: "cursor", Kind: valueString, Usage: "opaque cursor"},
	}, false, false, false)
	return cmd
}

func newMailDraftsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("drafts", "Work with API-owned mail drafts")
	cmd.Long = `Work with API-owned mail drafts.

Draft mutations are synchronous Chab API calls. Live mutations use one
Idempotency-Key per intended change, and --dry-run asks the server to validate
without creating or changing provider data. Draft attachments accept eligible
operation artifact IDs only.

Related commands:
  chab mail personas
  chab operations artifact`
	cmd.AddCommand(
		newMailDraftCreateCommand(f),
		newMailDraftShowCommand(f),
		newMailDraftUpdateCommand(f),
		newMailDraftDiscardCommand(f),
		newMailDraftAttachmentsCommand(f),
		newMailDraftSendCommand(f),
	)
	return cmd
}

func newMailDraftCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: withPreviewLabel("mail.drafts.create", "Create a mail draft"),
		Long: withPreviewNotice("mail.drafts.create", `Create a Chab API-owned mail draft.

Supply composition with convenience flags, --input, or --set. Recipient arrays
and body are exact JSON values. Live requests use an Idempotency-Key; use
--idempotency-key to supply one or let the CLI generate it. Use --dry-run for
server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
  chab mail drafts show
  chab mail drafts send`),
		Args: cobra.NoArgs,
		Example: `  chab mail drafts create --project-id 42 --connection-id 7 --mode new --to '[{"address":"ada@example.com"}]' --subject Update --body '{"format":"plain","content":"Hello"}'
  chab mail drafts create --input @draft.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "mail.drafts.create")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "mail.drafts.create", http.MethodPost, "mail/drafts", body, flags, false)
		},
	}
	registerMailCompositionFlags(cmd, flags, true)
	return cmd
}

func newMailDraftShowCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
	}{}
	cmd := &cobra.Command{
		Use:   "show <draft-id>",
		Short: withPreviewLabel("mail.drafts.read", "Show a mail draft"),
		Long: withPreviewNotice("mail.drafts.read", `Show a Chab API-owned mail draft.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail drafts update
  chab mail drafts send`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail drafts show 21 --project-id 42 --connection-id 7
  chab mail drafts show 21 --project-id 42 --connection-id 7 --json
  chab mail drafts show 21 --project-id 42 --connection-id 7 --jq .draft_id
  chab mail drafts show 21 --project-id 42 --connection-id 7 --template '{{.draft_id}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOnlyRaw(cmd, f, api.Path("mail", "drafts", args[0]), connectionQuery(flags.ProjectID, flags.ConnectionID))
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	return cmd
}

func newMailDraftUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "update <draft-id>",
		Short: withPreviewLabel("mail.drafts.update", "Update a mail draft"),
		Long: withPreviewNotice("mail.drafts.update", `Update a Chab API-owned mail draft.

Supply expected_draft_revision and changed composition fields with flags,
--input, or --set. Live requests use an Idempotency-Key; use --idempotency-key
to supply one or let the CLI generate it. Use --dry-run for server validation
without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
  chab mail drafts show
  chab mail drafts attachments add`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail drafts update 21 --project-id 42 --connection-id 7 --expected-draft-revision 2 --subject Updated
  chab mail drafts update 21 --input @draft-update.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "mail.drafts.update")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "mail.drafts.update", http.MethodPatch, api.Path("mail", "drafts", args[0]), body, flags, false)
		},
	}
	registerMailCompositionFlags(cmd, flags, false)
	return cmd
}

func newMailDraftDiscardCommand(f *cmdutil.Factory) *cobra.Command {
	return newMailDraftRevisionCommand(f, "discard", "Discard a mail draft", "mail.drafts.discard", "discard", false)
}

func newMailDraftSendCommand(f *cmdutil.Factory) *cobra.Command {
	return newMailDraftRevisionCommand(f, "send", "Send a mail draft", "mail.messages.send", "send", true)
}

func newMailDraftRevisionCommand(f *cmdutil.Factory, use, short, operationKey, suffix string, requireAck bool) *cobra.Command {
	flags := &jsonRequestFlags{}
	confirmationText := ""
	if requireAck {
		confirmationText = " Send requires local confirmation through --yes or an interactive prompt; in --no-prompt mode, --yes is required."
	}
	cmd := &cobra.Command{
		Use:   use + " <draft-id>",
		Short: withPreviewLabel(operationKey, short),
		Long: withPreviewNotice(operationKey, short+`.

Supply project_id, connection_id, and expected_draft_revision with flags,
--input, or --set. Live requests use an Idempotency-Key; use --idempotency-key
to supply one or let the CLI generate it. Use --dry-run for server validation
without mutation.`+confirmationText+`

Output modes: default human detail, --plain, --json.

Related commands:
  chab mail drafts show
  chab mail drafts update`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail drafts ` + use + ` 21 --project-id 42 --connection-id 7 --expected-draft-revision 3 --idempotency-key draft-` + use + `-001
  chab mail drafts ` + use + ` 21 --input @draft-` + use + `.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, operationKey)
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, operationKey, http.MethodPost, api.Path("mail", "drafts", args[0], suffix), body, flags, requireAck)
		},
	}
	registerJSONRequestFlags(cmd, flags, revisionFields(), true, false, true)
	return cmd
}

func newMailDraftAttachmentsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("attachments", "Work with draft attachments")
	cmd.Long = `Work with draft attachments.

Draft attachments accept eligible Chab operation artifact IDs only. Local files,
URLs, stored-file IDs and provider handles are not accepted by these commands.

Related commands:
  chab operations artifact
  chab mail drafts show`
	cmd.AddCommand(newMailDraftAttachmentAddCommand(f), newMailDraftAttachmentRemoveCommand(f))
	return cmd
}

func newMailDraftAttachmentAddCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "add <draft-id>",
		Short: withPreviewLabel("mail.draft_attachments.add", "Add a draft attachment"),
		Long: withPreviewNotice("mail.draft_attachments.add", `Add one eligible operation artifact to a draft.

Live requests use an Idempotency-Key; use --idempotency-key to supply one or
let the CLI generate it. Use --dry-run for server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
  chab operations artifact
  chab mail drafts attachments remove`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail drafts attachments add 21 --project-id 42 --connection-id 7 --expected-draft-revision 2 --artifact-id art_123
  chab mail drafts attachments add 21 --input @attachment.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "mail.draft_attachments.add")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "mail.draft_attachments.add", http.MethodPost, api.Path("mail", "drafts", args[0], "attachments"), body, flags, false)
		},
	}
	registerJSONRequestFlags(cmd, flags, append(revisionFields(), fieldFlag{Name: "artifact-id", Field: "source", Kind: valueArtifactID, Usage: "eligible operation artifact id"}), true, false, true)
	return cmd
}

func newMailDraftAttachmentRemoveCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "remove <draft-id> <attachment-id>",
		Short: withPreviewLabel("mail.draft_attachments.remove", "Remove a draft attachment"),
		Long: withPreviewNotice("mail.draft_attachments.remove", `Remove one staged draft attachment.

Live requests use an Idempotency-Key; use --idempotency-key to supply one or
let the CLI generate it. Use --dry-run for server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
  chab mail drafts attachments add
  chab mail drafts show`),
		Args: cobra.ExactArgs(2),
		Example: `  chab mail drafts attachments remove 21 staged_123 --project-id 42 --connection-id 7 --expected-draft-revision 3
  chab mail drafts attachments remove 21 staged_123 --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "mail.draft_attachments.remove")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "mail.draft_attachments.remove", http.MethodPost, api.Path("mail", "drafts", args[0], "attachments", args[1], "remove"), body, flags, false)
		},
	}
	registerJSONRequestFlags(cmd, flags, revisionFields(), true, false, true)
	return cmd
}

func newMailMessagesCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("messages", "Work with mail messages")
	cmd.Long = `Work with mail messages.

Message commands read sanitized bodies or apply the documented message-state
actions. State updates are synchronous Chab API calls with revision checks and
server dry-run support.

Related commands:
  chab mail threads
  chab mail attachments download`
	cmd.AddCommand(newMailMessageBodyCommand(f), newMailMessageStateCommand(f))
	return cmd
}

func newMailMessageBodyCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
		Format       string
	}{Format: "plain"}
	cmd := &cobra.Command{
		Use:   "body <message-id>",
		Short: withPreviewLabel("mail.messages.body.read", "Read a mail message body"),
		Long: withPreviewNotice("mail.messages.body.read", `Read one sanitized mail message body.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail threads show
  chab mail attachments download`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail messages body 84 --project-id 42 --connection-id 7
  chab mail messages body 84 --project-id 42 --connection-id 7 --format markdown --json
  chab mail messages body 84 --project-id 42 --connection-id 7 --jq .content
  chab mail messages body 84 --project-id 42 --connection-id 7 --template '{{.content}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := connectionQuery(flags.ProjectID, flags.ConnectionID)
			setStringQuery(q, "format", flags.Format)
			return readOnlyRaw(cmd, f, api.Path("mail", "messages", args[0], "body"), q)
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	cmd.Flags().StringVar(&flags.Format, "format", "plain", "body format: plain, html, or markdown")
	return cmd
}

func newMailMessageStateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "state <message-id>",
		Short: withPreviewLabel("mail.messages.state.update", "Update mail message state"),
		Long: withPreviewNotice("mail.messages.state.update", `Update mail message state with Chab's documented actions.

Supply project_id, connection_id, expected_state_revision, action, and any
required folder fields with flags, --input, or --set. Live requests use an
Idempotency-Key; use --idempotency-key to supply one or let the CLI generate
it. Use --dry-run for server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
  chab mail messages body
  chab mail threads list`),
		Args: cobra.ExactArgs(1),
		Example: `  chab mail messages state 84 --project-id 42 --connection-id 7 --expected-state-revision 4 --action mark_read
  chab mail messages state 84 --input @message-state.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "mail.messages.state.update")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "mail.messages.state.update", http.MethodPost, api.Path("mail", "messages", args[0], "state"), body, flags, false)
		},
	}
	registerJSONRequestFlags(cmd, flags, []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric mail connection id"},
		{Name: "expected-state-revision", Field: "expected_state_revision", Kind: valueInt, Usage: "expected message state revision"},
		{Name: "action", Field: "action", Kind: valueString, Usage: "state action"},
		{Name: "source-folder-id", Field: "source_folder_id", Kind: valueInt, Usage: "source folder id for archive, trash, or move"},
		{Name: "target-folder-id", Field: "target_folder_id", Kind: valueInt, Usage: "target folder id for move"},
	}, true, false, true)
	return cmd
}

func newMailAttachmentsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("attachments", "Work with message attachments")
	cmd.Long = `Work with message attachments.

Message attachments are downloaded as untrusted direct byte streams. The CLI
never uses a server filename as the local path and never overwrites an existing
file.

Related commands:
  chab mail messages body
  chab files download`
	cmd.AddCommand(newMailAttachmentDownloadCommand(f))
	return cmd
}

func newMailAttachmentDownloadCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &downloadFlags{MaxBytes: 25 * 1024 * 1024}
	query := struct {
		ProjectID    int64
		ConnectionID int64
	}{}
	cmd := &cobra.Command{
		Use:   "download <message-id> <attachment-id>",
		Short: withPreviewLabel("mail.attachments.download", "Download a mail attachment"),
		Long: withPreviewNotice("mail.attachments.download", `Download one mail attachment to an explicit new local path.

Mail attachment downloads are direct streams with no redirect support. Existing
output files are refused. When --checksum supplies a sha256:<hex> value, the
file is published only after the completed transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
  chab mail threads show
  chab files download`),
		Args: cobra.ExactArgs(2),
		Example: `  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 --output ./attachment.pdf
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 -o ./attachment.pdf --json
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 -o ./attachment.pdf --jq .bytes
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 -o ./attachment.pdf --template '{{.path}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAPIClient(cmd, f, false)
			if err != nil {
				return err
			}
			q := connectionQuery(query.ProjectID, query.ConnectionID)
			err = downloadToLocalPath(cmd, f, authn.Client, func(w io.Writer) (int64, api.ResponseMeta, error) {
				return authn.Client.Download(cmd.Context(), api.Path("mail", "messages", args[0], "attachments", args[1]), q, w, flags.MaxBytes)
			}, *flags, "")
			if err != nil {
				return output.WithCredentialContext(err, authn.Cred.Profile, authn.Cred.DisplayID)
			}
			return nil
		},
	}
	addProjectConnectionFlags(cmd, &query.ProjectID, &query.ConnectionID)
	registerDownloadFlags(cmd, flags)
	return cmd
}

func registerMailCompositionFlags(cmd *cobra.Command, flags *jsonRequestFlags, create bool) {
	fields := []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric mail connection id"},
		{Name: "persona-id", Field: "persona_id", Kind: valueInt, Usage: "numeric sender persona id"},
		{Name: "to", Field: "to", Kind: valueJSON, Usage: "ordered To recipients array as JSON"},
		{Name: "cc", Field: "cc", Kind: valueJSON, Usage: "ordered Cc recipients array as JSON"},
		{Name: "bcc", Field: "bcc", Kind: valueJSON, Usage: "ordered Bcc recipients array as JSON"},
		{Name: "subject", Field: "subject", Kind: valueString, Usage: "draft subject"},
		{Name: "body", Field: "body", Kind: valueJSON, Usage: `body object, for example {"format":"plain","content":"Hello"}`},
	}
	if create {
		fields = append(fields,
			fieldFlag{Name: "mode", Field: "mode", Kind: valueString, Usage: "draft mode: new or reply"},
			fieldFlag{Name: "reply-to-message-id", Field: "reply_to_message_id", Kind: valueInt, Usage: "message id for reply drafts"},
		)
	} else {
		fields = append(fields, fieldFlag{Name: "expected-draft-revision", Field: "expected_draft_revision", Kind: valueInt, Usage: "expected draft revision"})
	}
	registerJSONRequestFlags(cmd, flags, fields, true, false, true)
}

func revisionFields() []fieldFlag {
	return []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric mail connection id"},
		{Name: "expected-draft-revision", Field: "expected_draft_revision", Kind: valueInt, Usage: "expected draft revision"},
	}
}
