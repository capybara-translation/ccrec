package formatter

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/capybara-translation/ccrec/internal/parser"
)

// Options controls the Markdown output.
type Options struct {
	IncludeToolUse bool // Show tool use summaries.
	IncludeAll     bool // Disable filtering entirely.
	SourcePath     string
}

// htmlTagRe matches HTML tags. Used to escape HTML that could break Markdown rendering.
var htmlTagRe = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9-]*)\b[^>]*>`)

// FormatMarkdown converts parsed records into a Markdown document.
func FormatMarkdown(w io.Writer, records []*parser.Record, opts Options) error {
	// Sort by timestamp.
	sorted := make([]*parser.Record, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Filter unless --include-all.
	if !opts.IncludeAll {
		sorted = FilterRecords(sorted)
	}

	// Header.
	fmt.Fprintln(w, "# Conversation Log")
	fmt.Fprintln(w)
	if opts.SourcePath != "" {
		fmt.Fprintf(w, "**File:** `%s`\n", opts.SourcePath)
	}
	fmt.Fprintf(w, "**Messages:** %d\n", len(sorted))
	fmt.Fprintln(w)

	// Messages.
	for _, rec := range sorted {
		if err := writeMessage(w, rec, opts); err != nil {
			return err
		}
	}

	return nil
}

func writeMessage(w io.Writer, rec *parser.Record, opts Options) error {
	if rec.Message == nil {
		return nil
	}

	// Extract text.
	var text string
	if opts.IncludeToolUse {
		text = parser.ExtractTextWithToolUse(rec.Message.Content)
	} else {
		text = parser.ExtractText(rec.Message.Content)
	}

	text = strings.TrimSpace(text)
	if text == "" && !opts.IncludeAll {
		return nil
	}

	// Role heading.
	role := formatRole(rec.Type)
	fmt.Fprintf(w, "## %s\n\n", role)

	// Timestamp.
	fmt.Fprintf(w, "**Time:** %s\n\n", rec.Timestamp.Local().Format("2006-01-02 15:04:05"))

	// Content with HTML safety.
	safeText := escapeHTMLInMarkdown(text)
	fmt.Fprintln(w, safeText)
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	return nil
}

func formatRole(msgType string) string {
	switch msgType {
	case "user":
		return "User"
	case "assistant":
		return "Assistant"
	default:
		if len(msgType) > 0 {
			return strings.ToUpper(msgType[:1]) + msgType[1:]
		}
		return "Unknown"
	}
}

// escapeHTMLInMarkdown escapes HTML tags that are NOT inside fenced code blocks.
// This prevents raw HTML from being interpreted by Markdown renderers,
// which was a known issue in cclog.
func escapeHTMLInMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Toggle code block state.
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			result = append(result, line)
			continue
		}

		if inCodeBlock {
			// Inside code blocks, leave as-is.
			result = append(result, line)
		} else {
			// Outside code blocks, escape HTML tags.
			result = append(result, escapeHTMLTags(line))
		}
	}

	return strings.Join(result, "\n")
}

// escapeHTMLTags escapes HTML tags in a single line, preserving Markdown formatting.
// It only escapes actual HTML tags (not things like `x < 10`).
func escapeHTMLTags(line string) string {
	return htmlTagRe.ReplaceAllStringFunc(line, func(match string) string {
		return html.EscapeString(match)
	})
}
