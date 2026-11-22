package drawing

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	cf "compare-figure"
	xdraw "golang.org/x/image/draw"
)

func renderPDF(ctx context.Context, rasterCommand, pdf string, page, width int, output string) (image.Image, error) {
	tmp, err := os.MkdirTemp("", "drawing-pdf-render-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	prefix := filepath.Join(tmp, "page")
	cmdctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdctx, rasterCommand, "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-scale-to-x", strconv.Itoa(width), "-scale-to-y", "-1", "-singlefile", "-png", pdf, prefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(output)))
	}
	img, err := cf.DecodeImage(prefix + ".png")
	if err != nil {
		return nil, err
	}
	if output != "" {
		f, e := os.Create(output)
		if e != nil {
			return nil, e
		}
		e = png.Encode(f, img)
		ce := f.Close()
		if e != nil {
			return nil, e
		}
		if ce != nil {
			return nil, ce
		}
	}
	return img, nil
}

func cropPercent(src image.Image, left, top, right, bottom float64) image.Image {
	b := src.Bounds()
	x0 := b.Min.X + int(math.Round(float64(b.Dx())*left/100))
	y0 := b.Min.Y + int(math.Round(float64(b.Dy())*top/100))
	x1 := b.Min.X + int(math.Round(float64(b.Dx())*right/100))
	y1 := b.Min.Y + int(math.Round(float64(b.Dy())*bottom/100))
	x0 = max(b.Min.X, min(x0, b.Max.X-1))
	y0 = max(b.Min.Y, min(y0, b.Max.Y-1))
	x1 = max(x0+1, min(x1, b.Max.X))
	y1 = max(y0+1, min(y1, b.Max.Y))
	out := image.NewNRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	draw.Draw(out, out.Bounds(), src, image.Pt(x0, y0), draw.Src)
	return out
}
func resizeWidth(src image.Image, width int) image.Image {
	b := src.Bounds()
	if b.Dx() == width {
		return src
	}
	height := max(1, int(math.Round(float64(b.Dy())*float64(width)/float64(b.Dx()))))
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(out, out.Bounds(), src, b, draw.Src, nil)
	return out
}
func cropArgs(a map[string]any) (float64, float64, float64, float64, error) {
	l, t, r, b := number(a["left"], 0), number(a["top"], 0), number(a["right"], 100), number(a["bottom"], 100)
	if l < 0 || t < 0 || r > 100 || b > 100 || r <= l || b <= t {
		return 0, 0, 0, 0, fmt.Errorf("crop must satisfy 0 <= left < right <= 100 and 0 <= top < bottom <= 100")
	}
	return l, t, r, b, nil
}
func (h *Harness) viewPDF(ctx context.Context, a map[string]any) (string, string, error) {
	p, e := h.safe(stringValue(a["path"], ""))
	if e != nil {
		return "", "", e
	}
	if strings.ToLower(filepath.Ext(p)) != ".pdf" {
		return "REJECTED: view_pdf 只接受 .pdf（图片请用 view_image）", "", nil
	}
	page := max(1, int(number(a["page"], 1)))
	h.pdfViews[p+"#"+strconv.Itoa(page)]++
	zoom := min(6000, max(1, int(number(a["zoom"], number(a["zoom_width"], 1280)))))
	l, t, r, b, e := clampCropArgs(a)
	if e != nil {
		return "", "", e
	}
	pages, pageW, pageH, e := pdfPageInfo(ctx, p, page)
	if e != nil {
		return "", "", e
	}
	if pages > 0 && page > pages {
		return fmt.Sprintf("OUT OF RANGE: %s 共 %d 页，请求第 %d 页", filepath.Base(p), pages, page), "", nil
	}
	frac := (r - l) / 100
	if frac < .02 {
		frac = 1
	}
	renderW := min(6000, max(400, int(float64(zoom)/frac)+8))
	img, e := renderPDF(ctx, stringValue(h.Options["raster_command"], "pdftoppm"), p, page, renderW, "")
	if e != nil {
		return "", "", e
	}
	img = cropPercent(img, l, t, r, b)
	img = resizeWidth(img, zoom)
	out := filepath.Join(h.root, fmt.Sprintf("view_pdf_%d.jpg", page))
	f, e := os.Create(out)
	if e != nil {
		return "", "", e
	}
	e = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	ce := f.Close()
	if e != nil {
		return "", "", e
	}
	if ce != nil {
		return "", "", ce
	}
	note := fmt.Sprintf("PDF page %s p%d (crop %.0f%%,%.0f%%-%.0f%%,%.0f%%, width %dpx", filepath.Base(p), page, l, t, r, b, zoom)
	if pageW > 0 && pageH > 0 {
		note += fmt.Sprintf(", page %.1fmm x %.1fmm", pageW, pageH)
		if l > 0 || t > 0 || r < 100 || b < 100 {
			note += fmt.Sprintf(", this crop %.1fmm x %.1fmm", pageW*(r-l)/100, pageH*(b-t)/100)
		}
	}
	if pages > 0 {
		note += fmt.Sprintf(" (page %d/%d)", page, pages)
	}
	key := p + "#" + strconv.Itoa(page)
	note += h.budgetNote("view_pdf", key, h.pdfViews[key], int(number(h.Options["view_pdf_max"], 0)))
	return note + ") attached.", out, nil
}
func (h *Harness) imagePath(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("只允许相对路径: %s", rel)
	}
	norm := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(rel, "./"), "images/"))
	cur := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(h.currentImage, "./"), "images/"))
	subject := filepath.Dir(cur)
	candidates := []string{}
	if subject != "." && !strings.HasPrefix(norm, subject+"/") {
		candidates = append(candidates, filepath.Join(h.imagesDir, filepath.FromSlash(subject), filepath.FromSlash(norm)))
	}
	candidates = append(candidates, filepath.Join(h.imagesDir, filepath.FromSlash(norm)))
	for _, p := range candidates {
		real, e := filepath.EvalSymlinks(p)
		if e == nil && within(h.imagesDir, real) {
			if st, e := os.Stat(real); e == nil && !st.IsDir() {
				return real, nil
			}
		}
	}
	if !strings.Contains(norm, "/") {
		matches := []string{}
		root, err := filepath.EvalSymlinks(h.imagesDir)
		if err == nil {
			_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if entry.IsDir() && entry.Type()&os.ModeSymlink != 0 {
					return filepath.SkipDir
				}
				if !entry.IsDir() && entry.Name() == filepath.Base(norm) {
					resolved, e := filepath.EvalSymlinks(path)
					if e == nil && within(root, resolved) {
						matches = append(matches, resolved)
					}
				}
				return nil
			})
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("文件名不唯一，请给完整路径: %s", strings.Join(matches, ", "))
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("文件不存在: %s（%s）", rel, h.imagesDir)
}
func within(root, path string) bool {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	r, e := filepath.Rel(root, path)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
func (h *Harness) viewImage(a map[string]any) (string, string, error) {
	rel := stringValue(a["path"], "")
	p, e := h.imagePath(rel)
	if e != nil {
		return "", "", e
	}
	src, e := cf.DecodeImage(p)
	if e != nil {
		return "", "", e
	}
	bounds := src.Bounds()
	white := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(white, white.Bounds(), image.NewUniform(image.White), image.Point{}, draw.Src)
	draw.Draw(white, white.Bounds(), src, bounds.Min, draw.Over)
	l, t, r, b, e := cropArgs(a)
	if e != nil {
		return "", "", e
	}
	img := cropPercent(white, l, t, r, b)
	zoom := min(6000, max(1, int(number(a["zoom"], 1280))))
	img = resizeWidth(img, zoom)
	out := filepath.Join(h.root, "view_image.jpg")
	f, e := os.Create(out)
	if e != nil {
		return "", "", e
	}
	e = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	ce := f.Close()
	if e != nil {
		return "", "", e
	}
	if ce != nil {
		return "", "", ce
	}
	measurement, _ := MeasureImage(p, "", nil)
	note := measurement.Hint()
	if l != 0 || t != 0 || r != 100 || b != 100 {
		if crop := measurement.Crop(l, t, r, b).CropHint(); crop != "" {
			note += " " + crop + "."
		}
	}
	h.views[p]++
	note += h.budgetNote("view_image", p, h.views[p], int(number(h.Options["view_image_max"], 0)))
	return "Image " + rel + " (crop attached). " + note, out, nil
}
func (h *Harness) budgetNote(kind, key string, used, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	warn := max(1, int(math.Ceil(float64(maximum)*number(h.Options["warn_ratio"], .7))))
	label := kind + " on " + filepath.Base(strings.Split(key, "#")[0])
	if used > maximum {
		return " VIEW BUDGET SPENT (" + label + ": " + strconv.Itoa(used) + "/" + strconv.Itoa(maximum) + ")."
	}
	if used >= warn && !h.warned[key] {
		h.warned[key] = true
		return " VIEW BUDGET: " + label + " used " + strconv.Itoa(used) + "/" + strconv.Itoa(maximum) + ", " + strconv.Itoa(maximum-used) + " left."
	}
	return ""
}

func (h *Harness) imageContext(a map[string]any) (string, error) {
	content := stringValue(h.Options["md_content"], "")
	lines := strings.Split(content, "\n")
	refs := []imageRef{}
	for i, line := range lines {
		for _, m := range imageRefRE.FindAllStringSubmatch(line, -1) {
			refs = append(refs, imageRef{m[1], i})
		}
	}
	target := stringValue(a["image"], h.currentImage)
	idx := -1
	for i, r := range refs {
		if r.path == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		norm := strings.TrimPrefix(strings.TrimPrefix(filepath.ToSlash(target), "./"), "images/")
		hits := []int{}
		for i, r := range refs {
			p := strings.TrimPrefix(r.path, "images/")
			if (strings.Contains(norm, "/") && (p == norm || strings.HasSuffix(p, "/"+norm))) || (!strings.Contains(norm, "/") && filepath.Base(r.path) == norm) {
				hits = append(hits, i)
			}
		}
		if len(hits) == 1 {
			idx = hits[0]
		}
	}
	if idx < 0 {
		return "markdown 中找不到图片 " + target, nil
	}
	up, down := int(number(a["up"], 10)), int(number(a["down"], 10))
	if up <= 0 {
		up = 10
	}
	if down <= 0 {
		down = 10
	}
	if n := int(number(h.Options["max_up"], 0)); n > 0 {
		up = min(up, n)
	}
	if n := int(number(h.Options["max_down"], 0)); n > 0 {
		down = min(down, n)
	}
	block := func(i int) string {
		if i < 0 || i >= len(refs) {
			return "(none)"
		}
		ref := refs[i]
		delta := ref.line - refs[idx].line
		suffix := ""
		if delta != 0 {
			suffix = fmt.Sprintf(", %+d lines from this image", delta)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s (line %d%s)\n", ref.path, ref.line+1, suffix)
		for n := max(0, ref.line-up); n < min(len(lines), ref.line+down+1); n++ {
			prefix := "  "
			if n == ref.line {
				prefix = "> "
			}
			fmt.Fprintf(&b, "%s%4d | %s\n", prefix, n+1, lines[n])
		}
		return strings.TrimSuffix(b.String(), "\n")
	}
	return fmt.Sprintf("image %d of %d in this document: %s\n\n## PREVIOUS image ref:\n%s\n\n## THIS image context (up %d / down %d lines; request again with bigger up/down to expand):\n%s\n\n## NEXT image ref:\n%s", idx+1, len(refs), refs[idx].path, block(idx-1), up, down, block(idx), block(idx+1)), nil
}

type imageRef struct {
	path string
	line int
}

var imageRefRE = regexp.MustCompile(`(?i)(?:!\[.*?\]\(|<img[^>]*?src=["'])(images/.+?\.(?:jpg|jpeg|png|gif|webp))(?:\)|["'][^>]*>)`)

var _ image.Image
