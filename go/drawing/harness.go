package drawing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	cf "compare-figure"
	"compare-figure/luadraw"
)

// Harness is an independent LuaDraw drawing session and its eleven tools.
type Harness struct {
	Root, Reference            string
	Options                    map[string]any
	MinOverlap, MaxScaleDev    float64
	State                      map[string]any
	Context                    string
	Timeout                    time.Duration
	root, reference, imagesDir string
	currentImage               string
	views, pdfViews            map[string]int
	warned                     map[string]bool
	docs                       *luadraw.Docs
}

func New(root, reference, contextText string, threshold float64, timeout time.Duration, options map[string]any) (*Harness, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	ref, err := filepath.Abs(reference)
	if err != nil {
		return nil, err
	}
	if options == nil {
		options = map[string]any{}
	}
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	h := &Harness{Root: abs, Reference: ref, root: abs, reference: ref, Context: contextText, Timeout: timeout, Options: options,
		MinOverlap: threshold, MaxScaleDev: number(options["max_scale_dev"], .1),
		currentImage: stringValue(options["current_img"], filepath.Base(ref)), imagesDir: stringValue(options["images_dir"], filepath.Dir(ref)),
		views: map[string]int{}, pdfViews: map[string]int{}, warned: map[string]bool{}, State: newState()}
	if _, ok := options["accept_min_submits"]; !ok {
		options["accept_min_submits"] = 8
	}
	if _, ok := options["max_scale_dev"]; !ok {
		options["max_scale_dev"] = .1
	}
	return h, nil
}

func newState() map[string]any {
	return map[string]any{"submitted": false, "final_code": "", "last_pdf": "", "merges": []any{}, "uncertain": false, "best_overlap": 0.0, "scale_dev_at_best": 0.0, "accepted_as_is": false, "accept_note": "", "submit_attempts": 0, "compile_ok": false, "compare_called": false, "size_unit_at_best": "", "aspect_ref_at_best": 0.0, "aspect_cand_at_best": 0.0, "ink_ref_w_at_best": 0.0, "ink_ref_h_at_best": 0.0, "ink_cand_w_at_best": 0.0, "ink_cand_h_at_best": 0.0, "best_scale_x": 0.0, "best_scale_y": 0.0, "compile_err": "", "source_path": "figure.lua", "source_language": "luadraw", "drawing_backend": "", "generated_tex": ""}
}

