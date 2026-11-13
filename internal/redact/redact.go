// Package redact masks credentials in rendered Markdown using the Betterleaks
// default rule set. Scanning runs in process and makes no network requests.
package redact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/betterleaks/betterleaks/v2/config"
	"github.com/betterleaks/betterleaks/v2/report"
	"github.com/betterleaks/betterleaks/v2/scan"
	"github.com/betterleaks/betterleaks/v2/sources"
)

// maxDecodeDepth matches the Betterleaks CLI default so that base64 or
// otherwise encoded credentials are found as well.
const maxDecodeDepth = 5

// minRepeatLength is the shortest detected value whose other, unlabelled
// occurrences are also redacted. Shorter values (for example "hunter2") are
// too likely to be ordinary words, so only the detected occurrence is masked.
const minRepeatLength = 12

// Redactor masks credentials found by Betterleaks.
type Redactor struct {
	scanner *scan.Scanner
}

// New builds a Redactor from the Betterleaks default rules.
func New() (*Redactor, error) {
	cfg, err := config.Default()
	if err != nil {
		return nil, fmt.Errorf("load betterleaks rules: %w", err)
	}
	scanner, err := scan.New(cfg,
		// Transcripts are not code under review: an allow marker typed in a
		// conversation must not let a credential through.
		scan.WithAllowSignatures(),
		scan.WithMaxDecodeDepth(maxDecodeDepth),
	)
	if err != nil {
		return nil, fmt.Errorf("create betterleaks scanner: %w", err)
	}
	return &Redactor{scanner: scanner}, nil
}

// Redact returns text with each detected credential replaced by
// "[REDACTED:<rule-id>]". It fails rather than returning partially redacted
// text.
func (r *Redactor) Redact(ctx context.Context, text string) (string, error) {
	var findings []report.Finding
	if _, err := r.scanner.Scan(ctx, wholeText(text), func(f report.Finding) error {
		findings = append(findings, f)
		return nil
	}); err != nil {
		return "", fmt.Errorf("scan for secrets: %w", err)
	}
	return applyFindings(text, findings)
}

// wholeText yields the entire text as a single fragment. sources.Reader cuts
// input without a blank line into about 125 KB chunks and silently misses a
// secret that straddles a cut. Scanning one fragment costs parallelism, which
// is acceptable for transcript-sized Markdown.
type wholeText string

func (t wholeText) Fragments(_ context.Context, yield sources.FragmentsFunc) error {
	return yield(sources.Fragment{Raw: string(t)}, nil)
}

// Wrap returns a render function that buffers the output of render, redacts
// it, and only then writes it to the destination. Unredacted output never
// reaches the destination, and nothing is written if rendering or scanning
// fails.
func Wrap(render func(io.Writer) error) func(io.Writer) error {
	return func(w io.Writer) error {
		var buf bytes.Buffer
		if err := render(&buf); err != nil {
			return err
		}
		r, err := New()
		if err != nil {
			return err
		}
		redacted, err := r.Redact(context.Background(), buf.String())
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, redacted)
		return err
	}
}

type span struct {
	start, end int // byte offsets, end exclusive
	rule       string
}

// target is one value to mask: a primary finding or a component of a
// composite rule.
type target struct {
	rule        string
	value       string
	location    report.Location
	decodeDepth int
	// maskRepeats also masks unlabelled repeats of a long value. Components
	// are excluded because many are non-secret (usernames, hosts, tenant IDs).
	maskRepeats bool
}

func applyFindings(text string, findings []report.Finding) (string, error) {
	if len(findings) == 0 {
		return text, nil
	}
	lineStarts := lineStartOffsets(text)
	var spans []span
	var unlocated []string
	for _, t := range targets(findings) {
		found := targetSpans(text, lineStarts, t)
		if len(found) == 0 {
			unlocated = append(unlocated, t.rule)
		}
		spans = append(spans, found...)
	}
	if len(unlocated) > 0 {
		// Never include values here: the message may reach logs or stderr.
		return "", fmt.Errorf("could not locate %d detected secret(s) for redaction (rules: %s)", len(unlocated), strings.Join(unlocated, ", "))
	}
	return replaceSpans(text, spans), nil
}

// targets flattens findings and the components of composite rules. Component
// rules are not reported on their own, so a component such as
// aws-secret-access-key appears only inside its parent's ComponentSets.
func targets(findings []report.Finding) []target {
	var out []target
	for _, f := range findings {
		out = append(out, target{f.RuleID, f.Match.Value, f.Location, f.DecodeDepth, true})
		for _, set := range f.ComponentSets {
			for _, c := range set.Components {
				out = append(out, target{c.RuleID, c.Match.Value, c.Location, c.DecodeDepth, false})
			}
		}
	}
	return out
}

func targetSpans(text string, lineStarts []int, t target) []span {
	start, end, ok := locate(text, lineStarts, t.location)
	if ok && t.decodeDepth > 0 {
		// The location covers the encoded text; the decoded value does not
		// appear literally, so mask the whole encoded span.
		return []span{{start, end, t.rule}}
	}

	var spans []span
	if ok {
		matched := text[start:end]
		// The value usually ends a "key = value" match, so search from the
		// end to avoid hitting the same characters inside the key.
		if i := strings.LastIndex(matched, t.value); t.value != "" && i >= 0 {
			spans = append(spans, span{start + i, start + i + len(t.value), t.rule})
		} else if t.value == "" {
			spans = append(spans, span{start, end, t.rule})
		} else {
			ok = false
		}
	}
	// Without a trustworthy location, fall back to masking every literal
	// occurrence so that a location mismatch never leaks the value.
	if (!ok || t.maskRepeats && len(t.value) >= minRepeatLength) && t.value != "" {
		spans = append(spans, occurrences(text, t.value, t.rule)...)
	}
	return spans
}

// locate converts a one-based line and inclusive byte-column location into
// byte offsets of text.
func locate(text string, lineStarts []int, loc report.Location) (start, end int, ok bool) {
	if loc.StartLine < 1 || loc.EndLine < loc.StartLine || loc.EndLine > len(lineStarts) || loc.StartColumn < 1 || loc.EndColumn < 1 {
		return 0, 0, false
	}
	start = lineStarts[loc.StartLine-1] + loc.StartColumn - 1
	end = lineStarts[loc.EndLine-1] + loc.EndColumn
	if start >= end || end > len(text) {
		return 0, 0, false
	}
	return start, end, true
}

func lineStartOffsets(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func occurrences(text, value, rule string) []span {
	var spans []span
	for offset := 0; ; {
		i := strings.Index(text[offset:], value)
		if i < 0 {
			return spans
		}
		start := offset + i
		spans = append(spans, span{start, start + len(value), rule})
		offset = start + len(value)
	}
}

// replaceSpans merges overlapping or touching spans so that no fragment of a
// secret survives between two markers, then substitutes each merged span.
func replaceSpans(text string, spans []span) string {
	if len(spans) == 0 {
		return text
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})
	merged := spans[:1]
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end {
			last.end = max(last.end, s.end)
			continue
		}
		merged = append(merged, s)
	}

	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	for _, s := range merged {
		b.WriteString(text[prev:s.start])
		b.WriteString("[REDACTED:" + s.rule + "]")
		prev = s.end
	}
	b.WriteString(text[prev:])
	return b.String()
}
