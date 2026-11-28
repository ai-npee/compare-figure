package documents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"doc-library/pkg/doclibrary"
)

func TestModelDefinitionExposesOnlyCanonicalRegion(t *testing.T) {
	for _, schema := range []map[string]any{
		(&Docs{}).Definition()["function"].(map[string]any)["parameters"].(map[string]any),
		doclibrary.Schema()["request"].(map[string]any),
	} {
		props := schema["properties"].(map[string]any)
		region := props["region"].(map[string]any)
		if region["type"] != "object" {
			t.Fatalf("region is not an object: %#v", region)
		}
		for _, key := range []string{"left", "top", "right", "bottom"} {
			if _, exists := props[key]; exists {
				t.Fatalf("model exposes duplicate flat crop field %s", key)
			}
			if region["properties"].(map[string]any)[key].(map[string]any)["type"] != "number" {
				t.Fatalf("canonical region lacks numeric %s: %#v", key, region)
			}
		}
	}
}

func TestSparseModelCallsKeepHostSettingsOutOfDefinition(t *testing.T) {
	fn := (&Docs{}).Definition()["function"].(map[string]any)
	if fn["strict"] != false {
		t.Fatal("model options must remain optional")
	}
	schema := fn["parameters"].(map[string]any)
	if required, exists := schema["required"]; exists {
		t.Fatalf("sparse calls must not require unused options: %#v", required)
	}
	props := schema["properties"].(map[string]any)
	for _, key := range []string{"out_dir", "dpi", "passive", "inline_images", "output_version", "kinds", "extensions", "match", "left", "top", "right", "bottom"} {
		if _, exists := props[key]; exists {
			t.Fatalf("host/legacy parameter advertised to model: %s", key)
		}
	}
	if props["include_bbox"].(map[string]any)["type"] != "boolean" {
		t.Fatal("bbox must remain an optional boolean")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "guide.tex"), []byte("needle source body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := NewConfig(Config{Sources: []Source{{ID: "fixture", Root: root}}, CacheDir: t.TempDir(), Output: OutputSettings{Version: 2}})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"mode": "search", "query": "needle", "limit": 5, "max_chars": 5000},
		{"mode": "frequent", "query": "needle", "limit": 5, "max_chars": 5000},
	} {
		text, image, err := d.Execute(context.Background(), args, "")
		if err != nil || !strings.Contains(text, "needle") || image != "" || len(args) != 4 {
			t.Fatalf("sparse call failed or mutated: %#v %s %s %v", args, text, image, err)
		}
		for _, noise := range []string{"namespace", "cache hits", "bbox_pct", "page_size", "index_sha256"} {
			if strings.Contains(text, noise) {
				t.Fatalf("compact output leaked %s: %s", noise, text)
			}
		}
	}
}