func (h *Harness) Definitions() []map[string]any {
	tools := []map[string]any{
		toolSchema("write_file", "Write editable LuaDraw .lua source or a LaTeX .tex body containing a luadraw environment into your workspace. Full content replaces the file. Also allows .cls .sty .md. Use edit_file for small changes.", map[string]any{"path": prop("string", "relative path inside your workspace, e.g. figure.lua, class.cls, manual.md"), "content": prop("string", "full file content")}, "path", "content"),
		toolSchema("edit_file", "Edit a file in your workspace by exact find/replace (or append:true). Either (a) literal find/replace: give find+replace (find must occur exactly once unless replace_all=true), or (b) append: set append=true and give replace (the text is appended to the end of the file; find ignored). Never rewrite a whole file when a small edit suffices.", map[string]any{"path": prop("string", "workspace-relative file path"), "find": prop("string", "literal text to find (exact, once)"), "replace": prop("string", "replacement text / appended text"), "append": prop("boolean", "true = append replace to end of file"), "replace_all": prop("boolean", "replace every occurrence of find")}, "path", "replace"),
		toolSchema("read_file", "Read a text file (read-only): your workspace files AND, where allowed, the project files (source markdown, class manual, other chapters). Without start_line/end_line the whole file is returned (truncated when large); with start_line/end_line a numbered window of at most 400 lines is returned. Paths are workspace-relative. Mounts: work(rw). Plain paths are relative to work; use \"name:path\" or \"/name/path\" for another mount.", map[string]any{"path": prop("string", "file path relative to the workspace root (or name:path for another mount)"), "start_line": prop("integer", "optional first line (1-based) of a window"), "end_line": prop("integer", "optional last line (inclusive) of a window")}, "path"),
		toolSchema("grep", "Search files for a pattern (Go regexp; a pattern that does not compile is matched as plain text). path: a file, or a directory (recursive, text files only). Returns matched lines with 1-based line numbers; use read_file with those lines for context.", map[string]any{"pattern": prop("string", "Go regular expression"), "path": prop("string", "file or directory, workspace-relative; default = workspace root"), "max_matches": prop("integer", "cap on returned lines (default 50)")}, "pattern"),
		toolSchema("compile", "Compile an editable LuaDraw program (default figure.lua), or a .tex body containing a genuine luadraw environment. Do not pass code inline. The host prefers tested LuaLaTeX; if unavailable it evaluates the SAME LuaDraw program using standalone Lua/texlua, then compiles generated TikZ with XeLaTeX/pdfLaTeX. Pure hand-written TikZ and reference-raster embedding are rejected. Inspect standalone.pdf with view_pdf and compare_figure before submit. The receipt reports the actual backend and errors.", map[string]any{"path": prop("string", "Workspace-relative LuaDraw .lua program (default figure.lua), or .tex LuaDraw-environment body."), "merges": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "optional: image paths (as in the markdown) of page-boundary continuations absorbed into this figure"}}),
		toolSchema("view_pdf", "Render ONE page of a PDF and attach it as an image: PDFs you compiled (chapter/figure/book) AND, where mounted, the ORIGINAL book PDFs. Crop (percent) and zoom (target pixel width) work the same in both cases; the page is re-rendered from the PDF at the requested width, so zoom stays sharp. Mounts: work(rw). Plain paths are relative to work; use \"name:path\" or \"/name/path\" for another mount.", map[string]any{"path": prop("string", "PDF path (workspace-relative, or name:path for another mount)"), "page": prop("integer", "1-based page number (default 1)"), "left": prop("number", "crop left percent 0-100"), "top": prop("number", "crop top percent 0-100"), "right": prop("number", "crop right percent 0-100 (default 100)"), "bottom": prop("number", "crop bottom percent 0-100 (default 100)"), "zoom": prop("integer", "target width in px (default 1280)")}, "path"),
		toolSchema("compare_figure", "Compare the original image with a compiled figure. Returns a four-panel overlay (gray/R/G/B) or one selected panel; supports targeted crop and zoom for detailed re-checks. This is visual evidence, not semantic proof.", map[string]any{"pdf_path": prop("string", "Workspace-relative compiled PDF path; defaults to the latest successful compile."), "detail": map[string]any{"type": "string", "enum": []string{"all", "gray", "r", "g", "b", "rg", "rb", "gb"}, "description": "all returns a 2x2 gray/R/G/B board; a channel or channel combination returns that panel"}, "worst_region": prop("boolean", "Return only the automatically located weakest comparison region instead of the full figure"), "left": prop("number", "Crop left percentage, 0-100."), "top": prop("number", "Crop top percentage, 0-100."), "right": prop("number", "Crop right percentage, 0-100."), "bottom": prop("number", "Crop bottom percentage, 0-100."), "zoom": prop("integer", "Target output width in pixels; 0 keeps cropped native width.")}),
		toolSchema("submit", "Submit the exact editable LuaDraw source that was successfully compiled and compared. Use {path:'figure.lua'}; never submit generated TikZ as the editable source.", map[string]any{"path": prop("string", "Workspace-relative editable LuaDraw source; must equal the last successful compile."), "code": prop("string", "Final LuaDraw source inline, only when not using path."), "merges": prop("string", "Image paths (as in the markdown) of page-boundary continuations absorbed into this combined figure.")}),
		toolSchema("image_context", "TEXT context (no pixels): show the markdown lines around an image ref (expandable up/down window, like get_more_context) plus the previous/next image refs with their line numbers. Use it to detect cross-page continuations; use view_image to actually LOOK at an image.", map[string]any{"image": prop("string", "Image ref as in the markdown (default: the image this session is drawing). A bare file name is also accepted (resolved inside this document's image folder)."), "up": prop("integer", "Context lines above (default 10)."), "down": prop("integer", "Context lines below (default 10).")}),
		toolSchema("view_image", "LOOK at an image (pixels). path is the image as it appears in the markdown (images/<subject>/foo.jpg) or just its file name (foo.jpg); it is joined to this document's image folder — a wrong name is reported as an error. Compiled PDFs are NOT images: inspect them with view_pdf. Optionally crop a region by percentages (left/top/right/bottom, 0-100) and scale it up for detail inspection. For TEXT context around an image use image_context instead.", map[string]any{"path": prop("string", "Image file name (foo.jpg) or the markdown ref (images/<subject>/foo.jpg)."), "left": prop("number", "Crop left in percent (0-100), default 0."), "top": prop("number", "Crop top in percent (0-100), default 0."), "right": prop("number", "Crop right in percent (0-100), default 100."), "bottom": prop("number", "Crop bottom in percent (0-100), default 100."), "zoom": prop("integer", "Target width in pixels for the returned image (e.g. 1400 for detail view). Default: fit to 1280.")}, "path"),
	}
	root := stringValue(h.Options["luadraw_root"], filepath.Join(stringValue(h.Options["resource_root"], "."), "vendor", "luadraw-3.7", "luadraw"))
	cache := stringValue(h.Options["doc_cache_dir"], "")
	if cache == "" {
		cache = luadraw.CacheDirectory(stringValue(h.Options["resource_root"], ""))
	}
	docs, e := luadraw.NewDocs(root, cache)
	if e == nil {
		h.docs = docs
		tools = append(tools, docs.Definition())
	} else {
		tools = append(tools, docsToolDefinition())
	}
	return tools
}

