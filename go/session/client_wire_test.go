package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProtocolRequestBodies(t *testing.T) {
	tool := []map[string]any{{"type": "function", "function": map[string]any{"name": "submit", "description": "submit", "parameters": map[string]any{"type": "object"}}}}
	t.Run("responses image and reasoning", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if asMap(b["reasoning"])["effort"] != "high" {
				t.Errorf("reasoning %#v", b["reasoning"])
			}
			input := arr(b["input"])
			parts := arr(asMap(input[0])["content"])
			if asMap(parts[1])["type"] != "input_image" {
				t.Errorf("image part %#v", parts)
			}
			_, _ = w.Write([]byte(`{"status":"completed","output":[]}`))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"api": "responses", "reasoning_effort": "high"})
		_, e := c.Complete(context.Background(), []Message{{"role": "user", "content": []any{map[string]any{"type": "text", "text": "look"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}}}}}, tool, "none", 99)
		if e != nil {
			t.Fatal(e)
		}
	})
	t.Run("anthropic none thinking signed replay", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if asMap(b["tool_choice"])["type"] != "none" {
				t.Errorf("tool_choice %#v", b["tool_choice"])
			}
			if asMap(b["thinking"])["type"] != "enabled" {
				t.Errorf("thinking %#v", b["thinking"])
			}
			if _, ok := b["reasoning_effort"]; ok {
				t.Errorf("unexpected reasoning_effort %#v", b)
			}
			if _, ok := b["output_config"]; ok {
				t.Errorf("unexpected automatic output_config %#v", b)
			}
			if b["tool_stream"] != true {
				t.Errorf("tool_stream %#v", b["tool_stream"])
			}
			blocks := arr(asMap(arr(b["messages"])[0])["content"])
			if asMap(blocks[0])["signature"] != "sig" {
				t.Errorf("signed blocks %#v", blocks)
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"api": "anthropic", "thinking": map[string]any{"type": "enabled", "budget_tokens": 100}, "reasoning_effort": "high", "tool_stream": true})
		_, e := c.Complete(context.Background(), []Message{{"role": "assistant", "content": "reason", "_anthropic_blocks": []any{map[string]any{"type": "thinking", "thinking": "reason", "signature": "sig"}}}}, tool, "none", 99)
		if e != nil {
			t.Fatal(e)
		}
	})
	t.Run("openai options and fragmented multi-tools", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			for k, v := range map[string]any{"tool_stream": true, "user": "u", "reasoning_effort": "medium"} {
				if b[k] != v {
					t.Errorf("%s=%v want %v", k, b[k], v)
				}
			}
			w.Header().Set("Content-Type", "text/event-stream")
			events := []string{
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"write_file"}},{"index":1,"id":"b","function":{"name":"submit"}}]}}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\""}},{"index":1,"function":{"arguments":"{}"}}]}}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"a\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`}
			for _, line := range events {
				_, _ = w.Write([]byte(line + "\n\n"))
			}
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"tool_stream": true, "user": "u", "reasoning_effort": "medium", "stream": true})
		m, e := c.Complete(context.Background(), nil, tool, "auto", 99)
		if e != nil {
			t.Fatal(e)
		}
		calls := arr(m["tool_calls"])
		if len(calls) != 2 {
			t.Fatalf("calls %#v", calls)
		}
		fn := asMap(asMap(calls[0])["function"])
		if fn["name"] != "write_file" || fn["arguments"] != `{"path":"a"}` {
			t.Fatalf("first call %#v", fn)
		}
		if asMap(asMap(calls[1])["function"])["arguments"] != "{}" {
			t.Fatalf("second call %#v", calls[1])
		}
	})
}
func TestResponsesAndAnthropicSSEWire(t *testing.T) {
	for _, api := range []string{"responses", "anthropic"} {
		t.Run(api, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if api == "responses" {
					_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"reasoning\",\"encrypted_content\":\"cipher\"},{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}],\"usage\":{\"input_tokens\":4}}}\n\n"))
					return
				}
				_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2}}}\n\n" + "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" + "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"reason\"}}\n\n" + "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig\"}}\n\n" + "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" + "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n" + "data: {\"type\":\"message_stop\"}\n\n"))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, "m", "", time.Second, map[string]any{"api": api, "stream": true})
			m, e := c.Complete(context.Background(), []Message{{"role": "user", "content": "hi"}}, nil, "auto", 20)
			if e != nil {
				t.Fatal(e)
			}
			if api == "responses" {
				if TextOf(m) != "done" || len(arr(m["_response_items"])) != 2 {
					t.Fatalf("responses result %#v", m)
				}
			} else {
				blocks := arr(m["_anthropic_blocks"])
				if len(blocks) != 1 || asMap(blocks[0])["thinking"] != "reason" || asMap(blocks[0])["signature"] != "sig" {
					t.Fatalf("anthropic blocks %#v", blocks)
				}
			}
		})
	}
}
func TestStreamUnsupportedFallback(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte("stream unsupported"))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fallback"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "m", "", time.Second, map[string]any{"stream": true, "api_max_retries": 1})
	m, e := c.Complete(context.Background(), nil, nil, "auto", 10)
	if e != nil || TextOf(m) != "fallback" || n != 2 {
		t.Fatalf("n=%d m=%#v err=%v", n, m, e)
	}
}
func TestHTTPRetryPolicy(t *testing.T) {
	t.Run("transient then succeeds", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n++
			if n < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", 4*time.Second, map[string]any{"api_max_retries": 3})
		m, e := c.Complete(context.Background(), nil, nil, "auto", 10)
		if e != nil || TextOf(m) != "ok" || n != 3 {
			t.Fatalf("n=%d m=%#v err=%v", n, m, e)
		}
	})
	t.Run("ordinary 400 once", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n++; w.WriteHeader(http.StatusBadRequest) }))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"api_max_retries": 3})
		_, e := c.Complete(context.Background(), nil, nil, "auto", 10)
		if e == nil || n != 1 {
			t.Fatalf("n=%d err=%v", n, e)
		}
	})
	t.Run("cancel backoff", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
		defer srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"api_max_retries": 0})
		start := time.Now()
		_, e := c.Complete(ctx, nil, nil, "auto", 10)
		if e == nil || time.Since(start) > time.Second {
			t.Fatalf("err=%v elapsed=%v", e, time.Since(start))
		}
	})
}
