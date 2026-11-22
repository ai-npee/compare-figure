package luadraw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const docSchema = 3

type bbox [4]float64
type word struct {
	Text string  `json:"text"`
	X1   float64 `json:"x1"`
	Y1   float64 `json:"y1"`
	X2   float64 `json:"x2"`
	Y2   float64 `json:"y2"`
}
type block struct {
	Text string `json:"text"`
	BBox bbox   `json:"bbox"`
}
type page struct {
	Number int     `json:"page"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Blocks []block `json:"blocks"`
}
type docEntry struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Pages []page `json:"pages,omitempty"`
}
type fileMeta struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime_ns"`
}
type docIndex struct {
	Schema      int        `json:"schema"`
	Root        string     `json:"root"`
	Version     string     `json:"version"`
	Created     int64      `json:"created"`
	Key         string     `json:"key"`
	Fingerprint []fileMeta `json:"fingerprint"`
	Docs        []docEntry `json:"docs"`
}
type Docs struct {
	root, cache, path string
	mu                sync.Mutex
	index             *docIndex
}

// NewDocs creates a standalone Go documentation indexer. Poppler tools are required.
func NewDocs(root, cache string) (*Docs, error) {
	if root == "" {
		return nil, fmt.Errorf("LuaDraw documentation root is required")
	}
	root = abs(root)
	st, e := os.Stat(root)
	if e != nil || !st.IsDir() {
		return nil, fmt.Errorf("LuaDraw root not found: %s", root)
	}
	for _, n := range []string{"pdftotext", "pdfinfo", "pdftoppm"} {
		if lookup(n) == "" {
			return nil, fmt.Errorf("required Poppler executable not found: %s", n)
		}
	}
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "luadraw-doc-index")
	}
	cache = abs(cache)
	if e = os.MkdirAll(cache, 0700); e != nil {
		return nil, e
	}
	return &Docs{root: root, cache: cache, path: filepath.Join(cache, "luadraw-doc-index-v3.json")}, nil
}

