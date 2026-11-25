package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

const manualHelp = `Usage:
  compare-figure compile [figure.lua] --source figure.lua [--out DIR] [options]
  compare-figure document-search [QUERY] --mode search|read|view|list|find|grep|frequent [options]
  compare-figure docs [QUERY] --query QUERY [options] (alias: doc)
  compare-figure luadraw-doc --query QUERY [options]
  compare-figure luadraw-doc --request FILE | --stdin | --jsonl [options]

Standalone luadraw-doc:
  No reference image, model key or drawing session required.
  Accepts bare tool JSON or operation=docs envelopes; stdout is JSON.
  --request FILE             Read one JSON object (use - for stdin)
  --stdin                    Read one JSON object from stdin
  --jsonl                    Read requests line by line; errors do not stop later lines
  These input modes are mutually exclusive; JSON overrides explicit flag defaults.
  --cache-dir DIR            Shared writable index/frequency cache
  Default cache: Skill-root/build/doc-cache; COMPARE_FIGURE_CACHE overrides it.
  Uses the same Docs options below; docs-only, no run/compile JSON dispatch.

Commands:
  compile, luadraw-compile  Compile a LuaDraw figure without a model/API key.
  docs, luadraw-doc-search  Search, read, or view the LuaDraw manual.

Compile options:
  --source PATH             LuaDraw .lua or supported .tex source (required)
  --out DIR                 Output directory (fixed per-working-directory temporary slot)
  --engine auto|lualatex|xelatex|pdflatex
  --include-image            Attach a PDF preview (default true)
  --inline-images            Include preview bytes in JSON
  --resource-root DIR        Runtime resource root
  --luadraw-root DIR         LuaDraw distribution root

Docs options:
  --drawing-mode luadraw|mixed|latex|asymptote
  --doc-source SOURCE        Filter the selected documentation source
  --query TEXT               Search query (required for search mode)
  --mode search|read|view|list|find|grep|frequent  Manual operation (default search)
  --doc PATH                 Document or directory path for read/list/view
  --page N                   PDF page for text read or image view
  --entry-id ID              Select a frequent paragraph by stable short/full ID
  --recursive                List nested directories (default direct children)
  --left/--top/--right/--bottom N   Crop bounds in page percentages
  --zoom N                   View zoom
  --limit N --offset N       Search pagination
  --start-line N --end-line N Read line range
  --snippet-chars N          Search snippet size (default 1200)
  --max-chars N              Search/frequent total character budget
  --line-chars N             Frequent line/snippet size
  --regex                    Treat grep query as a regular expression
  --include-image            Attach a relevant manual image (default false; search only)
  --inline-images            Include image bytes in JSON
  --cache-dir DIR            Documentation index cache
  --out DIR                  Output directory (fixed per-working-directory temporary slot)
  --resource-root DIR        Runtime resource root
  --luadraw-root DIR         LuaDraw distribution root

Use --help with either command for this usage. Results are JSON on stdout.
`

