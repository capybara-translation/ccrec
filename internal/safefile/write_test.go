package safefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_ReplacesAtomicallyWithRequestedPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversation.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Write(path, 0o600, func(w io.Writer) error {
		_, err := io.WriteString(w, "complete")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "complete" {
		t.Fatalf("content = %q, want complete", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "conversation.md" {
		t.Fatalf("unexpected files after atomic write: %v", entries)
	}
}

func TestWrite_FailureKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversation.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("format failed")

	err := Write(path, 0o600, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("failed write replaced old content with %q", data)
	}
}
