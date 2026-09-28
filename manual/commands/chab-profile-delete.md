# chab profile delete

## Usage

```text
chab profile delete <name> [flags]
```

## Description

Delete a local profile from config.yml and remove its stored credential record when one exists.

This command is local-only. It never revokes a key on the server and never
prints the stored API key. It requires confirmation before any write unless
--yes is supplied. In --no-prompt or non-interactive mode without --yes, it
fails before changing auth.json or config.yml.

When the deleted profile is current, the current profile switches to the first
remaining saved profile in sorted order. If no profiles remain, the CLI-owned
config file is removed. Deleting the last profile fails before confirmation when
removing the file would discard comments or unknown top-level config keys.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
- [chab profile list](chab-profile-list.md)
- [chab config path](chab-config-path.md)

## Examples

```text
  chab profile delete staging
  chab profile delete staging --yes
  chab profile delete staging --yes --plain
```

## Output modes

human, plain

## Authentication

Runs without requiring a credential.

## Help topics

confirmation
