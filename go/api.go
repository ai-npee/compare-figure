package comparefigure

import (
	"compare-figure/measurement"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"

	xdraw "golang.org/x/image/draw"
)

// Request describes one comparison. CandidatePath accepts a raster image or PDF.
type Request struct {
	Version              string                   `json:"version"`
	Reference            string                   `json:"reference"`
	SizesCSV             string                   `json:"sizes_csv,omitempty"`
	Candidate            string                   `json:"candidate"`
	ReferenceMM          [2]float64               `json:"reference_mm,omitempty"`
	ReferenceMeasurement *measurement.Measurement `json:"reference_measurement,omitempty"`
	CandidateMM          [2]float64               `json:"candidate_mm,omitempty"`
	Detail               string                   `json:"detail,omitempty"`
	Crop                 [4]float64               `json:"crop,omitempty"`
	WorstRegion          bool                     `json:"worst_region,omitempty"`
	Zoom                 int                      `json:"zoom,omitempty"`
	RasterDPI            int                      `json:"raster_dpi,omitempty"`
	OutDir               string                   `json:"out_dir,omitempty"`
	InlineImages         bool                     `json:"inline_images,omitempty"`
	PassOverlap          float64                  `json:"pass_overlap,omitempty"`
	PassScale            float64                  `json:"pass_scale,omitempty"`
}

