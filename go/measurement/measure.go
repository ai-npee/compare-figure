package measurement

import (
	"encoding/csv"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"
)

type Measurement struct {
	WidthMM      float64 `json:"width_mm"`
	HeightMM     float64 `json:"height_mm"`
	PageWMM      float64 `json:"page_w_mm"`
	PixelsW      int     `json:"pixels_w"`
	PixelsH      int     `json:"pixels_h"`
	DPI          int     `json:"dpi"`
	Found        bool    `json:"found"`
	Aspect       string  `json:"aspect_str"`
	Source       string  `json:"source,omitempty"`
	MetadataPath string  `json:"metadata_path,omitempty"`
	ScalePXPerPT float64 `json:"scale_px_per_pt,omitempty"`
	Warning      string  `json:"warning,omitempty"`
}

func (m Measurement) Hint() string {
	if m.Found {
		page := ""
		if m.PageWMM > 0 {
			page = fmt.Sprintf(", about %.0f%% of the page width", m.WidthMM/m.PageWMM*100)
		}
		return fmt.Sprintf("ORIGINAL FIGURE SIZE: %.1fmm x %.1fmm on the page%s; bitmap %dx%dpx, effective resolution %d dpi, aspect %s.", m.WidthMM, m.HeightMM, page, m.PixelsW, m.PixelsH, m.DPI, m.Aspect)
	}
	if m.Aspect == "" {
		return ""
	}
	return fmt.Sprintf("ORIGINAL BITMAP: %dx%dpx, aspect %s.", m.PixelsW, m.PixelsH, m.Aspect)
}
func (m Measurement) Crop(left, top, right, bottom float64) Measurement {
	if !m.Found || m.PixelsW < 1 || m.PixelsH < 1 {
		return Measurement{}
	}
	fw, fh := (right-left)/100, (bottom-top)/100
	if fw <= 0 || fh <= 0 {
		return Measurement{}
	}
	m.PixelsW, m.PixelsH = int(float64(m.PixelsW)*fw), int(float64(m.PixelsH)*fh)
	m.WidthMM *= fw
	m.HeightMM *= fh
	if m.PixelsW > 0 {
		m.DPI = int(float64(m.PixelsW) / (m.WidthMM / 25.4))
	}
	return m
}
func (m Measurement) CropHint() string {
	if !m.Found {
		return ""
	}
	return fmt.Sprintf("this crop is %.1fmm x %.1fmm on the page", m.WidthMM, m.HeightMM)
}

type physical struct{ width, height, pageW float64 }
type csvRow struct {
	name string
	physical
}
type csvData struct {
	rows  []csvRow
	scale float64
}

func valid(v float64) bool        { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
func finitePair(v []float64) bool { return len(v) >= 2 && valid(v[0]) && valid(v[1]) }
func cleanPath(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "\ufeff"))
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimPrefix(s, "./")
	return filepath.ToSlash(filepath.Clean(s))
}

// MeasureImage retains the original API. parseRoot is accepted for compatibility and ignored.
func MeasureImage(path, parseRoot string, referenceMM []float64) (Measurement, error) {
	return Measure(path, parseRoot, "", referenceMM)
}

// Measure discovers physical dimensions from explicit mm or nearby sizes CSV. parseRoot is ignored.
func Measure(path, parseRoot, sizesCSV string, referenceMM []float64) (Measurement, error) {
	m := Measurement{}
	if f, err := os.Open(path); err == nil {
		cfg, _, e := image.DecodeConfig(f)
		_ = f.Close()
		if e == nil {
			m.PixelsW, m.PixelsH = cfg.Width, cfg.Height
			if m.PixelsW > 0 && m.PixelsH > 0 {
				m.Aspect = fmt.Sprintf("%.2f:1", float64(m.PixelsW)/float64(m.PixelsH))
				m.Source = "aspect_only"
			}
		}
	}
	if finitePair(referenceMM) {
		m.WidthMM, m.HeightMM, m.Found, m.Source = referenceMM[0], referenceMM[1], true, "explicit_mm"
		setDPI(&m)
		return m, nil
	}
	if p, data, warning := lookupCSV(path, sizesCSV); p != "" {
		if data != nil {
			if row, ok := matchCSV(path, p, data.rows); ok {
				m.WidthMM, m.HeightMM, m.PageWMM, m.Found, m.Source = row.width, row.height, row.pageW, true, "sizes_csv"
				m.MetadataPath, m.ScalePXPerPT = p, data.scale
				setDPI(&m)
				if m.Aspect == "" && valid(row.width) && valid(row.height) {
					m.Aspect = fmt.Sprintf("%.2f:1", row.width/row.height)
				}
				return m, nil
			}
		}
		m.Warning = warning
	}
	return m, nil
}
func setDPI(m *Measurement) {
	if m.PixelsW > 0 && valid(m.WidthMM) {
		m.DPI = int(float64(m.PixelsW) / (m.WidthMM / 25.4))
	}
}

