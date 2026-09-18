package main

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestCLI_StrictImageValidationExitCodes(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	tests := []struct {
		name     string
		imageURL string
	}{
		{name: "unsupported header", imageURL: "data:image/bmp;base64," + png},
		{name: "invalid base64 body", imageURL: "data:image/png;base64,%%%"},
		{name: "media type mismatch", imageURL: "data:image/jpeg;base64," + png},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transcript := filepath.Join(t.TempDir(), "rollout.jsonl")
			input := strings.Join([]string{
				`{"type":"session_meta","payload":{"id":"session-id"}}`,
				`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"keep text"},{"type":"input_image","image_url":"` + tt.imageURL + `"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text","user.image"],"turn_id":"turn-1"}}}`,
				`{"type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"UserMessage","content":[{"type":"text","text":"keep text"},{"type":"local_image","path":"/not/read"}]}}}`,
			}, "\n")
			if err := os.WriteFile(transcript, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}

			withoutImages := runCLIHelper(t, "-provider", "codex", "-strict", transcript)
			if withoutImages != 0 {
				t.Fatalf("exit without -images = %d, want 0", withoutImages)
			}

			output := filepath.Join(t.TempDir(), "output.md")
			withImages := runCLIHelper(t, "-provider", "codex", "-strict", "-images", "-o", output, transcript)
			if withImages == 0 {
				t.Fatal("exit with -images = 0, want non-zero")
			}
		})
	}
}

func TestCLI_ImageSaveFailureReturnsNonZeroWithoutPublishingMarkdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	root := t.TempDir()
	transcript := filepath.Join(root, "session.jsonl")
	input := `{"type":"user","message":{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + png + `"}}]}}`
	if err := os.WriteFile(transcript, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	attachments := filepath.Join(root, "attachments_output")
	if err := os.Symlink(realDir, attachments); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "output.md")

	if exitCode := runCLIHelper(t, "-provider", "claude", "-images", "-o", output, transcript); exitCode == 0 {
		t.Fatal("image save failure exited successfully")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("Markdown was published despite image save failure: %v", err)
	}
}

func runCLIHelper(t *testing.T, args ...string) int {
	t.Helper()
	commandArgs := append([]string{"-test.run=TestCLIHelperProcess", "--"}, args...)
	cmd := exec.Command(os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), "CCREC_CLI_HELPER=1")
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run CLI helper: %v", err)
	}
	return exitErr.ExitCode()
}

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("CCREC_CLI_HELPER") != "1" {
		return
	}
	separator := 0
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	os.Args = append([]string{"ccrec"}, os.Args[separator+1:]...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
	os.Exit(0)
}
