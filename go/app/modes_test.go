package app

import (
	"bytes"
	cf "compare-figure"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func callNative(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var out, diagnostic bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(""), &out, &diagnostic)
	if code != 0 {
		t.Fatalf("%v exit=%d stdout=%s stderr=%s", args, code, out.String(), diagnostic.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err, out.String())
	}
	return result
}
func fileCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}
func TestSemiAutomaticCompareDefaultsReuseAndThresholdFlags(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("COMPARE_FIGURE_OUTPUT_ROOT", cache)
	base := t.TempDir()
	path := filepath.Join(base, "reference.png")
	im := image.NewNRGBA(image.Rect(0, 0, 60, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 60; x++ {
			c := color.NRGBA{255, 255, 255, 255}
			if x > 9 && x < 40 && y > 10 && y < 35 {
				c = color.NRGBA{20, 30, 40, 255}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	if err := cf.WritePNG(path, im); err != nil {
		t.Fatal(err)
	}
	first := callNative(t, "compare", "--reference", path, "--candidate", path)
	if first["pass"] != true || first["similarity_percent"].(float64) < 99 {
		t.Fatal(first)
	}
	thresholds := first["thresholds"].(map[string]any)
	if thresholds["min_overlap"] != .17 || thresholds["min_overlap_percent"] != float64(17) || thresholds["max_scale_dev"] != .1 {
		t.Fatal(first)
	}
	count := fileCount(t, cache)
	for run := 0; run < 4; run++ {
		next := callNative(t, "compare", "--original", path, "--pdf", path)
		if first["selected_image"] != next["selected_image"] || first["out_dir"] != next["out_dir"] {
			t.Fatal("comparison path changed", first, next)
		}
	}
	if fileCount(t, cache) != count || count != 3 {
		t.Fatal("default output grew", count, fileCount(t, cache))
	}
	custom := callNative(t, "compare", "--reference", path, "--candidate", path, "--threshold", "85", "--max-scale-dev", "-1", "--inline-images")
	thresholds = custom["thresholds"].(map[string]any)
	if thresholds["min_overlap_percent"] != float64(85) || thresholds["max_scale_dev"] != float64(-1) {
		t.Fatal(custom)
	}
	if custom["images"].([]any)[0].(map[string]any)["base64"] == "" {
		t.Fatal(custom)
	}
	explicit := filepath.Join(base, "saved")
	saved := callNative(t, "compare", "--original", path, "--pdf", path, "--out", explicit)
	if saved["out_dir"] != explicit {
		t.Fatal(saved)
	}
	resultFile := filepath.Join(first["out_dir"].(string), "result.json")
	if _, err := os.Stat(resultFile); err != nil {
		t.Fatal(err)
	}
}
func TestSemiAutomaticDocumentQueryDefaultImageAndBoundedOutput(t *testing.T) {
	t.Setenv("COMPARE_FIGURE_OUTPUT_ROOT", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	first := callNative(t, "luadraw-doc-search", "Dpolyline3d")
	text := ""
	for _, item := range first["results"].([]any) {
		if body, ok := item.(map[string]any)["text"].(string); ok {
			text += body
		}
	}
	if !strings.Contains(text, "The method g:Dpolyline3d(L") {
		t.Fatal(text)
	}
	if images := first["images"].([]any); len(images) != 0 {
		t.Fatalf("default search attached images: %v", first)
	}
	view := callNative(t, "docs", "--mode", "view", "--doc", "doc/luadraw-doc-en.pdf", "--page", "1", "--left", "10", "--top", "15", "--right", "85", "--bottom", "90")
	images := view["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("explicit view should render one page crop: %v", view)
	}
	imagePath := images[0].(map[string]any)["path"].(string)
	if filepath.Base(imagePath) != "document.png" || filepath.Base(filepath.Dir(imagePath)) != "预览" {
		t.Fatal(imagePath)
	}
	count := fileCount(t, view["out_dir"].(string))
	for i := 0; i < 2; i++ {
		next := callNative(t, "docs", "--mode", "view", "--doc", "doc/luadraw-doc-en.pdf", "--page", "1", "--left", "10", "--top", "15", "--right", "85", "--bottom", "90")
		if next["images"].([]any)[0].(map[string]any)["path"] != imagePath {
			t.Fatal(next)
		}
	}
	if fileCount(t, view["out_dir"].(string)) != count {
		t.Fatal("documentation images accumulated")
	}
	// Explicit true remains compatible for callers that want search-attached crops.
	explicit := callNative(t, "docs", "--query", "Dpolyline3d", "--include-image")
	if len(explicit["images"].([]any)) != 1 {
		t.Fatalf("explicit search image missing: %v", explicit)
	}
}
