# ccrec

A CLI tool that converts Claude Code and Codex conversation transcripts (JSONL) into clean, readable Markdown.

## Features

- **Line-oriented parser** — Scans JSONL with a 16 MiB per-record limit and skips malformed or over-limit lines without discarding the rest of the transcript.
- **Smart filtering** — Strips system messages, metadata, API errors, interrupted requests, and empty messages by default.
- **HTML-safe output** — Escapes HTML tags outside fenced code blocks, preventing Markdown renderers from misinterpreting raw HTML in conversation content.
- **Tool use summaries** — Optionally includes concise summaries of tool calls (file paths, commands, grep patterns).
- **Image extraction** — Optionally decodes and saves base64-encoded images from transcripts.
- **Claude Code hook integration** — Runs as a Stop/SessionEnd hook to automatically save conversations to a directory (e.g., an Obsidian vault).
- **Codex hook integration** — Runs as a Stop/SessionEnd hook without exporting injected instructions or reasoning.

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

For Codex, ccrec selects exactly one visible-message source family to avoid duplicates: current `item_completed` events first, alternate or subagent `user_message` / `agent_message` events second, and `response_item` records only as a fallback. In the fallback path, user content is exported only when metadata explicitly marks it as `user.text` or `user.image`; older transcripts without that visibility metadata cannot safely reconstruct those user messages. Injected instructions, environment data, plugin metadata, permissions, reasoning, and tool output are not exported as conversation text.

With `-images`, Codex images are decoded only from embedded `user.image` data and associated with the corresponding visible `item_completed` message by turn ID and message order. ccrec never reads the `local_image.path` recorded in the transcript, because a modified transcript could otherwise copy an unrelated local file. The decoded bytes are checked against the declared PNG, JPEG, GIF, or WebP media type before being written.

Image association requires `content_item_kinds` metadata. In observed rollouts it was absent through Codex 0.145.0 and present in 0.155.0-alpha; versions 0.146–0.154 were not observed. Without that metadata, images cannot be associated, and `-strict -images` fails if the visible message contains unmatched local-image markers. The observed compaction samples had no association mismatch, but compaction and fork behavior has not been exhaustively verified.

Parsing memory grows with transcript size. The parser retains roughly two copies of the JSONL data before Go runtime overhead; decoded image bytes add further memory only when `-images` is enabled.

Session IDs are selected in this order: hook input, Codex session metadata, transcript filename, then a stable path-derived fallback. When an ID is available, Codex output names use the first eight hexadecimal characters of its SHA-256 digest. This keeps names short without relying on the shared timestamp prefix of Codex UUIDv7 IDs.

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
| `-strict` | Fail if messages or requested images cannot be extracted safely, or if an over-limit record is skipped |

When `-o` is used, ccrec writes complete same-directory temporary files and atomically replaces the Markdown and image destinations with mode `0600`; directories it creates use `0700`, while existing directory modes are unchanged. A destination symlink is replaced rather than followed, and an attachments-directory symlink is rejected. An image save failure aborts the Markdown replacement. Concurrent writers publish one complete result, but the last rename wins. A process killed before cleanup may leave a `.tmp-*` file, and containing directories are not fsynced.

## Behavior and compatibility changes

- Conversation order now follows JSONL source order for both providers. Sessions whose timestamps move backwards can differ from older ccrec output; equal timestamps previously did not guarantee source order.
- Codex hook output names use an eight-character hash of the complete session ID instead of the raw first eight characters. This removes the systematic UUIDv7 timestamp-prefix collision while keeping filenames compact; as with any 32-bit name, accidental hash collisions remain possible.
- Claude Code hook output keeps the legacy first eight characters of its UUID. Full-UUID files created by ccrec v0.12.0 remain untouched and may need to be renamed or removed manually.
- With `-all`, records that have neither renderable text nor requested images are no longer counted or emitted as empty message headings.
- Claude Code image-only user messages are now emitted when `-images` is enabled.

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
- Session ID comes from the hook ID, transcript metadata ID, or filename (in that order). Claude Code UUIDs use their legacy first eight characters; Codex uses an eight-character hash of the complete ID
- Overwrites the same file on every invocation within a session
- Skips subagent transcripts (only saves the main conversation)
- Skips execution when `stop_hook_active` is true (prevents infinite loops)
- Creates the output directory if it doesn't exist

## Codex Hook Integration

ccrec can run from Codex `Stop`, `SessionEnd`, or both. `Stop` updates the same conversation file after every response, while `SessionEnd` updates it when the main thread ends. Repeated invocations are safe because ccrec atomically overwrites the file for the same session.

Add the following to `~/.codex/hooks.json`, replacing the executable, repository root, and output paths with absolute paths:

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/ccrec hook -provider codex -images -base /Users/you/repos -dir /Users/you/Documents/conversations",
            "timeout": 30,
            "statusMessage": "Saving conversation log"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/ccrec hook -provider codex -images -base /Users/you/repos -dir /Users/you/Documents/conversations",
            "timeout": 3,
            "statusMessage": "Saving conversation log"
          }
        ]
      }
    ]
  }
}
```

Replace `/path/to/ccrec` with the absolute path to the installed binary. The example repository and output paths must also be replaced with absolute paths for your environment. You may configure either event instead of both. Codex limits `SessionEnd` command hooks to three seconds, so verify that image extraction completes within that limit for your transcript sizes; remove `-images` from that event if necessary.

New or changed non-managed hooks must be reviewed and trusted before Codex runs them. Open `/hooks` in Codex to review the definition. The hook uses the supplied `session_id`, `transcript_path`, and `cwd`, writes the Markdown atomically, creates files with mode `0600` and new directories with mode `0700`, and leaves existing directory modes unchanged. A null, empty, or missing transcript path is treated as a successful no-op.

Warnings are written to stderr, but Codex and Claude Code normally do not surface stderr when a hook exits successfully. A Codex subagent rollout can also contain no visible-message events and therefore fail with `-strict`; Codex `Stop` and `SessionEnd` hooks do not run for subagents.

## Testing

```bash
go test ./...
```

## Acknowledgments

Inspired by [cclog](https://github.com/annenpolka/cclog). See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for license details.

## License

[MIT](LICENSE)
