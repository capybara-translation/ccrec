package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/capybara-translation/ccrec/internal/formatter"
	"github.com/capybara-translation/ccrec/internal/hook"
	"github.com/capybara-translation/ccrec/internal/parser"
	"github.com/capybara-translation/ccrec/internal/safefile"
)

var version = "dev"

func init() {
	if version != "dev" {
		return // ldflags で設定済み（GoReleaser）
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hook":
			hook.Run(os.Args[2:])
			return
		case "-version", "--version":
			fmt.Println("ccrec " + version)
			return
		}
	}

	runConvert()
}

func runConvert() {
	var (
		output         string
		includeToolUse bool
		includeAll     bool
		includeImages  bool
		providerName   string
		strict         bool
	)

	flag.StringVar(&output, "o", "", "Output file path (default: stdout)")
	flag.BoolVar(&includeToolUse, "tools", false, "Include tool use summaries")
	flag.BoolVar(&includeAll, "all", false, "Disable filtering (include all messages)")
	flag.BoolVar(&includeImages, "images", false, "Extract and embed images (requires -o)")
	flag.StringVar(&providerName, "provider", "auto", "Transcript provider: auto, claude, or codex")
	flag.BoolVar(&strict, "strict", false, "Fail when supported messages cannot be extracted safely")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ccrec [options] <transcript.jsonl>\n")
		fmt.Fprintf(os.Stderr, "       ccrec hook [options]\n\n")
		fmt.Fprintf(os.Stderr, "Convert Claude Code or Codex conversation transcripts (JSONL) to Markdown.\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  hook    Run as a Claude Code or Codex lifecycle hook (reads stdin)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  ccrec session.jsonl                          # Output to stdout\n")
		fmt.Fprintf(os.Stderr, "  ccrec -o out.md session.jsonl                # Output to file\n")
		fmt.Fprintf(os.Stderr, "  ccrec -tools session.jsonl                   # Include tool summaries\n")
		fmt.Fprintf(os.Stderr, "  ccrec -all session.jsonl                     # Include all messages\n")
		fmt.Fprintf(os.Stderr, "  ccrec -images -o out.md session.jsonl        # Extract images\n")
		fmt.Fprintf(os.Stderr, "\n  ccrec hook -dir ~/obsidian/projects          # Run as Claude Code hook\n")
		fmt.Fprintf(os.Stderr, "  ccrec hook -images -dir ~/obsidian/projects  # With image extraction\n")
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := flag.Arg(0)

	provider, err := providerFromFlag(providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Parse JSONL.
	result, err := parser.ParseFileWithOptions(inputPath, parser.ParseOptions{Provider: provider, Strict: strict})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Line > 0 {
			fmt.Fprintf(os.Stderr, "warning: line %d: %s\n", diagnostic.Line, diagnostic.Message)
		} else {
			fmt.Fprintf(os.Stderr, "warning: %s\n", diagnostic.Message)
		}
	}
	records := result.Records

	if len(records) == 0 {
		if result.InputRecords == 0 {
			fmt.Fprintf(os.Stderr, "warning: no records found in %s\n", inputPath)
		} else if len(result.Diagnostics) == 0 {
			fmt.Fprintf(os.Stderr, "warning: transcript contained records but no supported messages were found (provider=%s)\n", result.Provider)
		}
		os.Exit(0)
	}

	// Prepare the output directory before rendering.
	if output != "" {
		dir := filepath.Dir(output)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintf(os.Stderr, "error: create directory %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// Format as Markdown.
	var attachmentsDir string
	if includeImages && output != "" {
		base := strings.TrimSuffix(filepath.Base(output), filepath.Ext(output))
		attachmentsDir = filepath.Join(filepath.Dir(output), "attachments_"+base)
	} else if includeImages {
		fmt.Fprintf(os.Stderr, "warning: -images requires -o to save image files, ignoring\n")
		includeImages = false
	}

	opts := formatter.Options{
		IncludeToolUse: includeToolUse,
		IncludeAll:     includeAll,
		IncludeImages:  includeImages,
		AttachmentsDir: attachmentsDir,
		SourcePath:     absOrOriginal(inputPath),
	}

	if output == "" {
		err = formatter.FormatMarkdown(os.Stdout, records, opts)
	} else {
		err = writeMarkdownFile(output, records, opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: format markdown: %v\n", err)
		os.Exit(1)
	}

	if output != "" {
		fmt.Fprintf(os.Stderr, "wrote %s\n", output)
	}
}

func providerFromFlag(value string) (parser.Provider, error) {
	return parser.ParseProvider(value)
}

func writeMarkdownFile(path string, records []*parser.Record, opts formatter.Options) error {
	return safefile.Write(path, 0o600, func(w io.Writer) error {
		return formatter.FormatMarkdown(w, records, opts)
	})
}

func absOrOriginal(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
