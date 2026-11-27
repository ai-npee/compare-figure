package luadraw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/documents"
)

func navigationDocs(t *testing.T) (string, *Docs) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "doc", "src"), 0700); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocs(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root, d
}
func docResponse(t *testing.T, d *Docs, args map[string]any) map[string]any {
	t.Helper()
	response, err := d.ExecuteResponse(context.Background(), args, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func docResults(t *testing.T, response map[string]any) []any {
	t.Helper()
	results, ok := response["results"].([]any)
	if !ok {
		t.Fatalf("missing structured results: %#v", response)
	}
	return results
}
func TestSourceSearchCanonicalParagraphPagingAndBudgets(t *testing.T) {
	root, d := navigationDocs(t)
	text := "-- alpha comment\nfunction alpha()\n  local a = \"needle\"\n  " + strings.Repeat("continuation words ", 100) + "\nend\n\nfunction beta()\n return \"needle\"\nend\n"
	if err := os.WriteFile(filepath.Join(root, "doc", "src", "sample.tex"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	first := docResponse(t, d, map[string]any{"mode": "search", "query": "needle", "limit": 1, "snippet_chars": 64, "max_chars": 256})
	hits := docResults(t, first)
	if len(hits) != 1 {
		t.Fatal(first)
	}
	hit := hits[0].(map[string]any)
	if hit["start_line"] != float64(1) || hit["end_line"] != float64(5) || len([]rune(hit["text"].(string))) > 65 {
		t.Fatalf("canonical paragraph/snippet failed: %#v", hit)
	}
	second := docResults(t, docResponse(t, d, map[string]any{"mode": "search", "query": "needle", "limit": 1, "offset": 1}))[0].(map[string]any)
	if second["start_line"] != float64(7) {
		t.Fatalf("pagination repeated: %#v", second)
	}
	long := docResults(t, docResponse(t, d, map[string]any{"mode": "search", "query": "needle", "limit": 1, "snippet_chars": 1200}))[0].(map[string]any)
	if len([]rune(long["text"].(string))) <= len([]rune(hit["text"].(string))) {
		t.Fatal("snippet budget ignored")
	}
}
func TestNavigationModesAndPathBoundaries(t *testing.T) {
	root, d := navigationDocs(t)
	base := filepath.Join(root, "doc", "src")
	if err := os.MkdirAll(filepath.Join(base, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"alpha.tex": "literal [needle]\nneedle 123\n", "nested/beta.tex": "needle 456\n", "section-1-Introduction.tex": "intro\n", "section-3-Graphics-Methods.tex": "graphics\n"} {
		if err := os.WriteFile(filepath.Join(base, filepath.FromSlash(name)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plain := docResults(t, docResponse(t, d, map[string]any{"mode": "list", "doc": "doc/src"}))
	if len(plain) != 4 {
		t.Fatalf("nonrecursive listing: %#v", plain)
	}
	recursive := docResults(t, docResponse(t, d, map[string]any{"mode": "list", "doc": "doc/src", "recursive": true}))
	if len(recursive) != 4 {
		t.Fatalf("recursive file listing: %#v", recursive)
	}
	found := docResults(t, docResponse(t, d, map[string]any{"mode": "find", "doc": "doc/src", "query": "section-*-*.tex"}))
	if len(found) != 2 {
		t.Fatalf("glob find failed: %#v", found)
	}
	literal := docResults(t, docResponse(t, d, map[string]any{"mode": "grep", "doc": "doc/src", "query": "[needle]"}))
	if len(literal) != 1 || literal[0].(map[string]any)["doc"] != "doc/src/alpha.tex" {
		t.Fatalf("literal grep: %#v", literal)
	}
	regex := docResults(t, docResponse(t, d, map[string]any{"mode": "grep", "doc": "doc/src", "query": "needle [0-9]+", "regex": true}))
	if len(regex) != 2 {
		t.Fatalf("regex grep: %#v", regex)
	}
	for _, unsafe := range []string{"/etc/passwd", "../etc/passwd", `doc\..\outside`, `C:\outside.tex`} {
		if _, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": unsafe}, t.TempDir()); err == nil {
			t.Fatalf("accepted unsafe path %q", unsafe)
		}
	}
	if _, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/section-missing.tex"}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "section-1-Introduction.tex") {
		t.Fatalf("missing path did not suggest real alternatives: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret.tex")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "escape.tex")); err != nil {
		t.Skip(err)
	}
	if _, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/escape.tex"}, t.TempDir()); err == nil {
		t.Fatal("symlink escape accepted")
	}
}
func TestReadLineCapAndQueryAlias(t *testing.T) {
	root, d := navigationDocs(t)
	var lines []string
	for i := 1; i <= 500; i++ {
		lines = append(lines, fmt.Sprintf("line-%03d", i))
	}
	if err := os.WriteFile(filepath.Join(root, "doc", "src", "long.tex"), []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	response := docResponse(t, d, map[string]any{"mode": "read", "query": "doc/src/long.tex", "start_line": 1, "end_line": 999, "max_chars": 32000})
	text, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "query": "doc/src/long.tex", "start_line": 1, "end_line": 999, "max_chars": 32000}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	results := docResults(t, response)
	if len(results) != 1 || results[0].(map[string]any)["end_line"] != float64(400) || !strings.Contains(text, "400: line-400") || strings.Contains(text, "401: line-401") {
		t.Fatalf("400 line cap failed: %#v", response)
	}
}
func TestGrepCaseSensitivityDefaultsAcrossDocumentAPIs(t *testing.T) {
	root, legacy := navigationDocs(t)
	if err := os.WriteFile(filepath.Join(root, "doc", "src", "case.tex"), []byte("Abc\nabc\n"), 0600); err != nil {
		t.Fatal(err)
	}
	generic, err := documents.New(root, t.TempDir(), "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	type executor interface {
		ExecuteResponse(context.Context, map[string]any, string) (map[string]any, error)
	}
	for name, client := range map[string]executor{"legacy": legacy, "generic": generic} {
		t.Run(name, func(t *testing.T) {
			for _, regex := range []bool{false, true} {
				query := "Abc"
				if regex {
					query = "^Abc$"
				}
				sensitive, insensitive := true, false
				for _, setting := range []struct {
					name     string
					explicit *bool
					want     int
				}{{name: "default", want: 1}, {name: "sensitive", explicit: &sensitive, want: 1}, {name: "insensitive", explicit: &insensitive, want: 2}} {
					args := map[string]any{"mode": "grep", "doc": "doc/src/case.tex", "query": query, "regex": regex}
					if setting.explicit != nil {
						args["case_sensitive"] = *setting.explicit
					}
					response, err := client.ExecuteResponse(context.Background(), args, t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					hits := docResults(t, response)
					if len(hits) != setting.want || hits[0].(map[string]any)["text"] != "Abc" {
						t.Fatalf("regex=%t %s: %#v", regex, setting.name, response)
					}
				}
			}
			search, err := client.ExecuteResponse(context.Background(), map[string]any{"mode": "search", "query": "ABC"}, t.TempDir())
			if err != nil || len(docResults(t, search)) != 1 {
				t.Fatalf("search default case behavior changed: %v %#v", err, search)
			}
		})
	}
}

func TestDocumentDefinitionKeepsLegacyAlias(t *testing.T) {
	_, d := navigationDocs(t)
	if got := d.Definition()["function"].(map[string]any)["name"]; got != "luadraw_doc" {
		t.Fatalf("legacy tool renamed: %v", got)
	}
}
