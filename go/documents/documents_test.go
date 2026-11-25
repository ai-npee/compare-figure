package documents_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/documents"
)

func resourceRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func execute(t *testing.T, d *documents.Docs, args map[string]any) map[string]any {
	t.Helper()
	r, err := d.ExecuteResponse(context.Background(), args, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func results(t *testing.T, r map[string]any) []any {
	t.Helper()
	v, ok := r["results"].([]any)
	if !ok {
		t.Fatalf("no results field: %#v", r)
	}
	return v
}

func TestFrequentSnippetSupportsExpandedBudgetsAndKeepsSmallerExplicitLimits(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 4; i++ {
		name := filepath.Join(root, "entry"+string(rune('a'+i))+".tex")
		body := strings.Repeat(string(rune('a'+i)), 900) + "\n"
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	d, err := documents.NewConfig(documents.Config{
		Sources:  []documents.Source{{ID: "fixture", Root: root, Extensions: []string{".tex"}}},
		CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		name := "entry" + string(rune('a'+i)) + ".tex"
		if _, _, err := d.Execute(context.Background(), map[string]any{
			"mode": "read", "doc": name, "start_line": 1, "end_line": 1,
		}, ""); err != nil {
			t.Fatal(err)
		}
	}
	large, err := d.FrequentSnippet(context.Background(), 8, 1024, 4096)
	if err != nil {
		t.Fatal(err)
	}
	largeLines := strings.Split(large, "\n")
	if len([]rune(large)) <= 2048 || len(largeLines) != 4 {
		t.Fatalf("expanded budget not used: lines=%d total=%d", len(largeLines), len([]rune(large)))
	}
	for _, line := range largeLines {
		if len([]rune(line)) > 1024 || len([]rune(line)) <= 512 {
			t.Fatalf("expanded line budget not used or exceeded: %d", len([]rune(line)))
		}
	}
	small, err := d.FrequentSnippet(context.Background(), 8, 512, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(small)) > 2048 {
		t.Fatalf("smaller explicit total budget ignored: %d", len([]rune(small)))
	}
	for _, line := range strings.Split(small, "\n") {
		if len([]rune(line)) > 512 {
			t.Fatalf("smaller explicit line budget ignored: %d", len([]rune(line)))
		}
	}
}

func TestBackendSourceSelectionAndExplicitLatexConfig(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"vendor/luadraw-3.7/luadraw/doc", "vendor/asymptote-3.16/doc", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{"vendor/luadraw-3.7/luadraw/doc/shared.tex": "needle LuaDraw\n", "vendor/asymptote-3.16/doc/shared.tex": "needle Asymptote\n", "docs/asymptote-upstream.json": `{"resource_root":"vendor/asymptote-3.16"}`} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for mode, want := range map[string]int{"luadraw": 1, "asymptote": 1, "mixed": 2, "latex": 0} {
		t.Run(mode, func(t *testing.T) {
			d, err := documents.New(root, t.TempDir(), mode)
			if err != nil {
				t.Fatal(err)
			}
			r := execute(t, d, map[string]any{"query": "needle"})
			if len(results(t, r)) != want {
				t.Fatalf("backend sources: %#v", r)
			}
			if mode == "latex" && !strings.Contains(r["text"].(string), "No bundled reference for native LaTeX") {
				t.Fatal(r)
			}
			if d.Definition()["function"].(map[string]any)["name"] != "document_search" {
				t.Fatal("unexpected generic tool name")
			}
		})
	}
	mixed, err := documents.New(root, t.TempDir(), "mixed")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = mixed.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/shared.tex"}, ""); err == nil || !strings.Contains(err.Error(), "multiple sources") {
		t.Fatalf("ambiguous source must be explicit: %v", err)
	}
	selected := results(t, execute(t, mixed, map[string]any{"mode": "read", "source": "asymptote", "doc": "doc/shared.tex"}))
	if len(selected) != 1 || selected[0].(map[string]any)["source"] != "asymptote" {
		t.Fatal(selected)
	}
	custom, err := documents.NewConfig(documents.Config{Sources: []documents.Source{{ID: "latex", Root: filepath.Join(root, "vendor", "luadraw-3.7", "luadraw")}}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(results(t, execute(t, custom, map[string]any{"query": "needle"}))) != 1 {
		t.Fatal("explicit LaTeX sources unavailable")
	}
	if _, err = documents.New(root, t.TempDir(), "unknown"); err == nil {
		t.Fatal("unknown backend accepted")
	}
}
func TestTextOnlySourcesDoNotRequirePDFToolsAndRespectFilters(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manual.texi"), []byte("@chapter Paths\nneedle native source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "excluded.tex"), []byte("needle hidden\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	d, err := documents.NewConfig(documents.Config{Sources: []documents.Source{{ID: "manual", Root: root, Extensions: []string{".texi"}}}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	r := execute(t, d, map[string]any{"query": "needle"})
	if len(results(t, r)) != 1 {
		t.Fatal(r)
	}
	if _, _, err = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "excluded.tex"}, ""); err == nil {
		t.Fatal("excluded document readable")
	}
}
func TestBundledLuaDrawRealQueriesReadViewNavigationAndWarmCache(t *testing.T) {
	root := resourceRoot(t)
	cache := t.TempDir()
	d, err := documents.New(root, cache, "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	first := execute(t, d, map[string]any{"mode": "search", "query": "Regardless of global options chosen package loads", "kinds": []string{"pdf"}, "limit": 4})
	hits := results(t, first)
	if len(hits) == 0 || len(first["images"].([]any)) != 0 {
		t.Fatal(first)
	}
	foundManual := false
	for _, v := range hits {
		if strings.Contains(v.(map[string]any)["doc"].(string), "luadraw-doc-en.pdf") {
			foundManual = true
		}
	}
	if !foundManual {
		t.Fatalf("official manual query missing: %#v", first)
	}
	symbol := execute(t, d, map[string]any{"mode": "search", "query": "Dpolyline3d", "kinds": []string{"pdf"}, "limit": 5})
	foundSignature := false
	for _, v := range results(t, symbol) {
		r := v.(map[string]any)
		if strings.Contains(r["text"].(string), "Dpolyline3d(L") && strings.Contains(r["text"].(string), "draw_options") {
			foundSignature = true
		}
		if r["page"] == nil || len(r["bbox"].([]any)) != 4 || len(r["bbox_pct"].([]any)) != 4 {
			t.Fatalf("PDF coordinates missing: %#v", r)
		}
	}
	if !foundSignature {
		t.Fatalf("API signature missing: %#v", symbol)
	}
	imageHit := execute(t, d, map[string]any{"mode": "search", "query": "Dpolyline3d", "kinds": []string{"pdf"}, "limit": 1, "include_image": true, "zoom": 400})
	if len(imageHit["images"].([]any)) != 1 {
		t.Fatal("explicit PDF image missing")
	}
	for _, name := range []string{"doc/src/luadraw-doc-en.tex", "files/luadraw_graph3d.lua"} {
		r := execute(t, d, map[string]any{"mode": "read", "doc": name, "start_line": 1, "end_line": 3})
		if len(results(t, r)) != 1 || !strings.Contains(r["text"].(string), "1:") {
			t.Fatal(r)
		}
	}
	for mode, query := range map[string]string{"list": "", "find": "graph3d.lua", "grep": "Dpolyline3d"} {
		args := map[string]any{"mode": mode, "query": query, "doc": "files"}
		if len(results(t, execute(t, d, args))) == 0 {
			t.Fatalf("real %s returned no matches", mode)
		}
	}
	byDoc := execute(t, d, map[string]any{"mode": "read", "doc": "doc/luadraw-doc-en.pdf#page=14"})
	byQuery := execute(t, d, map[string]any{"mode": "read", "query": "doc/luadraw-doc-en.pdf", "page": 14})
	if len(results(t, byDoc)) < 2 || len(results(t, byDoc)) != len(results(t, byQuery)) {
		t.Fatal("PDF query/page aliases or paragraph blocks lost")
	}
	if _, _, err = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/luadraw-doc-en.pdf#page=14", "page": 15}, ""); err == nil {
		t.Fatal("conflicting PDF page accepted")
	}
	view := execute(t, d, map[string]any{"mode": "view", "doc": "doc/luadraw-doc-en.pdf#page=1", "left": 10, "top": 15, "right": 85, "bottom": 90, "zoom": 400})
	images := view["images"].([]any)
	if len(images) != 1 {
		t.Fatal(view)
	}
	if images[0].(map[string]any)["width"] != float64(400) {
		t.Fatalf("crop zoom did not preserve output width: %#v", images)
	}
	if _, err = os.Stat(images[0].(map[string]any)["path"].(string)); err != nil {
		t.Fatal(err)
	}
	active := execute(t, d, map[string]any{"mode": "frequent", "source": "luadraw", "doc": "files/luadraw_graph3d.lua", "limit": 8})
	if len(results(t, active)) == 0 {
		t.Fatal("read source omitted from active frequency")
	}
	snippet, err := d.FrequentSnippet(context.Background(), 8, 256, 2048)
	if err != nil || snippet == "" || len([]rune(snippet)) > 2048 {
		t.Fatalf("passive frequent: %v %q", err, snippet)
	}
	execute(t, d, map[string]any{"mode": "search", "query": "LuaDraw"})
	other, err := documents.New(root, cache, "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	warm := execute(t, other, map[string]any{"mode": "search", "query": "LuaDraw"})["cache"].(map[string]any)
	if warm["misses"] != float64(0) || warm["hits"].(float64) == 0 {
		t.Fatalf("cold repeated index: %#v", warm)
	}
	if first["provenance"].(map[string]any)["revision"] != documents.DependencyRevision {
		t.Fatal("dependency provenance missing")
	}
	t.Logf("LuaDraw real queries: %d indexed files; all %v warm cache hits", int(warm["files"].(float64)), warm["hits"])
}
func TestBundledAsymptoteRealTexinfoExamplesAndMixed(t *testing.T) {
	root := resourceRoot(t)
	cache := t.TempDir()
	d, err := documents.New(root, cache, "asymptote")
	if err != nil {
		t.Fatal(err)
	}
	texinfo := execute(t, d, map[string]any{"mode": "read", "doc": "doc/asymptote.texi", "start_line": 1, "end_line": 12})
	if len(results(t, texinfo)) != 1 || !strings.Contains(texinfo["text"].(string), "Asymptote") {
		t.Fatal(texinfo)
	}
	search := execute(t, d, map[string]any{"mode": "search", "query": "draw", "extensions": []string{".asy"}, "limit": 3})
	if len(results(t, search)) != 3 {
		t.Fatal("Asy example search missing")
	}
	pdf := execute(t, d, map[string]any{"mode": "read", "doc": "doc/asymptote.pdf#page=1"})
	if len(results(t, pdf)) == 0 {
		t.Fatal("Asy PDF extraction missing")
	}
	mixed, err := documents.New(root, cache, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"luadraw", "asymptote"} {
		r := execute(t, mixed, map[string]any{"mode": "search", "source": source, "query": "draw", "limit": 2})
		for _, v := range results(t, r) {
			if v.(map[string]any)["source"] != source {
				t.Fatalf("mixed source filter failed: %#v", r)
			}
		}
	}
}
