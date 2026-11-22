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

func TestNativeSkillExportIsGoOnlyMovableAndIdempotent(t *testing.T) {
	resources, e := distribution.FindRoot("")
	if e != nil {
		t.Skip(e)
	}
	native := filepath.Join(t.TempDir(), "compare-figure")
	if runtime.GOOS == "windows" {
		native += ".exe"
	}
	compiler := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		compiler += ".exe"
	}
	cmd := exec.Command(compiler, "build", "-o", native, "./cmd/compare-figure")
	cmd.Dir = filepath.Join(resources, "go")
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(output))
	}
	target := filepath.Join(t.TempDir(), "skill")
	export := func() map[string]any {
		t.Helper()
		command := exec.Command(native, "export-skill", "--resource-root", resources, "--out", target)
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
	first := export()
	if first["runtime"] != "go" || first["written"].(float64) < 1 {
		t.Fatal(first)
	}
	before, e := os.ReadFile(filepath.Join(target, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	second := export()
	if second["written"] != float64(0) {
		t.Fatal("export rewrote unchanged assets", second)
	}
	after, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("manifest is not idempotent")
	}
	moved := filepath.Join(t.TempDir(), "moved-skill")
	if filepath.IsAbs(target) && filepath.Base(target) == "skill" {
		if e = os.Rename(target, moved); e != nil {
			t.Fatal(e)
		}
	}
	binary := filepath.Join(moved, "bin", runtime.GOOS+"-"+runtime.GOARCH, filepath.Base(native))
	command := exec.Command(binary, "--help")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "COMPARE_FIGURE_ROOT=")
	output, e := command.CombinedOutput()
	if e != nil || !bytes.Contains(output, []byte("native Go")) {
		t.Fatal(e, string(output))
	}
	var manifest struct {
		Runtime string `json:"runtime"`
		Files   []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if e = json.Unmarshal(after, &manifest); e != nil {
		t.Fatal(e)
	}
	if manifest.Runtime != "go" {
		t.Fatal(manifest.Runtime)
	}
	for _, item := range manifest.Files {
		ext := strings.ToLower(filepath.Ext(item.Path))
		if ext == ".py" || ext == ".pyc" || strings.Contains(item.Path, "runner.py") {
			t.Fatal("Python exported", item.Path)
		}
	}
	// Resource discovery must work from the moved binary, without project cwd.
	command = exec.Command(binary, "tools", "--stdin")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "COMPARE_FIGURE_ROOT=")
	command.Stdin = strings.NewReader(`{"operation":"docs","arguments":{"mode":"read","doc":"files/luadraw_base.lua","start_line":1,"end_line":3}}`)
	output, e = command.CombinedOutput()
	if e != nil {
		t.Fatal(e, string(output))
	}
	var response map[string]any
	if e = json.Unmarshal(output, &response); e != nil || response["ok"] != true {
		t.Fatal(e, string(output))
	}
}