// Definition returns the OpenAI-compatible function schema for luadraw_doc.
func (d *Docs) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{"name": "luadraw_doc", "description": "Search/read/view LuaDraw documentation. Search hits report PDF bbox in points; crop inputs are page percentages.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"type": "string", "enum": []string{"search", "read", "view"}}, "query": map[string]any{"type": "string"}, "doc": map[string]any{"type": "string"}, "page": map[string]any{"type": "integer"}, "left": map[string]any{"type": "number"}, "top": map[string]any{"type": "number"}, "right": map[string]any{"type": "number"}, "bottom": map[string]any{"type": "number"}, "zoom": map[string]any{"type": "integer"}, "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "start_line": map[string]any{"type": "integer"}, "end_line": map[string]any{"type": "integer"}, "include_image": map[string]any{"type": "boolean", "description": "Attach one relevant PDF crop when available"}}, "required": []string{}}}}
}
func (d *Docs) sourceFiles() []string {
	var out []string
	for _, spec := range []struct {
		dir string
		ext []string
	}{{"doc/src", []string{".tex"}}, {"examples", []string{".tex", ".lua"}}, {"files", []string{".lua"}}} {
		base := filepath.Join(d.root, spec.dir)
		_ = filepath.WalkDir(base, func(p string, entry os.DirEntry, e error) error {
			if e != nil || entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if !entry.IsDir() {
				for _, x := range spec.ext {
					if strings.EqualFold(filepath.Ext(p), x) {
						out = append(out, p)
						break
					}
				}
			}
			return nil
		})
	}
	doc := filepath.Join(d.root, "doc")
	entries, _ := os.ReadDir(doc)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 && !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			out = append(out, filepath.Join(doc, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}
func (d *Docs) fingerprint() []fileMeta {
	fs := d.sourceFiles()
	m := make([]fileMeta, 0, len(fs))
	for _, p := range fs {
		s, e := os.Stat(p)
		if e == nil {
			rel, _ := filepath.Rel(d.root, p)
			m = append(m, fileMeta{filepath.ToSlash(rel), s.Size(), s.ModTime().UnixNano()})
		}
	}
	return m
}
func versionDocs(root string) string {
	for _, p := range []string{"doc/src/luadraw-doc-en.tex", "doc/src/luadraw-doc-fr.tex"} {
		b, e := os.ReadFile(filepath.Join(root, p))
		if e == nil {
			m := regexp.MustCompile(`\\def\\version\{([^}]+)\}`).FindSubmatch(b)
			if len(m) > 1 {
				return string(m[1])
			}
		}
	}
	return drawVersion(root)
}
func (d *Docs) makeIndex(ctx context.Context, fp []fileMeta) (*docIndex, error) {
	h := sha256.New()
	idx := &docIndex{Schema: docSchema, Root: d.root, Version: versionDocs(d.root), Created: time.Now().Unix(), Fingerprint: fp}
	for _, m := range fp {
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00", m.Path, m.Size, m.Mtime)
		p := filepath.Join(d.root, filepath.FromSlash(m.Path))
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		_, _ = h.Write(b)
		entry := docEntry{Path: m.Path}
		if strings.EqualFold(filepath.Ext(p), ".pdf") {
			entry.Kind = "pdf"
			pages, e := extractPDF(ctx, p)
			if e != nil {
				return nil, fmt.Errorf("index PDF %s: %w", m.Path, e)
			}
			entry.Pages = pages
		} else {
			entry.Kind = "source"
			entry.Text = strings.ToValidUTF8(string(b), "�")
		}
		idx.Docs = append(idx.Docs, entry)
	}
	idx.Key = hex.EncodeToString(h.Sum(nil))
	return idx, nil
}
func (d *Docs) get(ctx context.Context) (*docIndex, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fp := d.fingerprint()
	if d.index != nil && sameMeta(d.index.Fingerprint, fp) {
		return d.index, nil
	}
	if b, e := os.ReadFile(d.path); e == nil {
		var c docIndex
		if json.Unmarshal(b, &c) == nil && c.Schema == docSchema && c.Root == d.root && sameMeta(c.Fingerprint, fp) {
			d.index = &c
			return &c, nil
		}
	}
	idx, e := d.makeIndex(ctx, fp)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(idx)
	if e != nil {
		return nil, e
	}
	tmp, e := os.CreateTemp(d.cache, "luadraw-index-*.tmp")
	if e == nil {
		_, we := tmp.Write(b)
		ce := tmp.Close()
		if we == nil && ce == nil {
			e = os.Rename(tmp.Name(), d.path)
		} else {
			e = we
			if e == nil {
				e = ce
			}
		}
		if e != nil {
			_ = os.Remove(tmp.Name())
		}
	}
	if e != nil {
		return nil, e
	}
	d.index = idx
	return idx, nil
}
func sameMeta(a, b []fileMeta) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Poppler bbox XML uses word coordinates. Group adjacent words into visual lines and preserve page dimensions.
type bboxXML struct {
	Pages []bboxPage `xml:"page"`
}
type bboxPage struct {
	Width  float64    `xml:"width,attr"`
	Height float64    `xml:"height,attr"`
	Words  []bboxWord `xml:"word"`
}
type bboxWord struct {
	X1   float64 `xml:"xMin,attr"`
	Y1   float64 `xml:"yMin,attr"`
	X2   float64 `xml:"xMax,attr"`
	Y2   float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

func extractPDF(ctx context.Context, p string) ([]page, error) {
	cmd := exec.CommandContext(ctx, lookup("pdftotext"), "-bbox", p, "-")
	b, e := cmd.Output()
	if e != nil {
		if x, ok := e.(*exec.ExitError); ok {
			return nil, fmt.Errorf("pdftotext: %s: %w", tail(string(x.Stderr), 3000), e)
		}
		return nil, e
	}
	decoder := xml.NewDecoder(strings.NewReader(string(b)))
	var xmlPages []bboxPage
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "page" {
			var pg bboxPage
			if err := decoder.DecodeElement(&pg, &start); err != nil {
				return nil, err
			}
			xmlPages = append(xmlPages, pg)
		}
	}
	out := make([]page, 0, len(xmlPages))
	for pi, pg := range xmlPages {
		lines := [][]bboxWord{}
		centers := []float64{}
		for _, w := range pg.Words {
			y := (w.Y1 + w.Y2) / 2
			at := -1
			for i, c := range centers {
				if absFloat(c-y) <= 3 {
					at = i
					break
				}
			}
			if at < 0 {
				lines = append(lines, []bboxWord{w})
				centers = append(centers, y)
			} else {
				lines[at] = append(lines[at], w)
				centers[at] = (centers[at] + y) / 2
			}
		}
		blocks := []block{}
		for _, line := range lines {
			sort.SliceStable(line, func(i, j int) bool { return line[i].X1 < line[j].X1 })
			var text strings.Builder
			bb := bbox{line[0].X1, line[0].Y1, line[0].X2, line[0].Y2}
			for i, w := range line {
				if i > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(strings.TrimSpace(w.Text))
				if w.X1 < bb[0] {
					bb[0] = w.X1
				}
				if w.Y1 < bb[1] {
					bb[1] = w.Y1
				}
				if w.X2 > bb[2] {
					bb[2] = w.X2
				}
				if w.Y2 > bb[3] {
					bb[3] = w.Y2
				}
			}
			if strings.TrimSpace(text.String()) != "" {
				blocks = append(blocks, block{text.String(), bb})
			}
		}
		out = append(out, page{pi + 1, pg.Width, pg.Height, blocks})
	}
	return out, nil
}
func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
func num(v any, def float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return def
}
func clamp(v, def float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	if v == 0 && def != 0 {
		return def
	}
	return v
}
func (d *Docs) safeDoc(idx *docIndex, name, kind string) (*docEntry, error) {
	p := abs(filepath.Join(d.root, filepath.FromSlash(name)))
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = abs(real)
	}
	root := d.root
	if real, err := filepath.EvalSymlinks(d.root); err == nil {
		root = abs(real)
	}
	rel, e := filepath.Rel(root, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("document path must stay inside LuaDraw root")
	}
	for i := range idx.Docs {
		if idx.Docs[i].Path == filepath.ToSlash(rel) && (kind == "" || idx.Docs[i].Kind == kind) {
			return &idx.Docs[i], nil
		}
	}
	return nil, fmt.Errorf("document not indexed under root: %s", name)
}
func (d *Docs) render(ctx context.Context, pdf string, pg page, args map[string]any, out string, bb *bbox) (string, error) {
	left, top, right, bottom := clamp(num(args["left"], 0), 0), clamp(num(args["top"], 0), 0), clamp(num(args["right"], 100), 100), clamp(num(args["bottom"], 100), 100)
	hasCrop := false
	for _, k := range []string{"left", "top", "right", "bottom"} {
		if _, ok := args[k]; ok {
			hasCrop = true
		}
	}
	if bb != nil && !hasCrop {
		w := pg.Width
		h := pg.Height
		cw := minF(w, maxF(bb[2]-bb[0]+w*.08, w*.82))
		ch := minF(h, maxF(bb[3]-bb[1]+h*.04, h*.42))
		cx := (bb[0] + bb[2]) / 2
		cy := (bb[1] + bb[3]) / 2
		left = maxF(0, (cx-cw/2)/w*100)
		right = minF(100, (cx+cw/2)/w*100)
		top = maxF(0, (cy-ch/2)/h*100)
		bottom = minF(100, (cy+ch/2)/h*100)
	}
	if right <= left || bottom <= top {
		return "", fmt.Errorf("crop bounds must have positive area")
	}
	zoom := int(num(args["zoom"], 1200))
	if zoom == 0 {
		zoom = 1200
	}
	if zoom < 200 {
		zoom = 200
	}
	if zoom > 5000 {
		zoom = 5000
	}
	_ = os.MkdirAll(out, 0700)
	whole := int(float64(zoom)/((right-left)/100) + 2)
	if whole < 400 {
		whole = 400
	}
	if whole > 12000 {
		whole = 12000
	}
	base, e := os.CreateTemp(out, "luadraw-page-*.png")
	if e != nil {
		return "", e
	}
	prefix := strings.TrimSuffix(base.Name(), filepath.Ext(base.Name()))
	base.Close()
	_ = os.Remove(base.Name())
	cmd := exec.CommandContext(ctx, lookup("pdftoppm"), "-f", strconv.Itoa(pg.Number), "-l", strconv.Itoa(pg.Number), "-scale-to-x", strconv.Itoa(whole), "-scale-to-y", "-1", "-png", pdf, prefix)
	if b, e := cmd.CombinedOutput(); e != nil {
		return "", fmt.Errorf("pdftoppm: %s: %w", tail(string(b), 3000), e)
	}
	rendered := prefix + "-" + strconv.Itoa(pg.Number) + ".png"
	if !exists(rendered) {
		matches, _ := filepath.Glob(prefix + "-*.png")
		if len(matches) > 0 {
			rendered = matches[0]
		}
	}
	defer os.Remove(rendered)
	f, e := os.Open(rendered)
	if e != nil {
		return "", e
	}
	im, e := png.Decode(f)
	f.Close()
	if e != nil {
		return "", e
	}
	bounds := im.Bounds()
	x1 := int(float64(bounds.Dx()) * left / 100)
	x2 := int(float64(bounds.Dx()) * right / 100)
	y1 := int(float64(bounds.Dy()) * top / 100)
	y2 := int(float64(bounds.Dy()) * bottom / 100)
	rect := image.Rect(x1, y1, x2, y2).Intersect(bounds)
	if rect.Empty() {
		return "", fmt.Errorf("crop is empty")
	}
	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), im, rect.Min, draw.Src)
	height := maxInt(1, int(float64(cropped.Bounds().Dy())*float64(zoom)/float64(cropped.Bounds().Dx())))
	dst := image.NewRGBA(image.Rect(0, 0, zoom, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)
	target := filepath.Join(out, fmt.Sprintf("luadraw-p%d-%d.png", pg.Number, time.Now().UnixNano()))
	f, e = os.Create(target)
	if e != nil {
		return "", e
	}
	e = png.Encode(f, dst)
	ce := f.Close()
	if e != nil {
		return "", e
	}
	if ce != nil {
		return "", ce
	}
	return target, nil
}
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func terms(s string) []string {
	re := regexp.MustCompile(`[^\W\d_]+|[\w]+`)
	m := re.FindAllString(strings.ToLower(s), -1)
	var r []string
	for _, x := range m {
		if len([]rune(x)) > 1 {
			r = append(r, x)
		}
	}
	if len(r) == 0 && s != "" {
		r = []string{strings.ToLower(s)}
	}
	return r
}