func lookupCSV(img, explicit string) (string, *csvData, string) {
	candidates := []string{}
	if explicit != "" {
		candidates = []string{explicit}
	} else {
		dir, _ := filepath.Abs(filepath.Dir(img))
		for i := 0; i < 6 && dir != ""; i++ {
			candidates = append(candidates, filepath.Join(dir, "sizes.csv"), filepath.Join(dir, "size.csv"))
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}
	warning := ""
	var lastPath string
	var lastData *csvData
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		data, err := readCSV(abs)
		if os.IsNotExist(err) && explicit == "" {
			continue
		}
		lastPath = abs
		if err != nil {
			warning = "sizes CSV invalid: " + err.Error()
			continue
		}
		if _, ok := matchCSV(img, abs, data.rows); ok {
			return abs, data, warning
		}
		warning = "sizes CSV has no matching row for " + filepath.Base(img)
		lastPath, lastData = abs, data
		if explicit != "" {
			return abs, data, warning
		}
	}
	return lastPath, lastData, warning
}
func readCSV(path string) (*csvData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	d := &csvData{}
	header := map[string]int{}
	for {
		rec, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if len(rec) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(rec[0]), "#") {
			line := strings.TrimSpace(strings.Join(rec, " "))
			if strings.Contains(line, "scale_px_per_pt=") {
				x := strings.TrimSpace(strings.SplitN(strings.SplitN(line, "scale_px_per_pt=", 2)[1], " ", 2)[0])
				if v, er := strconv.ParseFloat(x, 64); er == nil && valid(v) {
					d.scale = v
				}
			}
			continue
		}
		if len(header) == 0 {
			for i, v := range rec {
				header[strings.ToLower(strings.TrimSpace(v))] = i
			}
			if _, ok := header["file"]; ok {
				continue
			}
		}
		get := func(k string) string {
			i, ok := header[k]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		name := cleanPath(get("file"))
		w, we := strconv.ParseFloat(get("width_mm"), 64)
		h, he := strconv.ParseFloat(get("height_mm"), 64)
		pw, pe := strconv.ParseFloat(get("page_w_mm"), 64)
		if name == "" || we != nil || he != nil || !valid(w) || !valid(h) || (get("page_w_mm") != "" && (pe != nil || !valid(pw))) {
			continue
		}
		d.rows = append(d.rows, csvRow{name: name, physical: physical{w, h, pw}})
	}
	return d, nil
}
func matchCSV(img, csvPath string, rows []csvRow) (csvRow, bool) {
	abs, _ := filepath.Abs(img)
	dir := filepath.Dir(csvPath)
	rel := cleanPath(func() string {
		x, e := filepath.Rel(dir, abs)
		if e != nil {
			return filepath.Base(abs)
		}
		return x
	}())
	base := cleanPath(filepath.Base(abs))
	exact := []csvRow{}
	names := []csvRow{}
	for _, r := range rows {
		n := cleanPath(r.name)
		if n == rel {
			exact = append(exact, r)
		}
		if cleanPath(filepath.Base(n)) == base {
			names = append(names, r)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	if len(exact) > 1 {
		return csvRow{}, false
	}
	if len(names) == 1 {
		return names[0], true
	}
	return csvRow{}, false
}
