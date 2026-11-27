package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func outputFixture(t *testing.T, pdf bool) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"SKILL.md", "prompts/luadraw_figure.system.md", "vendor/luadraw-3.7/luadraw/files/luadraw.sty", "scripts/luadraw_runtime.lua"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "docs", "document-sources")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(root, "references")
	if err := os.MkdirAll(reference, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reference, "guide.tex"), []byte("needle source body\nsecond line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := "  - id: fixture\n    root: ../../references\n"
	if pdf {
		resources := docsTestResources(t)
		source += "  - id: pdf\n    root: " + filepath.ToSlash(filepath.Join(resources, "vendor", "luadraw-3.7", "luadraw", "doc")) + "\n    include: [luadraw-doc-en.pdf]\n"
	}
	config := "sources:\n" + source + "output: {version: 2, include_bbox: true}\n"
	if err := os.WriteFile(filepath.Join(dir, "luadraw.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDocumentSearchCompactJSONAndLegacyOverride(t *testing.T) {
	root := outputFixture(t, false)
	cache := t.TempDir()
	for _, command := range []string{"document-search", "luadraw-doc", "docs"} {
		for _, version := range []int{1, 2} {
			payload := map[string]any{"operation": "document_search", "resource_root": root, "cache_dir": cache, "out_dir": t.TempDir(), "options": map[string]any{"doc_output": map[string]any{"version": 1, "include_bbox": true}}, "arguments": map[string]any{"query": "needle", "output_version": version, "include_bbox": false}}
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := Run(context.Background(), []string{command, "--stdin"}, bytes.NewReader(data), &out, &out)
			if code != 0 {
				t.Fatalf("%s: %d %s", command, code, out.String())
			}
			r := decodeDocsResult(t, out.String())
			if r["version"] != "compare-figure-tools/1" || r["output_version"] != float64(version) || len(r["results"].([]any)) != 1 {
				t.Fatal(r)
			}
			for _, key := range []string{"text", "cache", "provenance", "document_images"} {
				_, exists := r[key]
				if exists != (version == 1) {
					t.Fatalf("v%d unexpected %s presence=%t: %#v", version, key, exists, r)
				}
			}
			if strings.Count(out.String(), "needle source body") != map[int]int{1: 2, 2: 1}[version] {
				t.Fatalf("body duplication v%d: %s", version, out.String())
			}
		}
	}
	// Flag defaults are inherited by null output options and explicit JSON false
	// remains distinguishable from omission.
	request, err := prepareDocsRequest(map[string]any{"arguments": map[string]any{"include_bbox": nil, "output_version": nil}}, map[string]any{"include_bbox": false, "output_version": 2})
	if err != nil {
		t.Fatal(err)
	}
	args := request["arguments"].(map[string]any)
	if args["include_bbox"] != false || args["output_version"] != 2 {
		t.Fatal(request)
	}
}

func TestDocumentOutputBBoxStartupAndSingleFalseWithImage(t *testing.T) {
	root := outputFixture(t, true)
	svc := &service{}
	cache := t.TempDir()
	for _, startup := range []bool{false, true} {
		for _, requestBBox := range []any{nil, false, true} {
			args := map[string]any{"mode": "search", "source": "pdf", "query": "Dpolyline3d", "limit": 1, "include_image": true, "zoom": 180}
			if requestBBox != nil {
				args["include_bbox"] = requestBBox
			}
			payload := map[string]any{"operation": "docs", "resource_root": root, "cache_dir": cache, "out_dir": t.TempDir(), "options": map[string]any{"doc_output": map[string]any{"version": 2, "include_bbox": startup}}, "arguments": args}
			result, err := svc.handle(context.Background(), payload)
			if err != nil {
				t.Fatal(err)
			}
			r := result.(map[string]any)
			hits := r["results"].([]any)
			if len(hits) != 1 {
				t.Fatal(r)
			}
			hit := hits[0].(map[string]any)
			want := startup
			if explicit, ok := requestBBox.(bool); ok {
				want = explicit
			}
			_, present := hit["bbox_pct"]
			if present != want || hit["bbox"] != nil || hit["page_width"] != nil || hit["page"] == nil {
				t.Fatalf("startup=%t request=%v: %#v", startup, requestBBox, hit)
			}
			encoded, err := json.Marshal(r["images"])
			if err != nil {
				t.Fatal(err)
			}
			var images []any
			if err = json.Unmarshal(encoded, &images); err != nil || len(images) != 1 {
				t.Fatalf("hidden bbox prevented image: %s %v", encoded, err)
			}
		}
	}
}

func TestDocumentOutputCLIExplicitFalseOverridesSourceDefault(t *testing.T) {
	root := outputFixture(t, true)
	for _, command := range []string{"document-search", "luadraw-doc"} {
		for _, bbox := range []string{"false", "true"} {
			args := []string{command, "--resource-root", root, "--cache-dir", filepath.Join(root, "cache"), "--query", "Dpolyline3d", "--doc-source", "pdf", "--limit", "1", "--output-version=2", "--include-bbox", bbox}
			var out bytes.Buffer
			if code := Run(context.Background(), args, nil, &out, &out); code != 0 {
				t.Fatalf("%d %s", code, out.String())
			}
			r := decodeDocsResult(t, out.String())
			hit := r["results"].([]any)[0].(map[string]any)
			_, present := hit["bbox_pct"]
			if present != (bbox == "true") {
				t.Fatalf("%s: %s", bbox, out.String())
			}
		}
	}
}
