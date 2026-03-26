package parser

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractText_PlainString(t *testing.T) {
	content := json.RawMessage(`"hello world"`)
	got := ExtractText(content)
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestExtractText_TextBlock(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"hello"}]`)
	got := ExtractText(content)
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestExtractText_MultipleTextBlocks(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`)
	got := ExtractText(content)
	if got != "first\n\nsecond" {
		t.Errorf("got %q, want %q", got, "first\n\nsecond")
	}
}

func TestExtractText_SkipsThinking(t *testing.T) {
	content := json.RawMessage(`[{"type":"thinking","thinking":"internal thought"},{"type":"text","text":"visible"}]`)
	got := ExtractText(content)
	if got != "visible" {
		t.Errorf("got %q, want %q", got, "visible")
	}
}

func TestExtractText_SkipsToolUse(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/x"}},{"type":"text","text":"result"}]`)
	got := ExtractText(content)
	if got != "result" {
		t.Errorf("got %q, want %q", got, "result")
	}
}

func TestExtractText_EmptyContent(t *testing.T) {
	got := ExtractText(nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	got = ExtractText(json.RawMessage(``))
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractText_EmptyTextBlocks(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":""},{"type":"text","text":"  "}]`)
	got := ExtractText(content)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractText_InvalidJSON(t *testing.T) {
	content := json.RawMessage(`not valid json`)
	got := ExtractText(content)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractTextWithToolUse_IncludesToolSummary(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/test.go"}},{"type":"text","text":"done"}]`)
	got := ExtractTextWithToolUse(content)
	if !strings.Contains(got, "*[Tool: Read: /tmp/test.go]*") {
		t.Errorf("got %q, want tool summary containing Read and file path", got)
	}
	if !strings.Contains(got, "done") {
		t.Errorf("got %q, want text content", got)
	}
}

func TestExtractTextWithToolUse_BashCommand(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_use","name":"Bash","input":{"command":"git status"}}]`)
	got := ExtractTextWithToolUse(content)
	if !strings.Contains(got, "`git status`") {
		t.Errorf("got %q, want command in backticks", got)
	}
}

func TestExtractTextWithToolUse_GrepPattern(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_use","name":"Grep","input":{"pattern":"func main"}}]`)
	got := ExtractTextWithToolUse(content)
	if !strings.Contains(got, "`func main`") {
		t.Errorf("got %q, want pattern in backticks", got)
	}
}

func TestExtractTextWithToolUse_LongCommandTruncated(t *testing.T) {
	longCmd := strings.Repeat("a", 100)
	content := json.RawMessage(`[{"type":"tool_use","name":"Bash","input":{"command":"` + longCmd + `"}}]`)
	got := ExtractTextWithToolUse(content)
	if !strings.Contains(got, "...") {
		t.Errorf("got %q, want truncated command with ellipsis", got)
	}
}

func TestExtractTextWithToolUse_EmptyToolName(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_use","name":"","input":{}}]`)
	got := ExtractTextWithToolUse(content)
	if got != "" {
		t.Errorf("got %q, want empty for nameless tool", got)
	}
}

func TestExtractTextWithToolUse_PlainString(t *testing.T) {
	content := json.RawMessage(`"just text"`)
	got := ExtractTextWithToolUse(content)
	if got != "just text" {
		t.Errorf("got %q, want %q", got, "just text")
	}
}
