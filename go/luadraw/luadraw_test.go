package luadraw

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
func TestSelfContainedTeXPreservesAcceptedPreamble(t *testing.T) {
	accepted := "\\documentclass{article}\n\\usepackage{tikz}\n\\usetikzlibrary{calc,arrows.meta}\n\\usepackage[3d]{luadraw}\n\\begin{document}\n\\begin{luadraw}{name=accepted}\nlocal ld=luadraw; local g=ld.graph3d:new{}; g:Show()\n\\end{luadraw}\n\\end{document}\n"
	tex := SelfContainedTeX(accepted, map[string]any{}, true)
	if !strings.Contains(tex, `\usetikzlibrary{calc,arrows.meta}`) || !strings.Contains(tex, `\usepackage[3d]{luadraw}`) {
		t.Fatalf("custom preamble or 3D package option was lost:\n%s", tex)
	}
	if strings.Count(tex, `\usepackage[3d]{luadraw}`) != 1 {
		t.Fatalf("LuaDraw package must be emitted once by the exporter:\n%s", tex)
	}
	if !strings.Contains(tex, `\begin{luadraw}{name=submitted}`) || strings.Contains(tex, `\begin{luadraw}{name=accepted}`) {
		t.Fatalf("accepted Lua code was not wrapped in the canonical environment:\n%s", tex)
	}
}

func TestSelfContainedTeXBuildsWithoutLuaOrTikZInputs(t *testing.T) {
	root := projectRoot(t)
	code := "local ld=luadraw\nlocal g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}\ng:Dline(-1,1,\"red,thick\")\ng:Show()\n"
	tex := SelfContainedTeX(code, map[string]any{}, false)
	if !strings.Contains(tex, `\begin{luadraw}{name=submitted}`) || strings.Contains(tex, `dofile(`) || strings.Contains(tex, `\input{`) {
		t.Fatalf("export is not self-contained:\n%s", tex)
	}
	for _, fallback := range []bool{false, true} {
		name := "lualatex"
		options := map[string]any{"resource_root": root, "output_name": "standalone"}
		if fallback {
			name = "fallback"
			options["mask_executables"] = []string{"lualatex"}
		}
		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			source := filepath.Join(work, "submitted.tex")
			if err := os.WriteFile(source, []byte(tex), 0600); err != nil {
				t.Fatal(err)
			}
			result, _ := Compile(context.Background(), source, work, options)
			if result["ok"] != true {
				errText, _ := result["error"].(string)
				if fallback && strings.Contains(errText, "no fallback combination") {
					t.Skipf("fallback tools unavailable: %s", errText)
				}
				t.Fatalf("standalone TeX compile failed: %v", result["error"])
			}
			if _, err := os.Stat(filepath.Join(work, "standalone.pdf")); err != nil {
				t.Fatalf("compiled PDF missing: %v", err)
			}
		})
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
	code := "-- 中文数学标签：x^2+y^2=z^2\nlocal ld=luadraw; local g=ld.graph3d:new{window={-2,2,-2,2},bbox=false}; g:Show()\n"
	body := SelfContainedTeX(code, map[string]any{}, false)
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