func toolSchema(name, description string, properties map[string]any, required ...string) map[string]any {
	p := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		p["required"] = required
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": p}}
}
func prop(t, d string) map[string]any { return map[string]any{"type": t, "description": d} }

func (h *Harness) Snapshot() map[string]any {
	out := cloneMap(h.State)
	out["_accepted_threshold"] = h.liveThreshold()
	out["_accepted_scale_limit"] = h.MaxScaleDev
	return out
}
func (h *Harness) Restore(state map[string]any) error {
	if state == nil {
		return errors.New("state must be an object")
	}
	h.State = cloneMap(state)
	return nil
}
func (h *Harness) IsSubmitted() bool { b, _ := h.State["submitted"].(bool); return b }
func (h *Harness) MeasureHint() string {
	if raw, ok := h.Options["measurement"].(map[string]any); ok {
		data, err := json.Marshal(raw)
		if err == nil {
			var measurement Measurement
			if json.Unmarshal(data, &measurement) == nil {
				return measurement.Hint()
			}
		}
	}
	m, _ := MeasureImage(h.reference, "", floatList(h.Options["reference_mm"]))
	return m.Hint()
}

func (h *Harness) Execute(ctx context.Context, name string, args map[string]any) (string, string, error) {
	if args == nil {
		args = map[string]any{}
	}
	var text, img string
	var err error
	switch name {
	case "write_file":
		text, err = h.writeFile(args)
	case "edit_file":
		text, err = h.editFile(args)
	case "read_file":
		text, err = h.readFile(args)
	case "grep":
		text, err = h.grep(args)
	case "compile":
		text, err = h.compile(ctx, args)
	case "view_pdf":
		text, img, err = h.viewPDF(ctx, args)
	case "compare_figure":
		text, img, err = h.compare(ctx, args)
	case "submit":
		text, err = h.submit(args)
	case "image_context":
		text, err = h.imageContext(args)
	case "view_image":
		text, img, err = h.viewImage(args)
	case "luadraw_doc":
		if h.docs == nil {
			_ = h.Definitions()
		}
		if h.docs == nil {
			return "", "", errors.New("LuaDraw documentation resources unavailable")
		}
		out := filepath.Join(h.root, "预览", "luadraw-doc")
		text, img, err = h.docs.Execute(ctx, args, out)
	default:
		text = "Unknown tool: " + name
	}
	if err != nil {
		return "ERROR: " + err.Error(), "", nil
	}
	return text, img, nil
}

func resolveWithMissingTail(path string) (string, error) {
	current := filepath.Clean(path)
	missing := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (h *Harness) safe(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if strings.HasPrefix(rel, "/work/") {
		rel = strings.TrimPrefix(rel, "/work/")
	} else if rel == "/work" {
		rel = "."
	} else if strings.HasPrefix(rel, "work:") {
		rel = strings.TrimPrefix(rel, "work:")
	} else if strings.Contains(rel, ":") {
		mount, rest, _ := strings.Cut(rel, ":")
		if mount != "work" {
			return "", fmt.Errorf("未知挂载点 %q（可用: work(rw)", mount)
		}
		rel = rest
	} else if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("未知挂载点；可用: work(rw)")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("只允许相对路径: %s", rel)
	}
	joined := filepath.Join(h.root, filepath.Clean(rel))
	resolved, e := resolveWithMissingTail(joined)
	if e != nil {
		return "", e
	}
	base, e := filepath.EvalSymlinks(h.root)
	if e != nil {
		return "", e
	}
	r, e := filepath.Rel(base, resolved)
	if e != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	return resolved, nil
}

