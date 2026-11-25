package app

import (
	"bytes"
	cf "compare-figure"
	"compare-figure/distribution"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFlatDocumentSearchEnvelopeSanitizesOnlyOuterFields(t *testing.T) {
	resources, err := distribution.FindRoot("")
	if err != nil {
		t.Skip(err)
	}
	svc := &service{resources: resources}
	flat := map[string]any{"operation": "docs", "request_id": "flat-1", "drawing_mode": "latex", "mode": "list", "limit": float64(1), "out_dir": filepath.Join(t.TempDir(), "flat"), "resource_root": resources, "cache_dir": filepath.Join(t.TempDir(), "cache"), "inline_images": false}
	result, err := svc.handle(context.Background(), flat)
	if err != nil {
		t.Fatalf("flat docs envelope was not sanitized: %v", err)
	}
	if result.(map[string]any)["ok"] != true {
		t.Fatal(result)
	}
	nested := map[string]any{"operation": "document_search", "drawing_mode": "latex", "out_dir": filepath.Join(t.TempDir(), "nested"), "arguments": map[string]any{"mode": "list", "unknown_option": true}}
	if _, err := svc.handle(context.Background(), nested); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("nested unknown argument was ignored: %v", err)
	}
}

func TestNativeJSONLContinuesAndReturnsImageContract(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "reference.png")
	im := image.NewNRGBA(image.Rect(0, 0, 48, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 48; x++ {
			c := color.NRGBA{255, 255, 255, 255}
			if x >= 9 && x < 31 && y >= 6 && y < 30 {
				c = color.NRGBA{30, 50, 80, 255}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	if e := cf.WritePNG(path, im); e != nil {
		t.Fatal(e)
	}
	request := map[string]any{"id": "valid", "reference": path, "candidate": path, "operation": "compare", "out_dir": filepath.Join(base, "comparison"), "inline_images": true}
	data, _ := json.Marshal(request)
	input := strings.NewReader("{\"id\":\"bad\",\"operation\":\"unknown\"}\n" + string(data) + "\n")
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"tools", "--jsonl"}, input, &out, &errOut)
	if code != 1 {
		t.Fatal(code, out.String(), errOut.String())
	}
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatal(out.String())
	}
	var bad, good map[string]any
	if e := json.Unmarshal(lines[0], &bad); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal(lines[1], &good); e != nil {
		t.Fatal(e)
	}
	if bad["ok"] != false || good["ok"] != true || good["id"] != "valid" {
		t.Fatal(bad, good)
	}
	images := good["images"].([]any)
	if images[0].(map[string]any)["base64"] == "" {
		t.Fatal("missing image content")
	}
}

func TestNativeSkillExportIsSourceOnlyMovableAndReexportable(t *testing.T) {
	resources, e := distribution.FindRoot("")
	if e != nil {
		t.Skip(e)
	}
	compiler := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		compiler += ".exe"
	}
	target := filepath.Join(t.TempDir(), "skill")
	export := func(root, destination string) map[string]any {
		t.Helper()
		command := exec.Command(compiler, "run", "./cmd/compare-figure", "export-skill", "--resource-root", root, "--out", destination)
		command.Dir = filepath.Join(resources, "go")
		output, e := command.CombinedOutput()
		if e != nil {
			t.Fatal(e, string(output))
		}
		var result map[string]any
		if e = json.Unmarshal(output, &result); e != nil {
			t.Fatal(e, string(output))
		}
		return result
	}
	first := export(resources, target)
	if first["runtime"] != "go" || first["distribution"] != "source-only" || first["written"].(float64) < 1 {
		t.Fatal(first)
	}
	before, e := os.ReadFile(filepath.Join(target, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	second := export(resources, target)
	if second["written"] != float64(0) {
		t.Fatal("export rewrote unchanged assets", second)
	}
	after, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("manifest is not idempotent")
	}
	var manifest struct {
		Schema       string `json:"schema"`
		Distribution string `json:"distribution"`
		Files        []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if e = json.Unmarshal(after, &manifest); e != nil {
		t.Fatal(e)
	}
	if manifest.Schema != "compare-figure-skill/2" || manifest.Distribution != "source-only" {
		t.Fatal(manifest)
	}
	seen := map[string]bool{}
	for _, item := range manifest.Files {
		seen[item.Path] = true
		if strings.HasSuffix(item.Path, "_test.go") || strings.Contains(item.Path, "/testdata/") || strings.HasPrefix(item.Path, "bin/") || strings.Contains(item.Path, "/build/") || strings.Contains(item.Path, "/.deps/") {
			t.Fatal("development artifact exported", item.Path)
		}
		ext := strings.ToLower(filepath.Ext(item.Path))
		if ext == ".py" || ext == ".pyc" || strings.Contains(item.Path, "runner.py") {
			t.Fatal("Python exported", item.Path)
		}
	}
	for _, required := range []string{"examples/runner.yaml.example", "go/go.mod", "go/go.sum", "go/cmd/compare-figure/main.go", "SKILL.md", "VERSION", "viewer.html", "scripts/export_skill.sh", "scripts/luadraw_runtime.lua", "scripts/viewer_math.js", "docs/prompt-guide.md", "docs/viewer-math.md", "docs/katex-upstream.json", "prompts/luadraw_figure.system.md", "vendor/luadraw-3.7/luadraw/files/luadraw.sty", "vendor/katex-0.18.2/katex.min.js", "vendor/katex-0.18.2/katex.min.css", "vendor/katex-0.18.2/LICENSE", "vendor/katex-0.18.2/RESOURCE-MANIFEST.json", "vendor/katex-0.18.2/fonts/KaTeX_Main-Regular.woff2"} {
		if !seen[required] {
			t.Fatal("missing source/runtime resource", required)
		}
	}
	if _, e = os.Stat(filepath.Join(target, "bin")); !os.IsNotExist(e) {
		t.Fatal("binary directory included", e)
	}
	moved := filepath.Join(t.TempDir(), "moved-skill")
	if e = os.Rename(target, moved); e != nil {
		t.Fatal(e)
	}
	buildDir := filepath.Join(moved, "build")
	if e = os.MkdirAll(buildDir, 0755); e != nil {
		t.Fatal(e)
	}
	binary := filepath.Join(buildDir, "compare-figure")
	command := exec.Command(compiler, "-C", filepath.Join(moved, "go"), "build", "-o", binary, "./cmd/compare-figure")
	if output, e := command.CombinedOutput(); e != nil {
		t.Fatal(e, string(output))
	}
	command = exec.Command(binary, "--help")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "COMPARE_FIGURE_ROOT=")
	output, e := command.CombinedOutput()
	if e != nil || !bytes.Contains(output, []byte("native Go")) {
		t.Fatal(e, string(output))
	}
	command = exec.Command(binary, "export-skill", "--resource-root", moved, "--out", filepath.Join(t.TempDir(), "second-export"))
	output, e = command.CombinedOutput()
	if e != nil || !bytes.Contains(output, []byte("source-only")) {
		t.Fatal(e, string(output))
	}
}
