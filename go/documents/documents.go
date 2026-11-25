package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"doc-library/pkg/doclibrary"
)

// Definition returns the shared model tool. The legacy LuaDraw wrapper renames
// this definition only; both entry points execute the same library.
func (d *Docs) Definition() map[string]any {
	schema := doclibrary.Schema()["request"].(map[string]any)
	props := schema["properties"].(map[string]any)
	for _, key := range []string{"left", "top", "right", "bottom"} {
		props[key] = map[string]any{"type": "number", "description": "Legacy PDF crop percentage; alternatively use region"}
	}
	props["case_sensitive"].(map[string]any)["description"] = "Grep defaults to true for legacy compatibility; other modes default to false. Set false explicitly for case-insensitive grep."
	props["source"].(map[string]any)["description"] = "Reference source ID (luadraw or asymptote for bundled sources); use with relative doc paths"
	return map[string]any{"type": "function", "function": map[string]any{"name": "document_search", "description": "Search/read/navigate local reference collections. Select source when paths occur in multiple collections. Search returns text and PDF page/bbox provenance; view explicitly renders pages or percentage regions. PDF coordinates do not map to TeX source lines.", "parameters": schema}}
}

func decodeRequest(args map[string]any, outdir string) (Request, error) {
	normalized := make(map[string]any, len(args))
	for k, v := range args {
		normalized[k] = v
	}
	if mode, _ := normalized["mode"].(string); mode == "grep" {
		if _, explicit := normalized["case_sensitive"]; !explicit {
			normalized["case_sensitive"] = true
		}
	}
	// Flat percentages are the original LuaDraw wire contract.
	region := map[string]any{"left": float64(0), "top": float64(0), "right": float64(100), "bottom": float64(100)}
	flat := false
	for k := range region {
		if v, ok := normalized[k]; ok {
			region[k] = v
			flat = true
			delete(normalized, k)
		}
	}
	if flat {
		if _, ok := normalized["region"]; ok {
			return Request{}, fmt.Errorf("use region or flat crop percentages, not both")
		}
		normalized["region"] = region
	}
	// A negative frequent limit historically means the default eight rows.
	if mode, _ := normalized["mode"].(string); mode == "frequent" {
		switch n := normalized["limit"].(type) {
		case int:
			if n < 0 {
				normalized["limit"] = 8
			}
		case float64:
			if n < 0 {
				normalized["limit"] = 8
			}
		}
	}
	if outdir != "" {
		normalized["out_dir"] = outdir
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return Request{}, err
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("document request: %w", err)
	}
	return request, nil
}

func (d *Docs) execute(ctx context.Context, args map[string]any, outdir string) (Response, error) {
	request, err := decodeRequest(args, outdir)
	if err != nil {
		return Response{}, err
	}
	if err = ctx.Err(); err != nil {
		return Response{}, err
	}
	if d.library == nil {
		mode := request.Mode
		if mode == "" {
			mode = "search"
		}
		switch mode {
		case "search", "read", "view", "list", "find", "grep", "frequent", "index", "status", "schema":
		default:
			return Response{}, fmt.Errorf("unknown mode %q", mode)
		}
		response := Response{Version: doclibrary.Version, OK: true, Mode: mode, Text: d.emptyMessage, Results: []doclibrary.Result{}, Images: []doclibrary.Image{}}
		if mode == "schema" {
			response.Schema = doclibrary.Schema()
		}
		return response, nil
	}
	request, targetWidth := cropResolution(request)
	response, err := d.library.Execute(ctx, request)
	if err != nil {
		if strings.Contains(err.Error(), "document not found or excluded") {
			return response, d.missingPathError(ctx, request, err)
		}
		return response, err
	}
	if targetWidth > 0 {
		if err = resizeImages(response.Images, targetWidth); err != nil {
			return response, err
		}
	}
	response.Text = formatResponse(response)
	return response, nil
}

// Execute preserves the tool's text plus optional image calling convention.
func (d *Docs) Execute(ctx context.Context, args map[string]any, outdir string) (string, string, error) {
	response, err := d.execute(ctx, args, outdir)
	if err != nil {
		return "", "", err
	}
	image := ""
	if len(response.Images) > 0 {
		image = response.Images[0].Path
	}
	return response.Text, image, nil
}

