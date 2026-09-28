package operationscmd

import (
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewDriveCommand builds chab drive.
func NewDriveCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("drive", "Work with connected drive files")
	cmd.Long = `Work with connected drive files.

Drive commands use Chab project and connection scoping. Read commands return
provider-neutral metadata. Downloads require an explicit local output path.
Export and write commands use the accepted-operation lifecycle and local action
records. Request the required drive scopes during browser device login, and
approve project access and spending separately. Connected-drive eligibility
and the current connection permissions still apply.

Related commands:
  chab files upload
  chab operations wait
  chab operations artifact download`
	cmd.AddCommand(
		newDriveConnectionsCommand(f),
		newDriveItemsCommand(f),
		newDriveFoldersCommand(f),
		newDriveSearchCommand(f),
		newDrivePermissionsCommand(f),
	)
	return cmd
}

func newDriveConnectionsCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID int64
		Page      cmdutil.CursorPaginationFlags
	}{}
	cmd := &cobra.Command{
		Use:   "connections",
		Short: withPreviewLabel("drive.connections.list", "List drive connections"),
		Long: withPreviewNotice("drive.connections.list", `List drive connections for one project.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab drive items list
  chab drive search`),
		Args: cobra.NoArgs,
		Example: `  chab drive connections --project-id 42
  chab drive connections --project-id 42 --all --json --include-meta
  chab drive connections --project-id 42 --jq '.[0].connection_id'
  chab drive connections --project-id 42 --template '{{range .}}{{.connection_id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cursorListRaw(cmd, f, "drive/connections", projectQuery(flags.ProjectID), &flags.Page)
		},
	}
	addProjectFlag(cmd, &flags.ProjectID)
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newDriveItemsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("items", "Work with drive items")
	cmd.Long = `Work with drive items.

Drive item reads return provider-neutral metadata. Mutations use Chab's
accepted-operation lifecycle, exact revision checks and idempotent recovery.

Related commands:
  chab drive folders create
  chab operations wait`
	cmd.AddCommand(
		newDriveItemsListCommand(f),
		newDriveItemsShowCommand(f),
		newDriveItemsDownloadCommand(f),
		newDriveItemsUploadCommand(f),
		newDriveItemsUpdateCommand(f),
		newDriveItemsExportCommand(f),
		newDriveItemsMoveCommand(f),
		newDriveItemsTrashCommand(f),
	)
	return cmd
}

func newDriveItemsListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
		ParentID     string
		Page         cmdutil.CursorPaginationFlags
	}{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: withPreviewLabel("drive.items.list", "List drive items"),
		Long: withPreviewNotice("drive.items.list", `List drive items at the selected root or under one parent folder.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab drive items show
  chab drive search`),
		Args: cobra.NoArgs,
		Example: `  chab drive items list --project-id 42 --connection-id 7
  chab drive items list --project-id 42 --connection-id 7 --parent-id dri_123 --json
  chab drive items list --project-id 42 --connection-id 7 --jq '.[0].item_id'
  chab drive items list --project-id 42 --connection-id 7 --template '{{range .}}{{.item_id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := connectionQuery(flags.ProjectID, flags.ConnectionID)
			setStringQuery(q, "parent_id", flags.ParentID)
			return cursorListRaw(cmd, f, "drive/items", q, &flags.Page)
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	cmd.Flags().StringVar(&flags.ParentID, "parent-id", "", "drive folder item id")
	registerCursorFlags(cmd, &flags.Page)
	return cmd
}

func newDriveItemsShowCommand(f *cmdutil.Factory) *cobra.Command {
	flags := struct {
		ProjectID    int64
		ConnectionID int64
	}{}
	cmd := &cobra.Command{
		Use:   "show <item-id>",
		Short: withPreviewLabel("drive.items.read", "Show drive item metadata"),
		Long: withPreviewNotice("drive.items.read", `Show drive item metadata.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab drive items download
  chab drive items update`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive items show dri_123 --project-id 42 --connection-id 7
  chab drive items show dri_123 --project-id 42 --connection-id 7 --json
  chab drive items show dri_123 --project-id 42 --connection-id 7 --jq .item_id
  chab drive items show dri_123 --project-id 42 --connection-id 7 --template '{{.item_id}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOnlyRaw(cmd, f, api.Path("drive", "items", args[0]), connectionQuery(flags.ProjectID, flags.ConnectionID))
		},
	}
	addProjectConnectionFlags(cmd, &flags.ProjectID, &flags.ConnectionID)
	return cmd
}

func newDriveItemsDownloadCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &downloadFlags{MaxBytes: defaultTransferMaxBytes}
	query := struct {
		ProjectID    int64
		ConnectionID int64
	}{}
	cmd := &cobra.Command{
		Use:   "download <item-id>",
		Short: withPreviewLabel("drive.items.download", "Download drive item bytes"),
		Long: withPreviewNotice("drive.items.download", `Download one ordinary drive file to an explicit new local path.

Drive downloads may be streamed by Chab or returned as a safe short-lived HTTPS
redirect. Redirect follow-up requests strip Chab authorization and cookies.
Existing output files are refused. When --checksum supplies a sha256:<hex>
value, the file is published only after the completed transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
  chab drive items show
  chab operations artifact download`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive items download dri_123 --project-id 42 --connection-id 7 --output ./report.pdf
  chab drive items download dri_123 --project-id 42 --connection-id 7 -o ./report.pdf --json
  chab drive items download dri_123 --project-id 42 --connection-id 7 -o ./report.pdf --jq .bytes
  chab drive items download dri_123 --project-id 42 --connection-id 7 -o ./report.pdf --template '{{.path}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			authn, err := resolveAPIClient(cmd, f, false)
			if err != nil {
				return err
			}
			q := connectionQuery(query.ProjectID, query.ConnectionID)
			err = downloadToLocalPath(cmd, f, authn.Client, func(w io.Writer) (int64, api.ResponseMeta, error) {
				return authn.Client.DownloadWithOptions(cmd.Context(), api.Path("drive", "items", args[0], "download"), w, api.DownloadOptions{Query: q, MaxBytes: flags.MaxBytes, FollowHTTPSRedirect: true})
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

func newDriveItemsUploadCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "upload",
		Short: withPreviewLabel("drive.items.upload", "Upload a stored source to drive"),
		Long: withPreviewNotice("drive.items.upload", `Upload an available stored file or eligible artifact into drive.

Supply source, name, project_id and connection_id with flags, --input, or
--set. Live requests use an Idempotency-Key and the accepted-operation
lifecycle; use --idempotency-key to supply one or let the CLI generate it. Use
--dry-run for server validation without mutation, and --wait to wait for the
accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab files upload
  chab operations wait`),
		Args: cobra.NoArgs,
		Example: `  chab drive items upload --project-id 42 --connection-id 7 --source '{"type":"file_id","id":"fil_123"}' --name report.pdf --conflict-mode fail
  chab drive items upload --input @drive-upload.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.items.upload")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.items.upload", http.MethodPost, "drive/items", body, flags, false)
		},
	}
	registerDriveMutationFlags(cmd, flags, []fieldFlag{
		{Name: "parent-id", Field: "parent_id", Kind: valueString, Usage: "target parent folder id"},
		{Name: "source", Field: "source", Kind: valueJSON, Usage: `source object, for example {"type":"file_id","id":"fil_123"}`},
		{Name: "name", Field: "name", Kind: valueString, Usage: "drive item name"},
		{Name: "conflict-mode", Field: "conflict_mode", Kind: valueString, Usage: "conflict mode: fail or create_copy"},
	}, true)
	return cmd
}

func newDriveItemsUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "update <item-id>",
		Short: withPreviewLabel("drive.items.update", "Replace drive item bytes"),
		Long: withPreviewNotice("drive.items.update", `Replace one drive file's bytes with an available stored file or eligible artifact.

Supply source, expected_revision, project_id and connection_id with flags,
--input, or --set. Live requests use an Idempotency-Key; use
--idempotency-key to supply one or let the CLI generate it. They require
confirmation through --yes or an interactive prompt because they change remote
file content. In --no-prompt mode, --yes is required. Use --dry-run for server
validation without mutation, and --wait to wait for the accepted operation to
finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab drive items show
  chab operations wait`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive items update dri_123 --project-id 42 --connection-id 7 --source '{"type":"file_id","id":"fil_123"}' --expected-revision rev_1 --yes
  chab drive items update dri_123 --input @drive-update.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.items.update")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.items.update", http.MethodPatch, api.Path("drive", "items", args[0]), body, flags, true)
		},
	}
	registerDriveMutationFlags(cmd, flags, []fieldFlag{
		{Name: "source", Field: "source", Kind: valueJSON, Usage: `source object, for example {"type":"file_id","id":"fil_123"}`},
		{Name: "expected-revision", Field: "expected_revision", Kind: valueString, Usage: "expected item revision"},
	}, true)
	return cmd
}

func newDriveItemsExportCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "export <item-id>",
		Short: withPreviewLabel("drive.items.export", "Export a drive-native document"),
		Long: withPreviewNotice("drive.items.export", `Export a drive-native document to a Chab operation artifact.

Supply project_id, connection_id and output_format with flags, --input, or
--set. Live requests use an Idempotency-Key and the accepted-operation
lifecycle; use --idempotency-key to supply one or let the CLI generate it. Use
--wait to wait for the accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab operations wait
  chab operations artifact download`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive items export dri_123 --project-id 42 --connection-id 7 --output-format pdf
  chab drive items export dri_123 --input @drive-export.json --wait --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.items.export")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.items.export", http.MethodPost, api.Path("drive", "items", args[0], "export"), body, flags, false)
		},
	}
	registerJSONRequestFlags(cmd, flags, []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric drive connection id"},
		{Name: "output-format", Field: "output_format", Kind: valueString, Usage: "export format: pdf, text, html, or markdown"},
	}, true, true, false)
	return cmd
}

func newDriveItemsMoveCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "move <item-id>",
		Short: withPreviewLabel("drive.items.move", "Move or rename a drive item"),
		Long: withPreviewNotice("drive.items.move", `Move or rename a drive item.

Supply target_parent_id, expected_revision, project_id and connection_id with
flags, --input, or --set. Live requests use an Idempotency-Key; use
--idempotency-key to supply one or let the CLI generate it. They require
confirmation through --yes or an interactive prompt because they change remote
parentage or names. In --no-prompt mode, --yes is required. Use --dry-run for
server validation without mutation, and --wait to wait for the accepted
operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab drive items show
  chab operations wait`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive items move dri_123 --project-id 42 --connection-id 7 --target-parent-id dri_456 --expected-revision rev_1 --yes
  chab drive items move dri_123 --input @drive-move.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.items.move")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.items.move", http.MethodPost, api.Path("drive", "items", args[0], "move"), body, flags, true)
		},
	}
	registerDriveMutationFlags(cmd, flags, []fieldFlag{
		{Name: "target-parent-id", Field: "target_parent_id", Kind: valueString, Usage: "target parent folder id"},
		{Name: "name", Field: "name", Kind: valueString, Usage: "new drive item name"},
		{Name: "expected-revision", Field: "expected_revision", Kind: valueString, Usage: "expected item revision"},
	}, true)
	return cmd
}

func newDriveItemsTrashCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "trash <item-id>",
		Short: withPreviewLabel("drive.items.trash", "Move a drive item to trash"),
		Long: withPreviewNotice("drive.items.trash", `Move a drive item to the connected provider trash.

Supply expected_revision, project_id and connection_id with flags, --input, or
--set. Live requests use an Idempotency-Key; use --idempotency-key to supply
one or let the CLI generate it. They require confirmation through --yes or an
interactive prompt because they change destructive state. In --no-prompt mode,
--yes is required. Use --dry-run for server validation without mutation, and
--wait to wait for the accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab drive items show
  chab operations wait`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive items trash dri_123 --project-id 42 --connection-id 7 --expected-revision rev_1 --yes
  chab drive items trash dri_123 --input @drive-trash.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.items.trash")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.items.trash", http.MethodPost, api.Path("drive", "items", args[0], "trash"), body, flags, true)
		},
	}
	registerDriveMutationFlags(cmd, flags, []fieldFlag{
		{Name: "expected-revision", Field: "expected_revision", Kind: valueString, Usage: "expected item revision"},
	}, true)
	return cmd
}

func newDriveFoldersCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("folders", "Work with drive folders")
	cmd.Long = `Work with drive folders.

Folder creation uses Chab's accepted-operation lifecycle and can be validated
with server dry-run before creating provider data.

Related commands:
  chab drive items list
  chab operations wait`
	cmd.AddCommand(newDriveFolderCreateCommand(f))
	return cmd
}

func newDriveFolderCreateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: withPreviewLabel("drive.folders.create", "Create a drive folder"),
		Long: withPreviewNotice("drive.folders.create", `Create a drive folder.

Supply name, conflict_mode, project_id and connection_id with flags, --input,
or --set. Live requests use an Idempotency-Key and the accepted-operation
lifecycle; use --idempotency-key to supply one or let the CLI generate it. Use
--dry-run for server validation without mutation, and --wait to wait for the
accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab drive items list
  chab operations wait`),
		Args: cobra.NoArgs,
		Example: `  chab drive folders create --project-id 42 --connection-id 7 --name Reports --conflict-mode fail
  chab drive folders create --input @drive-folder.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.folders.create")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.folders.create", http.MethodPost, "drive/folders", body, flags, false)
		},
	}
	registerDriveMutationFlags(cmd, flags, []fieldFlag{
		{Name: "parent-id", Field: "parent_id", Kind: valueString, Usage: "target parent folder id"},
		{Name: "name", Field: "name", Kind: valueString, Usage: "folder name"},
		{Name: "conflict-mode", Field: "conflict_mode", Kind: valueString, Usage: "conflict mode: fail or create_copy"},
	}, true)
	return cmd
}

func newDriveSearchCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "search",
		Short: withPreviewLabel("drive.search", "Search drive item names"),
		Long: withPreviewNotice("drive.search", `Search connected drive item names.

Supply project_id, connection_id and query with flags, --input, or --set.
Search is a synchronous read and does not use --idempotency-key or --dry-run.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab drive items show
  chab drive items list`),
		Args: cobra.NoArgs,
		Example: `  chab drive search --project-id 42 --connection-id 7 --query report
  chab drive search --input @drive-search.json --json --include-meta
  chab drive search --project-id 42 --connection-id 7 --query report --jq '.[0].item_id'
  chab drive search --project-id 42 --connection-id 7 --query report --template '{{range .}}{{.item_id}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.search")
			if err != nil {
				return err
			}
			return runJSONRead(cmd, f, http.MethodPost, "drive/search", body)
		},
	}
	registerJSONRequestFlags(cmd, flags, []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric drive connection id"},
		{Name: "query", Field: "query", Kind: valueString, Usage: "literal name query"},
		{Name: "limit", Field: "limit", Kind: valueInt, Usage: "result limit"},
		{Name: "cursor", Field: "cursor", Kind: valueString, Usage: "opaque cursor"},
	}, false, false, false)
	return cmd
}

func newDrivePermissionsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := connectedFamily("permissions", "Work with drive permissions")
	cmd.Long = `Work with drive permissions.

