package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestModePolicy(t *testing.T) {
	for _, c := range []struct {
		mode, path, code, lang string
		ok                     bool
	}{
		{"luadraw", "figure.lua", "local g = graph:new{}", "luadraw", true},
		{"luadraw", "figure.tex", `\begin{tikzpicture}\draw (0,0)--(1,1);\end{tikzpicture}`, "", false},
		{"latex", "figure.asy", "draw((0,0)--(1,1));", "", false},
		{"latex", "figure.tex", `\begin{luadraw}x\end{luadraw}`, "", false},
		{"asymptote", "figure.tex", `\begin{asy}draw((0,0)--(1,1));\end{asy}`, "asymptote", true},
		{"mixed", "figure.asy", "draw((0,0)--(1,1));", "asymptote", true},
		{"mixed", "figure.tex", `\begin{tikzpicture}\end{tikzpicture}`, "latex", true},
	} {
		lang, e := Language(c.path, c.code, c.mode)
		if (e == nil) != c.ok || (c.ok && lang != c.lang) {
			t.Errorf("%+v: %s %v", c, lang, e)
		}
	}
	if _, e := NormalizeMode("typo"); e == nil {
		t.Fatal("invalid mode accepted")
	}
}
func TestNativeAndAsymptoteCompileAndDelivery(t *testing.T) {
	for _, c := range []struct{ mode, ext, code string }{
		{"latex", ".tex", `\begin{tikzpicture}\draw[->] (0,0)--(2,0) node[right] {$x$};\draw (0,0)--(1,1);\end{tikzpicture}`},
		{"asymptote", ".asy", `size(70mm); draw((0,0)--(2,0),Arrow); draw(unitcircle); label("$x$",(2,0),E);`},
		{"asymptote", ".asy", `import three; settings.render=0; size(70mm); currentprojection=orthographic(3,2,1); draw(O--X,blue+linewidth(1)); draw(O--Y,green+linewidth(1)); draw(O--Z,red+linewidth(1)); draw((0,0,0)--(1,1,1)); label("$P$",(1,1,1),N);`},
	} {
		t.Run(c.mode, func(t *testing.T) {
			if _, e := exec.LookPath("lualatex"); e != nil {
				t.Skip("lualatex unavailable")
			}
			if c.mode == "asymptote" {
				cmd := exec.Command("asy", "--version")
				if e := cmd.Run(); e != nil {
					t.Skipf("asy unavailable: %v", e)
				}
			}
			dir := t.TempDir()
			source := filepath.Join(dir, "figure"+c.ext)
			if e := os.WriteFile(source, []byte(c.code), 0600); e != nil {
				t.Fatal(e)
			}
			opts := map[string]any{"drawing_mode": c.mode, "engine": "lualatex"}
			r, e := Compile(context.Background(), source, filepath.Join(dir, "out"), opts)
			if e != nil || r["ok"] != true {
				t.Fatalf("compile: %v %#v", e, r)
			}
			if r["source_language"] != c.mode {
				t.Fatal(r)
			}
			tex := SelfContainedTeX(c.code, opts, c.ext == ".tex", c.mode)
			texPath := filepath.Join(dir, "submitted.tex")
			if e = os.WriteFile(texPath, []byte(tex), 0600); e != nil {
				t.Fatal(e)
			}
			rebuilt, e := Compile(context.Background(), texPath, filepath.Join(dir, "delivery"), opts)
			if e != nil || rebuilt["ok"] != true {
				t.Fatalf("self-contained delivery: %v %#v", e, rebuilt)
			}
		})
	}
}
func TestNoStalePDFOnCompileFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "figure.tex")
	if _, e := exec.LookPath("lualatex"); e != nil {
		t.Skip("lualatex unavailable")
	}
	if e := os.WriteFile(src, []byte(`\rule{4mm}{4mm}`), 0600); e != nil {
		t.Fatal(e)
	}
	options := map[string]any{"drawing_mode": "latex", "engine": "lualatex"}
	good, e := Compile(context.Background(), src, dir, options)
	if e != nil || good["ok"] != true {
		t.Fatalf("initial compile: %v %#v", e, good)
	}
	if e = os.WriteFile(src, []byte(`\thiscommanddoesnotexist`), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := Compile(context.Background(), src, dir, options)
	if e != nil {
		t.Fatal(e)
	}
	if r["ok"] == true || !strings.Contains(r["error"].(string), "failed") {
		t.Fatalf("unexpected result %#v", r)
	}
	if _, err := os.Stat(good["pdf"].(string)); !os.IsNotExist(err) {
		t.Fatalf("failed rerun retained old PDF: %v", err)
	}
}
