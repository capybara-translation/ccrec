package parser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
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

func extractImages(content json.RawMessage) ([]ImageSource, error) {
	if len(content) == 0 {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, fmt.Errorf("decode image content: %w", err)
	}

	var images []ImageSource
	for _, b := range blocks {
		if b.Type != "image" {
			continue
		}
		if b.Source == nil {
			images = append(images, ImageSource{})
		} else {
			images = append(images, *b.Source)
		}
	}
	return images, nil
}

func validateImageSource(image ImageSource) (ImageSource, error) {
	if image.Type == "" && image.MediaType == "" && image.Data == "" {
		return ImageSource{}, fmt.Errorf("missing image source")
	}
	if image.Type != "base64" {
		return ImageSource{}, fmt.Errorf("unsupported image source type %q", image.Type)
	}
	if image.Data == "" {
		return ImageSource{}, fmt.Errorf("empty image data")
	}
	data, err := base64.StdEncoding.DecodeString(image.Data)
	if err != nil {
		return ImageSource{}, fmt.Errorf("decode base64 image: %w", err)
	}
	detectedType := http.DetectContentType(data)
	if !supportedImageMediaType(detectedType) {
		return ImageSource{}, fmt.Errorf("unsupported image data type %q", detectedType)
	}
	if image.MediaType != detectedType {
		return ImageSource{}, fmt.Errorf("image media type mismatch: declared %q, detected %q", image.MediaType, detectedType)
	}
	image.Data = ""
	image.Bytes = data
	return image, nil
}

func supportedImageMediaType(mediaType string) bool {
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func normalizeImages(images []ImageSource, line int, result *Result) []ImageSource {
	normalized := make([]ImageSource, 0, len(images))
	for _, image := range images {
		validated, err := validateImageSource(image)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line, Message: err.Error(), Strict: true})
			continue
		}
		normalized = append(normalized, validated)
	}
	return normalized
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
