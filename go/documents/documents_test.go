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

func TestMissingPathSuggestionsIncludeSubdirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"LICENSE", "README"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"doc", "examples"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "example.tex"), []byte("reference"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	d, err := documents.NewConfig(documents.Config{Sources: []documents.Source{{ID: "asymptote", Root: root}}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = d.Execute(context.Background(), map[string]any{"mode": "read", "source": "asymptote", "doc": "asymptote.texi", "start_line": 1, "end_line": 2}, t.TempDir())
	if err == nil {
		t.Fatal("expected missing path error")
	}
	for _, want := range []string{"asymptote:doc/", "asymptote:examples/", "mode=list doc=", "mode=find"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing suggestion %q: %v", want, err)
		}
	}
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
			if mode == "latex" {
				text, _, err := d.Execute(context.Background(), map[string]any{"query": "needle"}, "")
				if err != nil || !strings.Contains(text, "No bundled reference for native LaTeX") {
					t.Fatalf("%s %v", text, err)
				}
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
func sourceConfigFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range map[string]string{
		"vendor/luadraw-3.7/luadraw/doc/official.tex": "Dscene3d officialneedle sharedmarker\n",
		"vendor/luadraw-3.7/luadraw/files/graph.lua":  "official source\n",
		"vendor/asymptote-3.16/doc/manual.texi":       "asymptoteneedle sharedmarker\n",
		"docs/mesh-advice.md":                         "# Project experience\n\nprojectonlyfacet sharedmarker\n",
		"docs/unrelated.md":                           "projectonlyfacet sharedmarker unrelated\n",
		"docs/configuration.md":                       "projectonlyfacet sharedmarker private configuration\n",
		"docs/notes.md":                               "projectonlyfacet sharedmarker private history\n",
		"docs/private/run.md":                         "projectonlyfacet sharedmarker private run\n",
	} {
		writeSourceFixture(t, root, path, body)
	}
	return root
}

func writeSourceFixture(t *testing.T, root, path, body string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

const fixtureSourceConfig = `sources:
  - id: luadraw
    root: ../../vendor/luadraw-3.7/luadraw
  - id: project-guide
    root: ..
    include: [mesh-advice.md]
`

func assertSources(t *testing.T, response map[string]any, want ...string) {
	t.Helper()
	got := map[string]bool{}
	for _, item := range results(t, response) {
		r := item.(map[string]any)
		got[r["source"].(string)] = true
	}
	if len(got) != len(want) {
		t.Fatalf("source identities got %v want %v: %#v", got, want, response)
	}
	for _, source := range want {
		if !got[source] {
			t.Fatalf("missing structured/text source %s: %#v", source, response)
		}
	}
}

func TestModeSourceConfigUsesDeclaredSourcesAndExactSelectors(t *testing.T) {
	root := sourceConfigFixture(t)
	for _, mode := range []string{"luadraw", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			config := fixtureSourceConfig
			if mode == "mixed" {
				config += "  - id: asymptote\n    root: ../../vendor/asymptote-3.16\n"
			}
			writeSourceFixture(t, root, "docs/document-sources/"+mode+".yaml", config)
			d, err := documents.New(root, t.TempDir(), mode)
			if err != nil {
				t.Fatal(err)
			}
			// Neither the ID nor filename is special to the adapter. Adding this
			// reference required only the declarative source entry above.
			note := execute(t, d, map[string]any{"query": "projectonlyfacet", "limit": 20})
			assertSources(t, note, "project-guide")
			if len(results(t, note)) != 1 || results(t, note)[0].(map[string]any)["doc"] != "mesh-advice.md" {
				t.Fatalf("source include allowlist leaked other files: %#v", note)
			}
			hit := results(t, note)[0].(map[string]any)
			read := execute(t, d, map[string]any{"mode": "read", "source": hit["source"], "doc": hit["doc"], "start_line": 1, "end_line": 3})
			assertSources(t, read, "project-guide")
			if !strings.Contains(resultText(t, read), "projectonlyfacet") {
				t.Fatal("project follow-up lost body")
			}
			official := execute(t, d, map[string]any{"mode": "read", "source": "luadraw", "doc": "doc/official.tex", "start_line": 1, "end_line": 1})
			assertSources(t, official, "luadraw")
			if !strings.Contains(resultText(t, official), "officialneedle") {
				t.Fatal("official root/read changed")
			}
			assertSources(t, execute(t, d, map[string]any{"source": "luadraw", "query": "projectonlyfacet"}))
			assertSources(t, execute(t, d, map[string]any{"sources": []string{"luadraw"}, "query": "projectonlyfacet"}))
			for _, args := range []map[string]any{
				{"mode": "search", "query": "sharedmarker", "sources": []string{"luadraw", "project-guide"}, "limit": 20},
				{"mode": "grep", "query": "sharedmarker", "sources": []string{"luadraw", "project-guide"}, "limit": 20},
				{"mode": "list", "sources": []string{"luadraw", "project-guide"}, "recursive": true, "limit": 20},
				{"mode": "frequent", "sources": []string{"luadraw", "project-guide"}, "limit": 20},
			} {
				assertSources(t, execute(t, d, args), "luadraw", "project-guide")
				delete(args, "sources")
				args["source"] = "luadraw"
				assertSources(t, execute(t, d, args), "luadraw")
			}
			assertSources(t, execute(t, d, map[string]any{"mode": "find", "source": "luadraw", "query": "mesh-advice"}))
			assertSources(t, execute(t, d, map[string]any{"mode": "find", "query": "mesh-advice"}), "project-guide")
			for _, name := range []string{"unrelated.md", "configuration.md", "notes.md", "private/run.md"} {
				if _, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "source": "project-guide", "doc": name}, ""); err == nil {
					t.Fatalf("non-allowlisted project file readable: %s", name)
				}
			}
			if _, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "source": "luadraw", "doc": hit["doc"]}, ""); err == nil {
				t.Fatal("official read unexpectedly selected project reference")
			}
			if mode == "mixed" {
				assertSources(t, execute(t, d, map[string]any{"source": "asymptote", "query": "sharedmarker"}), "asymptote")
			}
		})
	}
}

