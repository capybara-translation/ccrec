package parser

import (
	"strings"
	"testing"
)

func TestParseReader_UserMessage(t *testing.T) {
	input := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-01-01T00:00:00Z"}`
	records, err := parseReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "user" {
		t.Errorf("type = %q, want %q", records[0].Type, "user")
	}
	if records[0].Message.Role != "user" {
		t.Errorf("role = %q, want %q", records[0].Message.Role, "user")
	}
}

func TestParseReader_AssistantWithContentBlocks(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response"}]},"timestamp":"2026-01-01T00:00:01Z"}`
	records, err := parseReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Message.Role != "assistant" {
		t.Errorf("role = %q, want %q", records[0].Message.Role, "assistant")
	}
}

func TestParseReader_MultipleLines(t *testing.T) {
	input := `{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2026-01-01T00:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second"}]},"timestamp":"2026-01-01T00:00:01Z"}
{"type":"file-history-snapshot","messageId":"abc","timestamp":"2026-01-01T00:00:02Z"}`
	records, err := parseReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
}

func TestParseReader_SkipsEmptyLines(t *testing.T) {
	input := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-01-01T00:00:00Z"}

{"type":"user","message":{"role":"user","content":"world"},"timestamp":"2026-01-01T00:00:01Z"}`
	records, err := parseReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
}

func TestParseReader_SkipsMalformedLines(t *testing.T) {
	input := `{"type":"user","message":{"role":"user","content":"ok"},"timestamp":"2026-01-01T00:00:00Z"}
this is not json
{"type":"user","message":{"role":"user","content":"also ok"},"timestamp":"2026-01-01T00:00:01Z"}`
	records, err := parseReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (malformed line skipped)", len(records))
	}
}

func TestParseReader_LargeLine(t *testing.T) {
	// Simulate a large tool result (> 64KB default scanner buffer).
	bigContent := strings.Repeat("x", 100_000)
	input := `{"type":"user","message":{"role":"user","content":"` + bigContent + `"},"timestamp":"2026-01-01T00:00:00Z"}`
	records, err := parseReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
}