func (h *Harness) writeFile(a map[string]any) (string, error) {
	rel := stringValue(a["path"], "")
	content, ok := a["content"].(string)
	if rel == "" || !ok {
		return "REJECTED: path and string content are required.", nil
	}
	ext := strings.ToLower(filepath.Ext(rel))
	if ext != ".lua" && ext != ".tex" && ext != ".cls" && ext != ".sty" && ext != ".md" {
		return "REJECTED: only .lua .cls .sty .tex .md files are allowed.", nil
	}
	p, e := h.safe(rel)
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		return "", e
	}
	if e = os.WriteFile(p, []byte(content), 0644); e != nil {
		return "", e
	}
	return fmt.Sprintf("WROTE %s (%d bytes)", rel, len([]byte(content))), nil
}
func (h *Harness) editFile(a map[string]any) (string, error) {
	rel := stringValue(a["path"], "")
	if rel == "" {
		return "ERROR: path 为空", nil
	}
	p, e := h.safe(rel)
	if e != nil {
		return "", e
	}
	old, e := os.ReadFile(p)
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return "", e
	}
	if len(old) > 0 && !utf8.Valid(old) {
		return "", fmt.Errorf("file is not valid UTF-8 text")
	}
	find, rep := stringValue(a["find"], ""), stringValue(a["replace"], "")
	if truth(a["append"]) {
		next := string(old)
		if next != "" && !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += rep
		if e = os.MkdirAll(filepath.Dir(p), 0755); e == nil {
			e = os.WriteFile(p, []byte(next), 0644)
		}
		return fmt.Sprintf("APPENDED %d bytes to %s.", len([]byte(rep)), rel), e
	}
	if errors.Is(e, fs.ErrNotExist) {
		return "NOT FOUND: " + rel + "（新建文件请用 write_file）", nil
	}
	if find == "" {
		return "REJECTED: find 为空（追加请设 append=true）", nil
	}
	n := strings.Count(string(old), find)
	if n == 0 {
		return "NOT FOUND: 给出的 find 文本在 " + rel + " 中没有出现（注意精确匹配，包括缩进）", nil
	}
	if n > 1 && !truth(a["replace_all"]) {
		return fmt.Sprintf("AMBIGUOUS: find 出现 %d 次；补充上下文让匹配唯一，或设 replace_all=true", n), nil
	}
	next := strings.Replace(string(old), find, rep, -1)
	if !truth(a["replace_all"]) {
		next = strings.Replace(string(old), find, rep, 1)
	}
	if e = os.WriteFile(p, []byte(next), 0644); e != nil {
		return "", e
	}
	return fmt.Sprintf("REPLACED %d occurrence(s) in %s (%d -> %d bytes).", n, rel, len(old), len([]byte(next))), nil
}
func (h *Harness) readFile(a map[string]any) (string, error) {
	p, e := h.safe(stringValue(a["path"], ""))
	if e != nil {
		return "", e
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return "", e
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("file is not valid UTF-8 text")
	}
	text := string(b)
	if _, ok := a["start_line"]; ok || has(a, "end_line") {
		lines := strings.Split(text, "\n")
		start := max(1, int(number(a["start_line"], 1)))
		end := min(len(lines), int(number(a["end_line"], float64(start))), start+399)
		if end < start {
			return fmt.Sprintf("(empty range; file has %d lines)", len(lines)), nil
		}
		var out strings.Builder
		fmt.Fprintf(&out, "[lines %d-%d of %d]\n", start, end, len(lines))
		for i := start; i <= end; i++ {
			line := lines[i-1]
			if len(line) > 500 {
				line = line[:500] + "..."
			}
			fmt.Fprintf(&out, "%d: %s\n", i, line)
		}
		return out.String(), nil
	}
	truncated := len(b) > 65536
	if truncated {
		text = string(bytes.ToValidUTF8(b[:65536], []byte{}))
	}
	count := strings.Count(text, "\n")
	if text != "" && !strings.HasSuffix(text, "\n") {
		count++
	}
	note := ""
	if truncated {
		note = "\n...[truncated; use start_line/end_line for later parts]..."
	}
	return fmt.Sprintf("[%s, %d bytes, %d lines]\n%s%s", filepath.Base(p), len(b), count, text, note), nil
}

