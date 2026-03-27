package formatter

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/capybara-translation/ccrec/internal/parser"
)

// Options controls the Markdown output.
type Options struct {
	IncludeToolUse bool   // Show tool use summaries.
	IncludeAll     bool   // Disable filtering entirely.
	IncludeImages  bool   // Extract and embed images.
	AttachmentsDir string // Absolute path for saving attachments (images, etc.).
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
		sorted = FilterRecords(sorted, opts.IncludeToolUse)
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
	imageCounter := 0
	for _, rec := range sorted {
		if err := writeMessage(w, rec, opts, &imageCounter); err != nil {
			return err
		}
	}

	return nil
}

func writeMessage(w io.Writer, rec *parser.Record, opts Options, imageCounter *int) error {
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

	// Extract images if enabled.
	var imagePaths []string
	if opts.IncludeImages && opts.AttachmentsDir != "" {
		images := parser.ExtractImages(rec.Message.Content)
		for _, img := range images {
			*imageCounter++
			path, err := saveImage(opts.AttachmentsDir, *imageCounter, img)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to save image: %v\n", err)
				continue
			}
			imagePaths = append(imagePaths, path)
		}
	}

	text = strings.TrimSpace(text)
	if text == "" && len(imagePaths) == 0 && !opts.IncludeAll {
		return nil
	}

	// Role heading.
	role := formatRole(rec.Type)
	fmt.Fprintf(w, "## %s\n\n", role)

	// Timestamp.
	fmt.Fprintf(w, "**Time:** %s\n\n", rec.Timestamp.Local().Format("2006-01-02 15:04:05"))

	// Images.
	for _, p := range imagePaths {
		fmt.Fprintf(w, "![image](%s)\n\n", p)
	}

	// Content with HTML safety.
	if text != "" {
		safeText := escapeHTMLInMarkdown(text)
		fmt.Fprintln(w, safeText)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	return nil
}

// saveImage decodes a base64 image and saves it to the attachments directory.
// Returns the relative path for Markdown reference.
func saveImage(attachmentsDir string, index int, img parser.ImageSource) (string, error) {
	if err := os.MkdirAll(attachmentsDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", attachmentsDir, err)
	}

	ext := ".png"
	switch img.MediaType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	fileName := fmt.Sprintf("image_%03d%s", index, ext)
	filePath := filepath.Join(attachmentsDir, fileName)

	data, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", filePath, err)
	}

	// Return relative path for Markdown reference.
	dirName := filepath.Base(attachmentsDir)
	return dirName + "/" + fileName, nil
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
