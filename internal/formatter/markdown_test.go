package formatter

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/capybara-translation/ccrec/internal/parser"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

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
		"**Time:** " + ts.Local().Format("2006-01-02 15:04:05"),
		"What is Go?",
		"## Assistant",
		"**Time:** " + ts.Add(5*time.Second).Local().Format("2006-01-02 15:04:05"),
		"Go is a programming language.",
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestFormatMarkdown_CodexFixtureGolden(t *testing.T) {
	result, err := parser.ParseFileWithOptions(filepath.Join("..", "parser", "testdata", "codex-0.145.0.jsonl"), parser.ParseOptions{Provider: parser.ProviderCodex, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := FormatMarkdown(&buf, result.Records, Options{}); err != nil {
		t.Fatal(err)
	}
	actual := regexp.MustCompile(`\*\*Time:\*\* [^\n]+`).ReplaceAllString(buf.String(), "**Time:** <TIME>")
	actual = strings.TrimRight(actual, "\n") + "\n"
	want, err := os.ReadFile(filepath.Join("testdata", "codex-0.145.0.golden.md"))
	if err != nil {
		t.Fatal(err)
	}
	if actual != string(want) {
		t.Fatalf("Codex Markdown differs from golden\n--- actual ---\n%s\n--- want ---\n%s", actual, want)
	}
}

func TestFormatMarkdown_PreservesSourceOrder(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// The source order is authoritative even when timestamps move backwards.
	records := []*parser.Record{
		{
			Type:      "assistant",
			Timestamp: ts,
			Message: &parser.Message{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"first in source"}]`),
			},
		},
		{
			Type:      "user",
			Timestamp: ts.Add(-1 * time.Second),
			Message: &parser.Message{
				Role:    "user",
				Content: json.RawMessage(`"second in source"`),
			},
		},
	}

	var buf bytes.Buffer
	err := FormatMarkdown(&buf, records, Options{})
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	assistantIdx := strings.Index(output, "## Assistant")
	userIdx := strings.Index(output, "## User")
	if assistantIdx > userIdx {
		t.Error("formatter should preserve source order instead of sorting by timestamp")
	}
}

func TestFormatMarkdown_PreservesSourceOrderForConsecutiveUserMessages(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []*parser.Record{
		makeRecord("user", "user", "first user message"),
		makeRecord("user", "user", "second user message"),
	}
	records[0].Timestamp = ts
	records[1].Timestamp = ts.Add(-time.Second)

	var buf bytes.Buffer
	if err := FormatMarkdown(&buf, records, Options{}); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if strings.Index(output, "first user message") > strings.Index(output, "second user message") {
		t.Fatal("formatter reordered consecutive user messages by timestamp")
	}
}

func TestFormatMarkdown_CodexUsesNormalizedVisibleText(t *testing.T) {
	records := []*parser.Record{
		{
			Role:     "assistant",
			Provider: parser.ProviderCodex,
			Text:     "<command-name>literal</command-name> and API Error",
			Message:  &parser.Message{Role: "assistant", Content: json.RawMessage(`"different legacy content"`)},
		},
	}

	var buf bytes.Buffer
	if err := FormatMarkdown(&buf, records, Options{}); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "<command-name>literal</command-name> and API Error") {
		t.Fatalf("normalized Codex text missing:\n%s", output)
	}
	if strings.Contains(output, "different legacy content") {
		t.Fatalf("formatter ignored normalized Codex text:\n%s", output)
	}
}

func TestFormatMarkdown_OmitsZeroTimestamp(t *testing.T) {
	record := makeRecord("user", "user", "no timestamp")
	record.Timestamp = time.Time{}

	var buf bytes.Buffer
	if err := FormatMarkdown(&buf, []*parser.Record{record}, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "**Time:**") || strings.Contains(buf.String(), "0001-01-01") {
		t.Fatalf("zero timestamp should be omitted:\n%s", buf.String())
	}
}

func TestFormatMarkdown_UsesNormalizedRole(t *testing.T) {
	records := []*parser.Record{
		{
			Role: "user",
			Message: &parser.Message{
				Role:    "user",
				Content: json.RawMessage(`"normalized user"`),
			},
		},
	}

	var buf bytes.Buffer
	if err := FormatMarkdown(&buf, records, Options{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "## User") || !strings.Contains(buf.String(), "normalized user") {
		t.Fatalf("normalized role was not formatted:\n%s", buf.String())
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

type failAfterWriter struct {
	remaining int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errors.New("simulated write failure")
	}
	if len(p) > w.remaining {
		n := w.remaining
		w.remaining = 0
		return n, errors.New("simulated write failure")
	}
	w.remaining -= len(p)
	return len(p), nil
}

func TestFormatMarkdown_PropagatesWriterFailure(t *testing.T) {
	w := &failAfterWriter{remaining: 12}
	if err := FormatMarkdown(w, nil, Options{}); err == nil {
		t.Fatal("FormatMarkdown should return the underlying writer error")
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

func TestEscapeHTMLInMarkdown_NonHTMLTagsPreserved(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"TMX seg tag", `<seg>Hello world</seg>`},
		{"TMX bpt/ept", `<bpt i="1">&lt;b&gt;</bpt>Bold<ept i="1">&lt;/b&gt;</ept>`},
		{"XLIFF trans-unit", `<trans-unit id="1"><target>text</target></trans-unit>`},
		{"XLIFF source", `<source xml:lang="en">Hello</source>`},
		{"custom XML", `<myCustomTag attr="val">content</myCustomTag>`},
		{"self-closing XML", `<x id="1"/>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTMLInMarkdown(tt.input)
			if got != tt.input {
				t.Errorf("non-HTML tag should NOT be escaped\ninput: %s\ngot:   %s", tt.input, got)
			}
		})
	}
}