type ImageArtifact struct {
	Path   string `json:"path"`
	MIME   string `json:"mime"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Base64 string `json:"base64,omitempty"`
}

type AlignmentRun struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
	DX    int     `json:"dx"`
	DY    int     `json:"dy"`
}

type Metrics struct {
	Similarity      float64        `json:"similarity"`
	Channels        [3]float64     `json:"channels"`
	Gray            float64        `json:"gray"`
	DX              int            `json:"dx"`
	DY              int            `json:"dy"`
	Runs            []AlignmentRun `json:"runs"`
	ScaleX          float64        `json:"scale_x"`
	ScaleY          float64        `json:"scale_y"`
	ScaleDev        float64        `json:"scale_dev"`
	SizeUnit        string         `json:"size_unit"`
	AspectRef       float64        `json:"aspect_ref"`
	AspectCandidate float64        `json:"aspect_cand"`
	AspectDev       float64        `json:"aspect_dev"`
	InkRefW         float64        `json:"ink_ref_w"`
	InkRefH         float64        `json:"ink_ref_h"`
	InkCandW        float64        `json:"ink_cand_w"`
	InkCandH        float64        `json:"ink_cand_h"`
	ShareRefW       float64        `json:"share_ref_w"`
	ShareRefH       float64        `json:"share_ref_h"`
	ShareCandW      float64        `json:"share_cand_w"`
	ShareCandH      float64        `json:"share_cand_h"`
}

type Thresholds struct {
	MinOverlap        float64 `json:"min_overlap"`
	MinOverlapPercent float64 `json:"min_overlap_percent"`
	MaxScaleDev       float64 `json:"max_scale_dev"`
	OverlapPass       bool    `json:"overlap_pass"`
	SizePass          bool    `json:"size_pass"`
}

type Normalization struct {
	ReferencePixels        [2]int  `json:"reference_pixels"`
	CandidatePixels        [2]int  `json:"candidate_pixels_before"`
	FittedPixels           [2]int  `json:"comparison_canvas_pixels"`
	CandidateScale         float64 `json:"candidate_pixel_scale"`
	Method                 string  `json:"method"`
	PhysicalMeasuredBefore bool    `json:"physical_measured_before_rescale"`
}

type Result struct {
	Text                 string                   `json:"text,omitempty"`
	ReferenceMeasurement *measurement.Measurement `json:"reference_measurement,omitempty"`
	CandidateMeasurement *measurement.Measurement `json:"candidate_measurement,omitempty"`
	Normalization        *Normalization           `json:"normalization,omitempty"`
	SimilarityPercent    float64                  `json:"similarity_percent"`
	Thresholds           *Thresholds              `json:"thresholds,omitempty"`
	Version              string                   `json:"version"`
	OK                   bool                     `json:"ok"`
	Error                string                   `json:"error,omitempty"`
	Pass                 bool                     `json:"pass"`
	Metrics              *Metrics                 `json:"metrics,omitempty"`
	Images               []ImageArtifact          `json:"images,omitempty"`
	SelectedImage        string                   `json:"selected_image,omitempty"`
	OutDir               string                   `json:"out_dir,omitempty"`
	Crop                 []float64                `json:"crop,omitempty"`
	MetricsPath          string                   `json:"metrics_path,omitempty"`
}

// Compare exposes production metrics independent of the CLI.
func Compare(reference, candidate image.Image, referenceMM, candidateMM [2]float64) (Metrics, *image.NRGBA, error) {
	if isNilImage(reference) || isNilImage(candidate) {
		return Metrics{}, nil, fmt.Errorf("reference and candidate images are required")
	}
	if reference.Bounds().Dx() < 1 || reference.Bounds().Dy() < 1 || candidate.Bounds().Dx() < 1 || candidate.Bounds().Dy() < 1 {
		return Metrics{}, nil, fmt.Errorf("images must have non-empty bounds")
	}
	for _, size := range append(referenceMM[:], candidateMM[:]...) {
		if math.IsNaN(size) || math.IsInf(size, 0) {
			return Metrics{}, nil, fmt.Errorf("physical dimensions must be finite")
		}
	}
	r, overlay, err := compareFigureImagesMM(reference, candidate, figureCanvasMM{referenceMM[0], referenceMM[1], candidateMM[0], candidateMM[1]})
	if err != nil {
		return Metrics{}, nil, err
	}
	runs := make([]AlignmentRun, 0, len(r.Runs))
	for _, run := range r.Runs {
		runs = append(runs, AlignmentRun{Name: run.Name, Score: run.Score, DX: run.DX, DY: run.DY})
	}
	return Metrics{Similarity: r.Similarity, Channels: r.Channels, Gray: r.Gray, DX: r.DX, DY: r.DY, Runs: runs, ScaleX: r.ScaleX, ScaleY: r.ScaleY, ScaleDev: r.ScaleDev, SizeUnit: r.SizeUnit, AspectRef: r.AspectRef, AspectCandidate: r.AspectCand, AspectDev: r.AspectDev, InkRefW: r.InkRefW, InkRefH: r.InkRefH, InkCandW: r.InkCandW, InkCandH: r.InkCandH, ShareRefW: r.ShareRefW, ShareRefH: r.ShareRefH, ShareCandW: r.ShareCandW, ShareCandH: r.ShareCandH}, overlay, nil
}

func WorstRegion(reference, candidate image.Image) [4]float64 {
	ref, cand := flattenOnWhite(reference), flattenOnWhite(candidate)
	left, top, right, bottom := worstFigureRegion(ref, cand)
	return [4]float64{left, top, right, bottom}
}

func Evidence(reference, candidate image.Image, dx, dy int, detail string, crop [4]float64, zoom int) (*image.NRGBA, error) {
	if isNilImage(reference) || isNilImage(candidate) {
		return nil, fmt.Errorf("reference and candidate images are required")
	}
	if reference.Bounds().Dx() < 1 || reference.Bounds().Dy() < 1 || candidate.Bounds().Dx() < 1 || candidate.Bounds().Dy() < 1 {
		return nil, fmt.Errorf("images must have non-empty bounds")
	}
	if detail == "" {
		detail = "all"
	}
	switch detail {
	case "all", "gray", "r", "g", "b", "rg", "rb", "gb":
	default:
		return nil, fmt.Errorf("invalid detail")
	}
	if crop == [4]float64{} {
		crop = [4]float64{0, 0, 100, 100}
	}
	for _, value := range crop {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("crop values must be finite")
		}
	}
	if crop[0] < 0 || crop[1] < 0 || crop[2] > 100 || crop[3] > 100 || crop[2] <= crop[0] || crop[3] <= crop[1] {
		return nil, fmt.Errorf("invalid crop percentages")
	}
	if zoom < 0 || zoom > 6000 {
		return nil, fmt.Errorf("zoom must be between 0 and 6000")
	}
	ref, cand := flattenOnWhite(reference), flattenOnWhite(candidate)
	img := figureOverlayDetail(ref, cand, dx, dy, detail)
	img = cropFigureImage(img, crop[0], crop[1], crop[2], crop[3])
	if zoom > 0 && img.Bounds().Dx() != zoom {
		h := max(1, figureRound(float64(img.Bounds().Dy())*float64(zoom)/float64(img.Bounds().Dx())))
		out := image.NewNRGBA(image.Rect(0, 0, zoom, h))
		xdraw.CatmullRom.Scale(out, out.Bounds(), img, img.Bounds(), draw.Src, nil)
		img = out
	}
	return img, nil
}

func isNilImage(img image.Image) bool {
	if img == nil {
		return true
	}
	v := reflect.ValueOf(img)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (r *Result) SetOutDir(dir string, imagePath string) { r.OutDir = dir; r.SelectedImage = imagePath }

// Exported convenience wrappers for consumers that need decoded-image handling.
func Flatten(src image.Image) *image.NRGBA { return flattenOnWhite(src) }
func Fit(ref, candidate *image.NRGBA) (*image.NRGBA, *image.NRGBA) {
	return fitFigureCanvases(ref, candidate)
}
func WritePNG(path string, img image.Image) error { return writePNG(path, img) }

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
