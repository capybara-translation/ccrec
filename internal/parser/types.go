package parser

import (
	"encoding/json"
	"time"
)

// Record represents a single line in the JSONL transcript file.
type Record struct {
	Type       string          `json:"type"`
	Message    *Message        `json:"message,omitempty"`
	UUID       string          `json:"uuid,omitempty"`
	ParentUUID string          `json:"parentUuid,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	SessionID  string          `json:"sessionId,omitempty"`
	IsMeta     bool            `json:"isMeta,omitempty"`
	Slug       string          `json:"slug,omitempty"`
	CWD        string          `json:"cwd,omitempty"`
}

// Message represents the message field within a record.
type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Model      string          `json:"model,omitempty"`
	StopReason *string         `json:"stop_reason,omitempty"`
	Usage      *Usage          `json:"usage,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ContentBlock represents a single block within an assistant's content array.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Source    *ImageSource    `json:"source,omitempty"`
}

// ImageSource represents the source of an image content block.
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}
