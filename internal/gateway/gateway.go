package gateway

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"llm-gateway/internal/webui"
)

type Config struct {
	APIKey, OpenAIKey, AnthropicKey string
	OpenAIURL, AnthropicURL         string
	Timeout                         time.Duration
}

type Gateway struct {
	config Config
	client *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Request struct {
	Model               string    `json:"model"`
	Messages            []Message `json:"messages"`
	Stream              bool      `json:"stream,omitempty"`
	MaxTokens           *int      `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64  `json:"temperature,omitempty"`
	StreamOptions       *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

func New(c Config) (*Gateway, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("GATEWAY_API_KEY is required")
	}
	if c.OpenAIKey == "" && c.AnthropicKey == "" {
		return nil, errors.New("configure at least one provider API key")
	}
	if c.OpenAIURL == "" {
		c.OpenAIURL = "https://api.openai.com/v1"
	}
	if c.AnthropicURL == "" {
		c.AnthropicURL = "https://api.anthropic.com/v1"
	}
	for _, raw := range []string{c.OpenAIURL, c.AnthropicURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("invalid provider base URL")
		}
	}
	if c.Timeout == 0 {
		c.Timeout = 2 * time.Minute
	}
	if c.Timeout < 0 {
		return nil, errors.New("timeout must be positive")
	}
	return &Gateway{c, &http.Client{Timeout: c.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	ui := webui.Handler()
	mux.Handle("GET /{$}", ui)
	mux.Handle("GET /assets/", ui)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /v1/chat/completions", g.chat)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
			want := sha256.Sum256([]byte("Bearer " + g.config.APIKey))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				fail(w, 401, "authentication_error", "invalid gateway API key")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (g *Gateway) chat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req Request
	if err := dec.Decode(&req); err != nil {
		fail(w, 400, "invalid_request_error", "invalid JSON or unsupported field (text messages only)")
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		fail(w, 400, "invalid_request_error", "expected one JSON object")
		return
	}
	provider, model, ok := strings.Cut(req.Model, "/")
	if !ok || strings.TrimSpace(model) == "" || (provider != "openai" && provider != "anthropic") {
		fail(w, 400, "invalid_request_error", "model must be openai/<model> or anthropic/<model>")
		return
	}
	if len(req.Messages) == 0 {
		fail(w, 400, "invalid_request_error", "messages must not be empty")
		return
	}
	seenConversation := false
	for _, m := range req.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" && m.Role != "developer" {
			fail(w, 400, "invalid_request_error", "unsupported message role")
			return
		}
		if provider == "anthropic" && (m.Role == "system" || m.Role == "developer") && seenConversation {
			fail(w, 400, "invalid_request_error", "Anthropic requires system/developer instructions before conversation messages")
			return
		}
		if m.Role == "user" || m.Role == "assistant" {
			seenConversation = true
		}
	}
	if !seenConversation {
		fail(w, 400, "invalid_request_error", "at least one user or assistant message is required")
		return
	}
	if (req.MaxTokens != nil && *req.MaxTokens <= 0) || (req.MaxCompletionTokens != nil && *req.MaxCompletionTokens <= 0) || (req.MaxTokens != nil && req.MaxCompletionTokens != nil) {
		fail(w, 400, "invalid_request_error", "provide one positive token limit")
		return
	}
	upper := 2.0
	if provider == "anthropic" {
		upper = 1
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > upper) {
		fail(w, 400, "invalid_request_error", fmt.Sprintf("temperature must be between 0 and %g", upper))
		return
	}
	if req.StreamOptions != nil && !req.Stream {
		fail(w, 400, "invalid_request_error", "stream_options requires stream=true")
		return
	}
	key, endpoint := g.config.OpenAIKey, strings.TrimRight(g.config.OpenAIURL, "/")+"/chat/completions"
	req.Model = model
	var payload any = req
	if provider == "anthropic" {
		key = g.config.AnthropicKey
		endpoint = strings.TrimRight(g.config.AnthropicURL, "/") + "/messages"
		payload = anthropicRequest(req)
	}
	if key == "" {
		fail(w, 503, "provider_unavailable", "provider API key is not configured")
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		fail(w, 500, "internal_error", "cannot encode request")
		return
	}
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		fail(w, 500, "internal_error", "cannot create upstream request")
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	if provider == "openai" {
		upstream.Header.Set("Authorization", "Bearer "+key)
	} else {
		upstream.Header.Set("x-api-key", key)
		upstream.Header.Set("anthropic-version", "2023-06-01")
	}
	response, err := g.client.Do(upstream)
	if err != nil {
		status := 502
		var netErr interface{ Timeout() bool }
		if errors.As(err, &netErr) && netErr.Timeout() {
			status = 504
		}
		fail(w, status, "upstream_error", "provider request failed")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := response.StatusCode
		if status == 401 || status == 403 || status < 400 {
			status = 502
		}
		if retry := response.Header.Get("Retry-After"); retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		fail(w, status, "upstream_error", fmt.Sprintf("%s returned HTTP %d", provider, response.StatusCode))
		return
	}
	if req.Stream {
		if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
			fail(w, 502, "upstream_error", "expected provider event stream")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		if provider == "anthropic" {
			err = streamAnthropic(w, response.Body, req)
		} else {
			err = streamOpenAI(w, response.Body)
		}
		if err != nil {
			_ = sendEvent(w, map[string]any{"error": map[string]string{"type": "upstream_error", "message": "provider stream interrupted or unsupported"}})
		}
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		fail(w, 502, "upstream_error", "provider response unreadable or too large")
		return
	}
	if provider == "anthropic" {
		result, err := convertAnthropic(body)
		if err != nil {
			fail(w, 502, "upstream_error", "unsupported provider response")
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if !json.Valid(body) {
		fail(w, 502, "upstream_error", "provider returned invalid JSON")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, kind, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"type": kind, "message": msg}})
}
