package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestShortHistoryCompactionDoesNotPanic(t *testing.T) {
	model := &scriptedCompleter{replies: []Message{{"role": "assistant", "content": "done"}}}
	s := NewSession(nil, model, nil, "sys", map[string]any{"context_limit": 1}, []Message{{"role": "system", "content": "sys"}})
	if _, err := s.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.compact(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestToolResultPruningPreservesUTF8(t *testing.T) {
	tr, err := NewTranscript(filepath.Join(t.TempDir(), "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	toolText := strings.Repeat("中文内容", 80)
	messages := []Message{{"role": "system", "content": "sys"}, {"role": "user", "content": "task"}, {"role": "tool", "tool_call_id": "x", "content": toolText}}
	for i := 0; i < 9; i++ {
		messages = append(messages, Message{"role": "user", "content": strings.Repeat("tail", 50)})
	}
	model := &scriptedCompleter{replies: []Message{{"role": "assistant", "content": "compact note"}, {"role": "assistant", "content": "done"}}}
	s := NewSession(nil, model, tr, "sys", map[string]any{"context_limit": 1, "compaction_at": 0.1, "prune_tool_chars": 7}, messages)
	if _, err = s.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	got := TextOf(model.seen[0][2])
	if !utf8.ValidString(got) {
		t.Fatalf("pruned tool text is invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "[tool result pruned]") {
		t.Fatalf("tool result was not pruned: %q", got)
	}
}

func TestCompactionCheckpointResumesWithoutExtraOpeningTurn(t *testing.T) {
	tr, err := NewTranscript(filepath.Join(t.TempDir(), "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	messages := []Message{{"role": "system", "content": "old system"}, {"role": "user", "content": "original task"}}
	for i := 0; i < 10; i++ {
		messages = append(messages, Message{"role": "user", "content": "history segment"})
	}
	model := &scriptedCompleter{replies: []Message{{"role": "assistant", "content": "summary of work"}, {"role": "assistant", "content": "finished"}}}
	s := NewSession(nil, model, tr, "fresh system", map[string]any{"context_limit": 1, "compaction_at": 0.1}, messages)
	if _, err = s.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if s.Compactions != 1 {
		t.Fatalf("compactions=%d", s.Compactions)
	}
	resumed, _, err := tr.Load("fresh system")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range resumed {
		if m["role"] == "user" && TextOf(m) == "original task" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("original task appeared %d times in resumed history", count)
	}
	if len(resumed) < 3 || !strings.Contains(TextOf(resumed[2]), "=== COMPRESSED SESSION CONTEXT") {
		t.Fatalf("checkpoint summary missing: %#v", resumed)
	}
}

func TestBudgetGraceForcesToolChoiceNone(t *testing.T) {
	toolReply := func(id string) Message {
		return Message{"role": "assistant", "content": "", "tool_calls": []any{tc(id, "write_file", map[string]any{})}}
	}
	model := &scriptedCompleter{replies: []Message{toolReply("1"), toolReply("2"), {"role": "assistant", "content": "best effort"}}}
	s := NewSession(&testTools{}, model, nil, "sys", map[string]any{"max_tool_rounds": 1, "tool_rounds_grace": 1}, nil)
	if _, err := s.Run(context.Background(), "task", nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"auto", "auto", "none"}
	if len(model.choices) != len(want) {
		t.Fatalf("choices %#v", model.choices)
	}
	for i := range want {
		if model.choices[i] != want[i] {
			t.Fatalf("choices %#v", model.choices)
		}
	}
}

func TestRoundsCountCustomCompleterRequests(t *testing.T) {
	model := &scriptedCompleter{replies: []Message{{"role": "assistant", "content": "done"}}}
	s := NewSession(nil, model, nil, "sys", nil, nil)
	if _, err := s.Run(context.Background(), "task", nil); err != nil {
		t.Fatal(err)
	}
	if s.Rounds != 1 {
		t.Fatalf("rounds=%d", s.Rounds)
	}
}
