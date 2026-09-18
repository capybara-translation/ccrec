package parser

import (
	"strings"
	"testing"
)

func BenchmarkParseCodexTranscript(b *testing.B) {
	input := strings.Repeat(`{"timestamp":"2026-09-17T00:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"benchmark answer"}}`+"\n", 1000)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for b.Loop() {
		if _, err := parseReaderWithOptions(strings.NewReader(input), ParseOptions{Provider: ProviderCodex}); err != nil {
			b.Fatal(err)
		}
	}
}
