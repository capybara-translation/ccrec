package parser

import "fmt"

// Provider identifies a transcript producer.
type Provider string

const (
	ProviderAuto   Provider = "auto"
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

// Phase identifies the user-visible phase of an assistant message.
type Phase string

const (
	PhaseNone       Phase = ""
	PhaseCommentary Phase = "commentary"
	PhaseFinal      Phase = "final_answer"
)

// ParseOptions controls transcript parsing.
type ParseOptions struct {
	Provider Provider
	Strict   bool
}

// Diagnostic describes a recoverable transcript issue.
type Diagnostic struct {
	Line    int
	Message string
	Strict  bool
}

// Result contains normalized records and transcript metadata.
type Result struct {
	Provider          Provider
	Records           []*Record
	Diagnostics       []Diagnostic
	InputRecords      int
	SupportedMessages int
	SessionID         string
}

// ParseProvider validates a provider flag value.
func ParseProvider(value string) (Provider, error) {
	provider := Provider(value)
	switch provider {
	case ProviderAuto, ProviderClaude, ProviderCodex:
		return provider, nil
	default:
		return "", fmt.Errorf("unknown provider %q (want auto, claude, or codex)", value)
	}
}