func (h *Harness) grep(a map[string]any) (string, error) {
	pat := stringValue(a["pattern"], "")
	rel := stringValue(a["path"], ".")
	root, e := h.safe(rel)
	if e != nil {
		return "", e
	}
	rx, compileErr := regexp.Compile(pat)
	files := []string{}
	info, e := os.Stat(root)
	if e != nil {
		return "", e
	}
	if !info.IsDir() {
		files = []string{root}
	} else {
		e = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == ".git" || d.Name() == ".trash" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				files = append(files, p)
			}
			return nil
		})
		if e != nil {
			return "", e
		}
		sort.Strings(files)
	}
	limit := min(200, max(1, int(number(a["max_matches"], 50))))
	matches, total := []string{}, 0
	explicit := strings.HasPrefix(rel, "work:") || strings.HasPrefix(rel, "/work/")
	for _, file := range files {
		resolved, resolveErr := filepath.EvalSymlinks(file)
		if resolveErr != nil || !within(h.root, resolved) {
			continue
		}
		st, e := os.Stat(file)
		if e != nil || st.Size() > 8<<20 {
			continue
		}
		b, e := os.ReadFile(file)
		if e != nil || bytes.IndexByte(b[:min(len(b), 8192)], 0) >= 0 {
			continue
		}
		if !utf8.Valid(b) {
			continue
		}
		text := string(b)
		for i, line := range strings.Split(text, "\n") {
			matched := false
			if compileErr != nil {
				matched = strings.Contains(line, pat)
			} else {
				matched = rx.MatchString(line)
			}
			if !matched {
				continue
			}
			total++
			if len(matches) < limit {
				shown, _ := filepath.Rel(h.root, file)
				shown = filepath.ToSlash(shown)
				if explicit {
					shown = "work:" + shown
				}
				matches = append(matches, fmt.Sprintf("%s:%d:%s", shown, i+1, line))
			}
		}
	}
	if total == 0 {
		return "NO MATCHES: " + pat, nil
	}
	if total > len(matches) {
		matches = append(matches, fmt.Sprintf("...(%d more, 收窄 pattern)", total-len(matches)))
	}
	return strings.Join(matches, "\n"), nil
}

var compileErrRE = regexp.MustCompile(`(?m)^! .*|^l\.\d+.*`)
var warnRE = regexp.MustCompile(`(?m)^(?:LaTeX|Package|Class|Module)\b.*Warning:.*|^(?:Overfull|Underfull) \\[hv]box.*`)

func summarizeCompile(log string) (string, string) {
	errs := uniqueStrings(compileErrRE.FindAllString(log, -1), 20)
	warns := uniqueStrings(warnRE.FindAllString(log, -1), 20)
	errText := strings.Join(errs, "\n")
	if errText == "" {
		if len(log) > 12000 {
			log = log[len(log)-12000:]
		}
		errText = log
	}
	warning := ""
	if len(warns) > 0 {
		warning = fmt.Sprintf("WARNINGS (%d):\n  - %s", len(warns), strings.Join(warns, "\n  - "))
	}
	return errText, warning
}
func uniqueStrings(in []string, n int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, strings.TrimSpace(s))
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

