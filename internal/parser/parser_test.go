package parser

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFile_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.jsonl")
	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("ParseFile should return an error for a missing file")
	}
	// Callers (hook.Run) rely on errors.Is to detect a missing transcript;
	// re-wrapping with %v instead of %w would break that silently.
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(err, fs.ErrNotExist) = false, want true (err: %v)", err)
	}
}

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

func TestParseReaderWithOptions_EmptyTranscriptIsNotDetectionError(t *testing.T) {
	result, err := parseReaderWithOptions(strings.NewReader("\n"), ParseOptions{Provider: ProviderAuto})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 0 || result.InputRecords != 0 {
		t.Fatalf("empty result = %d records, %d input records", len(result.Records), result.InputRecords)
	}
}

func TestParseReaderWithOptions_RejectsExplicitProviderMismatch(t *testing.T) {
	input := `{"type":"session_meta","payload":{"id":"session-id"},"timestamp":"2026-09-17T00:00:00Z"}`
	_, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderClaude})
	if err == nil {
		t.Fatal("explicit Claude provider should reject a Codex transcript")
	}
}

func TestParseReaderWithOptions_UnknownProviderShapeWarnsUnlessStrict(t *testing.T) {
	input := `{"type":"future-record","payload":{"value":1}}`
	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto})
	if err != nil {
		t.Fatalf("non-strict parse returned error: %v", err)
	}
	if len(result.Records) != 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("result = %d records, %d diagnostics; want no records and a warning", len(result.Records), len(result.Diagnostics))
	}

	if _, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto, Strict: true}); err == nil {
		t.Fatal("strict parsing should reject an unknown transcript shape")
	}
}

func TestParseReaderWithOptions_StrictErrorWithoutSourceLineOmitsLineZero(t *testing.T) {
	input := `{"type":"future-record","payload":{"value":1}}`
	_, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto, Strict: true})
	if err == nil {
		t.Fatal("strict parsing should reject an unknown transcript shape")
	}
	if strings.Contains(err.Error(), "line 0") {
		t.Fatalf("strict error contains synthetic line number: %v", err)
	}
}

func TestParseReaderWithOptions_MalformedOnlyWarnsUnlessStrict(t *testing.T) {
	input := "not json\n"
	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto})
	if err != nil {
		t.Fatalf("non-strict parse returned error: %v", err)
	}
	if len(result.Records) != 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("result = %d records, %d diagnostics; want no records and warnings", len(result.Records), len(result.Diagnostics))
	}

	if _, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto, Strict: true}); err == nil {
		t.Fatal("strict parsing should reject malformed-only input")
	}
}
