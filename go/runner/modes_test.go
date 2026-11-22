package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAutomaticModeDirectModelURLAndEphemeralKey(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, _ := setupModel(t)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_API_KEY", "")
	scale := -1.0
	key := "direct-mode-secret-not-persisted"
	opts := Options{Original: reference, Out: filepath.Join(t.TempDir(), "automatic"), ResourceRoot: resources, Model: "fake-vision", BaseURL: endpoint, APIKey: key, MaxScaleDev: &scale}
	result, err := Run(context.Background(), opts)
	if err != nil || !Bool(result, "submitted", false) || count.Load() != 1 {
		t.Fatal(err, result, count.Load())
	}
	metadata, err := readObject(filepath.Join(opts.Out, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if Num(metadata, "fig_min_overlap", 0) != .17 {
		t.Fatal(metadata)
	}
	for _, name := range []string{"run.json", "session.jsonl", "result.json"} {
		data, err := os.ReadFile(filepath.Join(opts.Out, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(key)) {
			t.Fatalf("ephemeral key persisted in %s", name)
		}
	}
}
