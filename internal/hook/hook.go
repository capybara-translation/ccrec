package hook

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/capybara-translation/ccrec/internal/formatter"
	"github.com/capybara-translation/ccrec/internal/parser"
)

// StopHookInput represents the JSON passed via stdin from Claude Code's Stop hook.
type StopHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	StopHookActive bool   `json:"stop_hook_active"`
	CWD            string `json:"cwd"`
}

// Run executes the hook subcommand.
func Run(args []string) {
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	dir := fs.String("dir", "", "Output directory (required)")
	tools := fs.Bool("tools", false, "Include tool use summaries")
	all := fs.Bool("all", false, "Disable filtering (include all messages)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ccrec hook -dir <output-directory>\n\n")
		fmt.Fprintf(os.Stderr, "Run as a Claude Code Stop hook. Reads hook JSON from stdin,\n")
		fmt.Fprintf(os.Stderr, "converts the transcript to Markdown, and saves to the output directory.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *dir == "" {
		fs.Usage()
		os.Exit(1)
	}

	// Expand ~ in dir path.
	outDir := expandHome(*dir)

	// Read stdin JSON.
	var input StopHookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "ccrec hook: failed to read stdin: %v\n", err)
		os.Exit(1)
	}

	// Skip if already in a stop hook chain (prevent infinite loops).
	if input.StopHookActive {
		return
	}

	if input.TranscriptPath == "" {
		fmt.Fprintf(os.Stderr, "ccrec hook: transcript_path is empty\n")
		os.Exit(1)
	}

	// Skip subagent transcripts.
	if strings.Contains(input.TranscriptPath, "/subagents/") {
		return
	}

	// Derive project name from cwd, falling back to transcript path.
	var projectName string
	if input.CWD != "" {
		projectName = filepath.Base(input.CWD)
	} else {
		projectName = ExtractProjectName(input.TranscriptPath)
	}
	if projectName == "" {
		fmt.Fprintf(os.Stderr, "ccrec hook: could not determine project name from cwd=%q transcript=%q\n", input.CWD, input.TranscriptPath)
		os.Exit(1)
	}

	// Parse transcript.
	records, err := parser.ParseFile(input.TranscriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ccrec hook: parse error: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		return
	}

	// Build output path: {dir}/{project}/{date}_{session_id_short}.md
	// Use the first non-zero timestamp (skip file-history-snapshot etc.)
	sessionDate := "unknown"
	for _, rec := range records {
		if !rec.Timestamp.IsZero() {
			sessionDate = rec.Timestamp.Local().Format("2006-01-02")
			break
		}
	}
	sessionID := extractSessionID(input.TranscriptPath)
	fileName := sessionDate + "_" + sessionID + ".md"

	projectDir := filepath.Join(outDir, projectName)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ccrec hook: mkdir %s: %v\n", projectDir, err)
		os.Exit(1)
	}

	outPath := filepath.Join(projectDir, fileName)
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ccrec hook: create %s: %v\n", outPath, err)
		os.Exit(1)
	}
	defer f.Close()

	opts := formatter.Options{
		SourcePath:     input.TranscriptPath,
		IncludeToolUse: *tools,
		IncludeAll:     *all,
	}
	if err := formatter.FormatMarkdown(f, records, opts); err != nil {
		fmt.Fprintf(os.Stderr, "ccrec hook: format error: %v\n", err)
		os.Exit(1)
	}
}

// ExtractProjectName derives a project name from a Claude Code transcript path.
//
// Path format: ~/.claude/projects/-Users-username-repos-projectname/session-id.jsonl
// The project directory name is hyphen-separated path segments; the last segment is the project name.
func ExtractProjectName(transcriptPath string) string {
	dir := filepath.Dir(transcriptPath)

	// Walk up to find the project directory (starts with "-").
	// In Run(), subagent paths are skipped before reaching here,
	// but this function handles them defensively since it is exported.
	for {
		base := filepath.Base(dir)
		if strings.HasPrefix(base, "-") {
			// Found the project directory (starts with "-").
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	base := filepath.Base(dir)

	// The project directory is encoded as hyphen-separated path components.
	// e.g., "-Users-junyatakaichi-repos-ccrec" → "ccrec"
	parts := strings.Split(base, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}

	return ""
}

// extractSessionID extracts a short session ID from the transcript file name.
// e.g., "42bb222a-a575-4386-bae8-2b0ce9a93d40.jsonl" → "42bb222a"
func extractSessionID(transcriptPath string) string {
	base := filepath.Base(transcriptPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if idx := strings.Index(base, "-"); idx > 0 {
		return base[:idx]
	}
	if len(base) > 8 {
		return base[:8]
	}
	return base
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
