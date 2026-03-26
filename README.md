# ccrec

A CLI tool that converts [Claude Code](https://docs.anthropic.com/en/docs/claude-code) conversation transcripts (JSONL) into clean, readable Markdown.

## Features

- **Streaming parser** — Processes JSONL line-by-line with a 16 MB buffer; handles transcripts of any size without loading them entirely into memory.
- **Smart filtering** — Strips system messages, metadata, API errors, interrupted requests, and empty messages by default.
- **HTML-safe output** — Escapes HTML tags outside fenced code blocks, preventing Markdown renderers from misinterpreting raw HTML in conversation content.
- **Tool use summaries** — Optionally includes concise summaries of tool calls (file paths, commands, grep patterns).

## Installation

Requires Go 1.21+.

```bash
go install github.com/capybara-translation/ccrec/cmd/ccrec@latest
```

Or build from source:

```bash
git clone https://github.com/capybara-translation/ccrec.git
cd ccrec
go build -o bin/ccrec ./cmd/ccrec
```

## Usage

```bash
# Output to stdout
ccrec session.jsonl

# Output to a file
ccrec -o output.md session.jsonl

# Include tool use summaries (Read, Bash, Grep, etc.)
ccrec -tools session.jsonl

# Disable filtering (include all messages)
ccrec -all session.jsonl
```

### Where are the transcript files?

Claude Code stores conversation transcripts as JSONL files under:

```
~/.claude/projects/<project-path>/<session-id>.jsonl
```

### Example output

```markdown
# Conversation Log

**File:** `/path/to/session.jsonl`
**Messages:** 42

## User

**Time:** 2026-01-15 10:30:00

What is a knowledge graph?

## Assistant

**Time:** 2026-01-15 10:30:05

A knowledge graph is a data structure that represents information as
entities (nodes) and relationships (edges)...
```

## Options

| Flag | Description |
|------|-------------|
| `-o <path>` | Write output to a file instead of stdout |
| `-tools` | Include tool use summaries in the output |
| `-all` | Disable filtering; include all messages |

## Testing

```bash
go test ./...
```

## License

[MIT](LICENSE)
