package luadraw

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func frequencyFixture(t *testing.T) (string, string, *Docs) {
	t.Helper()
	root := t.TempDir()
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "doc", "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "doc", "src", "notes.tex"), []byte("Alpha canonical paragraph line one.\nAlpha canonical paragraph line two.\n\nBeta canonical paragraph.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocs(root, cache)
	if err != nil {
		t.Fatal(err)
	}
	return root, cache, d
}
func passiveEntries(t *testing.T, d *Docs) []any {
	t.Helper()
	return docResults(t, docResponse(t, d, map[string]any{"mode": "frequent", "passive": true, "limit": 256, "max_chars": 32000}))
}
func TestFrequencySharedCacheConcurrentReadsExactAndPassive(t *testing.T) {
	root, cache, d := frequencyFixture(t)
	ctx := context.Background()
	response := docResponse(t, d, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2})
	namespace := response["cache"].(map[string]any)["namespace"].(string)
	freqPath := filepath.Join(cache, namespace, "frequency-v1.json")
	before, err := os.ReadFile(freqPath)
	if err != nil {
		t.Fatal(err)
	}
	snippet, err := d.FrequentSnippet(ctx, 8, 256, 4096)
	if err != nil || snippet == "" || len([]rune(snippet)) > 2048 {
		t.Fatalf("passive snippet: %q %v", snippet, err)
	}
	after, err := os.ReadFile(freqPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("passive snippet changed disk frequency")
	}
	const workers, reads = 4, 3
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other, err := NewDocs(root, cache)
			if err != nil {
				errs <- err
				return
			}
			for j := 0; j < reads; j++ {
				if _, _, err = other.Execute(ctx, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2}, ""); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	entries := passiveEntries(t, d)
	if len(entries) != 1 || entries[0].(map[string]any)["hits"] != float64(1+workers*reads) {
		t.Fatalf("concurrent updates lost: %#v", entries)
	}
}
func TestFrequencyReadWindowsCanonicalIDAndSelection(t *testing.T) {
	_, _, d := frequencyFixture(t)
	for _, line := range []int{1, 2} {
		docResponse(t, d, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": line, "end_line": line})
	}
	entries := passiveEntries(t, d)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
	first := entries[0].(map[string]any)
	if first["start_line"] != float64(1) || first["end_line"] != float64(2) || first["hits"] != float64(2) {
		t.Fatalf("read windows lost paragraph identity: %#v", first)
	}
	id := first["entry_id"].(string)
	docResponse(t, d, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 4, "end_line": 4})
	selected := docResults(t, docResponse(t, d, map[string]any{"mode": "frequent", "entry_id": id[:6], "limit": -1, "max_chars": 128}))
	if len(selected) != 1 || selected[0].(map[string]any)["entry_id"] != id {
		t.Fatalf("prefix selection failed: %#v", selected)
	}
	entries = passiveEntries(t, d)
	if entries[0].(map[string]any)["hits"] != float64(3) {
		t.Fatalf("selected paragraph did not increment: %#v", entries)
	}
	rank := docResults(t, docResponse(t, d, map[string]any{"mode": "frequent", "start_line": 2, "end_line": 2}))
	if len(rank) != 1 || !strings.Contains(rank[0].(map[string]any)["text"].(string), "Beta") {
		t.Fatalf("rank selection failed: %#v", rank)
	}
}
func TestFrequencyContentSHAInvalidationAndInvalidUTF8(t *testing.T) {
	root, _, d := frequencyFixture(t)
	docResponse(t, d, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2})
	p := filepath.Join(root, "doc", "src", "notes.tex")
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	body[0] = 'X'
	if err = os.WriteFile(p, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(p, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got, err := d.FrequentSnippet(context.Background(), 8, 256, 2048); err != nil || got != "" {
		t.Fatalf("same-size same-mtime SHA invalidation failed: %q %v", got, err)
	}
	bad := filepath.Join(root, "doc", "src", "invalid.tex")
	if err = os.WriteFile(bad, []byte{0xff, 0xfe}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/invalid.tex"}, ""); err == nil || !strings.Contains(err.Error(), "binary or invalid UTF-8") {
		t.Fatalf("invalid UTF-8 accepted: %v", err)
	}
}
