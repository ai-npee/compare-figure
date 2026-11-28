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
	// The CLI/library request also contains host output and transport settings.
	// Expose only operations the model chooses for this call, not that full API.
	all := doclibrary.Schema()["request"].(map[string]any)["properties"].(map[string]any)
	props := map[string]any{}
	for _, key := range []string{"mode", "query", "source", "sources", "doc", "page", "region", "zoom", "limit", "offset", "start_line", "end_line", "snippet_chars", "max_chars", "line_chars", "regex", "case_sensitive", "entry_id", "recursive", "include_image", "include_bbox"} {
		props[key] = all[key]
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	props["mode"] = map[string]any{"type": "string", "enum": []string{"search", "read", "view", "list", "find", "grep", "frequent"}}
	props["include_bbox"] = map[string]any{"type": "boolean", "description": "Optional: request available bbox percentages. Omit for the startup default (normally off)."}
	props["case_sensitive"].(map[string]any)["description"] = "Grep defaults to true for legacy compatibility; other modes default to false. Set false explicitly for case-insensitive grep."
	props["source"].(map[string]any)["description"] = "Exact configured reference source ID. Bundled IDs are luadraw (official resources and configured project references) and asymptote. Omit source to search the active mode's collections; choose a source only to narrow the library. For read/view use the exact returned source ID and relative doc path."
	props["sources"].(map[string]any)["description"] = "Exact configured source IDs, mutually exclusive with source. Select multiple collections explicitly; omitting both selectors uses all sources configured for the backend. No source aliases or implicit expansion."
	props["doc"].(map[string]any)["description"] = "Exact source-root-relative file for read/view, directory for list/grep/find. list doc=\".\" lists root files and subdirectories (directory paths end in /); requests have no persistent current directory. Discover unknown paths with find before read; e.g. Asymptote manual is doc/asymptote.texi, not asymptote.texi."
	return map[string]any{"type": "function", "function": map[string]any{"name": "document_search", "strict": false, "description": "Search/read/navigate local references. Supply only arguments needed for this call; omit unused options, empty strings and default-filled objects. Search returns text with source/document/page provenance; include_bbox optionally requests percentage locations. PDF coordinates do not map to TeX source lines.", "parameters": schema}}
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
		version := d.output.Version
		if request.OutputVersion != 0 {
			version = request.OutputVersion
		}
		if version != 1 && version != 2 {
			return Response{}, fmt.Errorf("document output version must be 1 or 2")
		}
		wireVersion := doclibrary.Version
		if version == 2 {
			wireVersion = doclibrary.CompactVersion
		}
		response := Response{Version: wireVersion, OK: true, Mode: mode, Text: d.emptyMessage, Results: []doclibrary.Result{}, Images: []doclibrary.Image{}}
		if version == 2 {
			response.Warnings = []string{d.emptyMessage}
		}
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
	if d.library == nil {
		return d.emptyMessage, image, nil
	}
	return formatResponse(response), image, nil
}

// ExecuteResponse keeps the core wire protocol: v2 has results as its only body,
// while v1 retains legacy text/cache/provenance. Locations are source-qualified.
func (d *Docs) ExecuteResponse(ctx context.Context, args map[string]any, outdir string) (map[string]any, error) {
	response, err := d.execute(ctx, args, outdir)
	if err != nil {
		return nil, err
	}
	if response.Version == doclibrary.Version && d.library != nil {
		response.Text = formatResponse(response)
	}
	data, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if response.Version == doclibrary.Version {
		result["provenance"] = map[string]any{"dependency": "doc-library", "revision": DependencyRevision, "coordinates": "PDF physical page and top-left bbox; source lines are text-file lines, not PDF-to-TeX mapping"}
	}
	return result, nil
}

func formatResponse(response Response) string {
	var rows []string
	truncated := response.Truncated
	for _, result := range response.Results {
		where := result.Source + " · " + result.Doc
		if result.Page > 0 {
			where += fmt.Sprintf(" · p.%d", result.Page)
		} else if result.StartLine > 0 {
			where += fmt.Sprintf(" · L%d–%d", result.StartLine, result.EndLine)
		}
		if response.Mode == "frequent" {
			where += fmt.Sprintf(" [%s] hits=%d", prefix(result.EntryID, 12), result.Hits)
		}
		if result.BBoxPct != nil {
			b := *result.BBoxPct
			where += fmt.Sprintf("\nbbox_pct=[%.2f,%.2f,%.2f,%.2f]", b[0], b[1], b[2], b[3])
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
		truncated = truncated || result.Truncated
	}
	if len(rows) == 0 {
		rows = append(rows, "No matches.")
	}
	for _, warning := range response.Warnings {
		if !routineScanWarning(warning) && !standardOutputWarning(warning, truncated, response.NextOffset) {
			rows = append(rows, warning)
		}
	}
	if truncated {
		rows = append(rows, "内容已截断；可用 read 继续精读。")
	}
	if response.NextOffset != nil {
		rows = append(rows, fmt.Sprintf("继续检索：offset=%d", *response.NextOffset))
	}
	return strings.Join(rows, "\n\n")
}
func routineScanWarning(warning string) bool {
	return strings.HasSuffix(warning, ": binary or invalid UTF-8 skipped") || strings.HasSuffix(warning, ": skipped binary file")
}
func standardOutputWarning(warning string, truncated bool, next *int) bool {
	if truncated && (warning == "result snippet truncated; increase snippet_chars, line_chars, or max_chars to read more" || warning == "result text budget reached; increase max_chars or narrow the request") {
		return true
	}
	return next != nil && warning == fmt.Sprintf("more results available at offset=%d", *next)
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
		location := r.Source + " · " + r.Doc
		if r.Page > 0 {
			location += fmt.Sprintf(" · p.%d", r.Page)
		} else if r.StartLine > 0 {
			location += fmt.Sprintf(" · L%d–%d", r.StartLine, r.EndLine)
		}
		row := fmt.Sprintf("[%s] %s hits=%d %s", prefix(r.EntryID, 12), location, r.Hits, strings.Join(strings.Fields(r.Text), " "))
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
