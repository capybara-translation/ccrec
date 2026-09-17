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

type envelope struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type parsedLine struct {
	line     int
	raw      []byte
	envelope envelope
}

// ParseFile preserves the original convenience API and auto-detects the
// provider. Recoverable diagnostics are written to stderr for compatibility.
func ParseFile(path string) ([]*Record, error) {
	result, err := ParseFileWithOptions(path, ParseOptions{Provider: ProviderAuto})
	if err != nil {
		return nil, err
	}
	writeDiagnostics(os.Stderr, result.Diagnostics)
	return result.Records, nil
}

// ParseFileWithOptions parses a transcript and returns normalized records.
func ParseFileWithOptions(path string, opts ParseOptions) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseReaderWithOptions(f, opts)
}

func parseReader(r io.Reader) ([]*Record, error) {
	result, err := parseReaderWithOptions(r, ParseOptions{Provider: ProviderAuto})
	if err != nil {
		return nil, err
	}
	writeDiagnostics(os.Stderr, result.Diagnostics)
	return result.Records, nil
}

func parseReaderWithOptions(r io.Reader, opts ParseOptions) (*Result, error) {
	if opts.Provider == "" {
		opts.Provider = ProviderAuto
	}
	if _, err := ParseProvider(string(opts.Provider)); err != nil {
		return nil, err
	}

	lines, diagnostics, inputRecords, err := readLines(r)
	if err != nil {
		return nil, err
	}
	if inputRecords == 0 {
		return &Result{
			Provider:    opts.Provider,
			Diagnostics: diagnostics,
		}, nil
	}

	provider := opts.Provider
	if provider == ProviderAuto {
		provider, err = detectProvider(lines)
		if err != nil {
			result := &Result{
				Provider:     ProviderAuto,
				InputRecords: inputRecords,
				Diagnostics:  diagnostics,
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: err.Error(), Strict: true})
			if opts.Strict {
				return nil, fmt.Errorf("strict parsing failed: %w", err)
			}
			return result, nil
		}
	} else if detected, detectErr := detectProvider(lines); detectErr == nil && detected != provider {
		return nil, fmt.Errorf("transcript provider is %s, not requested provider %s", detected, provider)
	}

	var result *Result
	switch provider {
	case ProviderClaude:
		result = parseClaudeLines(lines)
	case ProviderCodex:
		result = parseCodexLines(lines)
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
	result.Provider = provider
	result.InputRecords = inputRecords
	result.Diagnostics = append(diagnostics, result.Diagnostics...)
	if inputRecords > 0 && result.SupportedMessages == 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Message: fmt.Sprintf("transcript contained records but no supported messages were found (provider=%s)", provider),
			Strict:  true,
		})
	}

	if opts.Strict {
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Strict {
				return nil, fmt.Errorf("strict parsing failed: line %d: %s", diagnostic.Line, diagnostic.Message)
			}
		}
		if inputRecords > 0 && result.SupportedMessages == 0 {
			return nil, fmt.Errorf("strict parsing failed: transcript contained records but no supported messages were found (provider=%s)", provider)
		}
	}

	return result, nil
}

func readLines(r io.Reader) ([]parsedLine, []Diagnostic, int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBuf)

	var lines []parsedLine
	var diagnostics []Diagnostic
	lineNum := 0
	inputRecords := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		inputRecords++

		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Line:    lineNum,
				Message: fmt.Sprintf("json unmarshal: %v (skipped)", err),
			})
			continue
		}
		lines = append(lines, parsedLine{
			line:     lineNum,
			raw:      append([]byte(nil), line...),
			envelope: env,
		})
	}
	if err := scanner.Err(); err != nil {
		return lines, diagnostics, inputRecords, fmt.Errorf("scan error: %w", err)
	}
	return lines, diagnostics, inputRecords, nil
}

func detectProvider(lines []parsedLine) (Provider, error) {
	for _, line := range lines {
		env := line.envelope
		if len(env.Payload) > 0 {
			switch env.Type {
			case "session_meta", "response_item", "event_msg":
				return ProviderCodex, nil
			}
		}
		if len(env.Message) > 0 && (env.Type == "user" || env.Type == "assistant") {
			return ProviderClaude, nil
		}
	}
	return "", fmt.Errorf("could not detect transcript provider")
}

func parseClaudeLines(lines []parsedLine) *Result {
	result := &Result{Provider: ProviderClaude}
	for _, line := range lines {
		rec, err := parseLine(line.raw)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line.line, Message: err.Error() + " (skipped)"})
			continue
		}
		rec.Sequence = line.line
		rec.Provider = ProviderClaude
		if rec.Message != nil {
			rec.Role = rec.Message.Role
		}
		if rec.Role == "" {
			rec.Role = rec.Type
		}
		if rec.Message != nil && (rec.Role == "user" || rec.Role == "assistant") {
			result.SupportedMessages++
		}
		result.Records = append(result.Records, rec)
	}
	return result
}

func parseLine(line []byte) (*Record, error) {
	var rec Record
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &rec, nil
}

func writeDiagnostics(w io.Writer, diagnostics []Diagnostic) {
	for _, diagnostic := range diagnostics {
		if diagnostic.Line > 0 {
			fmt.Fprintf(w, "warning: line %d: %s\n", diagnostic.Line, diagnostic.Message)
		} else {
			fmt.Fprintf(w, "warning: %s\n", diagnostic.Message)
		}
	}
}
