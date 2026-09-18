# ccrec

A CLI tool that converts Claude Code and Codex conversation transcripts (JSONL) into clean, readable Markdown.

## Features

- **Line-oriented parser** — Scans JSONL with a 16 MB per-record limit and skips malformed lines without discarding the rest of the transcript.
- **Smart filtering** — Strips system messages, metadata, API errors, interrupted requests, and empty messages by default.
- **HTML-safe output** — Escapes HTML tags outside fenced code blocks, preventing Markdown renderers from misinterpreting raw HTML in conversation content.
- **Tool use summaries** — Optionally includes concise summaries of tool calls (file paths, commands, grep patterns).
- **Image extraction** — Optionally decodes and saves base64-encoded images from transcripts.
- **Claude Code hook integration** — Runs as a Stop/SessionEnd hook to automatically save conversations to a directory (e.g., an Obsidian vault).
- **Codex hook integration** — Saves the visible conversation from a Codex `SessionEnd` hook without exporting injected instructions or reasoning.

## Installation

### Homebrew (macOS)

```bash
brew install capybara-translation/tap/ccrec
```

Homebrew 6.0+ refuses to load formulae from third-party taps until you trust
them ([Tap Trust](https://docs.brew.sh/Tap-Trust)). If `brew install` or
`brew upgrade` fails with `Refusing to load formula ... from untrusted tap`,
trust the formula first:

```bash
brew trust --formula capybara-translation/tap/ccrec
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

# Select the provider explicitly (auto-detected by default)
ccrec -provider codex rollout.jsonl

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

Codex commonly stores local session transcripts under:

```
~/.codex/sessions/<year>/<month>/<day>/rollout-*.jsonl
```

Codex exposes `transcript_path` to hooks. Its transcript JSON format is not a stable hook interface, so ccrec uses conservative parsing and regression fixtures for supported formats.

For Codex, ccrec selects exactly one visible-message source family to avoid duplicates: current `item_completed` events first, legacy or subagent `user_message` / `agent_message` events second, and `response_item` records only as a fallback. In the fallback path, user content is exported only when metadata explicitly marks it as `user.text` or `user.image`; older transcripts without that visibility metadata cannot safely reconstruct those user messages. Injected instructions, environment data, plugin metadata, permissions, reasoning, and tool output are not exported as conversation text.

With `-images`, Codex images are decoded only from embedded `user.image` data and associated with the corresponding visible `item_completed` message by turn ID and message order. ccrec never reads the `local_image.path` recorded in the transcript, because a modified transcript could otherwise copy an unrelated local file. The decoded bytes are checked against the declared PNG, JPEG, GIF, or WebP media type before being written.

Session IDs are selected in this order: hook input, Codex session metadata, transcript filename, then a stable path-derived fallback. Unsafe filename characters are normalized and hashed.

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
| `-tools`    | Include Claude Code tool use summaries in the output (no effect for Codex) |
| `-all`      | Disable filtering; include all messages  |
| `-images`   | Extract and embed images (requires `-o`)  |
| `-provider <name>` | Input provider: `auto`, `claude`, or `codex` |
| `-strict` | Fail if messages cannot be extracted safely |

When `-o` is used, ccrec writes a complete same-directory temporary file and atomically replaces the destination with mode `0600`; directories it creates use `0700`. A destination symlink is replaced rather than followed, so its target is unchanged. Concurrent writers publish one complete result, but the last rename wins. A process killed before cleanup may leave a `.tmp-*` file, and the containing directory is not fsynced.

## Breaking changes in Codex support

- Conversation order now follows JSONL source order for both providers. Sessions whose timestamps move backwards can differ from older ccrec output; equal timestamps previously did not guarantee source order.
- Hook output names now use the complete collision-resistant session ID instead of the first eight characters. Existing short-ID files remain untouched, so an in-progress session spanning the upgrade can leave both names; rename or remove the older file manually if desired.

## Claude Code Hook Integration

ccrec can run as a [Claude Code hook](https://docs.anthropic.com/en/docs/claude-code/hooks) to automatically save conversations to a directory (e.g., an Obsidian vault) after every response.

```bash
ccrec hook -dir <output-directory>
ccrec hook -base ~/repos -dir <output-directory>
ccrec hook -project my-app -dir <output-directory>
```

The `hook` subcommand:

1. Reads the hook JSON from stdin (`transcript_path`, `session_id`, `cwd`, etc.)
2. Derives the project name from `CLAUDE_PROJECT_DIR` (falling back to cwd)
3. Converts the transcript to Markdown
4. Saves it as `<output-directory>/<project-name>/<date>_<session-id>.md`

The project directory is determined from the `CLAUDE_PROJECT_DIR` environment variable (set automatically by Claude Code), falling back to cwd. With `-base`, the project name is the relative path from the base to the project directory. For example, if the project directory is `~/repos/my-app/backend` and base is `~/repos`, the project name becomes `my-app/backend`. Without `-base`, only the directory basename is used (e.g., `backend`).

`-project` supplies an explicit safe relative project path and takes precedence over automatic project-name derivation. The hook also accepts `-provider`, `-strict`, `-tools`, `-all`, and `-images`; `-tools` is Claude Code-specific and has no effect on Codex transcripts.

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
- Session ID is the full sanitized hook ID, transcript metadata ID, or filename-derived ID (in that order)
- Overwrites the same file on every invocation within a session
- Skips subagent transcripts (only saves the main conversation)
- Skips execution when `stop_hook_active` is true (prevents infinite loops)
- Creates the output directory if it doesn't exist

## Codex Hook Integration

For final session archives, use Codex `SessionEnd`. It runs for the main thread when a session ends and has a maximum command timeout of three seconds. The initial Codex setup intentionally omits `-images` to reduce work within that limit. Add `-images` to the command if image export is needed, then verify the hook still completes within the timeout for your transcript sizes.

Add the following to `~/.codex/hooks.json`, replacing the executable, repository root, and output paths with absolute paths:

```json
{
  "hooks": {
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/ccrec hook -provider codex -base /Users/you/repos -dir /Users/you/Documents/conversations",
            "timeout": 3,
            "statusMessage": "Saving conversation log"
          }
        ]
      }
    ]
  }
}
```

Replace `/path/to/ccrec` with the absolute path to the installed binary. The example repository and output paths must also be replaced with absolute paths for your environment.

New or changed non-managed hooks must be reviewed and trusted before Codex runs them. Open `/hooks` in Codex to review the definition. The hook uses the supplied `session_id`, `transcript_path`, and `cwd`, writes the Markdown atomically, and uses owner-only permissions (`0600` files and `0700` directories). A null, empty, or missing transcript path is treated as a successful no-op.

## Testing

```bash
go test ./...
```

## Acknowledgments

Inspired by [cclog](https://github.com/annenpolka/cclog). See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for license details.

## License

[MIT](LICENSE)