func TestEscapeHTMLInMarkdown_MixedHTMLAndXML(t *testing.T) {
	input := `<div>HTML content</div> and <seg>XML content</seg>`
	got := escapeHTMLInMarkdown(input)

	if strings.Contains(got, "<div>") {
		t.Error("HTML <div> should be escaped")
	}
	if !strings.Contains(got, "<seg>XML content</seg>") {
		t.Errorf("XML <seg> should be preserved, got: %s", got)
	}
}

func TestEscapeHTMLInMarkdown_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"uppercase", `<DIV>content</DIV>`},
		{"mixed case", `<Span>content</Span>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTMLInMarkdown(tt.input)
			if got == tt.input {
				t.Errorf("HTML tag should be escaped regardless of case\ninput: %s\ngot:   %s", tt.input, got)
			}
		})
	}
}

func TestEscapeHTMLInMarkdown_SelfClosingHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"br", `line<br/>break`},
		{"img", `<img src="x.png"/>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTMLInMarkdown(tt.input)
			if got == tt.input {
				t.Errorf("self-closing HTML tag should be escaped\ninput: %s\ngot:   %s", tt.input, got)
			}
		})
	}
}

func TestEscapeHTMLInMarkdown_LegacyHTML(t *testing.T) {
	input := `<font color="red">old</font> and <center>centered</center>`
	got := escapeHTMLInMarkdown(input)
	if strings.Contains(got, "<font") {
		t.Error("legacy <font> should be escaped")
	}
	if strings.Contains(got, "<center>") {
		t.Error("legacy <center> should be escaped")
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

func TestSaveImage_UsesPrivatePermissions(t *testing.T) {
	attachments := filepath.Join(t.TempDir(), "attachments")
	if err := os.Mkdir(attachments, 0o755); err != nil {
		t.Fatal(err)
	}
	oldImage := filepath.Join(attachments, "image_001.png")
	if err := os.WriteFile(oldImage, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := saveImage(attachments, 1, parser.ImageSource{MediaType: "image/png", Data: tinyPNGBase64})
	if err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(attachments)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("attachments mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(filepath.Join(attachments, "image_001.png"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("image mode = %o, want 600", got)
	}
}

func TestFormatMarkdown_SavesNormalizedCodexImages(t *testing.T) {
	attachments := filepath.Join(t.TempDir(), "attachments")
	record := &parser.Record{
		Type:     "user",
		Role:     "user",
		Provider: parser.ProviderCodex,
		Text:     "with image",
		Images:   []parser.ImageSource{{MediaType: "image/png", Data: tinyPNGBase64}},
	}

	var buf bytes.Buffer
	if err := FormatMarkdown(&buf, []*parser.Record{record}, Options{IncludeImages: true, AttachmentsDir: attachments}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "![image](attachments/image_001.png)") {
		t.Fatalf("Markdown image missing:\n%s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(attachments, "image_001.png")); err != nil {
		t.Fatalf("saved image missing: %v", err)
	}
}

func TestFormatMarkdown_IncludesImageOnlyRecordOnlyWhenImagesEnabled(t *testing.T) {
	record := &parser.Record{
		Type:     "user",
		Role:     "user",
		Provider: parser.ProviderCodex,
		Images:   []parser.ImageSource{{MediaType: "image/png", Data: tinyPNGBase64}},
	}

	var withoutImages bytes.Buffer
	if err := FormatMarkdown(&withoutImages, []*parser.Record{record}, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(withoutImages.String(), "## User") {
		t.Fatalf("image-only record should be hidden without -images:\n%s", withoutImages.String())
	}

	var withImages bytes.Buffer
	attachments := filepath.Join(t.TempDir(), "attachments")
	if err := FormatMarkdown(&withImages, []*parser.Record{record}, Options{IncludeImages: true, AttachmentsDir: attachments}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withImages.String(), "## User") || !strings.Contains(withImages.String(), "![image](attachments/image_001.png)") {
		t.Fatalf("image-only record missing with -images:\n%s", withImages.String())
	}
}

func TestFormatMarkdown_DoesNotIncludeImageOnlyRecordWithAllButWithoutImages(t *testing.T) {
	record := &parser.Record{
		Type:     "user",
		Role:     "user",
		Provider: parser.ProviderCodex,
		Images:   []parser.ImageSource{{MediaType: "image/png", Data: tinyPNGBase64}},
	}

	var output bytes.Buffer
	if err := FormatMarkdown(&output, []*parser.Record{record}, Options{IncludeAll: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "## User") {
		t.Fatalf("-all emitted an image-only message without -images:\n%s", output.String())
	}
}

func TestFormatMarkdown_WritesValidImagesAndSkipsInvalidImages(t *testing.T) {
	attachments := filepath.Join(t.TempDir(), "attachments")
	record := &parser.Record{
		Type:     "user",
		Role:     "user",
		Provider: parser.ProviderCodex,
		Images: []parser.ImageSource{
			{MediaType: "image/png", Data: tinyPNGBase64},
			{MediaType: "image/png", Data: "bm90IGFuIGltYWdl"},
		},
	}

	var output bytes.Buffer
	if err := FormatMarkdown(&output, []*parser.Record{record}, Options{IncludeImages: true, AttachmentsDir: attachments}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(output.String(), "![image]"); got != 1 {
		t.Fatalf("rendered %d images, want 1:\n%s", got, output.String())
	}
	if _, err := os.Stat(filepath.Join(attachments, "image_001.png")); err != nil {
		t.Fatalf("valid image missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(attachments, "image_002.png")); !os.IsNotExist(err) {
		t.Fatalf("invalid image was written: %v", err)
	}
}

func TestFormatMarkdown_ExcludesInvalidImageOnlyRecord(t *testing.T) {
	record := &parser.Record{
		Type:     "user",
		Role:     "user",
		Provider: parser.ProviderCodex,
		Images:   []parser.ImageSource{{MediaType: "image/png", Data: "bm90IGFuIGltYWdl"}},
	}

	var output bytes.Buffer
	if err := FormatMarkdown(&output, []*parser.Record{record}, Options{IncludeImages: true, AttachmentsDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "**Messages:** 0") || strings.Contains(output.String(), "## User") {
		t.Fatalf("invalid image-only record was counted:\n%s", output.String())
	}
}

func TestSaveImage_RejectsNonImageData(t *testing.T) {
	attachments := filepath.Join(t.TempDir(), "attachments")
	_, err := saveImage(attachments, 1, parser.ImageSource{MediaType: "image/png", Data: "bm90IGFuIGltYWdl"})
	if err == nil {
		t.Fatal("non-image payload should be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(attachments, "image_001.png")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid image was written: %v", statErr)
	}
}

func TestSaveImage_RejectsMediaTypeMismatch(t *testing.T) {
	attachments := filepath.Join(t.TempDir(), "attachments")
	_, err := saveImage(attachments, 1, parser.ImageSource{MediaType: "image/jpeg", Data: tinyPNGBase64})
	if err == nil {
		t.Fatal("media type mismatch should be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(attachments, "image_001.jpg")); !os.IsNotExist(statErr) {
		t.Fatalf("mismatched image was written: %v", statErr)
	}
}
