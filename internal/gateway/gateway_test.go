package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T, h http.HandlerFunc) *Gateway {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	g, err := New(Config{APIKey: "local-secret", OpenAIKey: "oa-secret", AnthropicKey: "anth-secret", OpenAIURL: s.URL, AnthropicURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func call(g *Gateway, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer local-secret")
	w := httptest.NewRecorder()
	g.Handler().ServeHTTP(w, r)
	return w
}
func TestOpenAIRouting(t *testing.T) {
	g := setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer oa-secret" || r.Header.Get("x-api-key") != "" {
			t.Error("incorrect routing or credentials")
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Model != "test-model" || req.Messages[0].Content != "hello" {
			t.Error("incorrect payload")
		}
		io.WriteString(w, `{"id":"chat-1","choices":[{"message":{"role":"assistant","content":"hi"}}]}`)
	})
	w := call(g, `{"model":"openai/test-model","messages":[{"role":"user","content":"hello"}]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "chat-1") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
func TestAnthropicTranslation(t *testing.T) {
	g := setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" || r.Header.Get("x-api-key") != "anth-secret" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect Anthropic headers")
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["system"] != "be concise" || payload["max_tokens"] != float64(42) || payload["model"] != "test-model" {
			t.Errorf("bad translation: %v", payload)
		}
		if len(payload["messages"].([]any)) != 1 {
			t.Error("system must not remain in messages")
		}
		io.WriteString(w, `{"id":"msg-1","model":"test-model","content":[{"type":"text","text":"Olá"},{"type":"text","text":"!"}],"stop_reason":"max_tokens","usage":{"input_tokens":10,"output_tokens":3,"cache_read_input_tokens":2,"cache_creation_input_tokens":4}}`)
	})
	w := call(g, `{"model":"anthropic/test-model","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hello"}],"max_completion_tokens":42}`)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	var result struct {
		Choices []struct {
			Message      Message
			FinishReason string `json:"finish_reason"`
		}
		Usage struct {
			Total int `json:"total_tokens"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Choices[0].Message.Content != "Olá!" || result.Choices[0].FinishReason != "length" || result.Usage.Total != 19 {
		t.Fatalf("bad result %s", w.Body)
	}
}
func TestAuthAndValidation(t *testing.T) {
	g := setup(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached upstream") })
	w := httptest.NewRecorder()
	g.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	g.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	bodies := []string{
		`null`, `{}`, `{"model":"other/x","messages":[{"role":"user","content":"x"}]}`,
		`{"model":"openai/x","messages":[{"role":"user","content":[{"type":"text","text":"x"}]}]}`,
		`{"model":"openai/x","messages":[{"role":"user","content":"x"}],"tools":[]}`,
		`{"model":"openai/x","messages":[{"role":"user","content":"x"}]} {}`,
		`{"model":"openai/x","messages":[{"role":"user","content":"x"}],"max_tokens":0}`,
		`{"model":"openai/x","messages":[{"role":"user","content":"x"}],"max_tokens":2,"max_completion_tokens":3}`,
		`{"model":"anthropic/x","messages":[{"role":"user","content":"x"},{"role":"system","content":"x"}]}`,
		`{"model":"anthropic/x","messages":[{"role":"user","content":"x"}],"temperature":1.5}`,
		`{"model":"openai/x","messages":[{"role":"user","content":"x"}],"stream_options":{"include_usage":true}}`,
	}
	for _, body := range bodies {
		if w := call(g, body); w.Code != 400 {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	big := `{"model":"openai/x","messages":[{"role":"user","content":"` + strings.Repeat("x", 1<<20) + `"}]}`
	if w := call(g, big); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestUpstreamErrors(t *testing.T) {
	for _, status := range []int{401, 429, 500, 307} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			g := setup(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "10")
				w.WriteHeader(status)
				io.WriteString(w, "provider-secret")
			})
			w := call(g, `{"model":"openai/x","messages":[{"role":"user","content":"x"}]}`)
			want := status
			if status == 401 || status == 307 {
				want = 502
			}
			if w.Code != want || strings.Contains(w.Body.String(), "provider-secret") || w.Header().Get("Retry-After") != "10" {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

const anthropicEvents = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-1\",\"usage\":{\"input_tokens\":5}}}\n\n" +
	"data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
	"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Olá\"}}\n\n" +
	"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
	"data: {\"type\":\"message_stop\"}\n\n"

func TestStreaming(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			g := setup(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if provider == "anthropic" {
					io.WriteString(w, anthropicEvents)
				} else {
					io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Olá\"}}]}\n\ndata: [DONE]\n\n")
				}
			})
			w := call(g, fmt.Sprintf(`{"model":"%s/x","messages":[{"role":"user","content":"x"}],"stream":true,"stream_options":{"include_usage":true}}`, provider))
			body := w.Body.String()
			if w.Code != 200 || !w.Flushed || !strings.Contains(body, "Olá") || !strings.HasSuffix(body, "data: [DONE]\n\n") {
				t.Fatal(body)
			}
			if provider == "anthropic" && (!strings.Contains(body, `"total_tokens":7`) || !strings.Contains(body, `"finish_reason":"stop"`)) {
				t.Fatal(body)
			}
		})
	}
}
func TestBrokenStreams(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		g := setup(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "data: {\"type\":\"error\"}\n\n")
		})
		w := call(g, fmt.Sprintf(`{"model":"%s/x","messages":[{"role":"user","content":"x"}],"stream":true}`, provider))
		if !strings.Contains(w.Body.String(), "upstream_error") || strings.Contains(w.Body.String(), "[DONE]") {
			t.Fatal(w.Body)
		}
	}
}
func TestTimeoutAndCancellation(t *testing.T) {
	g := setup(t, func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() })
	g.client.Timeout = 20 * time.Millisecond
	w := call(g, `{"model":"openai/x","messages":[{"role":"user","content":"x"}]}`)
	if w.Code != 504 {
		t.Fatal(w.Code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"openai/x","messages":[{"role":"user","content":"x"}]}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer local-secret")
	w = httptest.NewRecorder()
	g.Handler().ServeHTTP(w, r)
	if w.Code != 502 {
		t.Fatal(w.Code)
	}
}
func TestSSEMultiline(t *testing.T) {
	var got string
	err := readSSE(strings.NewReader(": ping\r\nevent: test\r\ndata: {\r\ndata: \"x\":1}\r\n\r\n"), func(s string) error { got = s; return nil })
	if err != nil || got != "{\n\"x\":1}" {
		t.Fatalf("%q %v", got, err)
	}
}
func TestConfiguration(t *testing.T) {
	for _, c := range []Config{{}, {APIKey: "x"}, {APIKey: "x", OpenAIKey: "y", OpenAIURL: "ftp://example.com"}, {APIKey: "x", OpenAIKey: "y", Timeout: -1}} {
		if _, err := New(c); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	g, err := New(Config{APIKey: "local-secret", OpenAIKey: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if w := call(g, `{"model":"anthropic/x","messages":[{"role":"user","content":"x"}]}`); w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestStreamingFlushesBeforeProviderFinishes(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	g := setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			io.WriteString(w, "data: [DONE]\n\n")
		case <-r.Context().Done():
		}
	})
	server := httptest.NewServer(g.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"openai/x","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer local-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("first event was buffered until provider completion:", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 6)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "data: " {
		t.Fatalf("bad event prefix %q", buf)
	}
}

func TestPlaygroundRoutesAndAPIAuthentication(t *testing.T) {
	g := setup(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected upstream call") })
	for _, path := range []string{"/", "/v1/chat/completions", "/assets/missing.js"} {
		w := httptest.NewRecorder()
		g.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		switch path {
		case "/":
			if w.Code != 200 || !strings.Contains(w.Body.String(), "LLM Gateway") {
				t.Fatalf("UI unavailable: %d %s", w.Code, w.Body)
			}
			if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing security/cache headers")
			}
		case "/v1/chat/completions":
			if w.Code != 401 {
				t.Fatalf("API accessible without a key: %d", w.Code)
			}
		case "/assets/missing.js":
			if w.Code != 404 {
				t.Fatalf("invalid asset status: %d", w.Code)
			}
		}
	}
}
