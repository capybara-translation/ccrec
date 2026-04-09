package hook

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractProjectName(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "standard project path",
			path: "/Users/junya/.claude/projects/-Users-junya-repos-ccrec/abc123.jsonl",
			want: "ccrec",
		},
		{
			name: "workspace path",
			path: "/Users/junya/.claude/projects/-Users-junya-workspace/abc123.jsonl",
			want: "workspace",
		},
		{
			name: "nested project path",
			path: "/Users/junya/.claude/projects/-Users-junya-repos-my-app/abc123.jsonl",
			want: "app",
		},
		{
			name: "subagent path",
			path: "/Users/junya/.claude/projects/-Users-junya-repos-ccrec/abc123/subagents/agent-xyz.jsonl",
			want: "ccrec",
		},
		{
			name: "deep nested path",
			path: "/Users/junya/.claude/projects/-Users-junya-repos-goblog/session/subagents/agent-a19db5a.jsonl",
			want: "goblog",
		},
		{
			name: "path with trailing hyphens",
			path: "/Users/junya/.claude/projects/-Users-junya-repos-go-todo-app-my/abc.jsonl",
			want: "my",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProjectName(tt.path)
			if got != tt.want {
				t.Errorf("ExtractProjectName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDeriveProjectName(t *testing.T) {
	tests := []struct {
		name           string
		projectDir     string
		basePath       string
		transcriptPath string
		want           string
	}{
		{
			name:       "base strips prefix from project dir",
			projectDir: "/Users/junya/repos/my-app1/backend",
			basePath:   "/Users/junya/repos",
			want:       "my-app1/backend",
		},
		{
			name:       "base strips single level",
			projectDir: "/Users/junya/repos/my-app1",
			basePath:   "/Users/junya/repos",
			want:       "my-app1",
		},
		{
			name:       "project dir not under base falls back to Base",
			projectDir: "/other/path/project",
			basePath:   "/Users/junya/repos",
			want:       "project",
		},
		{
			name:       "project dir equals base falls back to Base",
			projectDir: "/Users/junya/repos",
			basePath:   "/Users/junya/repos",
			want:       "repos",
		},
		{
			name:       "no base uses filepath.Base",
			projectDir: "/Users/junya/repos/myproject",
			want:       "myproject",
		},
		{
			name:           "no project dir falls back to transcript",
			transcriptPath: "/Users/junya/.claude/projects/-Users-junya-repos-ccrec/abc123.jsonl",
			want:           "ccrec",
		},
		{
			name:       "trailing slashes on both paths",
			projectDir: "/Users/junya/repos/my-app1/backend/",
			basePath:   "/Users/junya/repos/",
			want:       "my-app1/backend",
		},
		{
			name:           "base set but project dir empty falls back to transcript",
			basePath:       "/Users/junya/repos",
			transcriptPath: "/Users/junya/.claude/projects/-Users-junya-repos-ccrec/abc123.jsonl",
			want:           "ccrec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveProjectName(tt.projectDir, tt.basePath, tt.transcriptPath)
			if got != tt.want {
				t.Errorf("deriveProjectName(%q, %q, %q) = %q, want %q",
					tt.projectDir, tt.basePath, tt.transcriptPath, got, tt.want)
			}
		})
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "standard UUID filename",
			path: "/path/to/42bb222a-a575-4386-bae8-2b0ce9a93d40.jsonl",
			want: "42bb222a",
		},
		{
			name: "short filename",
			path: "/path/to/abc.jsonl",
			want: "abc",
		},
		{
			name: "no hyphens long name",
			path: "/path/to/abcdefghijklmnop.jsonl",
			want: "abcdefgh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionID(tt.path)
			if got != tt.want {
				t.Errorf("extractSessionID(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRunIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the binary.
	binPath := filepath.Join(t.TempDir(), "ccrec")
	build := exec.Command("go", "build", "-o", binPath, "../../cmd/ccrec")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Create a minimal transcript file.
	transcriptDir := t.TempDir()
	transcriptPath := filepath.Join(transcriptDir, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")
	transcriptContent := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-01-15T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"timestamp":"2026-01-15T10:00:01Z"}
`
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		claudeDir      string // CLAUDE_PROJECT_DIR env var (empty = unset)
		cwd            string // cwd field in stdin JSON
		base           string // -base flag (empty = omit)
		wantProjectDir string // expected subdirectory under output dir
	}{
		{
			name:           "CLAUDE_PROJECT_DIR takes priority over cwd",
			claudeDir:      "/Users/junya/repos/my-app",
			cwd:            "/Users/junya/repos/other-app",
			wantProjectDir: "my-app",
		},
		{
			name:           "CLAUDE_PROJECT_DIR with base",
			claudeDir:      "/Users/junya/repos/my-app/backend",
			cwd:            "/Users/junya/repos/other-app",
			base:           "/Users/junya/repos",
			wantProjectDir: "my-app/backend",
		},
		{
			name:           "falls back to cwd when CLAUDE_PROJECT_DIR is unset",
			claudeDir:      "",
			cwd:            "/Users/junya/repos/fallback-app",
			wantProjectDir: "fallback-app",
		},
		{
			name:           "cwd with base when CLAUDE_PROJECT_DIR is unset",
			claudeDir:      "",
			cwd:            "/Users/junya/repos/my-app/frontend",
			base:           "/Users/junya/repos",
			wantProjectDir: "my-app/frontend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := t.TempDir()

			args := []string{"hook", "-dir", outDir}
			if tt.base != "" {
				args = append(args, "-base", tt.base)
			}

			input := HookInput{
				TranscriptPath: transcriptPath,
				CWD:            tt.cwd,
			}
			stdinBytes, _ := json.Marshal(input)

			cmd := exec.Command(binPath, args...)
			cmd.Stdin = strings.NewReader(string(stdinBytes))
			cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+tt.claudeDir)
			if tt.claudeDir == "" {
				// Ensure it's truly unset.
				env := []string{}
				for _, e := range os.Environ() {
					if !strings.HasPrefix(e, "CLAUDE_PROJECT_DIR=") {
						env = append(env, e)
					}
				}
				cmd.Env = env
			}

			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("hook failed: %v\n%s", err, out)
			}

			// Verify the output file was created in the expected project directory.
			expectedDir := filepath.Join(outDir, tt.wantProjectDir)
			entries, err := os.ReadDir(expectedDir)
			if err != nil {
				t.Fatalf("expected directory %s does not exist: %v", expectedDir, err)
			}

			found := false
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".md") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no .md file found in %s", expectedDir)
			}
		})
	}
}

func TestRunIntegration_SkipsEmptyOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the binary.
	binPath := filepath.Join(t.TempDir(), "ccrec")
	build := exec.Command("go", "build", "-o", binPath, "../../cmd/ccrec")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Transcript with only meta/system records — no meaningful user or assistant messages.
	metaOnlyContent := `{"type":"user","isMeta":true,"message":{"role":"user","content":"meta"},"timestamp":"2026-01-15T10:00:00Z"}
{"type":"system","message":{"role":"system","content":"system info"},"timestamp":"2026-01-15T10:00:01Z"}
`
	// Transcript with only tool_use (no text) — filtered out without --tools.
	toolOnlyContent := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/test.go"}}]},"timestamp":"2026-01-15T10:00:00Z"}
`

	tests := []struct {
		name       string
		transcript string
		extraArgs  []string
		wantFile   bool
	}{
		{
			name:       "meta-only transcript produces no output",
			transcript: metaOnlyContent,
			wantFile:   false,
		},
		{
			name:       "tool-only transcript without --tools produces no output",
			transcript: toolOnlyContent,
			wantFile:   false,
		},
		{
			name:       "tool-only transcript with --tools produces output",
			transcript: toolOnlyContent,
			extraArgs:  []string{"-tools"},
			wantFile:   true,
		},
		{
			name:       "meta-only transcript with --all produces output",
			transcript: metaOnlyContent,
			extraArgs:  []string{"-all"},
			wantFile:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := t.TempDir()
			transcriptDir := t.TempDir()
			transcriptPath := filepath.Join(transcriptDir, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")
			if err := os.WriteFile(transcriptPath, []byte(tt.transcript), 0o644); err != nil {
				t.Fatal(err)
			}

			args := []string{"hook", "-dir", outDir}
			args = append(args, tt.extraArgs...)

			input := HookInput{
				TranscriptPath: transcriptPath,
				CWD:            "/Users/junya/repos/test-project",
			}
			stdinBytes, _ := json.Marshal(input)

			cmd := exec.Command(binPath, args...)
			cmd.Stdin = strings.NewReader(string(stdinBytes))
			// Remove CLAUDE_PROJECT_DIR to use cwd fallback.
			env := []string{}
			for _, e := range os.Environ() {
				if !strings.HasPrefix(e, "CLAUDE_PROJECT_DIR=") {
					env = append(env, e)
				}
			}
			cmd.Env = env

			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("hook failed: %v\n%s", err, out)
			}

			projectDir := filepath.Join(outDir, "test-project")
			entries, _ := os.ReadDir(projectDir)
			found := false
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".md") {
					found = true
					break
				}
			}

			if tt.wantFile && !found {
				t.Errorf("expected .md file in %s but none found", projectDir)
			}
			if !tt.wantFile && found {
				t.Errorf("expected no .md file in %s but one was created", projectDir)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	got := expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("absolute path should be unchanged, got %q", got)
	}

	got = expandHome("~/Documents")
	if got == "~/Documents" {
		t.Error("~/Documents should be expanded")
	}

	got = expandHome("~")
	if got == "~" {
		t.Error("bare ~ should be expanded")
	}
}