func (h *Harness) compile(ctx context.Context, a map[string]any) (string, error) {
	rel := strings.TrimSpace(stringValue(a["path"], ""))
	if rel == "" {
		rel = "figure.lua"
	}
	source, e := h.safe(rel)
	if e != nil {
		return "", e
	}
	data, e := os.ReadFile(source)
	if errors.Is(e, fs.ErrNotExist) {
		return "NOT FOUND: " + rel + " (write editable LuaDraw source first)", nil
	}
	if e != nil {
		return "", e
	}
	code := string(data)
	if strings.TrimSpace(code) == "" {
		return "REJECTED: " + rel + " is empty.", nil
	}
	if items, ok := a["merges"].([]any); ok {
		for _, x := range items {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				h.State["merges"] = append(asSlice(h.State["merges"]), strings.TrimSpace(s))
			}
		}
	}
	h.State["compile_ok"] = false
	h.State["last_code"] = code
	h.State["last_pdf"] = ""
	h.State["compare_called"] = false
	h.State["best_overlap"] = 0.0
	h.State["scale_dev_at_best"] = 0.0
	h.State["source_path"] = rel
	result, e := luadraw.Compile(ctx, source, h.root, h.Options)
	if e != nil {
		h.State["compile_err"] = e.Error()
		return "COMPILE FAILED. Fix the LuaDraw program and compile again.\n" + e.Error(), nil
	}
	ok := truth(result["ok"])
	if !ok {
		err := stringValue(result["error"], stringValue(result["logs"], "Unknown LuaDraw compile error"))
		h.State["compile_err"] = err
		if len(err) > 12000 {
			err = err[len(err)-12000:]
		}
		return "COMPILE FAILED. Fix the LuaDraw program and compile again.\n" + err, nil
	}
	pdf := stringValue(result["pdf"], "")
	if pdf == "" {
		return "", errors.New("LuaDraw compile result has no PDF path")
	}
	canonical := filepath.Join(h.root, "standalone.pdf")
	if filepath.Clean(pdf) != filepath.Clean(canonical) {
		b, e := os.ReadFile(pdf)
		if e != nil {
			return "", e
		}
		if e = os.WriteFile(canonical, b, 0644); e != nil {
			return "", e
		}
	}
	h.State["compile_ok"] = true
	h.State["compile_err"] = ""
	h.State["last_pdf"] = canonical
	h.State["drawing_backend"] = result["backend"]
	h.State["drawing_engine"] = result["engine"]
	h.State["generated_tex"] = firstValue(result, "wrapper", "tex", "tex_path")
	h.State["generated_tikz"] = result["generated_tikz"]
	h.State["luadraw_version"] = stringValue(result["luadraw_version"], "3.7")
	size, _ := cf.PDFCanvasMM(ctx, canonical)
	return fmt.Sprintf("COMPILE OK. Artifact: standalone.pdf (1 page), %.1fmm x %.1fmm. LuaDraw %v; backend=%v; engine=%v. Inspect with view_pdf {path:\"standalone.pdf\",page:1}, then compare_figure.", size[0], size[1], h.State["luadraw_version"], h.State["drawing_backend"], h.State["drawing_engine"]), nil
}

func (h *Harness) liveThreshold() float64 {
	v := h.MinOverlap
	if fn, ok := h.Options["live_threshold"].(func() float64); ok {
		if n := fn(); n != 0 {
			v = n
		}
	} else if fn, ok := h.Options["live_threshold"].(func() (float64, error)); ok {
		if n, e := fn(); e == nil && n != 0 {
			v = n
		}
	}
	path := stringValue(h.Options["metadata_path"], "")
	if path != "" {
		var meta struct {
			FigMinOverlap float64 `json:"fig_min_overlap"`
		}
		if readJSON(path, &meta) == nil && meta.FigMinOverlap != 0 {
			v = meta.FigMinOverlap
		}
	}
	return v
}
func fallbackWhy(w []string) string {
	if len(w) > 0 {
		return strings.Join(w, "；")
	}
	return "对照原图看差在哪（线条、比例、标注位置）"
}

