package drawing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	cf "compare-figure"
)

func TestRealLuaDrawCompileCropCompareSubmitDeliver(t *testing.T) {
	for _, bin := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip("Poppler tools unavailable")
		}
	}
	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ldroot := filepath.Join(projectRoot, "vendor", "luadraw-3.7", "luadraw")
	if _, err = os.Stat(ldroot); err != nil {
		t.Skip("LuaDraw vendor resources unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	work := filepath.Join(dir, "work")
	if err = os.MkdirAll(work, 0755); err != nil {
		t.Fatal(err)
	}
	source := `local ld=luadraw
local g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}
g:Dline(-1,1,"red,thick")
g:Dcircle(0,1,nil,"blue")
g:Show()
`
	if err = os.WriteFile(filepath.Join(work, "figure.lua"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	options := map[string]any{"resource_root": ldroot, "luadraw_root": ldroot, "accept_min_submits": 8, "max_scale_dev": -1.0, "raster_dpi": 110, "view_pdf_max": 1, "mask_executables": []string{"lualatex"}}
	ref := filepath.Join(dir, "reference.png")
	h, err := New(work, ref, "", -1, 90*time.Second, options)
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := h.Execute(ctx, "compile", map[string]any{"path": "figure.lua"})
	if err != nil || !strings.Contains(text, "COMPILE OK") {
		t.Fatalf("compile: %s %v", text, err)
	}
	if err = renderAtDPI(ctx, filepath.Join(work, "standalone.pdf"), 110, ref); err != nil {
		t.Fatal(err)
	}
	text, imagePath, err := h.Execute(ctx, "view_pdf", map[string]any{"path": "standalone.pdf", "page": 1, "left": 20, "top": 20, "right": 80, "bottom": 80, "zoom": 700})
	if err != nil || !strings.Contains(text, "this crop") || imagePath == "" {
		t.Fatalf("crop view: %s %s %v", text, imagePath, err)
	}
	if _, err = cf.DecodeImage(imagePath); err != nil {
		t.Fatalf("crop image decode: %v", err)
	}
	if !strings.Contains(text, "VIEW BUDGET:") {
		t.Fatalf("missing first-view warning: %s", text)
	}
	text, _, err = h.Execute(ctx, "view_pdf", map[string]any{"path": "standalone.pdf", "page": 1, "left": 20, "top": 20, "right": 80, "bottom": 80, "zoom": 700})
	if err != nil || !strings.Contains(text, "VIEW BUDGET SPENT") {
		t.Fatalf("missing spent notice: %s %v", text, err)
	}
	text, imagePath, err = h.Execute(ctx, "compare_figure", map[string]any{"detail": "all"})
	if err != nil || imagePath == "" {
		t.Fatalf("compare: %s %s %v", text, imagePath, err)
	}
	if number(h.State["best_overlap"], 0) < .9 || stringValue(h.State["size_unit_at_best"], "") == "" {
		t.Fatalf("comparison metrics not recorded together: %#v", h.State)
	}
	text, _, err = h.Execute(ctx, "submit", map[string]any{"path": "figure.lua"})
	if err != nil || !strings.HasPrefix(text, "SUBMITTED.") {
		t.Fatalf("submit: %s %v", text, err)
	}
	result, err := h.Deliver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"submitted_pdf", "submitted_tex", "editable_source"} {
		path, ok := result[key].(string)
		if !ok || path == "" {
			t.Fatalf("delivery missing %s: %#v", key, result)
		}
		if _, e := os.Stat(path); e != nil {
			t.Fatalf("delivery artifact %s: %v", key, e)
		}
	}
	if _, e := exec.LookPath("pdftocairo"); e == nil {
		h.Options["raster_command"] = "pdftocairo"
		raster, e := h.Rasterize(ctx, result["submitted_pdf"].(string), filepath.Join(work, "cairo-render"))
		if e != nil {
			t.Fatalf("pdftocairo Rasterize: %v", e)
		}
		if _, e = cf.DecodeImage(raster); e != nil {
			t.Fatalf("pdftocairo image: %v", e)
		}
		_, cropImage, e := h.Execute(ctx, "view_pdf", map[string]any{"path": "submitted.pdf", "page": 1, "left": 15, "top": 15, "right": 85, "bottom": 85, "zoom": 500})
		if e != nil || cropImage == "" {
			t.Fatalf("pdftocairo view_pdf: %s %v", cropImage, e)
		}
	}
}

func renderAtDPI(ctx context.Context, pdf string, dpi int, output string) error {
	prefix := strings.TrimSuffix(output, filepath.Ext(output))
	cmd := exec.CommandContext(ctx, "pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-png", "-r", strconv.Itoa(dpi), pdf, prefix)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(data)))
	}
	return nil
}
