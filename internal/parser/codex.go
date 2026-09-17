package parser

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type codexRecord struct {
	Timestamp string       `json:"timestamp"`
	Type      string       `json:"type"`
	Payload   codexPayload `json:"payload"`
}

type codexPayload struct {
	Type      string         `json:"type"`
	Role      string         `json:"role"`
	Phase     Phase          `json:"phase"`
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	Text      string         `json:"text"`
	Message   string         `json:"message"`
	Content   []codexContent `json:"content"`
	Item      *codexItem     `json:"item"`
	Metadata  struct {
		ContentItemKinds []string `json:"content_item_kinds"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

type codexItem struct {
	Type    string         `json:"type"`
	Phase   Phase          `json:"phase"`
	Content []codexContent `json:"content"`
}

type codexContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func parseCodexLines(lines []parsedLine) *Result {
	result := &Result{Provider: ProviderCodex}
	records := make([]codexRecord, len(lines))

	hasCurrentVisibleEvents := false
	hasLegacyVisibleEvents := false
	for i, line := range lines {
		if err := json.Unmarshal(line.raw, &records[i]); err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line.line, Message: err.Error() + " (skipped)"})
			continue
		}
		record := records[i]
		if record.Type == "session_meta" && result.SessionID == "" {
			result.SessionID = firstNonEmpty(record.Payload.SessionID, record.Payload.ID)
		}
		if record.Type != "event_msg" {
			continue
		}
		// Codex rollout policy distinguishes paginated ItemCompleted events
		// from UserMessage/AgentMessage events used by older rollouts and some
		// subagent threads. Prefer the paginated family whenever both appear so
		// the same visible message is not emitted twice. See
		// codex-rs/rollout/src/policy.rs.
		if record.Payload.Type == "item_completed" && record.Payload.Item != nil {
			switch record.Payload.Item.Type {
			case "UserMessage", "AgentMessage":
				hasCurrentVisibleEvents = true
			}
		}
		switch record.Payload.Type {
		case "user_message", "agent_message":
			hasLegacyVisibleEvents = true
		}
	}

	for i, line := range lines {
		record := records[i]
		var normalized *Record
		switch {
		case hasCurrentVisibleEvents:
			normalized = normalizeCurrentCodexEvent(record, line.line, result)
		case hasLegacyVisibleEvents:
			normalized = normalizeLegacyCodexEvent(record, line.line, result)
		default:
			normalized = normalizeCodexResponse(record, line.line, result)
		}
		if normalized != nil {
			result.Records = append(result.Records, normalized)
		}
	}
	result.SupportedMessages = len(result.Records)

	return result
}

func normalizeCurrentCodexEvent(record codexRecord, line int, result *Result) *Record {
	if record.Type != "event_msg" || record.Payload.Type != "item_completed" || record.Payload.Item == nil {
		return nil
	}

	item := record.Payload.Item
	var role string
	switch item.Type {
	case "UserMessage":
		role = "user"
	case "AgentMessage":
		role = "assistant"
	default:
		return nil
	}
	return newNormalizedRecord(role, item.Phase, textFromCurrentCodexContent(item.Content), record.Timestamp, line, result)
}

func normalizeLegacyCodexEvent(record codexRecord, line int, result *Result) *Record {
	if record.Type != "event_msg" {
		return nil
	}

	var role string
	switch record.Payload.Type {
	case "user_message":
		role = "user"
	case "agent_message":
		role = "assistant"
	default:
		return nil
	}
	text := firstNonEmpty(record.Payload.Message, record.Payload.Text, textFromCodexContent(record.Payload.Content, "text"))
	return newNormalizedRecord(role, record.Payload.Phase, text, record.Timestamp, line, result)
}

func normalizeCodexResponse(record codexRecord, line int, result *Result) *Record {
	if record.Type != "response_item" || record.Payload.Type != "message" {
		return nil
	}

	switch record.Payload.Role {
	case "user":
		kinds := record.Payload.Metadata.ContentItemKinds
		hasUserText := slices.Contains(kinds, "user.text")
		hasUnknownKind := containsUnknownContentKind(kinds)
		if hasUnknownKind {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Line:    line,
				Message: "Codex user response_item contains an unknown content visibility kind",
				Strict:  true,
			})
		}
		if !hasUserText {
			if allKnownHiddenContentKinds(kinds) {
				return nil
			}
			if !hasUnknownKind {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Line:    line,
					Message: "skipped Codex user response_item without explicit user.text visibility",
					Strict:  true,
				})
			}
			return nil
		}
		return newNormalizedRecord("user", PhaseNone, textFromVisibleUserContent(record.Payload.Content, record.Payload.Metadata.ContentItemKinds), record.Timestamp, line, result)
	case "assistant":
		return newNormalizedRecord("assistant", record.Payload.Phase, textFromCodexContent(record.Payload.Content, "output_text"), record.Timestamp, line, result)
	default:
		return nil
	}
}

func newNormalizedRecord(role string, phase Phase, text, timestamp string, line int, result *Result) *Record {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	content, _ := json.Marshal(text)
	record := &Record{
		Type:     role,
		Role:     role,
		Phase:    phase,
		Sequence: line,
		Provider: ProviderCodex,
		Text:     text,
		Message: &Message{
			Role:    role,
			Content: content,
		},
	}
	if timestamp == "" {
		return record
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line, Message: fmt.Sprintf("invalid timestamp %q", timestamp)})
		return record
	}
	record.Timestamp = parsed
	return record
}

func textFromCodexContent(content []codexContent, requiredType string) string {
	var text strings.Builder
	for _, block := range content {
		if requiredType != "" && block.Type != requiredType {
			continue
		}
		text.WriteString(block.Text)
	}
	return text.String()
}

func textFromCurrentCodexContent(content []codexContent) string {
	var text strings.Builder
	for _, block := range content {
		// Codex 0.145.0 uses "text" for UserMessage and "Text" for
		// AgentMessage. Do not accept arbitrary blocks merely because they
		// happen to carry a text field.
		if block.Type != "text" && block.Type != "Text" {
			continue
		}
		text.WriteString(block.Text)
	}
	return text.String()
}

func textFromVisibleUserContent(content []codexContent, kinds []string) string {
	var text strings.Builder
	for i, block := range content {
		if i >= len(kinds) || kinds[i] != "user.text" || block.Type != "input_text" {
			continue
		}
		text.WriteString(block.Text)
	}
	return text.String()
}

func allKnownHiddenContentKinds(kinds []string) bool {
	if len(kinds) == 0 {
		return false
	}
	for _, kind := range kinds {
		if !isKnownHiddenContentKind(kind) {
			return false
		}
	}
	return true
}

func containsUnknownContentKind(kinds []string) bool {
	for _, kind := range kinds {
		if kind == "user.text" || isKnownHiddenContentKind(kind) {
			continue
		}
		return true
	}
	return false
}

func isKnownHiddenContentKind(kind string) bool {
	return strings.HasPrefix(kind, "agents_md.") ||
		strings.HasPrefix(kind, "environments.") ||
		strings.HasPrefix(kind, "plugins.") ||
		strings.HasPrefix(kind, "permissions.")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