Permission updates use Chab's supported grant/update/revoke shape only. Public
links, inherited permissions, ownership transfer and provider-native sharing
options are not exposed.

Related commands:
  chab drive items show
  chab operations wait`
	cmd.AddCommand(newDrivePermissionUpdateCommand(f))
	return cmd
}

func newDrivePermissionUpdateCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &jsonRequestFlags{}
	cmd := &cobra.Command{
		Use:   "update <item-id>",
		Short: withPreviewLabel("drive.permissions.update", "Update drive item permissions"),
		Long: withPreviewNotice("drive.permissions.update", `Update direct drive item permissions.

Supply action, principal, role when required, expected_permission_revision,
project_id and connection_id with flags, --input, or --set. Live requests use
an Idempotency-Key; use --idempotency-key to supply one or let the CLI
generate it. They require confirmation through --yes or an interactive prompt
because they change sharing. In --no-prompt mode, --yes is required. Use
--dry-run for server validation without mutation, and --wait to wait for the
accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
  chab drive items show
  chab operations wait`),
		Args: cobra.ExactArgs(1),
		Example: `  chab drive permissions update dri_123 --project-id 42 --connection-id 7 --action update --principal '{"type":"user","email":"ada@example.com"}' --role viewer --expected-permission-revision perm_1 --yes
  chab drive permissions update dri_123 --input @drive-permission.json --dry-run --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := resolveJSONRequest(cmd, f, flags, "drive.permissions.update")
			if err != nil {
				return err
			}
			return runRecoverableJSONAction(cmd, f, "drive.permissions.update", http.MethodPost, api.Path("drive", "items", args[0], "permissions"), body, flags, true)
		},
	}
	registerDriveMutationFlags(cmd, flags, []fieldFlag{
		{Name: "action", Field: "action", Kind: valueString, Usage: "permission action: grant, update, or revoke"},
		{Name: "principal", Field: "principal", Kind: valueJSON, Usage: `principal object, for example {"type":"user","email":"ada@example.com"}`},
		{Name: "role", Field: "role", Kind: valueString, Usage: "role for grant or update: viewer, commenter, or editor"},
		{Name: "expected-permission-revision", Field: "expected_permission_revision", Kind: valueString, Usage: "expected permission revision"},
	}, true)
	return cmd
}

func registerDriveMutationFlags(cmd *cobra.Command, flags *jsonRequestFlags, fields []fieldFlag, dryRun bool) {
	base := []fieldFlag{
		{Name: "project-id", Field: "project_id", Kind: valueInt, Usage: "numeric Chab project id"},
		{Name: "connection-id", Field: "connection_id", Kind: valueInt, Usage: "numeric drive connection id"},
	}
	registerJSONRequestFlags(cmd, flags, append(base, fields...), true, true, dryRun)
}
