package parser

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileWithOptions_CodexFixture(t *testing.T) {
	result, err := ParseFileWithOptions(filepath.Join("testdata", "codex-0.145.0.jsonl"), ParseOptions{Provider: ProviderCodex, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "00000000-1111-2222-3333-444444444444" {
		t.Fatalf("session ID = %q", result.SessionID)
	}

	wants := []struct {
		role  string
		phase Phase
		text  string
	}{
		{role: "user", text: "fixture prompt"},
		{role: "assistant", phase: PhaseCommentary, text: "fixture progress"},
		{role: "assistant", phase: PhaseFinal, text: "fixture answer"},
		{role: "user", text: "repeat"},
		{role: "user", text: "repeat"},
	}
	if len(result.Records) != len(wants) {
		t.Fatalf("got %d records, want %d", len(result.Records), len(wants))
	}
	for i, want := range wants {
		got := result.Records[i]
		text := ExtractText(got.Message.Content)
		if got.Role != want.role || got.Phase != want.phase || text != want.text {
			t.Errorf("record %d = (%q, %q, %q), want (%q, %q, %q)", i, got.Role, got.Phase, text, want.role, want.phase, want.text)
		}
		if strings.Contains(text, "SECRET") {
			t.Errorf("record %d leaked non-visible content: %q", i, text)
		}
	}
}

func TestParseFileWithOptions_CodexLegacyFixture(t *testing.T) {
	result, err := ParseFileWithOptions(filepath.Join("testdata", "codex-0.129.0-legacy.jsonl"), ParseOptions{Provider: ProviderCodex, Strict: true})
	if err != nil {
		t.Fatal(err)
	}

	wants := []struct {
		role  string
		phase Phase
		text  string
	}{
		{role: "user", text: "legacy prompt"},
		{role: "assistant", phase: PhaseFinal, text: "legacy answer"},
	}
	if len(result.Records) != len(wants) {
		t.Fatalf("got %d records, want %d", len(result.Records), len(wants))
	}
	for i, want := range wants {
		got := result.Records[i]
		if text := ExtractText(got.Message.Content); got.Role != want.role || got.Phase != want.phase || text != want.text {
			t.Errorf("record %d = (%q, %q, %q), want (%q, %q, %q)", i, got.Role, got.Phase, text, want.role, want.phase, want.text)
		}
	}
}

func TestParseFileWithOptions_CodexSyntheticNegativeBlocksAreNotVisible(t *testing.T) {
	result, err := ParseFileWithOptions(filepath.Join("testdata", "codex-defensive-negative.jsonl"), ParseOptions{Provider: ProviderCodex, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(result.Records))
	}
	for _, rec := range result.Records {
		if strings.Contains(rec.Text, "SECRET") {
			t.Fatalf("synthetic non-visible block leaked: %q", rec.Text)
		}
	}
}

func TestParseReaderWithOptions_CodexPrefersCurrentVisibleEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-full-id","session_id":"session-full-id"}}`,
		`{"timestamp":"2026-09-17T00:00:00.500Z","type":"event_msg","payload":{"type":"user_message","message":"legacy duplicate"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`,
		`{"timestamp":"2026-09-17T00:00:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"reasoning","text":"SECRET reasoning"},{"type":"text","text":"hello"},{"type":"tool_output","text":"SECRET tool output"}]}}}`,
		`{"timestamp":"2026-09-17T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"working"}]}}`,
		`{"timestamp":"2026-09-17T00:00:04Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","phase":"commentary","content":[{"type":"unknown","text":"SECRET unknown"},{"type":"Text","text":"working"}]}}}`,
		`{"timestamp":"2026-09-17T00:00:05Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","phase":"final_answer","content":[{"type":"Text","text":"done"}]}}}`,
		`{"timestamp":"2026-09-17T00:00:06Z","type":"event_msg","payload":{"type":"agent_message","message":"legacy duplicate","phase":"final_answer"}}`,
	}, "\n")

	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != ProviderCodex {
		t.Fatalf("provider = %q, want %q", result.Provider, ProviderCodex)
	}
	if result.SessionID != "session-full-id" {
		t.Fatalf("session ID = %q, want session-full-id", result.SessionID)
	}
	if len(result.Records) != 3 {
		t.Fatalf("got %d records, want 3", len(result.Records))
	}

	wants := []struct {
		role  string
		text  string
		phase Phase
	}{
		{role: "user", text: "hello"},
		{role: "assistant", text: "working", phase: PhaseCommentary},
		{role: "assistant", text: "done", phase: PhaseFinal},
	}
	for i, want := range wants {
		rec := result.Records[i]
		if rec.Role != want.role || ExtractText(rec.Message.Content) != want.text || rec.Phase != want.phase {
			t.Errorf("record %d = role %q text %q phase %q, want role %q text %q phase %q",
				i, rec.Role, ExtractText(rec.Message.Content), rec.Phase, want.role, want.text, want.phase)
		}
	}
}

