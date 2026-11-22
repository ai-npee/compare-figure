package comparefigure

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessRequestDefaultOutputIsBounded(t *testing.T) {
	t.Setenv("COMPARE_FIGURE_OUTPUT_ROOT", t.TempDir())
	source := filepath.Join(t.TempDir(), "source.png")
	im := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			pixel := color.NRGBA{255, 255, 255, 255}
			if x > 10 && x < 30 && y > 7 && y < 20 {
				pixel = color.NRGBA{20, 20, 20, 255}
			}
			im.SetNRGBA(x, y, pixel)
		}
	}
	if err := WritePNG(source, im); err != nil {
		t.Fatal(err)
	}
	request := Request{Version: "compare-figure/1", Reference: source, Candidate: source}
	first, err := ProcessRequest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 4; repeat++ {
		next, err := ProcessRequest(context.Background(), request)
		if err != nil || first.SelectedImage != next.SelectedImage || first.OutDir != next.OutDir {
			t.Fatal(err, next)
		}
	}
	entries, err := os.ReadDir(first.OutDir)
	if err != nil || len(entries) != 3 {
		t.Fatal(err, entries)
	}
	if first.Thresholds == nil || first.Thresholds.MinOverlap != .17 || !first.Pass {
		t.Fatal(first)
	}
}
