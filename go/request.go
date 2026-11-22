package comparefigure

import (
	"compare-figure/measurement"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ProcessRequest is the single image/PDF comparison entry used by both the
// JSON CLI and the drawing harness. Cropping never changes full-image scores.
func ProcessRequest(ctx context.Context, req Request) (Result, error) {
	fail := func(err error) (Result, error) { return Result{Version: "compare-figure/1", Error: err.Error()}, err }
	if req.Version != "compare-figure/1" {
		return fail(fmt.Errorf("version must be compare-figure/1"))
	}
	if strings.TrimSpace(req.Reference) == "" || strings.TrimSpace(req.Candidate) == "" {
		return fail(fmt.Errorf("reference and candidate are required"))
	}
	for _, v := range []float64{req.PassOverlap, req.PassScale} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fail(fmt.Errorf("thresholds must be finite"))
		}
	}
	ref, err := DecodeImage(req.Reference)
	if err != nil {
		return fail(fmt.Errorf("reference: %w", err))
	}
	referenceMeasurement, err := measurement.Measure(req.Reference, "", req.SizesCSV, req.ReferenceMM[:])
	if err != nil {
		return fail(fmt.Errorf("reference measurement: %w", err))
	}
	if saved := req.ReferenceMeasurement; saved != nil && saved.Found && saved.WidthMM == referenceMeasurement.WidthMM && saved.HeightMM == referenceMeasurement.HeightMM {
		pw, ph := referenceMeasurement.PixelsW, referenceMeasurement.PixelsH
		referenceMeasurement = *saved
		referenceMeasurement.PixelsW, referenceMeasurement.PixelsH = pw, ph
	}
	refMM := [2]float64{referenceMeasurement.WidthMM, referenceMeasurement.HeightMM}
	candidate := req.Candidate
	mm := req.CandidateMM
	if strings.EqualFold(filepath.Ext(candidate), ".pdf") {
		dpi := req.RasterDPI
		if dpi == 0 {
			dpi = 110
		}
		if dpi < 1 || dpi > 1200 {
			return fail(fmt.Errorf("raster_dpi must be 1..1200"))
		}
		dir, err := os.MkdirTemp("", "compare-figure-pdf-")
		if err != nil {
			return fail(err)
		}
		// This exact absolute directory is created here and owned by this request.
		absolute, _ := filepath.Abs(dir)
		if filepath.IsAbs(absolute) && strings.HasPrefix(filepath.Base(absolute), "compare-figure-pdf-") {
			defer os.RemoveAll(absolute)
		}
		candidate = filepath.Join(dir, "candidate.png")
		renderCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		output, e := exec.CommandContext(renderCtx, "pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-png", "-r", strconv.Itoa(dpi), req.Candidate, strings.TrimSuffix(candidate, ".png")).CombinedOutput()
		cancel()
		if e != nil {
			return fail(fmt.Errorf("pdftoppm: %w: %s", e, strings.TrimSpace(string(output))))
		}
		if mm == [2]float64{} {
			mm, _ = PDFCanvasMM(ctx, req.Candidate)
		}
	}
	cand, err := DecodeImage(candidate)
	if err != nil {
		return fail(fmt.Errorf("candidate: %w", err))
	}
	candidateMeasurement, err := measurement.Measure(candidate, "", "", mm[:])
	if err != nil {
		return fail(fmt.Errorf("candidate measurement: %w", err))
	}
	if strings.EqualFold(filepath.Ext(req.Candidate), ".pdf") && candidateMeasurement.Found {
		candidateMeasurement.Source = "pdf_page"
		if req.CandidateMM != [2]float64{} {
			candidateMeasurement.Source = "explicit_mm"
		}
		candidateMeasurement.MetadataPath = req.Candidate
	}
	mm = [2]float64{candidateMeasurement.WidthMM, candidateMeasurement.HeightMM}
	metrics, _, err := Compare(ref, cand, refMM, mm)
	if err != nil {
		return fail(err)
	}
	crop := req.Crop
	if crop == [4]float64{} {
		crop = [4]float64{0, 0, 100, 100}
		if req.WorstRegion {
			crop = WorstRegion(ref, cand)
		}
	}
	evidence, err := Evidence(ref, cand, metrics.DX, metrics.DY, req.Detail, crop, req.Zoom)
	if err != nil {
		return fail(err)
	}
	out := req.OutDir
	if out == "" {
		out, err = DefaultOutput("compare")
		if err != nil {
			return fail(err)
		}
	}
	err = os.MkdirAll(out, 0755)
	if err != nil {
		return fail(err)
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return fail(err)
	}
	path := filepath.Join(out, "comparison.png")
	if err = WritePNG(path, evidence); err != nil {
		return fail(err)
	}
	artifact, err := ImageArtifactFor(path, req.InlineImages)
	if err != nil {
		return fail(err)
	}
	metricsPath := filepath.Join(out, "metrics.json")
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err = AtomicWrite(metricsPath, append(data, '\n'), 0644); err != nil {
		return fail(err)
	}
	overlap, scale := req.PassOverlap, req.PassScale
	if overlap == 0 {
		overlap = .17
	}
	if scale == 0 {
		scale = .1
	}
	overlapPass, sizePass := (overlap < 0 || metrics.Similarity >= overlap), (scale < 0 || metrics.ScaleDev <= scale)
	result := Result{Version: "compare-figure/1", OK: true, Pass: overlapPass && sizePass, SimilarityPercent: metrics.Similarity * 100, Thresholds: &Thresholds{MinOverlap: overlap, MinOverlapPercent: overlap * 100, MaxScaleDev: scale, OverlapPass: overlapPass, SizePass: sizePass}, Metrics: &metrics, Images: []ImageArtifact{artifact}, SelectedImage: path, OutDir: out, Crop: crop[:], MetricsPath: metricsPath}
	rw, rh := ref.Bounds().Dx(), ref.Bounds().Dy()
	cw, ch := cand.Bounds().Dx(), cand.Bounds().Dy()
	fittedHeight := ch
	if cw != rw {
		fittedHeight = max(1, figureRound(float64(ch)*float64(rw)/float64(cw)))
	}
	result.ReferenceMeasurement = &referenceMeasurement
	result.CandidateMeasurement = &candidateMeasurement
	result.Normalization = &Normalization{ReferencePixels: [2]int{rw, rh}, CandidatePixels: [2]int{cw, ch}, FittedPixels: [2]int{rw, max(rh, fittedHeight)}, CandidateScale: float64(rw) / float64(cw), Method: "uniform-width-rescale-and-white-padding", PhysicalMeasuredBefore: metrics.SizeUnit == "mm"}
	result.Text = ComparisonText(result, req.Detail)
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err = AtomicWrite(filepath.Join(out, "result.json"), append(payload, '\n'), 0644); err != nil {
		return fail(err)
	}
	return result, nil
}

func DecodeImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	return im, err
}

// ImageArtifactFor creates the same image-aware contract for all tools.
func ImageArtifactFor(path string, inline bool) (ImageArtifact, error) {
	f, err := os.Open(path)
	if err != nil {
		return ImageArtifact{}, err
	}
	config, format, err := image.DecodeConfig(f)
	f.Close()
	if err != nil {
		return ImageArtifact{}, err
	}
	mime := "image/" + format
	if format == "jpg" {
		mime = "image/jpeg"
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ImageArtifact{}, err
	}
	result := ImageArtifact{Path: absolute, MIME: mime, Width: config.Width, Height: config.Height}
	if inline {
		data, e := os.ReadFile(path)
		if e != nil {
			return result, e
		}
		result.Base64 = base64.StdEncoding.EncodeToString(data)
	}
	return result, nil
}

var canvasRE = regexp.MustCompile(`(?m)^Page(?:\s+1)?\s+size:\s*([0-9.]+)\s+x\s+([0-9.]+)\s+pts`)

func PDFCanvasMM(ctx context.Context, path string) ([2]float64, error) {
	sub, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(sub, "pdfinfo", "-f", "1", "-l", "1", "-box", path)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return [2]float64{}, fmt.Errorf("pdfinfo: %w: %s", err, string(data))
	}
	matches := canvasRE.FindStringSubmatch(string(data))
	if len(matches) != 3 {
		return [2]float64{}, fmt.Errorf("PDF page size unavailable")
	}
	w, _ := strconv.ParseFloat(matches[1], 64)
	h, _ := strconv.ParseFloat(matches[2], 64)
	return [2]float64{w * 25.4 / 72, h * 25.4 / 72}, nil
}

// AtomicWrite preserves unchanged files and uses same-directory replacement.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".compare-write-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if filepath.Dir(temp) == filepath.Dir(path) {
			_ = os.Remove(temp)
		}
	}()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Chmod(temp, mode); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
