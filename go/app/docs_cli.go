package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func docsServiceCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("luadraw-doc", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	requestPath := fs.String("request", "", "JSON request file, or - for stdin")
	stdin := fs.Bool("stdin", false, "read one JSON request from stdin")
	jsonl := fs.Bool("jsonl", false, "read JSONL requests from stdin or --request")
	mode := fs.String("mode", "", "default docs mode")
	query := fs.String("query", "", "default docs query")
	doc := fs.String("doc", "", "default docs path")
	entryID := fs.String("entry-id", "", "frequent paragraph ID")
	cacheDir := fs.String("cache-dir", "", "documentation cache")
	resourceRoot := fs.String("resource-root", "", "runtime resources")
	drawRoot := fs.String("luadraw-root", "", "LuaDraw distribution")
	outDir := fs.String("out", "", "output directory")
	inlineImages := fs.Bool("inline-images", false, "include image bytes")
	includeImage := fs.Bool("include-image", false, "attach a relevant PDF crop")
	regex := fs.Bool("regex", false, "grep query is a regular expression")
	recursive := fs.Bool("recursive", false, "list nested directories")
	page := fs.Int("page", 0, "PDF page")
	zoom := fs.Int("zoom", 0, "view zoom")
	limit := fs.Int("limit", 0, "result limit")
	offset := fs.Int("offset", 0, "result offset")
	startLine := fs.Int("start-line", 0, "first source line")
	endLine := fs.Int("end-line", 0, "last source line")
	snippet := fs.Int("snippet-chars", 0, "search snippet size")
	maxChars := fs.Int("max-chars", 0, "result character budget")
	lineChars := fs.Int("line-chars", 0, "frequent excerpt line size")
	left := fs.Float64("left", 0, "crop left percentage")
	top := fs.Float64("top", 0, "crop top percentage")
	right := fs.Float64("right", 0, "crop right percentage")
	bottom := fs.Float64("bottom", 0, "crop bottom percentage")
	if helpArg(args) {
		_, _ = io.WriteString(out, manualHelp)
		return 0
	}
	if err := fs.Parse(args); err != nil {
		return writeError(out, err, 2)
	}
	if fs.NArg() != 0 {
		return writeError(out, fmt.Errorf("positional arguments are not accepted by luadraw-doc; use --query for flags mode or JSON input"), 2)
	}
	selected := 0
	if *requestPath != "" {
		selected++
	}
	if *stdin {
		selected++
	}
	if *jsonl {
		selected++
	}
	if selected > 1 {
		return writeError(out, fmt.Errorf("--request, --stdin, and --jsonl are mutually exclusive"), 2)
	}

	provided := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	defaults := map[string]any{}
	set := func(name string, value any) {
		if provided[name] {
			defaults[strings.ReplaceAll(name, "-", "_")] = value
		}
	}
	set("mode", *mode)
	set("query", *query)
	set("doc", *doc)
	set("entry-id", *entryID)
	set("page", *page)
	set("zoom", *zoom)
	set("limit", *limit)
	set("offset", *offset)
	set("start-line", *startLine)
	set("end-line", *endLine)
	set("snippet-chars", *snippet)
	set("max-chars", *maxChars)
	set("line-chars", *lineChars)
	set("left", *left)
	set("top", *top)
	set("right", *right)
	set("bottom", *bottom)
	set("include-image", *includeImage)
	set("regex", *regex)
	set("recursive", *recursive)
	topDefaults := map[string]any{
		"out": *outDir, "resource-root": *resourceRoot, "luadraw-root": *drawRoot,
		"cache-dir": *cacheDir, "inline-images": *inlineImages,
	}
	keys := map[string]string{"out": "out_dir", "resource-root": "resource_root", "luadraw-root": "luadraw_root", "cache-dir": "cache_dir", "inline-images": "inline_images"}
	for name, key := range keys {
		if provided[name] {
			defaults["_"+key] = topDefaults[name]
		}
	}

	svc := &service{}
	reader := in
	if *requestPath != "" && *requestPath != "-" {
		f, err := os.Open(*requestPath)
		if err != nil {
			return writeError(out, err, 2)
		}
		defer f.Close()
		reader = f
	}
	inputMode := *requestPath != "" || *stdin || *jsonl
	if !inputMode {
		args := cloneDocsDefaults(defaults)
		return executeDocsRequest(ctx, svc, map[string]any{"operation": "docs", "arguments": args, "out_dir": *outDir, "resource_root": *resourceRoot, "luadraw_root": *drawRoot, "cache_dir": *cacheDir, "inline_images": *inlineImages}, out, 2)
	}
	if *jsonl {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 32*1024*1024)
		failed := false
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var object map[string]any
			if err := json.Unmarshal(line, &object); err != nil {
				writeError(out, err, 1)
				failed = true
				continue
			}
			request, err := prepareDocsRequest(object, defaults)
			if err != nil {
				emitDocsInputError(out, err, object)
				failed = true
				continue
			}
			if executeDocsRequest(ctx, svc, request, out, 1) != 0 {
				failed = true
			}
		}
		if err := scanner.Err(); err != nil {
			writeError(out, err, 1)
			return 1
		}
		if failed {
			return 1
		}
		return 0
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return writeError(out, err, 2)
	}
	var object map[string]any
	if err = json.Unmarshal(data, &object); err != nil {
		return writeError(out, err, 2)
	}
	request, err := prepareDocsRequest(object, defaults)
	if err != nil {
		emitDocsInputError(out, err, object)
		return 2
	}
	return executeDocsRequest(ctx, svc, request, out, 2)
}