type hit struct {
	score      int
	doc        string
	page, line int
	kind, text string
	box        *bbox
	info       page
}

func (d *Docs) search(idx *docIndex, q string, limit, offset int) []hit {
	ts := terms(q)
	exact := strings.ToLower(strings.TrimSpace(q))
	var hits []hit
	for _, doc := range idx.Docs {
		if doc.Kind == "source" {
			for n, line := range strings.Split(doc.Text, "\n") {
				score := 0
				low := strings.ToLower(line)
				for _, t := range ts {
					score += strings.Count(low, t)
				}
				if exact != "" && strings.Contains(low, exact) {
					score += 1000
				}
				if score > 0 {
					hits = append(hits, hit{score: score, doc: doc.Path, kind: "source", line: n + 1, text: truncate(strings.TrimSpace(line), 400)})
				}
			}
		} else {
			for _, pg := range doc.Pages {
				for _, b := range pg.Blocks {
					score := 0
					low := strings.ToLower(b.Text)
					for _, t := range ts {
						score += strings.Count(low, t)
					}
					if exact != "" && strings.Contains(low, exact) {
						score = 10000
						if strings.Contains(low, exact+"(") && !strings.Contains(low, exact+"()") {
							score += 1000
						}
					}
					if score > 0 {
						rank := score
						if pg.Number > 10 {
							rank += 2
						} else {
							rank -= 3
						}
						if strings.Contains(b.Text, "\n") {
							rank += 2
						}
						bb := b.BBox
						hits = append(hits, hit{score: rank, doc: doc.Path, kind: "pdf", page: pg.Number, text: truncate(b.Text, 600), box: &bb, info: pg})
					}
				}
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.doc != b.doc {
			return a.doc < b.doc
		}
		if a.page != b.page {
			return a.page < b.page
		}
		return a.line < b.line
	})
	if offset >= len(hits) {
		return nil
	}
	end := offset + limit
	if end > len(hits) {
		end = len(hits)
	}
	return hits[offset:end]
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func intArg(m map[string]any, k string, d int) int { return int(num(m[k], float64(d))) }

// Execute performs search, source read, or page view. Query search attaches a PDF crop by default.
func (d *Docs) Execute(ctx context.Context, args map[string]any, outdir string) (string, string, error) {
	if args == nil {
		args = map[string]any{}
	}
	idx, e := d.get(ctx)
	if e != nil {
		return "", "", e
	}
	mode := strOpt(args, "mode", "search")
	maxMtime := int64(0)
	for _, m := range idx.Fingerprint {
		if m.Mtime > maxMtime {
			maxMtime = m.Mtime
		}
	}
	meta := fmt.Sprintf("LuaDraw %s | index_sha256 %s | schema %d | source_mtime_ns %d | indexed %s", idx.Version, idx.Key, idx.Schema, maxMtime, time.Unix(idx.Created, 0).Format("2006-01-02 15:04:05"))
	switch mode {
	case "search":
		q := strings.TrimSpace(fmt.Sprint(args["query"]))
		if q == "" || q == "<nil>" {
			return "", "", fmt.Errorf("search requires query")
		}
		limit := intArg(args, "limit", 5)
		if limit < 1 {
			limit = 1
		}
		if limit > 20 {
			limit = 20
		}
		off := intArg(args, "offset", 0)
		if off < 0 {
			off = 0
		}
		hits := d.search(idx, q, limit, off)
		var rows []string
		imagePath := ""
		for i, h := range hits {
			suffix := fmt.Sprintf(" line %d", h.line)
			if h.kind == "pdf" {
				suffix = fmt.Sprintf(" p.%d bbox=%.1f,%.1f,%.1f,%.1f", h.page, h.box[0], h.box[1], h.box[2], h.box[3])
			}
			rows = append(rows, fmt.Sprintf("%d. %s%s\n   %s", i+1, h.doc, suffix, h.text))
			include, ok := args["include_image"].(bool)
			if !ok {
				include = true
			}
			if imagePath == "" && h.kind == "pdf" && include {
				imagePath, e = d.render(ctx, filepath.Join(d.root, filepath.FromSlash(h.doc)), h.info, args, outdir, h.box)
				if e != nil {
					return "", "", e
				}
			}
		}
		body := "No matches."
		if len(rows) > 0 {
			body = strings.Join(rows, "\n")
		}
		return meta + "\n" + body, imagePath, nil
	case "read":
		name := strOpt(args, "doc", strOpt(args, "query", ""))
		doc, e := d.safeDoc(idx, name, "source")
		if e != nil {
			return "", "", e
		}
		lines := strings.Split(doc.Text, "\n")
		start := intArg(args, "start_line", 1)
		if start < 1 {
			start = 1
		}
		end := intArg(args, "end_line", start+79)
		if end > len(lines) {
			end = len(lines)
		}
		if end > start+399 {
			end = start + 399
		}
		if start > len(lines) {
			return fmt.Sprintf("%s\n%s lines %d-%d/%d", meta, doc.Path, start, start, len(lines)), "", nil
		}
		var b strings.Builder
		for n := start; n <= end; n++ {
			fmt.Fprintf(&b, "%d: %s\n", n, lines[n-1])
		}
		return fmt.Sprintf("%s\n%s lines %d-%d/%d\n%s", meta, doc.Path, start, end, len(lines), strings.TrimSuffix(b.String(), "\n")), "", nil
	case "view":
		name := strOpt(args, "doc", "")
		doc, e := d.safeDoc(idx, name, "pdf")
		if e != nil {
			return "", "", e
		}
		n := intArg(args, "page", 1)
		if n < 1 || n > len(doc.Pages) {
			return "", "", fmt.Errorf("page out of range: %d; document has %d pages", n, len(doc.Pages))
		}
		im, e := d.render(ctx, filepath.Join(d.root, filepath.FromSlash(doc.Path)), doc.Pages[n-1], args, outdir, nil)
		if e != nil {
			return "", "", e
		}
		return fmt.Sprintf("%s\nPDF %s page %d/%d attached.", meta, doc.Path, n, len(doc.Pages)), im, nil
	default:
		return "", "", fmt.Errorf("mode must be search, read, or view")
	}
}

var _ = strconv.IntSize