func TestModeSourceConfigIsGenericAndPrecedesOfficialFallback(t *testing.T) {
	root := t.TempDir() // No vendor tree: configuration defines the whole collection.
	writeSourceFixture(t, root, "references/guide.txt", "customreferencekeyword\n")
	for _, mode := range []string{"luadraw", "mixed", "asymptote", "latex"} {
		writeSourceFixture(t, root, "docs/document-sources/"+mode+".yaml", "sources:\n  - id: any-reference\n    root: ../../references\n    include: [guide.txt]\n")
		d, err := documents.New(root, t.TempDir(), mode)
		if err != nil {
			t.Fatal(err)
		}
		assertSources(t, execute(t, d, map[string]any{"query": "customreferencekeyword"}), "any-reference")
	}
}

func TestModeSourceConfigRejectsInvalidFilesWithoutFallback(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field":         fixtureSourceConfig + "unknown: true\n",
		"unknown source field":  "sources:\n  - id: luadraw\n    root: ../../vendor/luadraw-3.7/luadraw\n    unknown: true\n",
		"unknown nested field":  fixtureSourceConfig + "limits:\n  unknown: 1\n",
		"malformed yaml":        "sources: [\n",
		"second document":       fixtureSourceConfig + "---\nsources: []\n",
		"empty second document": fixtureSourceConfig + "---\n",
		"empty file":            "",
		"empty sources":         "sources: []\n",
		"missing root":          "sources:\n  - id: missing\n",
		"nonexistent root":      "sources:\n  - id: missing\n    root: ../../no-such-collection\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := sourceConfigFixture(t) // Valid official fallback must not hide errors.
			writeSourceFixture(t, root, "docs/document-sources/luadraw.yaml", body)
			_, err := documents.New(root, t.TempDir(), "luadraw")
			if err == nil || !strings.Contains(err.Error(), "luadraw.yaml") {
				t.Fatalf("invalid existing config silently fell back or lost location: %v", err)
			}
		})
	}
}

