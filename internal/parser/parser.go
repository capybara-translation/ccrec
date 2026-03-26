package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	// maxScannerBuf is the maximum buffer size for bufio.Scanner.
	// 16 MB handles lines with large tool results (file reads, web fetches, etc.).
	maxScannerBuf = 16 * 1024 * 1024
)

// ParseFile reads a JSONL transcript file and returns records via a channel.
// It streams line-by-line to avoid loading the entire file into memory.
func ParseFile(path string) ([]*Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	return parseReader(f)
}

func parseReader(r io.Reader) ([]*Record, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBuf)

	var records []*Record
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		rec, err := parseLine(line)
		if err != nil {
			// Skip malformed lines rather than aborting the entire file.
			fmt.Fprintf(os.Stderr, "warning: line %d: %v (skipped)\n", lineNum, err)
			continue
		}
		if rec != nil {
			records = append(records, rec)
		}
	}

	if err := scanner.Err(); err != nil {
		return records, fmt.Errorf("scan error: %w", err)
	}

	return records, nil
}

func parseLine(line []byte) (*Record, error) {
	var rec Record
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &rec, nil
}
