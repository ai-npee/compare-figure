package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutomaticModeDirectModelURLAndEphemeralKey(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, _ := setupModel(t)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_API_KEY", "")
	scale := -1.0
	key := "direct-mode-secret-not-persisted"
	opts := Options{Original: reference, Out: filepath.Join(t.TempDir(), "automatic"), ResourceRoot: resources, Model: "fake-vision", BaseURL: endpoint, APIKey: key, MaxScaleDev: &scale}
	result, err := Run(context.Background(), opts)
	if err != nil || !Bool(result, "submitted", false) || count.Load() != 1 {
		t.Fatal(err, result, count.Load())
	}
	metadata, err := readObject(filepath.Join(opts.Out, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if Num(metadata, "fig_min_overlap", 0) != .17 || metadata["drawing_mode"] != "luadraw" {
		t.Fatal(metadata)
	}
	for _, name := range []string{"run.json", "session.jsonl", "result.json"} {
		data, err := os.ReadFile(filepath.Join(opts.Out, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(key)) {
			t.Fatalf("ephemeral key persisted in %s", name)
		}
	}
	_, _, _, oldOptions, err := inputOptions(Object{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	metadata["input_fingerprint"] = oldOptions["_legacy_input_fingerprint"]
	delete(metadata, "drawing_mode")
	delete(metadata, "fingerprint_mode")
	options := Obj(metadata, "options")
	delete(options, "drawing_mode")
	if err = atomicJSON(filepath.Join(opts.Out, "run.json"), metadata); err != nil {
		t.Fatal(err)
	}
	resumed, err := Run(context.Background(), opts)
	if err != nil || !Bool(resumed, "resumed", false) || count.Load() != 1 {
		t.Fatal("legacy LuaDraw accepted state was not resumed", err, resumed, count.Load())
	}
	otherMode := opts
	otherMode.DrawingMode = "latex"
	if _, err := Run(context.Background(), otherMode); err == nil || !strings.Contains(err.Error(), "drawing mode") || count.Load() != 1 {
		t.Fatal("accepted result was reused across drawing modes", err, count.Load())
	}
}

func TestResumeCreditRequiresAndRebasesNonLuaEditableSource(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"submitted.tex", "submitted.pdf", "submitted.asy"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("artifact"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result := Object{"best_overlap": .9, "scale_dev": .02, "source_language": "asymptote"}
	if err := atomicJSON(filepath.Join(root, "result.json"), result); err != nil {
		t.Fatal(err)
	}
	if got := ResumeCredit(root, .17, .1, true); got != nil {
		t.Fatalf("accepted Asymptote output without editable source: %#v", got)
	}
	result["editable_source"] = filepath.Join("/old", "workspace", "submitted.asy")
	if err := atomicJSON(filepath.Join(root, "result.json"), result); err != nil {
		t.Fatal(err)
	}
	got := ResumeCredit(root, .17, .1, true)
	if got == nil || got["editable_source"] != filepath.Join(root, "submitted.asy") {
		t.Fatalf("editable Asymptote source was not rebased: %#v", got)
	}
}

func TestStrategyPromptsAreModeSpecific(t *testing.T) {
	mixed := strategySystem("mixed", "Chinese", 8, Object{"tools": Object{"asy": Object{"usable": true}}})
	for _, want := range []string{"source=luadraw", "source=asymptote", "Environment facts:", "actual 3D geometry", "image_context", "compile.merges", "// for comments"} {
		if !strings.Contains(mixed, want) {
			t.Fatalf("mixed prompt missing %q", want)
		}
	}
	latex := strategySystem("latex", "Chinese", 8, Object{})
	if strings.Contains(latex, "source=luadraw") || strings.Contains(latex, "source=asymptote") || !strings.Contains(latex, "no bundled drawing manual") {
		t.Fatal("native LaTeX prompt recommends unrelated references", latex)
	}
	if !strings.Contains(strategySystem("asymptote", "Chinese", 8, Object{}), "Asymptote") {
		t.Fatal("Asymptote prompt missing its drawing policy")
	}
}

func TestInputFingerprintIncludesDrawingMode(t *testing.T) {
	resources, reference := realFixture(t)
	base := Options{Original: reference, ResourceRoot: resources, DrawingMode: "latex"}
	_, _, latexFingerprint, _, err := inputOptions(Object{}, base)
	if err != nil {
		t.Fatal(err)
	}
	base.DrawingMode = "asymptote"
	_, _, asymptoteFingerprint, _, err := inputOptions(Object{}, base)
	if err != nil {
		t.Fatal(err)
	}
	if latexFingerprint == asymptoteFingerprint {
		t.Fatal("drawing mode did not affect input fingerprint")
	}
}