// ExecuteResponse exposes the library's structured results and cache counters in
// addition to the existing text/images payload. Locations are source-qualified.
func (d *Docs) ExecuteResponse(ctx context.Context, args map[string]any, outdir string) (map[string]any, error) {
	response, err := d.execute(ctx, args, outdir)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	result["provenance"] = map[string]any{"dependency": "doc-library", "revision": DependencyRevision, "coordinates": "PDF physical page and top-left bbox; source lines are text-file lines, not PDF-to-TeX mapping"}
	return result, nil
}

func formatResponse(response Response) string {
	meta := fmt.Sprintf("doc-library | index_sha256 %s | namespace %s | cache hits=%d misses=%d files=%d", response.Cache.Fingerprint, response.Cache.Namespace, response.Cache.Hits, response.Cache.Misses, response.Cache.Files)
	var rows []string
	for _, result := range response.Results {
		where := fmt.Sprintf("%s source=%s", result.Doc, result.Source)
		if result.Page > 0 {
			where = fmt.Sprintf("%s#page=%d source=%s", result.Doc, result.Page, result.Source)
			if result.StartLine > 0 {
				where += fmt.Sprintf(" blocks %d-%d", result.StartLine, result.EndLine)
			}
			if result.BBox != nil {
				b := *result.BBox
				where += fmt.Sprintf(" page_size_pts=%.1fx%.1f bbox_pts=%.1f,%.1f,%.1f,%.1f", result.PageWidth, result.PageHeight, b[0], b[1], b[2], b[3])
			}
			if result.BBoxPct != nil {
				b := *result.BBoxPct
				where += fmt.Sprintf(" bbox_pct=%.2f,%.2f,%.2f,%.2f", b[0], b[1], b[2], b[3])
			}
		} else if result.StartLine > 0 {
			where = fmt.Sprintf("%s lines %d-%d source=%s", result.Doc, result.StartLine, result.EndLine, result.Source)
		}
		if response.Mode == "frequent" {
			where += fmt.Sprintf(" [%s] hits=%d", prefix(result.EntryID, 12), result.Hits)
		}
		text := result.Text
		if response.Mode == "read" && result.Kind == "source" {
			lines := strings.Split(text, "\n")
			for i := range lines {
				lines[i] = fmt.Sprintf("%d: %s", result.StartLine+i, lines[i])
			}
			text = strings.Join(lines, "\n")
		}
		if text != "" {
			where += "\n" + text
		}
		rows = append(rows, where)
	}
	if len(rows) == 0 {
		rows = append(rows, "No matches.")
	}
	body := strings.Join(rows, "\n\n")
	if len(response.Warnings) > 0 {
		body += "\n" + strings.Join(response.Warnings, "\n")
	}
	return meta + "\n" + body
}

func prefix(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func fit(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}

// FrequentSnippet is passive: it never increments frequency, renders images, or
// extracts PDFs. Its complete output (including locations) is <=4096 characters.
func (d *Docs) FrequentSnippet(ctx context.Context, limit, lineChars, maxChars int) (string, error) {
	if d.library == nil {
		return "", nil
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 256 {
		limit = 256
	}
	if lineChars <= 0 {
		lineChars = 1024
	}
	if lineChars > 1024 {
		lineChars = 1024
	}
	if maxChars <= 0 || maxChars > 4096 {
		maxChars = 4096
	}
	response, err := d.library.Execute(ctx, Request{Mode: "frequent", Passive: true, Limit: limit, LineChars: lineChars, MaxChars: maxChars})
	if err != nil {
		return "", err
	}
	var rows []string
	used := 0
	for _, r := range response.Results {
		location := r.Doc
		if r.Page > 0 {
			location += fmt.Sprintf("#page=%d", r.Page)
		}
		row := fmt.Sprintf("[%s] %s:%s:%d-%d hits=%d %s", prefix(r.EntryID, 12), r.Source, location, r.StartLine, r.EndLine, r.Hits, strings.Join(strings.Fields(r.Text), " "))
		row = fit(row, lineChars)
		if used > 0 {
			used++
		}
		row = fit(row, maxChars-used)
		if row == "" {
			break
		}
		rows = append(rows, row)
		used += len([]rune(row))
		if used >= maxChars {
			break
		}
	}
	return strings.Join(rows, "\n"), nil
}