func (h *Harness) compare(ctx context.Context, a map[string]any) (string, string, error) {
	explicit := strings.TrimSpace(stringValue(a["pdf_path"], ""))
	if explicit == "" && (!truth(h.State["compile_ok"]) || stringValue(h.State["last_pdf"], "") == "") {
		return "Comparison unavailable: compile a LuaDraw figure successfully first, or provide pdf_path.", "", nil
	}
	pdf := stringValue(h.State["last_pdf"], "")
	if explicit != "" {
		var e error
		pdf, e = h.safe(explicit)
		if e != nil {
			return "", "", e
		}
	}
	if _, e := os.Stat(pdf); e != nil {
		return "Comparison unavailable: PDF not found.", "", nil
	}
	round := int(number(h.State["compare_round"], 0)) + 1
	out := filepath.Join(h.root, "compare", fmt.Sprintf("round_%04d", round))
	crop := []float64{}
	if has(a, "left") {
		crop = []float64{number(a["left"], 0), number(a["top"], 0), number(a["right"], 100), number(a["bottom"], 100)}
	} else if !truth(a["worst_region"]) && (has(a, "top") || has(a, "right") || has(a, "bottom")) {
		crop = []float64{number(a["left"], 0), number(a["top"], 0), number(a["right"], 100), number(a["bottom"], 100)}
	}
	refmm := floatList(h.Options["reference_mm"])
	if len(refmm) < 2 || refmm[0] <= 0 || refmm[1] <= 0 {
		if measurement, ok := h.Options["measurement"].(map[string]any); ok {
			refmm = []float64{number(measurement["width_mm"], number(measurement["WidthMM"], 0)), number(measurement["height_mm"], number(measurement["HeightMM"], 0))}
		} else {
			measured, _ := MeasureImage(h.reference, "", nil)
			refmm = []float64{measured.WidthMM, measured.HeightMM}
		}
	}
	rasterDir, e := os.MkdirTemp("", "drawing-compare-render-")
	if e != nil {
		return "", "", e
	}
	defer os.RemoveAll(rasterDir)
	candidateRaster, e := h.rasterizeAtDPI(ctx, pdf, filepath.Join(rasterDir, "candidate"))
	if e != nil {
		return "", "", e
	}
	candidateMM, _ := cf.PDFCanvasMM(ctx, pdf)
	passOverlap, passScale := h.MinOverlap, h.MaxScaleDev
	if passOverlap <= 0 {
		passOverlap = -1
	}
	if passScale <= 0 {
		passScale = -1
	}
	req := cf.Request{Version: "compare-figure/1", Reference: h.reference, Candidate: candidateRaster, CandidateMM: candidateMM, Detail: stringValue(a["detail"], "all"), Zoom: int(number(a["zoom"], 0)), WorstRegion: truth(a["worst_region"]), OutDir: out, RasterDPI: int(number(h.Options["raster_dpi"], 110)), PassOverlap: passOverlap, PassScale: passScale}
	if len(refmm) >= 2 {
		req.ReferenceMM = [2]float64{refmm[0], refmm[1]}
	}
	if data, ok := h.Options["measurement"].(map[string]any); ok {
		raw, _ := json.Marshal(data)
		var original Measurement
		if json.Unmarshal(raw, &original) == nil {
			req.ReferenceMeasurement = &original
		}
	}
	if len(crop) == 4 {
		req.Crop = [4]float64{crop[0], crop[1], crop[2], crop[3]}
	}
	result, e := cf.ProcessRequest(ctx, req)
	if e != nil {
		return "", "", e
	}
	if !result.OK || result.Metrics == nil {
		return "", "", fmt.Errorf("comparison failed: %s", result.Error)
	}
	m := result.Metrics
	h.State["compare_called"] = true
	h.State["compare_round"] = round
	if number(h.State["best_overlap"], 0) == 0 || m.Similarity > number(h.State["best_overlap"], 0) {
		putMetrics(h.State, m)
	}
	return result.Text, result.SelectedImage, nil
}
func putMetrics(s map[string]any, m *cf.Metrics) {
	s["best_overlap"] = m.Similarity
	s["scale_dev_at_best"] = m.ScaleDev
	s["best_scale_x"] = m.ScaleX
	s["best_scale_y"] = m.ScaleY
	s["size_unit_at_best"] = m.SizeUnit
	s["aspect_ref_at_best"] = m.AspectRef
	s["aspect_cand_at_best"] = m.AspectCandidate
	s["ink_ref_w_at_best"] = m.InkRefW
	s["ink_ref_h_at_best"] = m.InkRefH
	s["ink_cand_w_at_best"] = m.InkCandW
	s["ink_cand_h_at_best"] = m.InkCandH
}

