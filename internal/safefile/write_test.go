package safefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestWrite_ConcurrentWritersPublishOnlyCompleteContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversation.md")
	const writers = 12
	contents := make([]string, writers)
	var wg sync.WaitGroup
	for i := range writers {
		contents[i] = strings.Repeat(string(rune('a'+i)), 16*1024)
		wg.Add(1)
		go func(content string) {
			defer wg.Done()
			if err := Write(path, 0o600, func(w io.Writer) error {
				_, err := io.WriteString(w, content)
				return err
			}); err != nil {
				t.Errorf("Write: %v", err)
			}
		}(contents[i])
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	matched := false
	for _, want := range contents {
		if got == want {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("published content was partial or interleaved: %d bytes", len(data))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "conversation.md" {
		t.Fatalf("temporary files remained after concurrent writes: %v", entries)
	}
}

func TestWrite_ReplacesSymlinkWithoutFollowingIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	path := filepath.Join(dir, "conversation.md")
	if err := os.WriteFile(target, []byte("target remains"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if err := Write(path, 0o600, func(w io.Writer) error {
		_, err := io.WriteString(w, "new output")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("output path is still a symlink: info=%v err=%v", info, err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "target remains" {
		t.Fatalf("symlink target was modified: %q", data)
	}
}
