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
	TurnID    string         `json:"turn_id"`
	Text      string         `json:"text"`
	Message   string         `json:"message"`
	Content   []codexContent `json:"content"`
	Item      *codexItem     `json:"item"`
	Metadata  struct {
		ContentItemKinds []string `json:"content_item_kinds"`
		TurnID           string   `json:"turn_id"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

type codexItem struct {
	Type    string         `json:"type"`
	Phase   Phase          `json:"phase"`
	Content []codexContent `json:"content"`
}

type codexContent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url"`
}

type codexImageQueue struct {
	byTurn    map[string][][]ImageSource
	next      map[string]int
	ambiguous map[string]bool
}

func parseCodexLines(lines []parsedLine, opts ParseOptions) *Result {
	result := &Result{Provider: ProviderCodex}
	records := make([]codexRecord, len(lines))

	hasCurrentVisibleEvents := false
	hasLegacyVisibleEvents := false
	for i, line := range lines {
		if err := json.Unmarshal(line.raw, &records[i]); err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line.line, Message: err.Error() + " (skipped)"})
			records[i] = codexRecord{}
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
	var images *codexImageQueue
	if hasCurrentVisibleEvents && opts.Images {
		images = newCodexImageQueue(records, lines, result)
	}

	for i, line := range lines {
		record := records[i]
		var normalized *Record
		switch {
		case hasCurrentVisibleEvents:
			normalized = normalizeCurrentCodexEvent(record, line.line, result, images, opts.Images)
		case hasLegacyVisibleEvents:
			normalized = normalizeLegacyCodexEvent(record, line.line, result)
		default:
			normalized = normalizeCodexResponse(record, line.line, result, opts.Images)
		}
		if normalized != nil {
			result.Records = append(result.Records, normalized)
		}
	}
	result.SupportedMessages = len(result.Records)

	return result
}

func normalizeCurrentCodexEvent(record codexRecord, line int, result *Result, images *codexImageQueue, includeImages bool) *Record {
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
	var normalizedImages []ImageSource
	if role == "user" && includeImages {
		expectedImages := countCodexContentType(item.Content, "local_image")
		queuedImages, found := images.pop(record.Payload.TurnID)
		switch {
		case expectedImages == 0 && len(queuedImages) == 0:
		case !found || len(queuedImages) != expectedImages:
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Line:    line,
				Message: fmt.Sprintf("Codex image count mismatch: expected %d local image(s), found %d embedded user.image(s)", expectedImages, len(queuedImages)),
				Strict:  true,
			})
		default:
			normalizedImages = queuedImages
		}
	}
	return newNormalizedRecord(role, item.Phase, textFromCurrentCodexContent(item.Content), normalizedImages, record.Timestamp, line, result)
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
	return newNormalizedRecord(role, record.Payload.Phase, text, nil, record.Timestamp, line, result)
}

func normalizeCodexResponse(record codexRecord, line int, result *Result, includeImages bool) *Record {
	if record.Type != "response_item" || record.Payload.Type != "message" {
		return nil
	}

	switch record.Payload.Role {
	case "user":
		kinds := record.Payload.Metadata.ContentItemKinds
		hasUserText := slices.Contains(kinds, "user.text")
		hasUserImage := slices.Contains(kinds, "user.image")
		hasUnknownKind := containsUnknownContentKind(kinds)
		if hasUnknownKind {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Line:    line,
				Message: "Codex user response_item contains an unknown content visibility kind",
				Strict:  true,
			})
		}
		if !hasUserText && !hasUserImage {
			if allKnownHiddenContentKinds(kinds) {
				return nil
			}
			if !hasUnknownKind {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Line:    line,
					Message: "skipped Codex user response_item without explicit user.text or user.image visibility",
					Strict:  true,
				})
			}
			return nil
		}
		var images []ImageSource
		if includeImages {
			images = imagesFromVisibleUserContent(record.Payload.Content, kinds, line, result)
		}
		return newNormalizedRecord("user", PhaseNone, textFromVisibleUserContent(record.Payload.Content, kinds), images, record.Timestamp, line, result)
	case "assistant":
		return newNormalizedRecord("assistant", record.Payload.Phase, textFromCodexContent(record.Payload.Content, "output_text"), nil, record.Timestamp, line, result)
	default:
		return nil
	}
}

func newNormalizedRecord(role string, phase Phase, text string, images []ImageSource, timestamp string, line int, result *Result) *Record {
	text = strings.TrimSpace(text)
	if text == "" && len(images) == 0 {
		return nil
	}
	record := &Record{
		Type:     role,
		Role:     role,
		Phase:    phase,
		Sequence: line,
		Provider: ProviderCodex,
		Text:     text,
		Images:   images,
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

func newCodexImageQueue(records []codexRecord, lines []parsedLine, result *Result) *codexImageQueue {
	queue := &codexImageQueue{
		byTurn:    make(map[string][][]ImageSource),
		next:      make(map[string]int),
		ambiguous: make(map[string]bool),
	}
	eventCounts := make(map[string]int)
	for i, record := range records {
		if record.Type == "event_msg" && record.Payload.Type == "item_completed" &&
			record.Payload.Item != nil && record.Payload.Item.Type == "UserMessage" && record.Payload.TurnID != "" {
			eventCounts[record.Payload.TurnID]++
		}
		if record.Type != "response_item" || record.Payload.Type != "message" || record.Payload.Role != "user" {
			continue
		}
		kinds := record.Payload.Metadata.ContentItemKinds
		if !slices.Contains(kinds, "user.text") && !slices.Contains(kinds, "user.image") {
			continue
		}
		turnID := record.Payload.Metadata.TurnID
		if turnID == "" {
			continue
		}
		images := imagesFromVisibleUserContent(record.Payload.Content, kinds, lines[i].line, result)
		queue.byTurn[turnID] = append(queue.byTurn[turnID], images)
	}
	for turnID, eventCount := range eventCounts {
		if len(queue.byTurn[turnID]) != eventCount {
			queue.ambiguous[turnID] = true
		}
	}
	return queue
}

func (q *codexImageQueue) pop(turnID string) ([]ImageSource, bool) {
	if q == nil || turnID == "" || q.ambiguous[turnID] {
		return nil, false
	}
	groups := q.byTurn[turnID]
	index := q.next[turnID]
	if index >= len(groups) {
		return nil, false
	}
	q.next[turnID] = index + 1
	return groups[index], true
}

func imagesFromVisibleUserContent(content []codexContent, kinds []string, line int, result *Result) []ImageSource {
	var images []ImageSource
	for i, block := range content {
		if i >= len(kinds) || kinds[i] != "user.image" || block.Type != "input_image" {
			continue
		}
		image, err := imageSourceFromDataURL(block.ImageURL)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line, Message: err.Error(), Strict: true})
			continue
		}
		validated, err := validateImageSource(image)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: line, Message: err.Error(), Strict: true})
			continue
		}
		images = append(images, validated)
	}
	return images
}

func imageSourceFromDataURL(value string) (ImageSource, error) {
	header, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") || data == "" {
		return ImageSource{}, fmt.Errorf("invalid Codex user.image data URL")
	}
	mediaType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	return ImageSource{Type: "base64", MediaType: mediaType, Data: data}, nil
}

func countCodexContentType(content []codexContent, target string) int {
	count := 0
	for _, block := range content {
		if block.Type == target {
			count++
		}
	}
	return count
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
		if kind == "user.text" || kind == "user.image" || isKnownHiddenContentKind(kind) {
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
