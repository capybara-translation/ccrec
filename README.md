# ccrec

A CLI tool that converts [Claude Code](https://docs.anthropic.com/en/docs/claude-code) conversation transcripts (JSONL) into clean, readable Markdown.

## Features

- **Streaming parser** — Processes JSONL line-by-line with a 16 MB buffer; handles transcripts of any size without loading them entirely into memory.
- **Smart filtering** — Strips system messages, metadata, API errors, interrupted requests, and empty messages by default.
- **HTML-safe output** — Escapes HTML tags outside fenced code blocks, preventing Markdown renderers from misinterpreting raw HTML in conversation content.
- **Tool use summaries** — Optionally includes concise summaries of tool calls (file paths, commands, grep patterns).
- **Image extraction** — Optionally decodes and saves base64-encoded images from transcripts.
- **Claude Code hook integration** — Runs as a Stop/SessionEnd hook to automatically save conversations to a directory (e.g., an Obsidian vault).

## Installation

### Homebrew (macOS)

```bash
brew install capybara-translation/tap/ccrec
```

### go install

Requires Go 1.25+.

```bash
go install github.com/capybara-translation/ccrec/cmd/ccrec@latest
```

### Build from source

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

# Extract and embed images (requires -o)
ccrec -images -o output.md session.jsonl
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
| `-tools`    | Include tool use summaries in the output |
| `-all`      | Disable filtering; include all messages  |
| `-images`   | Extract and embed images (requires `-o`)  |

## Claude Code Hook Integration

ccrec can run as a [Claude Code hook](https://docs.anthropic.com/en/docs/claude-code/hooks) to automatically save conversations to a directory (e.g., an Obsidian vault) after every response.

```bash
ccrec hook -dir <output-directory>
ccrec hook -base ~/repos -dir <output-directory>
```

The `hook` subcommand:

1. Reads the hook JSON from stdin (`transcript_path`, `session_id`, `cwd`, etc.)
2. Derives the project name from `CLAUDE_PROJECT_DIR` (falling back to cwd)
3. Converts the transcript to Markdown
4. Saves it as `<output-directory>/<project-name>/<date>_<session-id>.md`

The project directory is determined from the `CLAUDE_PROJECT_DIR` environment variable (set automatically by Claude Code), falling back to cwd. With `-base`, the project name is the relative path from the base to the project directory. For example, if the project directory is `~/repos/my-app/backend` and base is `~/repos`, the project name becomes `my-app/backend`. Without `-base`, only the directory basename is used (e.g., `backend`).

### Setup

Add the following to your Claude Code settings (`~/.claude/settings.json`):

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/ccrec hook -images -base <your-repos-root> -dir <output-directory>"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/ccrec hook -images -base <your-repos-root> -dir <output-directory>"
          }
        ]
      }
    ]
  }
}
```

Replace the placeholders: `/path/to/ccrec` with the actual binary path, `<your-repos-root>` with your repositories root (e.g., `~/repos`), and `<output-directory>` with where you want the Markdown files saved (e.g., `~/Documents/obsidian/vault/projects`).

### Behavior

- Saves to `<output-directory>/<project-name>/<date>_<session-id>.md`
- Project directory is `CLAUDE_PROJECT_DIR` if set, otherwise cwd
- With `-base`, project name is the relative path from base to the project directory (e.g., `my-app/backend`)
- Without `-base`, project name is the project directory basename (e.g., `backend`)
- Date is derived from the first message timestamp (stable across midnight)
- Session ID is the first 8 characters of the transcript filename
- Overwrites the same file on every invocation within a session
- Skips subagent transcripts (only saves the main conversation)
- Skips execution when `stop_hook_active` is true (prevents infinite loops)
- Creates the output directory if it doesn't exist

## Testing

```bash
go test ./...
```

## Acknowledgments

Inspired by [cclog](https://github.com/annenpolka/cclog). See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for license details.

## License

[MIT](LICENSE)
