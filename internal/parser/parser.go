package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// maxScannerBuf is the maximum retained size of one JSONL record. Longer
// records are discarded without preventing later records from being parsed.
const maxScannerBuf = 16 * 1024 * 1024

type jsonPresence bool

func (p *jsonPresence) UnmarshalJSON([]byte) error {
	*p = true
	return nil
}

type envelope struct {
	Type    string       `json:"type"`
	Message jsonPresence `json:"message,omitempty"`
	Payload jsonPresence `json:"payload,omitempty"`
}

type parsedLine struct {
	line     int
	raw      []byte
	envelope envelope
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
		result = parseClaudeLines(lines, opts)
	case ProviderCodex:
		result = parseCodexLines(lines, opts)
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
				return nil, strictDiagnosticError(diagnostic)
			}
		}
	}

	return result, nil
}

func strictDiagnosticError(diagnostic Diagnostic) error {
	if diagnostic.Line > 0 {
		return fmt.Errorf("strict parsing failed: line %d: %s", diagnostic.Line, diagnostic.Message)
	}
	return fmt.Errorf("strict parsing failed: %s", diagnostic.Message)
}

func readLines(r io.Reader) ([]parsedLine, []Diagnostic, int, error) {
	reader := bufio.NewReaderSize(r, 64*1024)

	var lines []parsedLine
	var diagnostics []Diagnostic
	lineNum := 0
	inputRecords := 0
	for {
		var line []byte
		tooLong := false
		reachedEOF := false
		for {
			fragment, err := reader.ReadSlice('\n')
			if err == nil || errors.Is(err, io.EOF) {
				fragment = bytesTrimLineEnding(fragment)
			}
			if !tooLong {
				if len(line)+len(fragment) > maxScannerBuf {
					line = nil
					tooLong = true
				} else {
					line = append(line, fragment...)
				}
			}

			switch {
			case err == nil:
			case errors.Is(err, bufio.ErrBufferFull):
				continue
			case errors.Is(err, io.EOF):
				reachedEOF = true
			default:
				return lines, diagnostics, inputRecords, fmt.Errorf("read transcript: %w", err)
			}
			break
		}

		if reachedEOF && len(line) == 0 && !tooLong {
			break
		}
		lineNum++
		line = bytesTrimLineEnding(line)
		if tooLong {
			inputRecords++
			diagnostics = append(diagnostics, Diagnostic{
				Line:    lineNum,
				Message: fmt.Sprintf("record exceeds %d-byte limit (skipped)", maxScannerBuf),
				Strict:  true,
			})
			if reachedEOF {
				break
			}
			continue
		}
		if len(line) == 0 {
			if reachedEOF {
				break
			}
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
			raw:      line,
			envelope: env,
		})
		if reachedEOF {
			break
		}
	}
	return lines, diagnostics, inputRecords, nil
}

func bytesTrimLineEnding(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}

func detectProvider(lines []parsedLine) (Provider, error) {
	for _, line := range lines {
		env := line.envelope
		if env.Payload {
			switch env.Type {
			case "session_meta", "response_item", "event_msg":
				return ProviderCodex, nil
			}
		}
		if env.Message && (env.Type == "user" || env.Type == "assistant") {
			return ProviderClaude, nil
		}
	}
	return "", fmt.Errorf("could not detect transcript provider")
}

func parseClaudeLines(lines []parsedLine, opts ParseOptions) *Result {
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
			if opts.Images {
				images, imageErr := extractImages(rec.Message.Content)
				if imageErr != nil {
					result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line.line, Message: imageErr.Error(), Strict: true})
				} else {
					rec.Images = normalizeImages(images, line.line, result)
				}
			}
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
