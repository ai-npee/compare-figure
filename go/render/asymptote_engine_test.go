package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Regression from the real figure 6.9 run: Asymptote succeeded with XeLaTeX,
// but its exported xeCJK preamble failed in the auto-selected outer LuaLaTeX.
func TestAsymptoteAutoEnginePreservesXeCJKDelivery(t *testing.T) {
	for _, tool := range []string{"asy", "xelatex"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable", tool)
		}
	}
	if err := exec.Command("asy", "--version").Run(); err != nil {
		t.Skipf("asy unavailable: %v", err)
	}
	code := `settings.tex="xelatex";
texpreamble("\usepackage{fontspec}\usepackage{xeCJK}\setCJKmainfont{FandolSong-Regular.otf}");
size(20mm);
draw(unitcircle);
label("图",(0,0));
`
	dir := t.TempDir()
	source := filepath.Join(dir, "figure.asy")
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	options := map[string]any{"drawing_mode": "asymptote", "engine": "auto"}
	first, err := Compile(context.Background(), source, filepath.Join(dir, "original"), options)
	if err != nil || first["ok"] != true || first["engine"] != "xelatex" {
		t.Fatalf("Asy compile: %v %#v", err, first)
	}
	// Only the final embedded-source TeX is copied to a separate input directory.
	texDir := t.TempDir()
	texPath := filepath.Join(texDir, "submitted.tex")
	if err := os.WriteFile(texPath, []byte(SelfContainedTeX(code, options, false, "asymptote")), 0600); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := Compile(context.Background(), texPath, filepath.Join(texDir, "out"), options)
	if err != nil || rebuilt["ok"] != true || rebuilt["engine"] != "xelatex" {
		t.Fatalf("isolated auto delivery: %v %#v", err, rebuilt)
	}
}