func TestParseReaderWithOptions_StrictClaudeRejectsUnsupportedOnlyTranscript(t *testing.T) {
	input := `{"type":"file-history-snapshot","message":{"role":"system","content":"snapshot"},"timestamp":"2026-01-01T00:00:00Z"}`

	_, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderClaude, Strict: true})
	if err == nil {
		t.Fatal("strict parsing should reject a Claude transcript without supported messages")
	}
}

func TestParseReaderWithOptions_CodexResponseFallbackExcludesKnownHiddenContext(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET AGENTS CONTENT"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["agents_md.instructions"]}}}`,
		`{"timestamp":"2026-09-17T00:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET MIXED CONTEXT"},{"type":"input_text","text":"visible prompt"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["agents_md.instructions","user.text"]}}}`,
		`{"timestamp":"2026-09-17T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"visible answer"}]}}`,
	}, "\n")

	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(result.Records))
	}
	for _, rec := range result.Records {
		if strings.Contains(ExtractText(rec.Message.Content), "SECRET") {
			t.Fatal("injected context was emitted")
		}
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("known hidden context should be skipped silently: %#v", result.Diagnostics)
	}
}

func TestParseReaderWithOptions_CodexResponseFallbackConcatenatesVisibleChunks(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_text","text":" "},{"type":"input_text","text":"world"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text","user.text","user.text"]}}}`,
	}, "\n")

	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(result.Records))
	}
	if got := result.Records[0].Text; got != "hello world" {
		t.Fatalf("visible text = %q, want %q", got, "hello world")
	}
}

func TestParseReaderWithOptions_CodexKnownHiddenUserMetadataIsSilent(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hidden"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["agents_md.instructions"]}}}`,
		`{"timestamp":"2026-09-17T00:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}}`,
	}, "\n")

	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("known hidden metadata produced diagnostics: %#v", result.Diagnostics)
	}
}

func TestParseReaderWithOptions_CodexUnknownUserMetadataIsStrict(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"future data"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["future.visible_kind"]}}}`,
	}, "\n")

	if _, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex, Strict: true}); err == nil {
		t.Fatal("unknown user metadata should fail strict parsing")
	}
}

func TestParseReaderWithOptions_CodexMixedVisibleAndUnknownUserMetadataIsStrict(t *testing.T) {
	tests := []struct {
		name    string
		content string
		kinds   string
	}{
		{name: "visible and unknown", content: `[{"type":"input_text","text":"visible"},{"type":"input_text","text":"future data"}]`, kinds: `["user.text","future.visible_kind"]`},
		{name: "known hidden and unknown", content: `[{"type":"input_text","text":"hidden"},{"type":"input_text","text":"future data"}]`, kinds: `["agents_md.instructions","future.kind"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Join([]string{
				`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
				`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":` + tt.content + `,"internal_chat_message_metadata_passthrough":{"content_item_kinds":` + tt.kinds + `}}}`,
			}, "\n")

			if _, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex, Strict: true}); err == nil {
				t.Fatal("unknown metadata should fail strict parsing")
			}
		})
	}
}

func TestParseReaderWithOptions_CodexDoesNotDeduplicateRepeatedVisibleMessages(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"text","text":"repeat"}]}}}`,
		`{"timestamp":"2026-09-17T00:00:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"text","text":"repeat"}]}}}`,
	}, "\n")

	result, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderAuto})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(result.Records))
	}
	if result.Records[0].Sequence >= result.Records[1].Sequence {
		t.Fatalf("sequence was not preserved: %d then %d", result.Records[0].Sequence, result.Records[1].Sequence)
	}
}

func TestParseProvider(t *testing.T) {
	for input, want := range map[string]Provider{
		"auto":   ProviderAuto,
		"claude": ProviderClaude,
		"codex":  ProviderCodex,
	} {
		got, err := ParseProvider(input)
		if err != nil {
			t.Fatalf("ParseProvider(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("ParseProvider(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := ParseProvider("other"); err == nil {
		t.Fatal("ParseProvider(other) should fail")
	}
}

func TestParseReaderWithOptions_StrictRejectsAmbiguousCodexUser(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-09-17T00:00:00Z","type":"session_meta","payload":{"id":"session-id"}}`,
		`{"timestamp":"2026-09-17T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"ambiguous"}]}}`,
	}, "\n")

	_, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex, Strict: true})
	if err == nil {
		t.Fatal("strict parsing should reject ambiguous Codex user input")
	}
}
