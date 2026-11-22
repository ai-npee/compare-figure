package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProtocolRequests(t *testing.T) {
	for _, api := range []string{"openai", "responses", "anthropic"} {
		t.Run(api, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var b map[string]any
				_ = json.NewDecoder(r.Body).Decode(&b)
				if api == "responses" && r.URL.Path != "/responses" {
					t.Errorf("path %s", r.URL.Path)
				}
				if api == "anthropic" && r.URL.Path != "/messages" {
					t.Errorf("path %s", r.URL.Path)
				}
				if api == "openai" && r.URL.Path != "/chat/completions" {
					t.Errorf("path %s", r.URL.Path)
				}
				if api == "anthropic" {
					if r.Header.Get("x-api-key") != "secret" {
						t.Error("missing anthropic key")
					}
					_, _ = w.Write([]byte(`{"content":[{"type":"thinking","thinking":"plan","signature":"sig"},{"type":"text","text":"ok"}],"usage":{"input_tokens":4,"output_tokens":2},"stop_reason":"end_turn"}`))
					return
				}
				if api == "responses" {
					_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"reasoning","encrypted_content":"opaque"},{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":4,"output_tokens":2}}`))
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, "m", "secret", time.Second, map[string]any{"api": api})
			m, e := c.Complete(context.Background(), []Message{{"role": "system", "content": "sys"}, {"role": "user", "content": "hi"}}, nil, "auto", 32)
			if e != nil {
				t.Fatal(e)
			}
			if TextOf(m) != "ok" {
				t.Fatalf("message %#v", m)
			}
			if c.Finish == "" {
				t.Fatal("finish missing")
			}
		})
	}
}
func TestSSEAndStreamFallback(t *testing.T) {
	t.Run("chat sse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n"))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "", time.Second, map[string]any{"stream": true})
		m, e := c.Complete(context.Background(), nil, nil, "auto", 12)
		if e != nil || TextOf(m) != "hello" {
			t.Fatalf("%v %#v", e, m)
		}
	})
	t.Run("explicit unsupported only", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n++
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if n == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte("stream unsupported"))
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fallback"},"finish_reason":"stop"}]}`))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "", time.Second, map[string]any{"stream": true})
		m, e := c.Complete(context.Background(), nil, nil, "auto", 10)
		if e != nil || TextOf(m) != "fallback" || n != 2 {
			t.Fatalf("fallback n=%d err=%v message=%v", n, e, m)
		}
	})
}
func TestTranscriptV3FixtureAndInterruptedCall(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "session.jsonl")
	fixture := `{"type":"message","message":{"role":"user","content":"legacy"}}` + "\n" + `{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"x","type":"function","function":{"name":"write_file","arguments":"{}"}}]}` + "\n"
	if e := os.WriteFile(p, []byte(fixture), 0600); e != nil {
		t.Fatal(e)
	}
	tr, e := NewTranscript(p)
	if e != nil {
		t.Fatal(e)
	}
	ms, stats, e := tr.Load("system")
	if e != nil {
		t.Fatal(e)
	}
	if len(ms) != 4 || stats["rounds"] != 1 || ms[3]["role"] != "tool" {
		t.Fatalf("resume %#v %#v", ms, stats)
	}
	if TextOf(ms[1]) != "legacy" {
		t.Fatalf("legacy message missing %#v", ms[1])
	}
}
func TestImagePartDataURL(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.png")
	_ = os.WriteFile(p, []byte("not really png"), 0600)
	v, e := ImagePart(p, 100)
	if e != nil {
		t.Fatal(e)
	}
	u := asMap(v["image_url"])["url"].(string)
	if !strings.HasPrefix(u, "data:image/png;base64,") {
		t.Fatal(u)
	}
}
func TestResponsesAndAnthropicSSE(t *testing.T) {
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
