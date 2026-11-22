package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestRunnerVisualCheckerMultiturnRevisionAndFailOpen(t *testing.T) {
	for _, revise := range []bool{true, false} {
		name := "revision"
		if !revise {
			name = "no-verdict"
		}
		t.Run(name, func(t *testing.T) {
			resources, reference := realFixture(t)
			counter := &atomic.Int64{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				round := counter.Add(1)
				body, _ := io.ReadAll(r.Body)
				if round == 2 && bytes.Count(body, []byte("data:image/")) != 2 {
					t.Error("checker did not receive original and redrawn images")
				}
				message := Object{"role": "assistant", "content": ""}
				switch round {
				case 1:
					message["tool_calls"] = modelCalls()
				case 2:
					if revise {
						argument, _ := json.Marshal(Object{"report": Object{"status": "issues", "issues": "Review the drawing"}})
						message["tool_calls"] = []Object{{"id": "check-issues", "type": "function", "function": Object{"name": "submit", "arguments": string(argument)}}}
					} else {
						message["content"] = "No verdict"
					}
				case 3, 4, 5, 6:
					calls := modelCalls()
					call := calls[round-3]
					call["id"] = "revision-" + call["id"].(string)
					if round == 3 {
						argument, _ := json.Marshal(Object{"path": "figure.lua", "content": fixtureLua + "-- reviewed diagram\n"})
						call["function"].(map[string]any)["arguments"] = string(argument)
					}
					message["tool_calls"] = []Object{call}
				case 7:
					argument, _ := json.Marshal(Object{"report": Object{"status": "pass"}})
					message["tool_calls"] = []Object{{"id": "check-pass", "type": "function", "function": Object{"name": "submit", "arguments": string(argument)}}}
				default:
					t.Error("unexpected model request", round)
					message["content"] = "stop"
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Object{"choices": []any{Object{"message": message, "finish_reason": "tool_calls"}}, "usage": Object{"prompt_tokens": 200, "completion_tokens": 40}})
			}))
			defer server.Close()
			opts := runnerOptions(t, resources, reference, server.URL+"/v1")
			opts.FigureCheck = true
			t.Setenv("COMPARE_FIGURE_TEST_KEY", "fake-key")
			result, e := Run(context.Background(), opts)
			if e != nil || !Bool(result, "submitted", false) {
				t.Fatal(e, result)
			}
			source, e := os.ReadFile(Str(result, "editable_source", ""))
			if e != nil {
				t.Fatal(e)
			}
			if revise {
				if counter.Load() != 7 || !bytes.Contains(source, []byte("reviewed diagram")) {
					t.Fatal("correction did not complete all tool turns", counter.Load(), string(source))
				}
				if warnings, ok := result["warnings"].([]string); ok && len(warnings) > 0 {
					t.Fatal(warnings)
				}
			} else {
				if counter.Load() != 2 || !bytes.Equal(source, []byte(fixtureLua)) {
					t.Fatal("fail-open changed accepted source", counter.Load(), string(source))
				}
				if warnings, ok := result["warnings"].([]string); !ok || len(warnings) == 0 {
					t.Fatal("missing checker warning", result)
				}
			}
		})
	}
}
