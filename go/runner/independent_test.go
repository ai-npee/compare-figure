package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentMarkdownCSVAutomaticRunner(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, _ := setupModel(t)
	book := t.TempDir()
	images := filepath.Join(book, "images", "Book")
	if err := os.MkdirAll(images, 0755); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(images, "figure.png")
	data, _ := os.ReadFile(reference)
	if err := os.WriteFile(image, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(images, "sizes.csv"), []byte("# scale_px_per_pt=1.1361\nfile,width_mm,height_mm,page_w_mm\nfigure.png,86.63,103.71,143.93\n"), 0600); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(book, "Book.md")
	if err := os.WriteFile(md, []byte("# Book\nUse triangle geometry and preserve the label.\n![](images/Book/figure.png)\nFollow-up explanation.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	limit := -1.0
	opts := Options{SourceMD: md, ImageRef: "images/Book/figure.png", Out: filepath.Join(t.TempDir(), "run"), ResourceRoot: resources, Model: "fake-vision", BaseURL: endpoint, APIKey: "test-key", MaxScaleDev: &limit}
	result, err := Run(context.Background(), opts)
	if err != nil || !Bool(result, "submitted", false) || count.Load() != 1 {
		t.Fatal(err, result)
	}
	meta, err := readObject(filepath.Join(opts.Out, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	measured := Obj(Obj(meta, "options"), "measurement")
	if Str(measured, "source", "") != "sizes_csv" || Num(measured, "width_mm", 0) != 86.63 || Num(measured, "height_mm", 0) != 103.71 || Num(measured, "page_w_mm", 0) != 143.93 {
		t.Fatal(meta)
	}
	if !strings.Contains(Str(meta, "context", ""), "Use triangle geometry") {
		t.Fatal(meta)
	}
	transcript, _ := os.ReadFile(filepath.Join(opts.Out, "session.jsonl"))
	if !strings.Contains(string(transcript), "physical ink") || !strings.Contains(string(transcript), "Soft-ink overlap:") {
		t.Fatal("measurement provenance or receipt lost after image copy")
	}
	saved, _ := json.Marshal(result)
	if !strings.Contains(string(saved), "submitted_pdf") {
		t.Fatal(result)
	}
}