func TestModeSourceConfigRelativePathsRelocateAndCacheArgumentWins(t *testing.T) {
	original := sourceConfigFixture(t)
	writeSourceFixture(t, original, "docs/document-sources/luadraw.yaml", fixtureSourceConfig+"cache_dir: ../../build/configured-cache\n")
	moved := filepath.Join(t.TempDir(), "portable-skill")
	if err := os.CopyFS(moved, os.DirFS(original)); err != nil {
		t.Fatal(err)
	}
	writeSourceFixture(t, original, "docs/mesh-advice.md", "stale-original-reference\n")
	t.Chdir(t.TempDir())
	d, err := documents.New(moved, "", "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	assertSources(t, execute(t, d, map[string]any{"query": "projectonlyfacet"}), "project-guide")
	entries, err := os.ReadDir(filepath.Join(moved, "build", "configured-cache"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("relative cache did not relocate with config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(original, "build", "configured-cache")); !os.IsNotExist(err) {
		t.Fatalf("relocated config used original cache: %v", err)
	}
	fresh := sourceConfigFixture(t)
	writeSourceFixture(t, fresh, "docs/document-sources/luadraw.yaml", fixtureSourceConfig+"cache_dir: ../../build/configured-cache\n")
	override := t.TempDir()
	d, err = documents.New(fresh, override, "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	execute(t, d, map[string]any{"query": "projectonlyfacet"})
	entries, err = os.ReadDir(override)
	if err != nil || len(entries) == 0 {
		t.Fatalf("cache argument did not override configuration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, "build", "configured-cache")); !os.IsNotExist(err) {
		t.Fatalf("configured cache used despite explicit argument: %v", err)
	}
}

func TestMissingModeConfigAndLegacyRootsKeepOfficialSources(t *testing.T) {
	root := sourceConfigFixture(t)
	d, err := documents.New(root, t.TempDir(), "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	assertSources(t, execute(t, d, map[string]any{"query": "sharedmarker"}), "luadraw")
	writeSourceFixture(t, root, "docs/document-sources/luadraw.yaml", fixtureSourceConfig)
	legacy, err := documents.New(filepath.Join(root, "vendor", "luadraw-3.7", "luadraw"), t.TempDir(), "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	assertSources(t, execute(t, legacy, map[string]any{"query": "projectonlyfacet"}), "project-guide")
	writeSourceFixture(t, root, "independent/nested/official/doc/manual.tex", "legacyofficialneedle\n")
	independent := filepath.Join(root, "independent", "nested", "official")
	t.Chdir(root) // Neither ancestor config nor cwd config may attach to this root.
	legacy, err = documents.New(independent, t.TempDir(), "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	assertSources(t, execute(t, legacy, map[string]any{"query": "legacyofficialneedle"}), "luadraw")
	assertSources(t, execute(t, legacy, map[string]any{"query": "projectonlyfacet"}))
	asymptote, err := documents.New(root, t.TempDir(), "asymptote")
	if err != nil {
		t.Fatal(err)
	}
	assertSources(t, execute(t, asymptote, map[string]any{"query": "sharedmarker"}), "asymptote")
}

func TestBundledLuaDrawRealSurfaceNotesKeywords(t *testing.T) {
	d, err := documents.New(resourceRoot(t), t.TempDir(), "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{"cutfacet2", "Dscene3d"} {
		r := execute(t, d, map[string]any{"mode": "search", "query": keyword, "kinds": []string{"source"}, "extensions": []string{".md"}, "limit": 20})
		assertSources(t, r, "luadraw-notes")
		if !strings.Contains(resultText(t, r), keyword) {
			t.Fatalf("real configured notes keyword missing: %s", keyword)
		}
	}
	read := execute(t, d, map[string]any{"mode": "read", "source": "luadraw-notes", "doc": "luadraw-surface-rendering.md", "start_line": 23, "end_line": 29})
	assertSources(t, read, "luadraw-notes")
	if !strings.Contains(resultText(t, read), "cutfacet2") {
		t.Fatal("real configured note follow-up did not read expected source lines")
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
		if r["page"] == nil || r["bbox"] != nil || r["bbox_pct"] != nil {
			t.Fatalf("compact PDF location/projection incorrect: %#v", r)
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
		if len(results(t, r)) != 1 || results(t, r)[0].(map[string]any)["start_line"] != float64(1) {
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
	warm := execute(t, other, map[string]any{"mode": "search", "query": "LuaDraw", "output_version": 1})["cache"].(map[string]any)
	if warm["misses"] != float64(0) || warm["hits"].(float64) == 0 {
		t.Fatalf("cold repeated index: %#v", warm)
	}
	legacy := execute(t, d, map[string]any{"query": "LuaDraw", "output_version": 1})
	if legacy["provenance"].(map[string]any)["revision"] != documents.DependencyRevision {
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
	if len(results(t, texinfo)) != 1 || !strings.Contains(resultText(t, texinfo), "Asymptote") {
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

func resultText(t *testing.T, response map[string]any) string {
	t.Helper()
	var text []string
	for _, item := range results(t, response) {
		r := item.(map[string]any)
		if body, ok := r["text"].(string); ok {
			text = append(text, body)
		}
		if doc, ok := r["doc"].(string); ok {
			text = append(text, doc)
		}
	}
	return strings.Join(text, "\n")
}
