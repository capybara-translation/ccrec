package formatter

import (
	"strings"

	"github.com/capybara-translation/ccrec/internal/parser"
)

// excludedPatterns are substrings that indicate non-content messages.
var excludedPatterns = []string{
	"API Error",
	"[Request interrupted",
	"<command-name>",
	"<bash-input>",
	"<local-command-stdout>",
	"Caveat: The messages below were generated",
}

// FilterRecords returns only records that contain meaningful conversation content.
func FilterRecords(records []*parser.Record) []*parser.Record {
	var filtered []*parser.Record

	for _, rec := range records {
		if shouldInclude(rec) {
			filtered = append(filtered, rec)
		}
	}

	return filtered
}

func shouldInclude(rec *parser.Record) bool {
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

	text := parser.ExtractText(rec.Message.Content)
	if strings.TrimSpace(text) == "" {
		return false
	}

	for _, pat := range excludedPatterns {
		if strings.Contains(text, pat) {
			return false
		}
	}

	return true
}
