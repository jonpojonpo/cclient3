package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamCallback receives streaming events from the SSE parser.
type StreamCallback interface {
	OnMessageStart(msg Response)
	OnContentBlockStart(index int, block ResponseBlock)
	OnTextDelta(index int, text string)
	OnThinkingDelta(index int, thinking string)
	OnInputJSONDelta(index int, partialJSON string)
	OnContentBlockStop(index int)
	OnMessageDelta(delta MessageDelta, usage *Usage)
	OnMessageStop()
	OnError(err error)
}

// ParseSSEStream reads an SSE stream line by line and dispatches to the callback.
func ParseSSEStream(r io.Reader, cb StreamCallback) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var eventType string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			eventType = ""
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if err := dispatchEvent(eventType, []byte(data), cb); err != nil {
				cb.OnError(err)
			}
			continue
		}
	}

	return scanner.Err()
}

func dispatchEvent(eventType string, data []byte, cb StreamCallback) error {
	switch eventType {
	case "message_start":
		var evt MessageStartEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return fmt.Errorf("parse message_start: %w", err)
		}
		cb.OnMessageStart(evt.Message)

	case "content_block_start":
		var evt ContentBlockStartEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return fmt.Errorf("parse content_block_start: %w", err)
		}
		cb.OnContentBlockStart(evt.Index, evt.ContentBlock)

	case "content_block_delta":
		var evt ContentBlockDeltaEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return fmt.Errorf("parse content_block_delta: %w", err)
		}
		switch evt.Delta.Type {
		case "text_delta":
			cb.OnTextDelta(evt.Index, evt.Delta.Text)
		case "thinking_delta":
			cb.OnThinkingDelta(evt.Index, evt.Delta.Thinking)
		case "input_json_delta":
			cb.OnInputJSONDelta(evt.Index, evt.Delta.PartialJSON)
		}

	case "content_block_stop":
		var evt ContentBlockStopEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return fmt.Errorf("parse content_block_stop: %w", err)
		}
		cb.OnContentBlockStop(evt.Index)

	case "message_delta":
		var evt MessageDeltaEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return fmt.Errorf("parse message_delta: %w", err)
		}
		cb.OnMessageDelta(evt.Delta, evt.Usage)

	case "message_stop":
		cb.OnMessageStop()

	case "ping":
		// ignore

	case "error":
		return fmt.Errorf("stream error: %s", string(data))
	}

	return nil
}
