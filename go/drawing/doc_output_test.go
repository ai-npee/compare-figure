package drawing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDocumentDefinitionsAndExecutionShareStartupOutput(t *testing.T) {
	resource := t.TempDir()
	for path, body := range map[string]string{"docs/document-sources/luadraw.yaml": "sources:\n  - id: fixture\n    root: ../../references\noutput: {version: 2, include_bbox: true}\n", "references/guide.tex": "needle source body\n"} {
		name := filepath.Join(resource, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	options := map[string]any{"resource_root": resource, "doc_cache_dir": t.TempDir(), "doc_output": map[string]any{"version": 2, "include_bbox": false}}
	h, err := New(t.TempDir(), "reference.png", "", .17, time.Second, options)
	if err != nil {
		t.Fatal(err)
	}
	check := func(def map[string]any) {
		t.Helper()
		props := def["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)
		for _, key := range []string{"include_bbox", "region"} {
			if _, ok := props[key]; !ok {
				t.Fatalf("missing %s", key)
			}
		}
		for _, key := range []string{"out_dir", "dpi", "passive", "inline_images", "output_version"} {
			if _, ok := props[key]; ok {
				t.Fatalf("host-only setting advertised to model: %s", key)
			}
		}
		region := props["region"].(map[string]any)
		if region["type"] != "object" || region["additionalProperties"] != false {
			t.Fatalf("invalid canonical region: %#v", region)
		}
		for _, key := range []string{"left", "top", "right", "bottom"} {
			if _, exists := props[key]; exists {
				t.Fatalf("model exposes legacy flat crop field %s", key)
			}
			if region["properties"].(map[string]any)[key].(map[string]any)["type"] != "number" {
				t.Fatalf("canonical region lacks numeric %s", key)
			}
		}
	}
	check(docsToolDefinition())
	found := false
	for _, def := range h.Definitions() {
		if def["function"].(map[string]any)["name"] == "document_search" {
			check(def)
			found = true
		}
	}
	if !found || h.docs == nil {
		t.Fatal("shared document tool missing")
	}
	response, err := h.docs.ExecuteResponse(context.Background(), map[string]any{"query": "needle"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if response["version"] != "doc-library/2" || response["text"] != nil {
		t.Fatal(response)
	}
	text, _, err := h.Execute(context.Background(), "document_search", map[string]any{"mode": "read", "doc": "guide.tex", "start_line": 1, "end_line": 1})
	if err != nil || !strings.Contains(text, "fixture · guide.tex · L1–1\n1: needle source body") || strings.Contains(text, "cache") {
		t.Fatalf("%s %v", text, err)
	}
	if _, err = New(t.TempDir(), "reference.png", "", .17, time.Second, map[string]any{"doc_output": map[string]any{"include_bbox": "false"}}); err == nil {
		t.Fatal("invalid output setting ignored")
	}
}
