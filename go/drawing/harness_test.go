package drawing

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testHarness(t *testing.T, opts map[string]any) *Harness {
	t.Helper()
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref.png")
	f, e := os.Create(ref)
	if e != nil {
		t.Fatal(e)
	}
	im := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	for y := 4; y < 20; y++ {
		for x := 5; x < 25; x++ {
			im.Set(x, y, color.Black)
		}
	}
	if e = png.Encode(f, im); e != nil {
		t.Fatal(e)
	}
	_ = f.Close()
	h, e := New(filepath.Join(dir, "work"), ref, "", .17, time.Second, opts)
	if e != nil {
		t.Fatal(e)
	}
	return h
}

func TestDefinitionsRetainElevenTools(t *testing.T) {
	h := testHarness(t, nil)
	defs := h.Definitions()
	if len(defs) != 11 {
		t.Fatalf("got %d definitions", len(defs))
	}
	expected := []string{"write_file", "edit_file", "read_file", "grep", "compile", "view_pdf", "compare_figure", "submit", "image_context", "view_image", "document_search"}
	for i, d := range defs {
		fn := d["function"].(map[string]any)
		if fn["name"] != expected[i] {
			t.Fatalf("definition %d name=%v", i, fn["name"])
		}
		if _, ok := fn["description"].(string); !ok {
			t.Fatalf("definition %s missing description", expected[i])
		}
		params := fn["parameters"].(map[string]any)
		if params["type"] != "object" {
			t.Fatalf("definition %s invalid parameters", expected[i])
		}
	}
	submit := defs[7]["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, ok := submit["accept_as_is"]; ok {
		t.Fatal("best-effort control must remain hidden")
	}
}

func TestReadEditGrepAndPathBoundary(t *testing.T) {
	h := testHarness(t, nil)
	if text, _, e := h.Execute(context.Background(), "write_file", map[string]any{"path": "draft.lua", "content": "alpha\nbeta\nalpha\n"}); e != nil || !strings.Contains(text, "WROTE") {
		t.Fatalf("write: %s %v", text, e)
	}
	text, _, e := h.Execute(context.Background(), "edit_file", map[string]any{"path": "draft.lua", "find": "alpha", "replace": "omega"})
	if e != nil || !strings.Contains(text, "AMBIGUOUS") {
		t.Fatalf("edit ambiguity: %s %v", text, e)
	}
	text, _, e = h.Execute(context.Background(), "edit_file", map[string]any{"path": "draft.lua", "find": "alpha", "replace": "omega", "replace_all": true})
	if e != nil || !strings.Contains(text, "REPLACED 2") {
		t.Fatalf("replace: %s %v", text, e)
	}
	text, _, e = h.Execute(context.Background(), "read_file", map[string]any{"path": "draft.lua", "start_line": 2})
	if e != nil || !strings.Contains(text, "2: beta") {
		t.Fatalf("read: %s %v", text, e)
	}
	text, _, e = h.Execute(context.Background(), "grep", map[string]any{"pattern": "omega", "path": ".", "max_matches": 5})
	if e != nil || !strings.Contains(text, "draft.lua:1:omega") {
		t.Fatalf("grep: %s %v", text, e)
	}
	if _, e = h.safe("../escape"); e == nil {
		t.Fatal("path traversal should be rejected")
	}
	if _, e = h.safe("other:/etc/passwd"); e == nil {
		t.Fatal("unknown mount should be rejected")
	}
}

func TestMeasurementSizesCSVRefreshAndExplicitOverride(t *testing.T) {
	root := t.TempDir()
	images := filepath.Join(root, "nested", "images")
	if e := os.MkdirAll(images, 0755); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(images, "sample.png")
	f, e := os.Create(target)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 400, 200))); e != nil {
		t.Fatal(e)
	}
	_ = f.Close()
	sizes := filepath.Join(images, "sizes.csv")
	if e = os.WriteFile(sizes, []byte("# scale_px_per_pt=1.1361\nfile,width_mm,height_mm,page_w_mm\nsample.png,25,12.5,100\n"), 0644); e != nil {
		t.Fatal(e)
	}
	m, e := MeasureImage(target, "", nil)
	if e != nil || !m.Found || m.Source != "sizes_csv" || m.WidthMM != 25 || m.HeightMM != 12.5 || m.PageWMM != 100 {
		t.Fatalf("unexpected CSV measurement %#v %v", m, e)
	}
	updated := []byte("file,width_mm,height_mm,page_w_mm\nsample.png,50,25,120\n")
	if e = os.WriteFile(sizes, updated, 0644); e != nil {
		t.Fatal(e)
	}
	refreshed, e := MeasureImage(target, "", nil)
	if e != nil || refreshed.WidthMM != 50 || refreshed.HeightMM != 25 || refreshed.PageWMM != 120 {
		t.Fatalf("CSV did not refresh after update: %#v %v", refreshed, e)
	}
	explicit, e := MeasureImage(target, "", []float64{30, 20})
	if e != nil || explicit.Source != "explicit_mm" || explicit.WidthMM != 30 || explicit.HeightMM != 20 {
		t.Fatalf("explicit dimensions %#v %v", explicit, e)
	}
	if crop := m.Crop(0, 0, 50, 50); !strings.Contains(crop.CropHint(), "this crop is") {
		t.Fatal(crop.CropHint())
	}
}

func TestSubmitAttemptsThresholdAndAcceptedSnapshot(t *testing.T) {
	h := testHarness(t, map[string]any{"accept_min_submits": 8, "max_scale_dev": .1})
	h.State["last_code"] = "local g=luadraw\n"
	h.State["compile_ok"] = true
	h.State["compare_called"] = true
	h.State["best_overlap"] = .05
	h.State["scale_dev_at_best"] = .3
	h.State["size_unit_at_best"] = "aspect"
	h.State["aspect_ref_at_best"] = 1
	h.State["aspect_cand_at_best"] = 1.3
	h.State["final_code"] = ""
	for i := 0; i < 7; i++ {
		text, e := h.submit(map[string]any{"code": "local g=luadraw\n", "accept_as_is": true, "note": "minor mismatch"})
		if e != nil || !strings.HasPrefix(text, "REJECTED:") {
			t.Fatalf("attempt %d: %s %v", i+1, text, e)
		}
	}
	text, e := h.submit(map[string]any{"code": "local g=luadraw\n", "accept_as_is": true, "note": "minor mismatch"})
	if e != nil || !strings.HasPrefix(text, "SUBMITTED as best-effort") {
		t.Fatalf("attempt 8: %s %v", text, e)
	}
	snap := h.Snapshot()
	if snap["_accepted_threshold"] != .17 || snap["_accepted_scale_limit"] != .1 {
		t.Fatalf("accepted gates absent: %#v", snap)
	}
	snap["final_code"] = "mutated"
	if h.State["final_code"] == "mutated" {
		t.Fatal("snapshot must deep copy")
	}
	encoded, _ := json.Marshal(h.State)
	restored := map[string]any{}
	_ = json.Unmarshal(encoded, &restored)
	h2 := testHarness(t, nil)
	if e = h2.Restore(restored); e != nil || !h2.IsSubmitted() {
		t.Fatalf("restore accepted state: %v %#v", e, h2.State)
	}
	if e = h2.Restore(nil); e == nil {
		t.Fatal("nil restore expected error")
	}
}
