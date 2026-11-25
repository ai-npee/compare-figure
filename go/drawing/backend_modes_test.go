package drawing

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAlternativeBackendToolAcceptanceAndDelivery(t *testing.T) {
	for _, bin := range []string{"lualatex", "pdftoppm", "pdfinfo"} {
		if _, e := exec.LookPath(bin); e != nil {
			t.Skip(bin + " unavailable")
		}
	}
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	for _, tc := range []struct{ mode, path, code, language string }{
		{"latex", "figure.tex", `\begin{tikzpicture}\draw (0,0) circle (1);\draw[->] (-1.3,0)--(1.5,0);\end{tikzpicture}`, "latex"},
		{"asymptote", "figure.asy", `size(50mm); draw(unitcircle); draw((-1.3,0)--(1.5,0),Arrow);`, "asymptote"},
		{"mixed", "figure.tex", `\begin{tikzpicture}\draw (0,0)--(1,1)--(2,0)--cycle;\end{tikzpicture}`, "latex"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			if tc.language == "asymptote" {
				if e := exec.Command("asy", "--version").Run(); e != nil {
					t.Skipf("asy unavailable: %v", e)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			dir := t.TempDir()
			ref := filepath.Join(dir, "reference.png")
			h, e := New(filepath.Join(dir, "work"), ref, "", .17, 120*time.Second, map[string]any{"drawing_mode": tc.mode, "resource_root": root, "max_scale_dev": -1.0, "raster_dpi": 90})
			if e != nil {
				t.Fatal(e)
			}
			call := func(name string, args map[string]any) string {
				t.Helper()
				text, _, err := h.Execute(ctx, name, args)
				if err != nil || strings.HasPrefix(text, "ERROR") {
					t.Fatalf("%s: %s %v", name, text, err)
				}
				return text
			}
			call("write_file", map[string]any{"path": tc.path, "content": tc.code})
			if text := call("compile", map[string]any{"path": tc.path}); !strings.Contains(text, "COMPILE OK") {
				t.Fatal(text)
			}
			if e = renderAtDPI(ctx, filepath.Join(h.Root, "standalone.pdf"), 90, ref); e != nil {
				t.Fatal(e)
			}
			call("compare_figure", nil)
			if text := call("submit", map[string]any{"path": tc.path}); !strings.HasPrefix(text, "SUBMITTED.") {
				t.Fatal(text)
			}
			score := h.State["best_overlap"]
			result, e := h.Deliver(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if result["source_language"] != tc.language || result["drawing_mode"] != tc.mode || result["best_overlap"] != score {
				t.Fatalf("delivery metadata %#v", result)
			}
			for _, key := range []string{"editable_source", "editable_tex", "submitted_pdf"} {
				if _, e = os.Stat(result[key].(string)); e != nil {
					t.Fatal(e)
				}
			}
			// Idempotent accepted-state delivery must not require a model or change score.
			again, e := h.Deliver(ctx)
			if e != nil || again["best_overlap"] != score {
				t.Fatalf("redelivery %v %#v", e, again)
			}
		})
	}
}