func TestDefaultFilledModelSearchAndLegacyCropCompatibility(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "guide.tex"), []byte("needle source body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := NewConfig(Config{Sources: []Source{{ID: "fixture", Root: root}}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// Responses gateways can fill every optional model field, including region.
	args := map[string]any{
		"mode": "search", "query": "needle", "source": "", "sources": []string{},
		"doc": "", "kinds": []string{}, "extensions": []string{}, "regex": false,
		"case_sensitive": false, "match": "any", "limit": 0, "offset": 0,
		"snippet_chars": 0, "max_chars": 0, "line_chars": 0, "start_line": 0,
		"end_line": 0, "page": 0, "out_dir": "", "dpi": 0, "zoom": 0,
		"recursive": false, "include_image": false, "inline_images": false,
		"entry_id": "", "passive": false, "output_version": 0, "include_bbox": false,
	}
	args["region"] = map[string]any{"left": 0, "top": 0, "right": 100, "bottom": 100}
	response, err := d.execute(context.Background(), args, "")
	if err != nil || len(response.Results) != 1 || !strings.Contains(response.Results[0].Text, "needle") || len(response.Images) != 0 {
		t.Fatalf("default-filled search: %#v %v", response, err)
	}
	args["region"] = map[string]any{"left": 0, "top": 0, "right": 0, "bottom": 0}
	if _, err := d.execute(context.Background(), args, ""); err == nil || !strings.Contains(err.Error(), "region must have positive area") {
		t.Fatalf("zero-area canonical region accepted: %v", err)
	}
	legacy := map[string]any{"mode": "search", "query": "needle", "left": 10, "top": 15, "right": 85, "bottom": 90}
	request, err := decodeRequest(legacy, "")
	if err != nil || request.Region == nil || *request.Region != (doclibrary.Region{Left: 10, Top: 15, Right: 85, Bottom: 90}) {
		t.Fatalf("legacy crop lost: %#v %v", request.Region, err)
	}
	if text, image, err := d.Execute(context.Background(), legacy, ""); err != nil || !strings.Contains(text, "needle") || image != "" {
		t.Fatalf("legacy Execute: %s %s %v", text, image, err)
	}
	legacy["region"] = map[string]any{"left": 20, "top": 25, "right": 75, "bottom": 80}
	if _, _, err := d.Execute(context.Background(), legacy, ""); err == nil || !strings.Contains(err.Error(), "use region or flat crop percentages, not both") {
		t.Fatalf("conflicting explicit crops accepted: %v", err)
	}
}

func TestModelFormatUsesProjectedLocationsAndIndependentFooters(t *testing.T) {
	box := [4]float64{1, 2, 3, 4}
	next := 7
	response := Response{Mode: "search", Results: []doclibrary.Result{{Source: "manual", Doc: "doc/book.pdf", Kind: "pdf", Page: 3, StartLine: 9, EndLine: 10, Text: "body", BBox: &box, PageWidth: 600, PageHeight: 800}}, NextOffset: &next, Warnings: []string{"manual:source.tex: skipped binary file", "PDF extraction failed: broken.pdf"}}
	text := formatResponse(response)
	for _, want := range []string{"manual · doc/book.pdf · p.3", "body", "offset=7", "PDF extraction failed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	for _, noise := range []string{"cache", "namespace", "bbox", "page_size", "blocks", "L9", "skipped binary", "已截断"} {
		if strings.Contains(text, noise) {
			t.Fatalf("unexpected %q: %s", noise, text)
		}
	}
	response.NextOffset = nil
	response.Results[0].BBoxPct = &box
	response.Results[0].Truncated = true
	text = formatResponse(response)
	if strings.Count(text, "bbox_pct=") != 1 || !strings.Contains(text, "内容已截断") || strings.Contains(text, "offset=") {
		t.Fatal(text)
	}
	response.Mode = "read"
	response.Results = []doclibrary.Result{{Source: "notes", Doc: "guide.tex", Kind: "source", StartLine: 12, EndLine: 13, Text: "first\nsecond"}}
	text = formatResponse(response)
	if !strings.Contains(text, "notes · guide.tex · L12–13\n12: first\n13: second") {
		t.Fatal(text)
	}
}

func TestHostOutputDefaultsAndExplicitConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "doc"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "doc", "guide.tex"), []byte("needle source body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	host, err := New(root, t.TempDir(), "luadraw")
	if err != nil {
		t.Fatal(err)
	}
	assertProtocol := func(d *Docs, args map[string]any, want string) {
		t.Helper()
		r, e := d.ExecuteResponse(context.Background(), args, "")
		if e != nil {
			t.Fatal(e)
		}
		if r["version"] != want {
			t.Fatalf("protocol: %#v", r)
		}
		for _, key := range []string{"text", "cache", "provenance"} {
			_, exists := r[key]
			if exists != (want == doclibrary.Version) {
				t.Fatalf("%s field %s: %#v", want, key, r)
			}
		}
	}
	assertProtocol(host, map[string]any{"query": "needle"}, doclibrary.CompactVersion)
	assertProtocol(host, map[string]any{"query": "needle", "output_version": 1}, doclibrary.Version)
	legacy, err := NewConfig(Config{Sources: []Source{{ID: "manual", Root: root}}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	assertProtocol(legacy, map[string]any{"query": "needle"}, doclibrary.Version)
	// YAML without a version inherits host2; explicit YAML1 remains legacy.
	skill := t.TempDir()
	cfgDir := filepath.Join(skill, "docs", "document-sources")
	if err = os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{0, 1, 2} {
		yaml := "sources:\n  - id: manual\n    root: " + root + "\n"
		if version == 1 {
			yaml += "output: {version: 1, include_bbox: true}\n"
		}
		if version == 2 {
			yaml += "output: {version: 2, include_bbox: true}\n"
		}
		if err = os.WriteFile(filepath.Join(cfgDir, "luadraw.yaml"), []byte(yaml), 0600); err != nil {
			t.Fatal(err)
		}
		d, e := New(skill, t.TempDir(), "luadraw")
		if e != nil {
			t.Fatal(e)
		}
		want := doclibrary.CompactVersion
		if version == 1 {
			want = doclibrary.Version
		}
		assertProtocol(d, map[string]any{"query": "needle"}, want)
		disabled := false
		override, e := NewWithOutput(skill, t.TempDir(), "luadraw", OutputSettings{Version: 2, IncludeBBox: &disabled})
		if e != nil {
			t.Fatal(e)
		}
		if override.output.IncludeBBox == nil || *override.output.IncludeBBox {
			t.Fatal("explicit startup false lost")
		}
		assertProtocol(override, map[string]any{"query": "needle"}, doclibrary.CompactVersion)
	}
}

func TestOutputOptionsPreserveOptionalFalseAndRejectInvalidFields(t *testing.T) {
	for _, options := range []map[string]any{nil, {}, {"doc_output": nil}} {
		out, err := OutputFromOptions(options)
		if err != nil || out.IncludeBBox != nil || out.Version != 0 {
			t.Fatalf("inherit: %#v %v", out, err)
		}
	}
	out, err := OutputFromOptions(map[string]any{"doc_output": map[string]any{"include_bbox": false, "version": 2}})
	if err != nil || out.IncludeBBox == nil || *out.IncludeBBox || out.Version != 2 {
		t.Fatalf("false: %#v %v", out, err)
	}
	req, err := decodeRequest(map[string]any{"include_bbox": false, "output_version": 2}, "")
	if err != nil || req.IncludeBBox == nil || *req.IncludeBBox || req.OutputVersion != 2 {
		t.Fatalf("request false: %#v %v", req, err)
	}
	for _, raw := range []any{true, map[string]any{"include_bbox": "false"}, map[string]any{"version": 3}, map[string]any{"include_box": false}} {
		if _, err := OutputFromOptions(map[string]any{"doc_output": raw}); err == nil {
			t.Fatalf("accepted %#v", raw)
		}
	}
}

func TestModelFormatCollapsesOnlyKnownStructuredFooters(t *testing.T) {
	next := 3
	snippet := "result snippet truncated; increase snippet_chars, line_chars, or max_chars to read more"
	budget := "result text budget reached; increase max_chars or narrow the request"
	more := "more results available at offset=3"
	unknown := "source extraction skipped binary content unexpectedly: I/O error"
	r := Response{Mode: "search", Truncated: true, NextOffset: &next, Warnings: []string{snippet, budget, more, unknown, "manual:source.tex: binary or invalid UTF-8 skipped", "PDF extraction failed: skipped binary payload"}}
	text := formatResponse(r)
	if strings.Count(text, "offset=3") != 1 || strings.Count(text, "内容已截断") != 1 {
		t.Fatal(text)
	}
	for _, want := range []string{unknown, "PDF extraction failed: skipped binary payload"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	for _, noise := range []string{snippet, budget, more, "manual:source.tex: binary"} {
		if strings.Contains(text, noise) {
			t.Fatal(text)
		}
	}
	// Without matching structured information, even familiar warning text remains.
	r.Truncated = false
	r.NextOffset = nil
	text = formatResponse(r)
	for _, want := range []string{snippet, budget, more} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
}
