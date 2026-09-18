// Package safefile provides crash-safe replacement of sensitive output files.
package safefile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Write renders a complete file into a same-directory temporary file, syncs
// it, and atomically replaces path. The previous file remains intact if render
// or synchronization fails.
func Write(path string, mode os.FileMode, render func(io.Writer) error) (err error) {
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temporary output: %w", err)
	}
	if err := render(temp); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary output: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}
