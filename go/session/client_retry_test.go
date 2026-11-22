package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRetryDelayMatchesPythonSequence(t *testing.T) {
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for attempt, expected := range want {
		if got := retryDelay(attempt); got != expected {
			t.Errorf("attempt %d delay %s; want %s", attempt, got, expected)
		}
	}
}

func TestStreamHintFallbackAndSanitizedError(t *testing.T) {
	t.Run("not support hint on generic 400", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n++
			if n == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("endpoint does not support streamed responses"))
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"stream": true, "api_max_retries": 1})
		m, e := c.Complete(context.Background(), nil, nil, "auto", 10)
		if e != nil || TextOf(m) != "ok" || n != 2 {
			t.Fatalf("n=%d m=%#v err=%v", n, m, e)
		}
	})
	t.Run("error body never echoed", func(t *testing.T) {
		secret := "sensitive-provider-payload"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(secret))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"api_max_retries": 3})
		_, e := c.Complete(context.Background(), nil, nil, "auto", 10)
		if e == nil || strings.Contains(e.Error(), secret) || !strings.Contains(e.Error(), "credentials") {
			t.Fatalf("unexpected error: %v", e)
		}
	})
	t.Run("hint beyond capped error read is ignored", func(t *testing.T) {
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(strings.Repeat("x", 16<<10) + " unsupported stream"))
		}))
		defer srv.Close()
		c := NewClient(srv.URL, "m", "k", time.Second, map[string]any{"stream": true, "api_max_retries": 1})
		_, e := c.Complete(context.Background(), nil, nil, "auto", 10)
		if e == nil || n != 1 {
			t.Fatalf("n=%d err=%v", n, e)
		}
	})
}