// manualCLI runs non-model LuaDraw compile and documentation operations.
func manualCLI(ctx context.Context, command string, args []string, in io.Reader, out io.Writer) int {
	command = strings.ToLower(command)
	if command == "luadraw-doc" {
		return docsServiceCLI(ctx, args, in, out)
	}
	compile := command == "compile" || command == "luadraw-compile"
	docs := command == "docs" || command == "luadraw-doc-search"
	if !compile && !docs {
		return writeError(out, fmt.Errorf("unknown manual command %q (use compile or docs)", command), 2)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var source, query, mode, doc, entryID, outDir, engine, resourceRoot, drawRoot, cacheDir, drawingMode, docSource string
	var page, zoom, limit, offset, startLine, endLine, snippetChars, maxChars, lineChars int
	var left, top, right, bottom float64
	var includeImage, inlineImages, regex, recursive bool
	includeImage = compile
	fs.StringVar(&source, "source", "", "LuaDraw source")
	fs.StringVar(&query, "query", "", "search query")
	fs.StringVar(&mode, "mode", "search", "search/read/view")
	fs.StringVar(&doc, "doc", "", "manual document")
	fs.StringVar(&entryID, "entry-id", "", "stable frequent paragraph ID")
	fs.BoolVar(&recursive, "recursive", false, "list nested directories")
	fs.StringVar(&outDir, "out", "", "output directory")
	fs.StringVar(&engine, "engine", "auto", "compiler engine")
	fs.StringVar(&drawingMode, "drawing-mode", "", "luadraw/mixed/latex/asymptote")
	fs.StringVar(&docSource, "doc-source", "", "documentation source ID")
	fs.StringVar(&resourceRoot, "resource-root", "", "runtime resources")
	fs.StringVar(&drawRoot, "luadraw-root", "", "LuaDraw distribution")
	fs.StringVar(&cacheDir, "cache-dir", "", "documentation cache")
	fs.IntVar(&page, "page", 0, "PDF page")
	fs.IntVar(&zoom, "zoom", 0, "view zoom")
	fs.IntVar(&limit, "limit", 0, "search result count")
	fs.IntVar(&offset, "offset", 0, "search result offset")
	fs.IntVar(&startLine, "start-line", 0, "first line")
	fs.IntVar(&endLine, "end-line", 0, "last line")
	fs.IntVar(&snippetChars, "snippet-chars", 0, "search snippet characters")
	fs.IntVar(&maxChars, "max-chars", 0, "total character budget")
	fs.IntVar(&lineChars, "line-chars", 0, "frequent line characters")
	fs.BoolVar(&regex, "regex", false, "grep regular expression")
	fs.Float64Var(&left, "left", 0, "crop left percentage")
	fs.Float64Var(&top, "top", 0, "crop top percentage")
	fs.Float64Var(&right, "right", 0, "crop right percentage")
	fs.Float64Var(&bottom, "bottom", 0, "crop bottom percentage")
	fs.BoolVar(&includeImage, "include-image", compile, "attach an image")
	fs.BoolVar(&inlineImages, "inline-images", false, "include image bytes")
	fs.Usage = func() { _, _ = io.WriteString(out, manualHelp) }
	if helpArg(args) {
		fs.Usage()
		return 0
	}
	ordered, err := flagsBeforePositionals(args, map[string]bool{"include-image": true, "inline-images": true, "regex": true, "recursive": true})
	if err != nil {
		return writeError(out, err, 2)
	}
	if err = fs.Parse(ordered); err != nil {
		if err == flag.ErrHelp {
			fs.Usage()
			return 0
		}
		return writeError(out, err, 2)
	}
	pos := fs.Args()
	if compile {
		if source == "" && len(pos) > 0 {
			source = pos[0]
			pos = pos[1:]
		}
		if len(pos) > 0 {
			return writeError(out, fmt.Errorf("unexpected arguments: %s", strings.Join(pos, " ")), 2)
		}
		if source == "" {
			return writeError(out, fmt.Errorf("compile requires --source PATH or a positional LuaDraw source"), 2)
		}
		request := map[string]any{"operation": "compile", "source": source, "out_dir": outDir, "include_image": includeImage, "inline_images": inlineImages, "resource_root": resourceRoot, "luadraw_root": drawRoot, "drawing_mode": drawingMode, "options": map[string]any{"engine": engine, "drawing_mode": drawingMode}}
		return callManualService(ctx, request, out)
	}
	if query == "" && len(pos) > 0 {
		query = pos[0]
		pos = pos[1:]
	}
	if len(pos) > 0 {
		return writeError(out, fmt.Errorf("unexpected arguments: %s", strings.Join(pos, " ")), 2)
	}
	mode = strings.ToLower(mode)
	if mode != "search" && mode != "read" && mode != "view" && mode != "list" && mode != "find" && mode != "grep" && mode != "frequent" {
		return writeError(out, fmt.Errorf("invalid docs mode %q; expected search, read, view, list, find, grep, or frequent", mode), 2)
	}
	if (mode == "search" || mode == "find" || mode == "grep") && strings.TrimSpace(query) == "" {
		return writeError(out, fmt.Errorf("docs search requires --query TEXT or a positional query"), 2)
	}
	if doc == "" && (mode == "read" || mode == "view") {
		doc = query
	}
	if (mode == "read" || mode == "view") && doc == "" {
		return writeError(out, fmt.Errorf("docs %s requires --doc PATH", mode), 2)
	}
	arguments := map[string]any{"mode": mode, "query": query, "doc": doc, "source": docSource, "include_image": includeImage, "regex": regex, "recursive": recursive, "entry_id": entryID}
	values := map[string]any{"page": page, "zoom": zoom, "limit": limit, "offset": offset, "start-line": startLine, "end-line": endLine, "snippet-chars": snippetChars, "max-chars": maxChars, "line-chars": lineChars, "left": left, "top": top, "right": right, "bottom": bottom}
	fs.Visit(func(f *flag.Flag) {
		if value, ok := values[f.Name]; ok {
			arguments[strings.ReplaceAll(f.Name, "-", "_")] = value
		}
	})
	request := map[string]any{"operation": "document_search", "drawing_mode": drawingMode, "out_dir": outDir, "resource_root": resourceRoot, "luadraw_root": drawRoot, "cache_dir": cacheDir, "inline_images": inlineImages, "arguments": arguments}
	return callManualService(ctx, request, out)
}

func callManualService(ctx context.Context, request map[string]any, out io.Writer) int {
	result, err := (&service{}).handle(ctx, request)
	if err != nil {
		return writeError(out, err, 2)
	}
	data, _ := result.(map[string]any)
	if data != nil && data["ok"] == false {
		emit(out, result)
		return 2
	}
	emit(out, result)
	return 0
}

func helpArg(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// Go's flag parser stops at the first positional argument; move positional values
// behind options while preserving option/value pairing so either order works.
func flagsBeforePositionals(args []string, boolFlags map[string]bool) ([]string, error) {
	flags, positional := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(token, "-") || token == "-" {
			positional = append(positional, token)
			continue
		}
		flags = append(flags, token)
		name, _, hasValue := strings.Cut(strings.TrimLeft(token, "-"), "=")
		if !hasValue && !boolFlags[name] {
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return append(flags, positional...), nil
}