func cloneDocsDefaults(defaults map[string]any) map[string]any {
	out := make(map[string]any, len(defaults))
	for key, value := range defaults {
		if !strings.HasPrefix(key, "_") {
			out[key] = value
		}
	}
	return out
}

func prepareDocsRequest(object map[string]any, defaults map[string]any) (map[string]any, error) {
	if object == nil {
		return nil, fmt.Errorf("request must be an object")
	}
	operation := "docs"
	if raw, ok := object["operation"]; ok {
		value, valid := raw.(string)
		if !valid {
			return nil, fmt.Errorf("operation must be docs")
		}
		operation = strings.ToLower(value)
	}
	if operation != "docs" {
		return nil, fmt.Errorf("luadraw-doc only accepts operation=docs")
	}
	request := map[string]any{"operation": "docs"}
	if id, ok := object["id"]; ok {
		request["id"] = id
	}
	for _, key := range []string{"out_dir", "resource_root", "luadraw_root", "cache_dir", "inline_images"} {
		if value, ok := object[key]; ok && value != nil && value != "" {
			request[key] = value
		}
	}
	for _, key := range []string{"out_dir", "resource_root", "luadraw_root", "cache_dir", "inline_images"} {
		if value, ok := request[key]; !ok || value == "" {
			if fallback, exists := defaults["_"+key]; exists && fallback != "" {
				request[key] = fallback
			}
		}
	}
	args := object
	if raw, ok := object["arguments"]; ok {
		value, valid := raw.(map[string]any)
		if !valid {
			return nil, fmt.Errorf("arguments must be an object")
		}
		args = value
	}
	merged := cloneDocsDefaults(defaults)
	for key, value := range args {
		merged[key] = value
	}
	request["arguments"] = merged
	return request, nil
}

func executeDocsRequest(ctx context.Context, svc *service, request map[string]any, out io.Writer, failureCode int) int {
	result, err := svc.handle(ctx, request)
	id, hasID := request["id"]
	if err != nil {
		emitDocsError(out, err, id, hasID)
		return failureCode
	}
	data, _ := result.(map[string]any)
	if data != nil && hasID {
		data["id"] = id
	}
	if data != nil && data["ok"] == false {
		emit(out, result)
		return failureCode
	}
	emit(out, result)
	return 0
}

func emitDocsInputError(out io.Writer, err error, object map[string]any) {
	id, hasID := object["id"]
	emitDocsError(out, err, id, hasID)
}

func emitDocsError(out io.Writer, err error, id any, hasID bool) {
	result := map[string]any{"version": "compare-figure-tools/1", "ok": false, "error": err.Error()}
	if hasID {
		result["id"] = id
	}
	emit(out, result)
}
