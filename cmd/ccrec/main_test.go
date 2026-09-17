package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/capybara-translation/ccrec/internal/formatter"
	"github.com/capybara-translation/ccrec/internal/parser"
)

func TestProviderFromFlag(t *testing.T) {
	for input, want := range map[string]parser.Provider{
		"auto":   parser.ProviderAuto,
		"claude": parser.ProviderClaude,
		"codex":  parser.ProviderCodex,
	} {
		got, err := providerFromFlag(input)
		if err != nil {
			t.Fatalf("providerFromFlag(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("providerFromFlag(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := providerFromFlag("unsupported"); err == nil {
		t.Fatal("unsupported provider should return an error")
	}
}

func TestWriteMarkdownFile_UsesPrivateAtomicOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.md")
	records := []*parser.Record{
		{
			Type: "user",
			Message: &parser.Message{
				Role:    "user",
				Content: json.RawMessage(`"hello"`),
			},
		},
	}
	if err := writeMarkdownFile(path, records, formatter.Options{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
