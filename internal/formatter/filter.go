package formatter

import (
	"strings"

	"github.com/capybara-translation/ccrec/internal/parser"
)

// excludedPatterns are substrings found anywhere in the message.
var excludedPatterns = []string{
	"API Error",
	"[Request interrupted",
}

// excludedPrefixes match only when the message starts with these substrings,
// reducing false positives from mid-text occurrences.
var excludedPrefixes = []string{
	"<command-name>",
	"<command-message>",
	"<bash-input>",
	"Caveat: The messages below were generated",
}

// FilterRecords returns only records that contain meaningful conversation content.
func FilterRecords(records []*parser.Record, includeToolUse bool) []*parser.Record {
	var filtered []*parser.Record

	for _, rec := range records {
		if shouldInclude(rec, includeToolUse) {
			filtered = append(filtered, rec)
		}
	}

	return filtered
}

func shouldInclude(rec *parser.Record, includeToolUse bool) bool {
	// Only include user and assistant messages.
	switch rec.Type {
	case "user", "assistant":
		// continue
	default:
		return false
	}

	if rec.IsMeta {
		return false
	}

	if rec.Message == nil {
		return false
	}

	var text string
	if includeToolUse {
		text = parser.ExtractTextWithToolUse(rec.Message.Content)
	} else {
		text = parser.ExtractText(rec.Message.Content)
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	for _, pat := range excludedPatterns {
		if strings.Contains(trimmed, pat) {
			return false
		}
	}

	for _, prefix := range excludedPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return false
		}
	}

	return true
}
