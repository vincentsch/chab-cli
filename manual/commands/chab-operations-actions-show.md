# chab operations actions show

## Usage

```text
chab operations actions show <action-id> [flags]
```

## Description

Show a public projection of one local action record.

Use --json for structured output and --plain for the action id.

Related commands:
- [chab operations resume](chab-operations-resume.md)
- [chab operations actions list](chab-operations-actions-list.md)

## Examples

```text
  chab operations actions show act_123
  chab operations actions show act_123 --json
  chab operations actions show act_123 --plain
```

## Output modes

human, JSON, plain

## Authentication

Runs without requiring a credential.
