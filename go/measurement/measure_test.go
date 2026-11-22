package measurement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeImage(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAYAAAD0In+KAAAAFUlEQVR42mNkYPj/n4GBgYGJAQoAAgwAAf8C/q0AAAAASUVORK5CYII="
	b, _ := decodeBase64(png)
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}
func decodeBase64(s string) ([]byte, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out []byte
	var val, bits int
	for _, r := range s {
		if r == '=' {
			break
		}
		i := strings.IndexRune(alphabet, r)
		if i < 0 {
			continue
		}
		val = val<<6 | i
		bits += 6
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(val>>bits))
			val &= (1 << bits) - 1
		}
	}
	return out, nil
}

func TestCSVIndependentHeightAndScaleProvenance(t *testing.T) {
	d := t.TempDir()
	img := writeImage(t, d, "03.jpg")
	csv := filepath.Join(d, "sizes.csv")
	os.WriteFile(csv, []byte("# scale_px_per_pt=1.1361\nfile,width_mm,height_mm,page_w_mm\n03.jpg,86.63,103.71,143.93\n"), 0644)
	m, err := MeasureImage(img, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Found || m.Source != "sizes_csv" || m.WidthMM != 86.63 || m.HeightMM != 103.71 || m.PageWMM != 143.93 || m.ScalePXPerPT != 1.1361 {
		t.Fatalf("bad measurement: %+v", m)
	}
	b, _ := json.Marshal(m)
	if !strings.Contains(string(b), "metadata_path") {
		t.Fatal("metadata provenance missing")
	}
}
func TestExplicitBeatsCSV(t *testing.T) {
	d := t.TempDir()
	img := writeImage(t, d, "x.png")
	os.WriteFile(filepath.Join(d, "sizes.csv"), []byte("file,width_mm,height_mm\nx.png,10,20\n"), 0644)
	m, _ := Measure(img, "", filepath.Join(d, "sizes.csv"), []float64{30, 40})
	if m.Source != "explicit_mm" || m.WidthMM != 30 || m.HeightMM != 40 {
		t.Fatalf("%+v", m)
	}
}
func TestDuplicateBasenameDoesNotMatch(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "c")
	os.Mkdir(sub, 0755)
	img := writeImage(t, sub, "same.png")
	os.WriteFile(filepath.Join(d, "sizes.csv"), []byte("file,width_mm,height_mm\na/same.png,10,20\nb/same.png,30,40\n"), 0644)
	m, _ := MeasureImage(img, "", nil)
	if m.Found || m.Warning == "" {
		t.Fatalf("duplicate should not match: %+v", m)
	}
}
func TestCropKeepsCSVHeightRatio(t *testing.T) {
	m := Measurement{Found: true, WidthMM: 100, HeightMM: 50, PixelsW: 1000, PixelsH: 500}
	c := m.Crop(0, 0, 50, 25)
	if c.WidthMM != 50 || c.HeightMM != 12.5 {
		t.Fatalf("%+v", c)
	}
}
func TestAncestorCSV(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "one", "two")
	os.MkdirAll(sub, 0755)
	img := writeImage(t, sub, "z.jpg")
	os.WriteFile(filepath.Join(d, "sizes.csv"), []byte("file,width_mm,height_mm\none/two/z.jpg,11,22\n"), 0644)
	m, _ := MeasureImage(img, "", nil)
	if !m.Found || m.WidthMM != 11 || m.HeightMM != 22 {
		t.Fatalf("%+v", m)
	}
}
func TestInvalidCSVWarnsAndFallsBackToAspect(t *testing.T) {
	d := t.TempDir()
	img := writeImage(t, d, "none.jpg")
	os.WriteFile(filepath.Join(d, "sizes.csv"), []byte("file,width_mm,height_mm\nnone.jpg,NaN,4\nother.jpg,-1,3\n"), 0644)
	m, err := MeasureImage(img, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Found || m.Aspect == "" || m.Warning == "" {
		t.Fatalf("expected aspect fallback and warning: %+v", m)
	}
}
func TestQuotedWindowsPathAndBOM(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "figs")
	os.Mkdir(sub, 0755)
	img := writeImage(t, sub, "q.png")
	p := filepath.Join(d, "sizes.csv")
	data := "\ufeff# scale_px_per_pt=1.2\nfile,width_mm,height_mm,page_w_mm\n\"figs\\q.png\",12,34,100\n"
	os.WriteFile(p, []byte(data), 0644)
	m, err := MeasureImage(img, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Found || m.WidthMM != 12 || m.HeightMM != 34 || m.ScalePXPerPT != 1.2 {
		t.Fatalf("quoted path failed: %+v", m)
	}
}