func (h *Harness) Deliver(ctx context.Context) (map[string]any, error) {
	if !h.IsSubmitted() {
		return nil, errors.New("No accepted LuaDraw source")
	}
	snap, ok := h.State["accepted_snapshot"].(map[string]any)
	if ok && snap["final_code"] == h.State["final_code"] {
		h.State = cloneMap(snap)
	}
	code := stringValue(h.State["final_code"], "")
	suffix := filepath.Ext(stringValue(h.State["source_path"], "figure.lua"))
	sourceName := "submitted.lua"
	if suffix != ".lua" {
		sourceName = "submitted.body.tex"
	}
	source := filepath.Join(h.root, sourceName)
	if err := os.WriteFile(source, []byte(code), 0644); err != nil {
		return nil, err
	}
	compiled, err := luadraw.Compile(ctx, source, h.root, h.Options)
	if err != nil {
		return nil, err
	}
	if !truth(compiled["ok"]) {
		return nil, fmt.Errorf("Accepted LuaDraw source could not be rebuilt: %s", stringValue(compiled["error"], "unknown error"))
	}
	copyArtifact := func(src, dst string) error {
		if src == "" {
			return fmt.Errorf("compile result missing artifact")
		}
		b, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		return os.WriteFile(dst, b, 0644)
	}
	tex := filepath.Join(h.root, "submitted.tex")
	texCode := luadraw.SelfContainedTeX(code, h.Options, sourceName == "submitted.body.tex")
	if err = os.WriteFile(tex, []byte(texCode), 0644); err != nil {
		return nil, err
	}
	texOptions := map[string]any{}
	for key, value := range h.Options {
		texOptions[key] = value
	}
	texOptions["output_name"] = "submitted_rebuild"
	if extra := luadraw.OriginalTeXPreamble(code, sourceName == "submitted.body.tex"); extra != "" {
		if configured, ok := texOptions["preamble"].(string); ok && configured != "" {
			texOptions["preamble"] = configured + "\n" + extra
		} else {
			texOptions["preamble"] = extra
		}
	}
	texBuild, err := luadraw.Compile(ctx, tex, h.root, texOptions)
	if err != nil {
		return nil, err
	}
	if !truth(texBuild["ok"]) {
		return nil, fmt.Errorf("self-contained submitted.tex could not be rebuilt: %s", stringValue(texBuild["error"], "unknown error"))
	}
	pdf := filepath.Join(h.root, "submitted.pdf")
	if err = copyArtifact(stringValue(texBuild["pdf"], ""), pdf); err != nil {
		return nil, err
	}
	h.State["generated_tikz"] = texBuild["generated_tikz"]
	h.State["drawing_engine"] = texBuild["engine"]
	h.State["drawing_backend"] = texBuild["backend"]
	warnings := []string{}
	preview := filepath.Join(h.root, "预览", "submitted.png")
	_, err = h.Rasterize(ctx, pdf, strings.TrimSuffix(preview, filepath.Ext(preview)))
	if err != nil {
		warnings = append(warnings, "Preview failed; vector PDF retained: "+err.Error())
		preview = ""
	}
	result := map[string]any{"submitted": true, "source_language": "luadraw", "luadraw_version": h.State["luadraw_version"], "backend": h.State["drawing_backend"], "engine": h.State["drawing_engine"], "editable_source": source, "editable_lua": source, "editable_tex": tex, "submitted_tex": tex, "submitted_pdf": pdf, "preview": nil, "merges": h.State["merges"], "uncertain": h.State["uncertain"], "best_overlap": h.State["best_overlap"], "scale_dev": h.State["scale_dev_at_best"], "accepted_as_is": h.State["accepted_as_is"], "accept_note": h.State["accept_note"], "submit_attempts": h.State["submit_attempts"], "warnings": warnings, "generated_tikz": h.State["generated_tikz"], "generated_lua": texBuild["generated_lua"]}
	if preview != "" {
		result["preview"] = preview
	}
	data, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return nil, e
	}
	if e = cf.AtomicWrite(filepath.Join(h.root, "result.json"), append(data, '\n'), 0644); e != nil {
		return nil, e
	}
	return result, nil
}

func cloneMap(in map[string]any) map[string]any {
	b, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}
func asSlice(v any) []any {
	if x, ok := v.([]any); ok {
		return x
	}
	return []any{}
}
func number(v any, d float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, e := strconv.ParseFloat(x, 64)
		if e == nil {
			return f
		}
	}
	return d
}
func stringValue(v any, d string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return d
}
func truth(v any) bool                    { b, _ := v.(bool); return b }
func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }
func floatList(v any) []float64 {
	switch x := v.(type) {
	case []float64:
		return x
	case []any:
		r := make([]float64, 0, len(x))
		for _, v := range x {
			r = append(r, number(v, 0))
		}
		return r
	}
	return nil
}
func firstValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := stringValue(m[k], ""); s != "" {
			return s
		}
	}
	return ""
}
