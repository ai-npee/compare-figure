package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/distribution"
)

func TestManualCLIHelpAndValidation(t *testing.T) {
	for _, command := range []string{"compile", "luadraw-compile", "docs", "luadraw-doc-search"} {
		var out bytes.Buffer
		if code := manualCLI(context.Background(), command, []string{"--help"}, nil, &out); code != 0 || !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("help %s: code=%d output=%q", command, code, out.String())
		}
	}
	cases := []struct {
		command string
		args    []string
		want    string
	}{
		{"compile", nil, "requires --source"},
		{"docs", nil, "requires --query"},
		{"docs", []string{"--mode", "read"}, "requires --doc"},
		{"docs", []string{"--mode", "view"}, "requires --doc"},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if code := manualCLI(context.Background(), tc.command, tc.args, nil, &out); code != 2 || !strings.Contains(out.String(), tc.want) {
			t.Fatalf("%s %v: code=%d output=%q", tc.command, tc.args, code, out.String())
		}
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["ok"] != false {
			t.Fatalf("not a JSON error: %v %s", err, out.String())
		}
	}
}

func TestManualCLIPositionalArgumentsAndOptionReordering(t *testing.T) {
	for _, args := range [][]string{
		{"Dpolyline3d", "--mode=invalid"},
		{"--mode", "invalid", "Dpolyline3d"},
	} {
		var out bytes.Buffer
		if code := manualCLI(context.Background(), "docs", args, nil, &out); code != 2 || !strings.Contains(out.String(), "invalid docs mode") {
			t.Fatalf("positional parsing %v: code=%d output=%s", args, code, out.String())
		}
	}
	var out bytes.Buffer
	if code := manualCLI(context.Background(), "compile", []string{"not-present.lua", "--engine", "bad"}, nil, &out); code != 2 || !strings.Contains(out.String(), "open not-present.lua") {
		t.Fatalf("compile options after positional: code=%d output=%s", code, out.String())
	}
}

func TestDocumentSearchCLIMixedAsymptoteSourceFilter(t *testing.T) {
	resources := docsTestResources(t)
	var out bytes.Buffer
	args := []string{"--drawing-mode", "mixed", "--doc-source", "asymptote", "--mode", "list", "--limit", "1", "--resource-root", resources, "--cache-dir", filepath.Join(t.TempDir(), "cache"), "--include-image=false"}
	if code := manualCLI(context.Background(), "docs", args, nil, &out); code != 0 {
		t.Fatalf("document-search smoke failed: %d %s", code, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err, out.String())
	}
	rows, ok := result["results"].([]any)
	if !ok || len(rows) == 0 || rows[0].(map[string]any)["source"] != "asymptote" {
		t.Fatalf("doc-source filter did not select asymptote: %#v", result)
	}
}

func TestManualCLIDocsReadRealSourceWithoutModel(t *testing.T) {
	for _, tool := range []string{"pdftotext", "pdfinfo", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("Poppler unavailable: %s", tool)
		}
	}
	resources, err := distribution.FindRoot("")
	if err != nil {
		t.Skip(err)
	}
	var out bytes.Buffer
	args := []string{"--mode", "read", "--doc", "files/luadraw_base.lua", "--resource-root", resources, "--cache-dir", filepath.Join(t.TempDir(), "cache"), "--out", filepath.Join(t.TempDir(), "out"), "--start-line", "1", "--end-line", "2", "--include-image=false"}
	if code := manualCLI(context.Background(), "luadraw-doc-search", args, nil, &out); code != 0 {
		t.Fatal(code, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err, out.String())
	}
	if result["ok"] != true || result["results"].([]any)[0].(map[string]any)["doc"] != "files/luadraw_base.lua" {
		t.Fatal(result)
	}
}
