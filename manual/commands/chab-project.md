# chab project

## Usage

```text
chab project [flags]
```

## Description

Manage projects visible to the active team API key.

List projects with shared pagination and API-authoritative filters, show one by
an opaque id, create or update projects, use fixed-status pause, resume, and
archive mutations, or delete projects.

Related commands:
- [chab login](chab-login.md)
- [chab whoami](chab-whoami.md)
- [chab doctor](chab-doctor.md)

## Subcommands

- [chab project archive](chab-project-archive.md) - Archive a project
- [chab project create](chab-project-create.md) - Create a project
- [chab project delete](chab-project-delete.md) - Delete a project
- [chab project list](chab-project-list.md) - List projects
- [chab project pause](chab-project-pause.md) - Pause a project
- [chab project resume](chab-project-resume.md) - Resume a project
- [chab project show](chab-project-show.md) - Show a project
- [chab project update](chab-project-update.md) - Update a project

## Examples

```text
  chab project list
  chab project show <project-id>
  chab project create --name "Demo"
  chab project update <project-id> --status paused
  chab project pause <project-id>
  chab project resume <project-id>
  chab project archive <project-id>
  chab project delete <project-id>
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
