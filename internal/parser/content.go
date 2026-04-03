package parser

import (
	"encoding/json"
	"regexp"
	"strings"
)

// localCommandStdoutTagRe matches <local-command-stdout> open/close tags.
var localCommandStdoutTagRe = regexp.MustCompile(`</?local-command-stdout>`)

// ExtractText extracts human-readable text from a message's content field.
// content can be either a plain string or an array of ContentBlock objects.
// It skips thinking blocks and tool_use/tool_result blocks by default.
func ExtractText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	// Try as a plain string first.
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return stripSystemTags(s)
	}

	// Try as an array of content blocks.
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		// If it's neither string nor array, return empty.
		return ""
	}

	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(stripSystemTags(b.Text)); t != "" {
				parts = append(parts, t)
			}
		// Skip: thinking, tool_use, tool_result
		}
	}

	return strings.Join(parts, "\n\n")
}

// ExtractTextWithToolUse extracts text and includes tool use summaries.
func ExtractTextWithToolUse(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return stripSystemTags(s)
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(stripSystemTags(b.Text)); t != "" {
				parts = append(parts, t)
			}
		case "tool_use":
			parts = append(parts, formatToolUse(b))
		case "tool_result":
			// Tool results are in user messages; skip for brevity.
		// Skip: thinking
		}
	}

	return strings.Join(parts, "\n\n")
}

// ExtractImages extracts image blocks from a message's content field.
func ExtractImages(content json.RawMessage) []ImageSource {
	if len(content) == 0 {
		return nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil
	}

	var images []ImageSource
	for _, b := range blocks {
		if b.Type == "image" && b.Source != nil && b.Source.Data != "" {
			images = append(images, *b.Source)
		}
	}
	return images
}

// stripSystemTags removes <local-command-stdout> tags from text, keeping the content inside.
// Other system tags (command-name, etc.) are preserved so they can be caught by excludedPatterns.
func stripSystemTags(s string) string {
	return strings.TrimSpace(localCommandStdoutTagRe.ReplaceAllString(s, ""))
}

func formatToolUse(b ContentBlock) string {
	if b.Name == "" {
		return ""
	}

	summary := "*[Tool: " + b.Name
	// Extract a brief hint from input if possible.
	if len(b.Input) > 0 {
		var input map[string]json.RawMessage
		if err := json.Unmarshal(b.Input, &input); err == nil {
			if cmd, ok := input["command"]; ok {
				var c string
				if json.Unmarshal(cmd, &c) == nil {
					if len(c) > 80 {
						c = c[:80] + "..."
					}
					summary += ": `" + c + "`"
				}
			} else if fp, ok := input["file_path"]; ok {
				var p string
				if json.Unmarshal(fp, &p) == nil {
					summary += ": " + p
				}
			} else if pat, ok := input["pattern"]; ok {
				var p string
				if json.Unmarshal(pat, &p) == nil {
					summary += ": `" + p + "`"
				}
			}
		}
	}
	summary += "]*"
	return summary
}
