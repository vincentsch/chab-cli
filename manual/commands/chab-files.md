# chab files

## Usage

```text
chab files [flags]
```

## Description

Work with stored files.

Stored files are private Chab inputs for conversion and connected-drive
workflows. Uploads are scanned asynchronously; use files wait before using a
new file in another operation when the server reports scan_pending. Request
file read/write and any required operation scopes during browser device login.
Project access and spending require separate approval.

Related commands:
- [chab convert file](chab-convert-file.md)
- [chab drive items upload](chab-drive-items-upload.md)
- [chab operations artifact download](chab-operations-artifact-download.md)

## Subcommands

- [chab files delete](chab-files-delete.md) - Delete a stored file (preview)
- [chab files download](chab-files-download.md) - Download stored file bytes (preview)
- [chab files list](chab-files-list.md) - List stored files (preview)
- [chab files resume](chab-files-resume.md) - Resume a stored-file upload (preview)
- [chab files show](chab-files-show.md) - Show stored file metadata (preview)
- [chab files upload](chab-files-upload.md) - Upload a stored file (preview)
- [chab files wait](chab-files-wait.md) - Wait for stored file readiness (preview)

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
