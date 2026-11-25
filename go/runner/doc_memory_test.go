package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/documents"
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
	large := boundedDocumentationMemory(strings.Join([]string{
		strings.Repeat("a", 900),
		strings.Repeat("b", 900),
		strings.Repeat("c", 900),
	}, "\n"), 8, 1024, 4096)
	largeLines := strings.Split(large, "\n")
	if len([]rune(large)) <= 2048 || len([]rune(largeLines[0])) <= 512 {
		t.Fatalf("new default-sized budget was truncated at the old limit: total=%d first_line=%d", len([]rune(large)), len([]rune(largeLines[0])))
	}
	if len(largeLines) != 3 || len([]rune(large)) > 4096 {
		t.Fatalf("new budget not respected: lines=%d total=%d", len(largeLines), len([]rune(large)))
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
	if err := os.MkdirAll(filepath.Join(root, "files"), 0700); err != nil {
		t.Fatal(err)
	}
	text := "Dcircle constructs a real LuaDraw circle.\nThe radius and center determine its geometry.\n"
	if err := os.WriteFile(filepath.Join(dir, "memory.tex"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := documents.New(root, cache, "luadraw")
	if err != nil {
		t.Skipf("documentation environment unavailable: %v", err)
	}
	if _, _, err = docs.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/memory.tex", "start_line": 1, "end_line": 2}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	options := Object{"resource_root": root, "doc_cache_dir": cache, "drawing_mode": "luadraw"}
	before, err := docs.FrequentSnippet(context.Background(), 8, 1024, 4096)
	if err != nil {
		t.Fatal(err)
	}
	got, err := documentationMemory(context.Background(), Object{}, options)
	if err != nil {
		t.Fatal(err)
	}
	after, err := docs.FrequentSnippet(context.Background(), 8, 1024, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("passive prompt memory changed frequency counters")
	}
	if !strings.Contains(got, "Dcircle") || !strings.Contains(got, "memory.tex") {
		t.Fatalf("recorded reference missing: %q", got)
	}
	if len([]rune(got)) > 4096 || len(strings.Split(got, "\n")) > 8 {
		t.Fatalf("default prompt budgets exceeded: %s", got)
	}
}
