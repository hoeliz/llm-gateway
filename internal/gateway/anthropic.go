package gateway

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func anthropicRequest(r Request) any {
	limit := 1024
	if r.MaxTokens != nil {
		limit = *r.MaxTokens
	}
	if r.MaxCompletionTokens != nil {
		limit = *r.MaxCompletionTokens
	}
	var system []string
	messages := make([]Message, 0, len(r.Messages))
	for _, m := range r.Messages {
		if m.Role == "system" || m.Role == "developer" {
			system = append(system, m.Content)
		} else {
			messages = append(messages, m)
		}
	}
	p := map[string]any{"model": r.Model, "messages": messages, "max_tokens": limit, "stream": r.Stream}
	if len(system) > 0 {
		p["system"] = strings.Join(system, "\n\n")
	}
	if r.Temperature != nil {
		p["temperature"] = *r.Temperature
	}
	return p
}

type usage struct {
	Input       int `json:"input_tokens"`
	Output      int `json:"output_tokens"`
	CacheRead   int `json:"cache_read_input_tokens"`
	CacheCreate int `json:"cache_creation_input_tokens"`
}

func (u usage) openAI() any {
	in := u.Input + u.CacheRead + u.CacheCreate
	return map[string]int{"prompt_tokens": in, "completion_tokens": u.Output, "total_tokens": in + u.Output}
}

type anthropicMessage struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      usage  `json:"usage"`
}

func finishReason(reason string) (string, error) {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop", nil
	case "max_tokens":
		return "length", nil
	case "refusal":
		return "content_filter", nil
	default:
		return "", errors.New("unsupported stop reason")
	}
}
func convertAnthropic(body []byte) (any, error) {
	var m anthropicMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	if m.ID == "" || m.Model == "" {
		return nil, errors.New("missing message metadata")
	}
	var text strings.Builder
	for _, b := range m.Content {
		if b.Type != "text" {
			return nil, errors.New("only text responses supported")
		}
		text.WriteString(b.Text)
	}
	reason, err := finishReason(m.StopReason)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": m.ID, "object": "chat.completion", "created": time.Now().Unix(), "model": m.Model, "choices": []any{map[string]any{"index": 0, "message": Message{"assistant", text.String()}, "finish_reason": reason}}, "usage": m.Usage.openAI()}, nil
}

// readSSE supports CRLF, comments and multiline data; each event is bounded to 1 MiB.
func readSSE(r io.Reader, consume func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var lines []string
	size := 0
	dispatch := func() error {
		if len(lines) == 0 {
			return nil
		}
		err := consume(strings.Join(lines, "\n"))
		lines = nil
		size = 0
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			v := strings.TrimPrefix(line, "data:")
			v = strings.TrimPrefix(v, " ")
			size += len(v)
			if size > 1<<20 {
				return errors.New("event too large")
			}
			lines = append(lines, v)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return dispatch()
}

var errDone = errors.New("stream complete")

func sendData(w http.ResponseWriter, data string) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return http.NewResponseController(w).Flush()
}
func sendEvent(w http.ResponseWriter, event any) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return sendData(w, string(b))
}
func streamOpenAI(w http.ResponseWriter, r io.Reader) error {
	err := readSSE(r, func(data string) error {
		if data != "[DONE]" && !json.Valid([]byte(data)) {
			return errors.New("invalid event")
		}
		if err := sendData(w, data); err != nil {
			return err
		}
		if data == "[DONE]" {
			return errDone
		}
		return nil
	})
	if errors.Is(err, errDone) {
		return nil
	}
	if err == nil {
		return io.ErrUnexpectedEOF
	}
	return err
}
func streamAnthropic(w http.ResponseWriter, r io.Reader, req Request) error {
	id := ""
	created := time.Now().Unix()
	var tokens usage
	finished := false
	chunk := func(delta any, reason any) error {
		return sendEvent(w, map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
	}
	err := readSSE(r, func(data string) error {
		var e struct {
			Type         string           `json:"type"`
			Message      anthropicMessage `json:"message"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			return err
		}
		switch e.Type {
		case "message_start":
			if id != "" || e.Message.ID == "" {
				return errors.New("invalid message start")
			}
			id = e.Message.ID
			tokens = e.Message.Usage
			return chunk(map[string]string{"role": "assistant", "content": ""}, nil)
		case "content_block_start":
			if id == "" || e.ContentBlock.Type != "text" {
				return errors.New("unsupported content")
			}
			if e.ContentBlock.Text != "" {
				return chunk(map[string]string{"content": e.ContentBlock.Text}, nil)
			}
		case "content_block_delta":
			if id == "" || finished || e.Delta.Type != "text_delta" {
				return errors.New("unsupported delta")
			}
			return chunk(map[string]string{"content": e.Delta.Text}, nil)
		case "message_delta":
			if id == "" || finished {
				return errors.New("invalid message delta")
			}
			tokens.Output = e.Usage.Output
			reason, err := finishReason(e.Delta.StopReason)
			if err != nil {
				return err
			}
			finished = true
			return chunk(map[string]string{}, reason)
		case "message_stop":
			if !finished {
				return errors.New("incomplete message")
			}
			if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
				if err := sendEvent(w, map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": req.Model, "choices": []any{}, "usage": tokens.openAI()}); err != nil {
					return err
				}
			}
			if err := sendData(w, "[DONE]"); err != nil {
				return err
			}
			return errDone
		case "error":
			return errors.New("provider stream error")
		}
		return nil
	})
	if errors.Is(err, errDone) {
		return nil
	}
	if err == nil {
		return io.ErrUnexpectedEOF
	}
	return err
}
