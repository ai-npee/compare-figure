package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"compare-figure/distribution"
	"compare-figure/luadraw"
)

func TestAgentToolCLIRequiresWorkspaceAndInitialReference(t *testing.T) {
	var out bytes.Buffer
	if code := agentToolCLI(context.Background(), []string{"--name", "write_file"}, nil, &out); code != 2 {
		t.Fatalf("missing workspace code=%d out=%s", code, out.String())
	}
	workspace := t.TempDir()
	out.Reset()
	if code := agentToolCLI(context.Background(), []string{"--workspace", workspace, "--name", "write_file", "--args", `{"path":"x.lua","content":"x"}`}, nil, &out); code != 2 || !strings.Contains(out.String(), "--original IMAGE is required") {
		t.Fatalf("missing initial reference code=%d out=%s", code, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result["ok"] != false || result["images"] == nil {
		t.Fatalf("error JSON contract: %v %s", err, out.String())
	}
}

func TestAgentToolCLIPersistsReferenceBinding(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "agent-workspace")
	first := writeAgentTestImage(t, t.TempDir(), "first.png")
	second := writeAgentTestImage(t, t.TempDir(), "second.png")
	argsJSON := `{"path":"note.md","content":"persisted"}`
	sourceMD := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(sourceMD, []byte("paragraph before\n![](images/first.png)\nparagraph after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := agentToolCLI(context.Background(), []string{"--workspace", workspace, "--original", first, "--name", "write_file", "--threshold", "17", "--drawing-mode", "mixed", "--source-md", sourceMD, "--image-ref", "images/first.png", "--images-dir", filepath.Dir(first), "--args", argsJSON}, nil, &out)
	if code != 0 {
		t.Fatal(code, out.String())
	}
	stateData, err := os.ReadFile(filepath.Join(workspace, agentStateName))
	if err != nil {
		t.Fatal(err)
	}
	var stored agentRecord
	if err = json.Unmarshal(stateData, &stored); err != nil || stored.Threshold != .17 || stored.Options["drawing_mode"] != "mixed" {
		t.Fatalf("threshold or drawing mode did not persist: %#v err=%v", stored, err)
	}
	out.Reset()
	code = agentToolCLI(context.Background(), []string{"--workspace", workspace, "--original", first, "--name", "read_file", "--threshold", "50", "--max-scale-dev", ".2", "--context", "updated context", "--args", `{"path":"note.md"}`}, nil, &out)
	if code != 0 {
		t.Fatalf("explicit config update failed: code=%d out=%s", code, out.String())
	}
	var response map[string]any
	if err = json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err, out.String())
	}
	state := response["state"].(map[string]any)
	if state["_accepted_threshold"] != .5 || state["_accepted_scale_limit"] != .2 {
		t.Fatalf("explicit thresholds not applied: %#v", state)
	}
	storedData, err := os.ReadFile(filepath.Join(workspace, agentStateName))
	if err != nil || json.Unmarshal(storedData, &stored) != nil || stored.Threshold != .5 || stored.Context != "updated context" || stored.Options["max_scale_dev"] != .2 || stored.Options["drawing_mode"] != "mixed" {
		t.Fatalf("persisted override mismatch: %#v err=%v", stored, err)
	}
	out.Reset()
	code = agentToolCLI(context.Background(), []string{"--workspace", workspace, "--drawing-mode", "latex", "--name", "read_file", "--args", `{"path":"note.md"}`}, nil, &out)
	if code != 2 || !strings.Contains(out.String(), "cannot reuse this state") {
		t.Fatalf("mixed workspace accepted a LaTeX strategy override: code=%d out=%s", code, out.String())
	}
	delete(stored.Options, "drawing_mode")
	if err = saveAgentRecord(filepath.Join(workspace, agentStateName), &stored); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = agentToolCLI(context.Background(), []string{"--workspace", workspace, "--drawing-mode", "mixed", "--name", "read_file", "--args", `{"path":"note.md"}`}, nil, &out)
	if code != 2 || !strings.Contains(out.String(), "mode luadraw") {
		t.Fatalf("legacy LuaDraw state accepted a mixed override: code=%d out=%s", code, out.String())
	}
	out.Reset()
	code = agentToolCLI(context.Background(), []string{"--workspace", workspace, "--name", "image_context", "--args", `{"image":"images/first.png"}`}, nil, &out)
	if code != 0 || !strings.Contains(out.String(), "image 1 of 1") {
		t.Fatalf("source-md not restored for image_context: code=%d out=%s", code, out.String())
	}
	out.Reset()
	code = agentToolCLI(context.Background(), []string{"--workspace", workspace, "--original", second, "--name", "read_file", "--args", `{"path":"note.md"}`}, nil, &out)
	if code != 2 || !strings.Contains(out.String(), "different reference") {
		t.Fatalf("different reference accepted: code=%d out=%s", code, out.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "note.md"))
	if err != nil || string(data) != "persisted" {
		t.Fatalf("workspace action not saved: %q %v", data, err)
	}
}

func TestAgentToolCLIMultiCallCompileCompareSubmitDeliver(t *testing.T) {
	for _, bin := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip("Poppler tools unavailable")
		}
	}
	resources, err := distribution.FindRoot("")
	if err != nil {
		t.Skip(err)
	}
	luadrawRoot := filepath.Join(resources, "vendor", "luadraw-3.7", "luadraw")
	if _, err = os.Stat(luadrawRoot); err != nil {
		t.Skip("LuaDraw vendor resources unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	source := `local ld=luadraw
local g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}
g:Dline(-1,1,"red,thick")
g:Dcircle(0,1,nil,"blue")
g:Show()
`
	prep := filepath.Join(base, "reference-build")
	if err = os.Mkdir(prep, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(base, "fixture.lua")
	if err = os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	compiled, err := luadraw.Compile(ctx, sourcePath, prep, map[string]any{"resource_root": resources, "luadraw_root": luadrawRoot, "max_scale_dev": .1})
	if err != nil || compiled["ok"] != true {
		t.Skipf("real LuaDraw compiler unavailable: %v %v", err, compiled["error"])
	}
	reference := filepath.Join(base, "reference.png")
	prefix := strings.TrimSuffix(reference, filepath.Ext(reference))
	cmd := exec.CommandContext(ctx, "pdftoppm", "-singlefile", "-png", "-r", "110", compiled["pdf"].(string), prefix)
	if data, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("render reference: %v %s", e, data)
	}

	invoke := func(name, raw string, first bool) map[string]any {
		t.Helper()
		args := []string{"--workspace", workspace, "--name", name, "--resource-root", resources, "--timeout", "90s"}
		if first {
			args = append(args, "--original", reference)
		}
		if raw != "" {
			args = append(args, "--args", raw)
		}
		var out bytes.Buffer
		code := agentToolCLI(ctx, args, nil, &out)
		if code != 0 && !(name == "submit" && strings.Contains(out.String(), "REJECTED:")) {
			t.Fatalf("tool %s: code=%d JSON=%s", name, code, out.String())
		}
		var result map[string]any
		if e := json.Unmarshal(out.Bytes(), &result); e != nil {
			t.Fatalf("tool %s invalid JSON: %v %s", name, e, out.String())
		}
		return result
	}

	written := invoke("write_file", `{"path":"figure.lua","content":`+mustJSONString(t, source)+`}`, true)
	if written["ok"] != true || !strings.Contains(written["text"].(string), "WROTE") {
		t.Fatal(written)
	}
	compiledResult := invoke("compile", `{"path":"figure.lua"}`, false)
	if compiledResult["ok"] != true || !strings.Contains(compiledResult["text"].(string), "COMPILE OK") {
		t.Fatal(compiledResult)
	}
	compared := invoke("compare_figure", `{}`, false)
	if compared["ok"] != true {
		t.Fatal(compared)
	}
	images, ok := compared["images"].([]any)
	if !ok || len(images) != 1 || images[0].(map[string]any)["base64"] == "" || images[0].(map[string]any)["path"] == "" {
		t.Fatalf("image path/base64 contract missing: %#v", compared["images"])
	}
	for i := 0; i < 8; i++ {
		accepted := invoke("submit", `{"path":"figure.lua"}`, false)
		if i == 7 && (!strings.HasPrefix(accepted["text"].(string), "SUBMITTED.") || accepted["state"].(map[string]any)["submitted"] != true) {
			t.Fatalf("expected accepted persisted state on eighth submit: %#v", accepted)
		}
	}
	delivered := invoke("deliver", `{}`, false)
	if delivered["ok"] != true || !strings.Contains(delivered["text"].(string), "submitted_pdf") {
		t.Fatal(delivered)
	}
	if len(delivered["images"].([]any)) != 1 {
		t.Fatalf("delivery preview missing: %#v", delivered["images"])
	}
	deliveredAgain := invoke("deliver", `{}`, false)
	if deliveredAgain["ok"] != true || !strings.Contains(deliveredAgain["text"].(string), "submitted_pdf") {
		t.Fatalf("repeated delivery failed: %#v", deliveredAgain)
	}
	for _, name := range []string{"submitted.lua", "submitted.tex", "submitted.pdf", agentStateName} {
		if _, err = os.Stat(filepath.Join(workspace, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func writeAgentTestImage(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	img := image.NewNRGBA(image.Rect(0, 0, 48, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 48; x++ {
			c := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if x >= 9 && x < 31 && y >= 6 && y < 30 {
				c = color.NRGBA{R: 20, G: 40, B: 60, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
