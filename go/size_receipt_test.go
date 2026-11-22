package comparefigure

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeReceiptRaster(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	ink := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	for y := height / 4; y < height*3/4; y++ {
		for x := width / 4; x < width*3/4; x++ {
			if (x+y)%7 < 2 {
				img.SetNRGBA(x, y, ink)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessRequestSizesCSVReceipt(t *testing.T) {
	d := t.TempDir()
	ref := filepath.Join(d, "03.png")
	cand := filepath.Join(d, "candidate.png")
	writeReceiptRaster(t, ref, 96, 80)
	writeReceiptRaster(t, cand, 96, 80)
	csvPath := filepath.Join(d, "sizes.csv")
	if err := os.WriteFile(csvPath, []byte("# scale_px_per_pt=1.1361\nfile,width_mm,height_mm,page_w_mm\n03.png,86.63,103.71,143.93\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := ProcessRequest(t.Context(), Request{Version: "compare-figure/1", Reference: ref, SizesCSV: csvPath, Candidate: cand, CandidateMM: [2]float64{86.63, 103.71}, OutDir: filepath.Join(d, "out")})
	if err != nil {
		t.Fatal(err)
	}
	if result.ReferenceMeasurement == nil || result.ReferenceMeasurement.Source != "sizes_csv" || result.ReferenceMeasurement.WidthMM != 86.63 || result.ReferenceMeasurement.HeightMM != 103.71 || result.ReferenceMeasurement.PageWMM != 143.93 {
		t.Fatalf("CSV receipt missing: %#v", result.ReferenceMeasurement)
	}
	if result.ReferenceMeasurement.ScalePXPerPT != 1.1361 || result.ReferenceMeasurement.MetadataPath == "" {
		t.Fatalf("CSV provenance missing: %#v", result.ReferenceMeasurement)
	}
}

func TestProcessRequestExplicitCandidateSizeFailsOnlySizeGate(t *testing.T) {
	d := t.TempDir()
	ref, cand := filepath.Join(d, "ref.png"), filepath.Join(d, "cand.png")
	writeReceiptRaster(t, ref, 96, 80)
	writeReceiptRaster(t, cand, 96, 80)
	result, err := ProcessRequest(t.Context(), Request{Version: "compare-figure/1", Reference: ref, ReferenceMM: [2]float64{40, 30}, Candidate: cand, CandidateMM: [2]float64{80, 60}, OutDir: filepath.Join(d, "out"), PassOverlap: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics == nil || result.Metrics.Similarity < .999 {
		t.Fatalf("same raster overlap changed: %#v", result.Metrics)
	}
	if result.Thresholds == nil || !result.Thresholds.OverlapPass || result.Thresholds.SizePass || result.Pass {
		t.Fatalf("expected size-only rejection: thresholds=%#v pass=%v", result.Thresholds, result.Pass)
	}
	if result.Normalization == nil || !result.Normalization.PhysicalMeasuredBefore {
		t.Fatalf("normalization receipt missing: %#v", result.Normalization)
	}
}

func TestProcessRequestResampledSamePhysicalSize(t *testing.T) {
	d := t.TempDir()
	ref, cand := filepath.Join(d, "ref.png"), filepath.Join(d, "cand.png")
	writeReceiptRaster(t, ref, 80, 60)
	writeReceiptRaster(t, cand, 160, 120)
	result, err := ProcessRequest(t.Context(), Request{Version: "compare-figure/1", Reference: ref, ReferenceMM: [2]float64{40, 30}, Candidate: cand, CandidateMM: [2]float64{40, 30}, OutDir: filepath.Join(d, "out")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics == nil || result.Metrics.ScaleDev > .001 {
		t.Fatalf("same physical size drifted: %#v", result.Metrics)
	}
	if result.Normalization == nil || result.Normalization.CandidateScale != .5 {
		t.Fatalf("resample normalization missing: %#v", result.Normalization)
	}
}

func TestProcessRequestWithoutCSVReportsAspectOnly(t *testing.T) {
	d := t.TempDir()
	ref, cand := filepath.Join(d, "ref.png"), filepath.Join(d, "cand.png")
	writeReceiptRaster(t, ref, 80, 60)
	writeReceiptRaster(t, cand, 80, 60)
	result, err := ProcessRequest(t.Context(), Request{Version: "compare-figure/1", Reference: ref, Candidate: cand, OutDir: filepath.Join(d, "out")})
	if err != nil {
		t.Fatal(err)
	}
	if result.ReferenceMeasurement == nil || result.ReferenceMeasurement.Source != "aspect_only" || result.ReferenceMeasurement.Found {
		t.Fatalf("reference falsely physical: %#v", result.ReferenceMeasurement)
	}
	if result.CandidateMeasurement == nil || result.CandidateMeasurement.Source != "aspect_only" || result.CandidateMeasurement.Found {
		t.Fatalf("candidate falsely physical: %#v", result.CandidateMeasurement)
	}
	if result.Metrics == nil || result.Metrics.SizeUnit != "aspect" {
		t.Fatalf("expected aspect comparison: %#v", result.Metrics)
	}
}
