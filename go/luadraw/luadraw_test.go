package luadraw

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	p := filepath.Clean(filepath.Join("..", "..", "vendor", "luadraw-3.7", "luadraw"))
	a, e := filepath.Abs(p)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func TestLuaQuoteUnicodePath(t *testing.T) {
	got := luaQuote("a/图.lua")
	if got != `"a/\229\155\190.lua"` {
		t.Fatalf("unexpected Lua quote: %s", got)
	}
}

func TestDocsSearchReadViewAndWarmCache(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	cache := t.TempDir()
	out := t.TempDir()
	d, e := NewDocs(root, cache)
	if e != nil {
		t.Fatal(e)
	}
	text, img, e := d.Execute(ctx, map[string]any{"mode": "search", "query": "Regardless of global options chosen package loads", "limit": 4}, out)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(text, "index_sha256") || !strings.Contains(text, "luadraw-doc-en.pdf") {
		t.Fatalf("expected official manual hit, got %s", text)
	}
	if img == "" {
		t.Fatal("query-only search should attach a PDF crop")
	}
	if _, e = os.Stat(img); e != nil {
		t.Fatal(e)
	}
	symbolText, symbolImage, e := d.Execute(ctx, map[string]any{"mode": "search", "query": "Dpolyline3d", "limit": 1}, out)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(symbolText, "Dpolyline3d(L") || !strings.Contains(symbolText, "draw_options") {
		t.Fatalf("exact symbol search must lead with its complete API signature: %s", symbolText)
	}
	boxMatch := regexp.MustCompile(`bbox=([0-9.]+),([0-9.]+),([0-9.]+),([0-9.]+)`).FindStringSubmatch(symbolText)
	if len(boxMatch) != 5 {
		t.Fatalf("expected a real PDF bbox, got %s", symbolText)
	}
	var coords [4]float64
	for i := range coords {
		coords[i], e = strconv.ParseFloat(boxMatch[i+1], 64)
		if e != nil {
			t.Fatal(e)
		}
	}
	if coords[2] <= coords[0] || coords[3] <= coords[1] {
		t.Fatalf("bbox must have positive area, got %v", coords)
	}
	if symbolImage == "" {
		t.Fatal("exact PDF hit should attach its local crop")
	}
	if _, e = os.Stat(symbolImage); e != nil {
		t.Fatalf("symbol crop missing: %v", e)
	}
	src := "doc/src/luadraw-doc-en.tex"
	text, img, e = d.Execute(ctx, map[string]any{"mode": "read", "doc": src, "start_line": 1, "end_line": 3}, out)
	if e != nil || img != "" || !strings.Contains(text, "lines 1-3/") {
		t.Fatalf("read failed: %v %s", e, text)
	}
	text, img, e = d.Execute(ctx, map[string]any{"mode": "view", "doc": "doc/luadraw-doc-en.pdf", "page": 1, "left": 10, "top": 15, "right": 85, "bottom": 90, "zoom": 400}, out)
	if e != nil || img == "" || !strings.Contains(text, "page 1/") {
		t.Fatalf("view failed: %v %s", e, text)
	}
	indexPath := filepath.Join(cache, "luadraw-doc-index-v3.json")
	before, e := os.Stat(indexPath)
	if e != nil {
		t.Fatal(e)
	}
	d2, e := NewDocs(root, cache)
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = d2.Execute(ctx, map[string]any{"mode": "search", "query": "LuaDraw", "include_image": false}, out)
	if e != nil {
		t.Fatal(e)
	}
	after, e := os.Stat(indexPath)
	if e != nil {
		t.Fatal(e)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("warm index unexpectedly rewritten")
	}
}
func TestDocsPathBoundary(t *testing.T) {
	d, e := NewDocs(projectRoot(t), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "../../etc/passwd"}, t.TempDir())
	if e == nil {
		t.Fatal("expected path boundary rejection")
	}
}
func TestDocsIndexInvalidationAndSymlinkBoundary(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "doc", "src")
	if e := os.MkdirAll(base, 0700); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(base, "notes.tex")
	if e := os.WriteFile(source, []byte("before\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cache := t.TempDir()
	d, e := NewDocs(root, cache)
	if e != nil {
		t.Fatal(e)
	}
	first, _, e := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/notes.tex"}, t.TempDir())
	if e != nil || !strings.Contains(first, "before") {
		t.Fatalf("initial index failed: %v", e)
	}
	if e = os.WriteFile(source, []byte("before\nafter\n"), 0600); e != nil {
		t.Fatal(e)
	}
	future := time.Now().Add(2 * time.Second)
	if e = os.Chtimes(source, future, future); e != nil {
		t.Fatal(e)
	}
	second, _, e := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2}, t.TempDir())
	if e != nil || !strings.Contains(second, "2: after") {
		t.Fatalf("stale source index: %v %s", e, second)
	}
	outside := filepath.Join(t.TempDir(), "secret.tex")
	if e = os.WriteFile(outside, []byte("secret"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(base, "escape.tex")
	if e = os.Symlink(outside, link); e != nil {
		t.Skipf("symlink unavailable: %v", e)
	}
	_, _, e = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/escape.tex"}, t.TempDir())
	if e == nil {
		t.Fatal("symlink escape should be rejected")
	}
}

func TestCompileLuaFallbackAndSourcePolicy(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "图形.lua")
	content := `local ld=luadraw; local g=ld.graph:new{window={-2,2,-2,2},bbox=false}; g:Dline(-1,1,"red"); g:Show()` + "\n"
	if e := os.WriteFile(src, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
	work := filepath.Join(t.TempDir(), "输出目录 with spaces")
	if e := os.MkdirAll(work, 0700); e != nil {
		t.Fatal(e)
	}
	res, _ := Compile(ctx, src, work, map[string]any{"resource_root": root, "helper_path": helperSearch(root), "mask_executables": []string{"lualatex"}})
	if res["ok"] != true {
		if strings.Contains(res["error"].(string), "no fallback combination") {
			t.Skipf("real fallback unavailable on this host: %v", res["error"])
		}
		t.Fatalf("fallback compile failed: %v", res["error"])
	}
	if res["backend"] != "standalone-lua+generated-TikZ" {
		t.Fatalf("unexpected backend %v", res["backend"])
	}
	if res["engine"] != "xelatex" && res["engine"] != "pdflatex" {
		t.Fatalf("unexpected engine %v", res["engine"])
	}
	if _, e := os.Stat(src); e != nil {
		t.Fatal("source was removed", e)
	}
	if res["generated_lua"] == nil || res["tikz"] == nil {
		t.Fatal("missing generated artifact paths")
	}
}
func TestCompileFallbackFailureCollectsLogsOnFreshProbe(t *testing.T) {
	root := projectRoot(t)
	helperSource := helperSearch(root)
	body, e := os.ReadFile(helperSource)
	if e != nil {
		t.Fatal(e)
	}
	helper := filepath.Join(t.TempDir(), "shared helper.lua")
	if e = os.WriteFile(helper, body, 0600); e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(t.TempDir(), "图.lua")
	lua := `local ld=luadraw; local g=ld.graph:new{window={-1,1,-1,1},bbox=false}; g:Dline(-1,1,"red"); g:Show()` + "\n"
	if e = os.WriteFile(src, []byte(lua), 0600); e != nil {
		t.Fatal(e)
	}
	res, _ := Compile(context.Background(), src, t.TempDir(), map[string]any{"resource_root": root, "helper_path": helper, "mask_executables": []string{"lualatex"}, "tex_packages": []string{"luadrawtest_missing_package_zz"}})
	if res["ok"] == true {
		t.Fatal("expected the deliberately missing package to fail")
	}
	if strings.Contains(res["error"].(string), "no fallback combination") {
		t.Skipf("real fallback unavailable on this host: %v", res["error"])
	}
	logs, ok := res["logs"].([]any)
	if !ok {
		t.Fatalf("logs must have consistent []any type, got %T", res["logs"])
	}
	found := false
	for _, v := range logs {
		m, ok := v.(map[string]any)
		if ok && m["engine"] == "xelatex" || ok && m["engine"] == "pdflatex" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fallback TeX failure log missing: %#v", logs)
	}
}
func TestMapSliceAcceptsFreshAndCachedRepresentations(t *testing.T) {
	m := map[string]any{"usable": true}
	if len(mapSlice([]map[string]any{m})) != 1 || len(mapSlice([]any{m})) != 1 {
		t.Fatal("candidate conversion rejected a supported slice representation")
	}
}

func TestCompileChineseMath3DEnvironment(t *testing.T) {
	root := projectRoot(t)
	src := filepath.Join(t.TempDir(), "图形.tex")
	body := "\\begin{luadraw}\n-- 中文数学标签：x^2+y^2=z^2\nlocal ld=luadraw; local g=ld.graph3d:new{window={-2,2,-2,2},bbox=false}; g:Show()\n\\end{luadraw}\n"
	if e := os.WriteFile(src, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := Probe(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	work := t.TempDir()
	res, _ := Compile(context.Background(), src, work, map[string]any{"resource_root": root, "mask_executables": []string{"lualatex"}})
	if res["ok"] != true {
		if info["standalone_lua_usable"] != true {
			t.Skipf("real fallback unavailable on this host: %v", res["error"])
		}
		t.Fatalf("3D Chinese environment failed: %v", res["error"])
	}
	if res["backend"] != "standalone-lua+generated-TikZ" {
		t.Fatalf("expected fallback backend, got %v", res["backend"])
	}
	if res["generated_lua"] == nil || res["tikz"] == nil {
		t.Fatal("expected extracted Lua and generated TikZ")
	}
}

func TestCompileRejectsHandwrittenTikZ(t *testing.T) {
	root := projectRoot(t)
	src := filepath.Join(t.TempDir(), "figure.lua")
	if e := os.WriteFile(src, []byte("tex.print('\\\\draw (0,0)--(1,1);')\n"), 0600); e != nil {
		t.Fatal(e)
	}
	res, _ := Compile(context.Background(), src, t.TempDir(), map[string]any{"resource_root": root})
	if res["ok"] == true || !strings.Contains(res["error"].(string), "LuaDraw is mandatory") {
		t.Fatalf("expected policy rejection, got %#v", res)
	}
}
