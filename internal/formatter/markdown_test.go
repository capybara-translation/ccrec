package formatter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/capybara-translation/ccrec/internal/parser"
)

func TestFormatMarkdown_BasicOutput(t *testing.T) {
	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	records := []*parser.Record{
		{
			Type:      "user",
			Timestamp: ts,
			Message: &parser.Message{
				Role:    "user",
				Content: json.RawMessage(`"What is Go?"`),
			},
		},
		{
			Type:      "assistant",
			Timestamp: ts.Add(5 * time.Second),
			Message: &parser.Message{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"Go is a programming language."}]`),
			},
		},
	}

	var buf bytes.Buffer
	err := FormatMarkdown(&buf, records, Options{})
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	checks := []string{
		"# Conversation Log",
		"**Messages:** 2",
		"## User",
		"**Time:** 2026-01-15 10:30:00",
		"What is Go?",
		"## Assistant",
		"**Time:** 2026-01-15 10:30:05",
		"Go is a programming language.",
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestFormatMarkdown_SortsByTimestamp(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Records given in reverse order.
	records := []*parser.Record{
		{
			Type:      "assistant",
			Timestamp: ts.Add(1 * time.Second),
			Message: &parser.Message{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"second"}]`),
			},
		},
		{
			Type:      "user",
			Timestamp: ts,
			Message: &parser.Message{
				Role:    "user",
				Content: json.RawMessage(`"first"`),
			},
		},
	}

	var buf bytes.Buffer
	err := FormatMarkdown(&buf, records, Options{})
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	userIdx := strings.Index(output, "## User")
	assistantIdx := strings.Index(output, "## Assistant")
	if userIdx > assistantIdx {
		t.Error("User should appear before Assistant after sorting")
	}
}

func TestFormatMarkdown_DoesNotMutateInput(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []*parser.Record{
		{
			Type:      "assistant",
			Timestamp: ts.Add(1 * time.Second),
			Message: &parser.Message{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"second"}]`),
			},
		},
		{
			Type:      "user",
			Timestamp: ts,
			Message: &parser.Message{
				Role:    "user",
				Content: json.RawMessage(`"first"`),
			},
		},
	}

	var buf bytes.Buffer
	_ = FormatMarkdown(&buf, records, Options{})

	// Original order should be preserved.
	if records[0].Type != "assistant" {
		t.Error("input records should not be mutated by FormatMarkdown")
	}
}

func TestFormatMarkdown_IncludesSourcePath(t *testing.T) {
	var buf bytes.Buffer
	err := FormatMarkdown(&buf, nil, Options{SourcePath: "/path/to/session.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "**File:** `/path/to/session.jsonl`") {
		t.Error("output should include source path")
	}
}

func TestFormatMarkdown_IncludeAllDisablesFiltering(t *testing.T) {
	ts := time.Now()
	records := []*parser.Record{
		{
			Type:      "assistant",
			Timestamp: ts,
			Message: &parser.Message{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"API Error: something"}]`),
			},
		},
	}

	// Without IncludeAll, should be filtered.
	var buf1 bytes.Buffer
	_ = FormatMarkdown(&buf1, records, Options{})
	if strings.Contains(buf1.String(), "API Error") {
		t.Error("API Error should be filtered by default")
	}

	// With IncludeAll, should be included.
	var buf2 bytes.Buffer
	_ = FormatMarkdown(&buf2, records, Options{IncludeAll: true})
	if !strings.Contains(buf2.String(), "API Error") {
		t.Error("API Error should be included with IncludeAll")
	}
}

func TestEscapeHTMLInMarkdown_OutsideCodeBlock(t *testing.T) {
	input := `This has <div class="foo">html</div> in it`
	got := escapeHTMLInMarkdown(input)
	if strings.Contains(got, "<div") {
		t.Errorf("HTML tag should be escaped, got: %s", got)
	}
	if !strings.Contains(got, "&lt;div") {
		t.Errorf("should contain escaped tag, got: %s", got)
	}
}

func TestEscapeHTMLInMarkdown_InsideCodeBlock(t *testing.T) {
	input := "```html\n<div>keep me</div>\n```"
	got := escapeHTMLInMarkdown(input)
	if !strings.Contains(got, "<div>keep me</div>") {
		t.Errorf("HTML inside code block should NOT be escaped, got: %s", got)
	}
}

func TestEscapeHTMLInMarkdown_MixedContent(t *testing.T) {
	input := "Before <span>escaped</span>\n```\n<div>preserved</div>\n```\nAfter <p>escaped</p>"
	got := escapeHTMLInMarkdown(input)

	if strings.Contains(got, "<span>") {
		t.Error("span outside code block should be escaped")
	}
	if !strings.Contains(got, "<div>preserved</div>") {
		t.Error("div inside code block should be preserved")
	}
	if strings.Contains(got, "<p>escaped</p>") {
		t.Error("p outside code block should be escaped")
	}
}

func TestEscapeHTMLInMarkdown_NotHTMLTag(t *testing.T) {
	input := "x < 10 && y > 5"
	got := escapeHTMLInMarkdown(input)
	if got != input {
		t.Errorf("comparison operators should not be escaped, got: %s", got)
	}
}

func TestFormatRole(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user", "User"},
		{"assistant", "Assistant"},
		{"system", "System"},
		{"", "Unknown"},
	}
	for _, tt := range tests {
		got := formatRole(tt.input)
		if got != tt.want {
			t.Errorf("formatRole(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
