package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/luadraw"
)

func TestDocumentationMemoryBudgetsAndDisabled(t *testing.T) {
	text := "  引用一 " + strings.Repeat("几何", 500) + "\n\n引用二 " + strings.Repeat("立体", 500) + "\n引用三"
	got := boundedDocumentationMemory(text, 2, 32, 50)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 || len([]rune(got)) > 50 {
		t.Fatalf("budget not respected: %q", got)
	}
	for _, line := range lines {
		if len([]rune(line)) > 32 {
			t.Fatalf("line too long %q", line)
		}
	}
	empty, err := documentationMemory(context.Background(), Object{"docs": Object{"frequent": Object{"prompt_enabled": false}}}, Object{"luadraw_root": "not-a-real-root"})
	if err != nil || empty != "" {
		t.Fatalf("disabled memory touched library: %s %v", empty, err)
	}
}

func TestDocumentationMemoryUsesRecordedParagraphs(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	dir := filepath.Join(root, "doc", "src")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	text := "Dcircle constructs a real LuaDraw circle.\nThe radius and center determine its geometry.\n"
	if err := os.WriteFile(filepath.Join(dir, "memory.tex"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := luadraw.NewDocs(root, cache)
	if err != nil {
		t.Skipf("documentation environment unavailable: %v", err)
	}
	if _, _, err = docs.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/memory.tex", "start_line": 1, "end_line": 2}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	options := Object{"luadraw_root": root, "doc_cache_dir": cache}
	cachePath := filepath.Join(cache, "luadraw-doc-frequency-v1.json")
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := documentationMemory(context.Background(), Object{}, options)
	if err != nil {
		t.Fatal(err)
	}
	after, readErr := os.ReadFile(cachePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(before) != string(after) {
		t.Fatal("passive prompt memory changed frequency counters")
	}
	if !strings.Contains(got, "Dcircle") || !strings.Contains(got, "memory.tex") {
		t.Fatalf("recorded reference missing: %q", got)
	}
	if len([]rune(got)) > 2048 || len(strings.Split(got, "\n")) > 8 {
		t.Fatalf("default prompt budgets exceeded: %s", got)
	}
}
