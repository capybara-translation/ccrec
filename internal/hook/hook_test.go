package hook

import "testing"

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

func TestExpandHome(t *testing.T) {
	got := expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("absolute path should be unchanged, got %q", got)
	}

	got = expandHome("~/Documents")
	if got == "~/Documents" {
		t.Error("~ should be expanded")
	}
}
