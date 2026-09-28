# chab llm models

## Usage

```text
chab llm models [flags]
```

## Description

List available LLM models.

JSON output is the API model catalog. Use --plain for raw catalog output.

Related commands:
- [chab llm generate](chab-llm-generate.md)

```text
chab operations schema llm.models
```

## Examples

```text
  chab llm models
  chab llm models --json
  chab llm models --plain
```

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.
