package runner

import (
	"compare-figure/session"
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestResumeRetainsRejectedSubmissionAttempts(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, _ := setupModel(t)
	opts := runnerOptions(t, resources, reference, endpoint)
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "")
	if _, err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected missing key")
	}
	transcript, err := session.NewTranscript(filepath.Join(opts.Out, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 8; index++ {
		id := fmt.Sprintf("previous-%d", index)
		if err = transcript.Message(session.Message{"role": "assistant", "content": "", "tool_calls": []any{Object{"id": id, "type": "function", "function": Object{"name": "submit", "arguments": "{}"}}}}); err != nil {
			t.Fatal(err)
		}
		if err = transcript.Message(session.Message{"role": "tool", "tool_call_id": id, "content": "REJECTED: previous attempt"}); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "fake-key")
	result, err := Run(context.Background(), opts)
	if err != nil || !Bool(result, "submitted", false) {
		t.Fatal(err, result)
	}
	if Num(result, "submit_attempts", 0) != 9 || count.Load() != 1 {
		t.Fatal("resume reset submission attempts", result, count.Load())
	}
}
