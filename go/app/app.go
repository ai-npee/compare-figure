package app

import (
	"bufio"
	"bytes"
	cf "compare-figure"
	"compare-figure/distribution"
	"compare-figure/documents"
	"compare-figure/luadraw"
	"compare-figure/render"
	"compare-figure/runner"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const help = `compare-figure: native Go figure comparison and drawing runner

Drawing modes (run/compile/environment --drawing-mode): luadraw (default), mixed, latex, asymptote.
  Drawing mode selects the drawing language; --engine remains the TeX compiler.
  latex permits native TikZ/PGFPlots and other TeX packages; asymptote is Asymptote-only.

Modes:
  Automatic: run uses your model/base URL/key and draws through the runner.
  Semi-automatic: the current Agent writes editable vector sources and calls the tools below;
                 no drawing model/API key is needed. Default overlap: 17%.

Commands:
  run, auto           Autonomous session; identical reruns resume automatically
  compile             Compile an editable figure source (alias: luadraw-compile)
  docs, document-search  Search/read/view local drawing references (alias: doc)
  luadraw-doc          Legacy LuaDraw documentation JSON/JSONL service
  compare             Compare image/PDF: --reference REF --candidate PDF
  tool, agent-tool    Persist and execute any of the 11 drawing tools
  tools               JSON/JSONL service: compare, docs, compile, environment, run
  environment         Real LuaDraw/Lua/TeX toolchain detection
  export-skill        Idempotently export both work modes in the Go-only Skill
  build               Build binaries for requested OS/architecture targets

Examples:
  compare-figure run --original ref.png --out run-dir --model MODEL --base-url URL --api-key-env MODEL_KEY
  compare-figure luadraw-doc --request request.json
  compare-figure luadraw-doc --jsonl < requests.jsonl
  compare-figure luadraw-doc-search Dpolyline3d
  compare-figure compile figure.lua
  compare-figure compare --reference ref.png --candidate figure.pdf --threshold 17

Compare flags: --threshold PERCENT (17), --max-scale-dev FRACTION (0.10),
--detail, --crop L,T,R,B, --zoom, --worst-region, --reference-mm W,H,
--candidate-mm W,H, --inline-images, --out DIR. JSON: --stdin/--request/--jsonl.
Successful comparisons exit 0 even when pass=false; inspect pass and thresholds.

Omitting --out for compare/docs/compile reuses one temporary output slot per
working directory and operation. Save artifacts before the next call or use an
explicit output directory. Parallel tasks should use distinct --out directories.

Automatic run accepts --model, --base-url, --api, --api-key-env, --api-key-file,
--api-key, --config, --pass-threshold PERCENT, --resume and other session options.
Use --resource-root when resources are not beside the installed binary.
Run configuration: --config PATH > COMPARE_FIGURE_CONFIG > current-directory
runner.yaml/runner.yml/runner.json > the same names in the Skill resource root.
Select one file (no implicit merging). --config none disables file discovery.
Use run --check --original REF --out DIR to inspect config_path without contacting a model.
Independent compile/docs/compare/tool/environment commands do not load runner config.
`

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, help)
		return 0
	}
	command := args[0]
	rest := args[1:]
	if strings.HasPrefix(command, "-") {
		command = "compare"
		rest = args
	}
	switch command {
	case "tool", "agent-tool":
		return agentToolCLI(ctx, rest, in, out)
	case "compare", "tools":
		return jsonCLI(ctx, command, rest, in, out)
	case "compile", "luadraw-compile", "docs", "document-search", "doc", "luadraw-doc-search", "luadraw-doc", "doc-search":
		if command == "doc-search" || command == "document-search" || command == "doc" {
			command = "docs"
		}
		return manualCLI(ctx, command, rest, in, out)
	case "run", "auto":
		return runCLI(ctx, rest, in, out)
	case "environment":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		fs.SetOutput(errOut)
		resource := fs.String("resource-root", "", "runtime resources")
		root := fs.String("luadraw-root", "", "LuaDraw distribution")
		mode := fs.String("drawing-mode", "", "luadraw/mixed/latex/asymptote")
		if e := fs.Parse(rest); e != nil {
			return writeError(out, e, 2)
		}
		base, e := distribution.FindRoot(*resource)
		if e != nil {
			return writeError(out, e, 2)
		}
		customRoot := *root != ""
		if *root == "" {
			*root = filepath.Join(base, "vendor", "luadraw-3.7", "luadraw")
		}
		selected, e := render.NormalizeMode(*mode)
		if e != nil {
			return writeError(out, e, 2)
		}
		var result map[string]any
		if selected == "luadraw" && customRoot {
			result, e = luadraw.Probe(ctx, *root)
			if result != nil {
				result["drawing_mode"] = selected
			}
		} else {
			result, e = render.Probe(ctx, base, selected)
		}
		if e != nil {
			return writeError(out, e, 2)
		}
		emit(out, result)
		ready := runner.Bool(result, "ready", runner.Bool(result, "usable", false))
		if !ready {
			return 2
		}
		return 0
	case "export-skill":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		fs.SetOutput(errOut)
		resource := fs.String("resource-root", "", "runtime resources")
		target := fs.String("out", "", "export directory")
		version := fs.String("version", "", "release SemVer (default VERSION / SKILL metadata)")
		if e := fs.Parse(rest); e != nil {
			return writeError(out, e, 2)
		}
		result, e := distribution.Export(ctx, distribution.ExportOptions{Root: *resource, Out: *target, Version: *version})
		if e != nil {
			return writeError(out, e, 2)
		}
		emit(out, result)
		return 0
	case "build":
		return buildCLI(ctx, rest, out, errOut)
	default:
		return writeError(out, fmt.Errorf("unknown command %q", command), 2)
	}
}
func emit(out io.Writer, value any) { _ = json.NewEncoder(out).Encode(value) }
func writeError(out io.Writer, err error, code int) int {
	emit(out, map[string]any{"version": "compare-figure-tools/1", "ok": false, "error": err.Error()})
	return code
}

func runCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := runner.Options{}
	fs.StringVar(&opts.Original, "original", "", "reference image")
	fs.StringVar(&opts.Context, "context", "", "surrounding text")
	fs.StringVar(&opts.ContextFile, "context-file", "", "context text file")
	fs.StringVar(&opts.SourceMD, "source-md", "", "full Markdown")
	fs.StringVar(&opts.ImageRef, "image-ref", "", "Markdown reference")
	fs.StringVar(&opts.ImagesDir, "images-dir", "", "image directory")
	var referenceMM string
	fs.StringVar(&opts.SizesCSV, "sizes-csv", "", "reference sizes.csv (auto-detected beside the image)")
	fs.StringVar(&referenceMM, "reference-mm", "", "printed width,height in mm")
	fs.StringVar(&opts.Out, "out", "", "session directory")
	fs.StringVar(&opts.Config, "config", "", "JSON/YAML config")
	fs.StringVar(&opts.ResourceRoot, "resource-root", "", "runtime resources")
	fs.StringVar(&opts.LuaDrawRoot, "luadraw-root", "", "LuaDraw distribution")
	fs.StringVar(&opts.Model, "model", "", "model alias/name")
	fs.StringVar(&opts.BaseURL, "base-url", "", "model endpoint")
	fs.StringVar(&opts.API, "api", "", "openai/responses/anthropic")
	fs.StringVar(&opts.APIKeyEnv, "api-key-env", "", "key variable")
	fs.StringVar(&opts.APIKey, "api-key", "", "ephemeral API key; environment/file also supported")
	fs.Func("api-key-file", "read ephemeral API key from a local file", func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		opts.APIKey = strings.TrimSpace(string(data))
		if opts.APIKey == "" {
			return fmt.Errorf("API key file is empty")
		}
		return nil
	})
	fs.Func("pass-threshold", "overlap percentage", func(value string) error { n, e := strconv.ParseFloat(value, 64); opts.PassThreshold = &n; return e })
	fs.Func("max-scale-dev", "size deviation fraction", func(value string) error { n, e := strconv.ParseFloat(value, 64); opts.MaxScaleDev = &n; return e })
	fs.Func("accept-min-submits", "best-effort attempt minimum", func(value string) error { n, e := strconv.Atoi(value); opts.AcceptMinSubmits = &n; return e })
	rounds := func(value string) error { n, e := strconv.Atoi(value); opts.MaxTurns = &n; return e }
	fs.Func("max-turns", "tool round budget", rounds)
	fs.Func("max-tool-rounds", "tool round budget", rounds)
	fs.Func("max-tokens", "output token budget", func(value string) error { n, e := strconv.Atoi(value); opts.MaxTokens = &n; return e })
	fs.Float64Var(&opts.Timeout, "timeout", 180, "request timeout seconds")
	fs.StringVar(&opts.Engine, "engine", "", "auto/lualatex/xelatex/pdflatex")
	fs.StringVar(&opts.DrawingMode, "drawing-mode", "", "luadraw/mixed/latex/asymptote")
	fs.StringVar(&opts.OutputLang, "output-lang", "", "explanation language")
	fs.StringVar(&opts.CheckerModel, "checker-model", "", "checker model alias")
	fs.BoolVar(&opts.FigureCheck, "figure-check", false, "visual checker")
	fs.BoolVar(&opts.Resume, "resume", false, "reuse stored inputs")
	fs.BoolVar(&opts.Check, "check", false, "preflight without an API call or session files")
	environment := fs.Bool("environment", false, "environment report")
	request := fs.String("request", "", "JSON runner options")
	stdin := fs.Bool("stdin", false, "read JSON runner options")
	// flag supports one value; retain the prototype's --reference-mm WIDTH HEIGHT form.
	normalized := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--reference-mm" && i+2 < len(args) && !strings.Contains(args[i+1], ",") {
			normalized = append(normalized, "--reference-mm", args[i+1]+","+args[i+2])
			i += 2
		} else {
			normalized = append(normalized, args[i])
		}
	}
	if e := fs.Parse(normalized); e != nil {
		if e == flag.ErrHelp {
			fmt.Fprint(out, help)
			return 0
		}
		return writeError(out, e, 2)
	}
	if fs.NArg() != 0 {
		return writeError(out, fmt.Errorf("unexpected arguments: %v", fs.Args()), 2)
	}
	modeCLI := opts.DrawingMode
	if *request != "" || *stdin {
		reader := in
		if *request != "" {
			f, e := os.Open(*request)
			if e != nil {
				return writeError(out, e, 2)
			}
			defer f.Close()
			reader = f
		}
		if e := json.NewDecoder(reader).Decode(&opts); e != nil {
			return writeError(out, e, 2)
		}
		if modeCLI != "" {
			opts.DrawingMode = modeCLI
		}
	}
	if referenceMM != "" {
		parts := strings.Split(referenceMM, ",")
		if len(parts) != 2 {
			return writeError(out, fmt.Errorf("reference-mm requires two dimensions"), 2)
		}
		for _, part := range parts {
			n, e := strconv.ParseFloat(part, 64)
			if e != nil {
				return writeError(out, e, 2)
			}
			opts.ReferenceMM = append(opts.ReferenceMM, n)
		}
	}
	if *environment {
		values := []string{}
		if opts.ResourceRoot != "" {
			values = append(values, "--resource-root", opts.ResourceRoot)
		}
		if opts.LuaDrawRoot != "" {
			values = append(values, "--luadraw-root", opts.LuaDrawRoot)
		}
		mode := opts.DrawingMode
		root, e := distribution.FindRoot(opts.ResourceRoot)
		if e != nil {
			return writeError(out, e, 2)
		}
		path, e := runner.ResolveConfigPath(opts.Config, root)
		if e != nil {
			return writeError(out, e, 2)
		}
		config, e := runner.LoadConfig(path)
		if e != nil {
			return writeError(out, e, 2)
		}
		if mode == "" {
			mode = runner.Str(runner.Obj(config, "drawing"), "mode", "")
		}
		if mode != "" {
			values = append(values, "--drawing-mode", mode)
		}
		return Run(ctx, append([]string{"environment"}, values...), in, out, io.Discard)
	}
	result, e := runner.Run(ctx, opts)
	if e != nil {
		return writeError(out, e, 2)
	}
	emit(out, result)
	if result["ok"] == false {
		return 2
	}
	return 0
}

type service struct {
	resources string
	docs      map[string]*documents.Docs
}

func (s *service) root(explicit string) (string, error) {
	if explicit != "" {
		return distribution.FindRoot(explicit)
	}
	if s.resources != "" {
		return distribution.FindRoot(s.resources)
	}
	return distribution.FindRoot("")
}
func (s *service) handle(ctx context.Context, request map[string]any) (any, error) {
	operation := runner.Str(request, "operation", "compare")
	switch operation {
	case "compare":
		var req cf.Request
		data, _ := json.Marshal(request)
		if e := json.Unmarshal(data, &req); e != nil {
			return nil, e
		}
		if req.Version == "" {
			req.Version = "compare-figure/1"
		}
		if md := runner.Str(request, "source_md", ""); md != "" {
			reference, _, _, _, err := runner.ResolveMarkdown(md, runner.Str(request, "image_ref", ""), req.Reference, "", "")
			if err != nil {
				return nil, err
			}
			req.Reference = reference
		}
		return cf.ProcessRequest(ctx, req)
	case "run":
		arguments := request
		if value, ok := request["arguments"].(map[string]any); ok {
			arguments = value
		}
		var opts runner.Options
		data, _ := json.Marshal(arguments)
		if e := json.Unmarshal(data, &opts); e != nil {
			return nil, e
		}
		if mode := runner.Str(request, "drawing_mode", ""); mode != "" {
			opts.DrawingMode = mode
		}
		if opts.ResourceRoot == "" {
			opts.ResourceRoot = s.resources
		}
		return runner.Run(ctx, opts)
	case "environment":
		root, e := s.root(runner.Str(request, "resource_root", ""))
		if e != nil {
			return nil, e
		}
		return render.Probe(ctx, root, runner.Str(request, "drawing_mode", ""))
	case "docs", "document_search", "compile":
		root, e := s.root(runner.Str(request, "resource_root", ""))
		if e != nil {
			return nil, e
		}
		target := runner.Str(request, "out_dir", "")
		if target == "" {
			operationName := operation
			if operationName == "document_search" {
				operationName = "docs"
			}
			target, e = cf.DefaultOutput(operationName)
			if e != nil {
				return nil, e
			}
		}
		target, e = filepath.Abs(target)
		if e != nil {
			return nil, e
		}
		if e = os.MkdirAll(target, 0755); e != nil {
			return nil, e
		}
		inline := runner.Bool(request, "inline_images", false)
		if operation == "docs" || operation == "document_search" {
			arguments := request
			if value, ok := request["arguments"].(map[string]any); ok {
				arguments = value
			}
			mode := runner.Str(request, "drawing_mode", runner.Str(arguments, "drawing_mode", "luadraw"))
			mode, e = render.NormalizeMode(mode)
			if e != nil {
				return nil, e
			}
			cache := runner.Str(request, "cache_dir", luadraw.CacheDirectory(root))
			library := runner.Str(request, "luadraw_root", "")
			resource := root
			if mode == "luadraw" && library != "" {
				resource = library
			}
			key := resource + "\n" + cache + "\n" + mode
			if s.docs == nil {
				s.docs = map[string]*documents.Docs{}
			}
			tool := s.docs[key]
			if tool == nil {
				tool, e = documents.New(resource, cache, mode)
				if e != nil {
					return nil, e
				}
				s.docs[key] = tool
			}
			args := runner.Clone(arguments)
			if _, nested := request["arguments"].(map[string]any); !nested {
				for _, key := range []string{"operation", "id", "request_id", "out_dir", "resource_root", "luadraw_root", "cache_dir", "inline_images", "drawing_mode", "doc_source"} {
					delete(args, key)
				}
			}
			delete(args, "drawing_mode")
			if source := runner.Str(arguments, "doc_source", ""); source != "" {
				args["source"] = source
			}
			delete(args, "doc_source")
			response, e := tool.ExecuteResponse(ctx, args, target)
			if e != nil {
				return nil, e
			}
			text := runner.Str(response, "text", "")
			documentImages, _ := response["images"].([]any)
			images := []cf.ImageArtifact{}
			for _, item := range documentImages {
				image, ok := item.(map[string]any)
				if !ok {
					continue
				}
				imagePath := runner.Str(image, "path", "")
				if imagePath == "" {
					continue
				}
				imagePath, e = retainDocumentImage(target, imagePath)
				if e != nil {
					return nil, e
				}
				image["path"] = imagePath
				artifact, e := cf.ImageArtifactFor(imagePath, inline)
				if e != nil {
					return nil, e
				}
				images = append(images, artifact)
			}
			response["document_images"] = documentImages
			response["version"], response["ok"], response["text"], response["images"], response["out_dir"] = "compare-figure-tools/1", true, text, images, target
			return response, nil
		}
		options := runner.Clone(runner.Obj(request, "options"))
		if mode := runner.Str(request, "drawing_mode", ""); mode != "" {
			options["drawing_mode"] = mode
		} else if _, ok := options["drawing_mode"]; !ok {
			options["drawing_mode"] = ""
		}
		if _, ok := options["resource_root"]; !ok {
			options["resource_root"] = root
		}
		if _, ok := options["luadraw_root"]; !ok {
			options["luadraw_root"] = runner.Str(request, "luadraw_root", filepath.Join(root, "vendor", "luadraw-3.7", "luadraw"))
		}
		if _, ok := options["helper_path"]; !ok {
			options["helper_path"] = filepath.Join(root, "scripts", "luadraw_runtime.lua")
		}
		result, e := render.Compile(ctx, runner.Str(request, "source", ""), target, options)
		if e != nil {
			return result, e
		}
		result["version"], result["operation"], result["out_dir"], result["images"] = "compare-figure-tools/1", "compile", target, []cf.ImageArtifact{}
		if runner.Bool(result, "ok", false) && runner.Bool(request, "include_image", true) {
			prefix := filepath.Join(target, "预览", "compiled")
			if e = os.MkdirAll(filepath.Dir(prefix), 0755); e != nil {
				return nil, e
			}
			cmd := exec.CommandContext(ctx, "pdftoppm", "-singlefile", "-png", "-r", fmt.Sprint(runner.Num(request, "raster_dpi", 110)), runner.Str(result, "pdf", ""), prefix)
			output, e := cmd.CombinedOutput()
			if e != nil {
				result["warnings"] = []string{fmt.Sprintf("PDF retained; preview failed: %v %s", e, string(output))}
				return result, nil
			}
			artifact, e := cf.ImageArtifactFor(prefix+".png", inline)
			if e != nil {
				return nil, e
			}
			result["images"] = []cf.ImageArtifact{artifact}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown operation %s", operation)
	}
}

func jsonCLI(ctx context.Context, command string, args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	request := fs.String("request", "", "JSON file")
	_ = fs.Bool("stdin", false, "JSON stdin")
	jsonl := fs.Bool("jsonl", false, "JSONL service")
	outDir := fs.String("out", "", "output directory")
	original := fs.String("original", "", "reference image")
	fs.StringVar(original, "reference", "", "reference image (alias)")
	candidate := fs.String("pdf", "", "candidate PDF/image")
	fs.StringVar(candidate, "candidate", "", "candidate PDF/image (alias)")
	threshold := 17.0
	fs.Float64Var(&threshold, "threshold", 17, "minimum overlap PERCENT (default 17)")
	fs.Float64Var(&threshold, "pass-threshold", 17, "minimum overlap PERCENT (alias)")
	maxScale := fs.Float64("max-scale-dev", .1, "maximum size deviation fraction; negative disables")
	detail := fs.String("detail", "", "evidence channels: all,gray,r,g,b,rg,rb,gb")
	zoom := fs.Int("zoom", 0, "evidence target width")
	cropText := fs.String("crop", "", "left,top,right,bottom percentages")
	worst := fs.Bool("worst-region", false, "show worst region after full-image scoring")
	compareMD := fs.String("source-md", "", "Markdown containing reference image")
	compareRef := fs.String("image-ref", "", "selected Markdown image link or unique basename")
	sizesCSV := fs.String("sizes-csv", "", "reference sizes.csv; defaults to nearby sizes.csv")
	referenceMM := fs.String("reference-mm", "", "printed width,height millimetres")
	candidateMM := fs.String("candidate-mm", "", "printed width,height millimetres")
	inline := fs.Bool("inline-images", false, "include base64 image bytes")
	dpi := fs.Int("dpi", 110, "raster DPI")
	resources := fs.String("resource-root", "", "runtime resources")
	if e := fs.Parse(args); e != nil {
		if e == flag.ErrHelp {
			fmt.Fprint(out, help)
			return 0
		}
		return writeError(out, e, 2)
	}
	if fs.NArg() != 0 {
		return writeError(out, fmt.Errorf("unexpected arguments: %v", fs.Args()), 2)
	}
	provided := map[string]bool{}
	fs.Visit(func(flag *flag.Flag) { provided[flag.Name] = true })
	applyComparison := func(req *cf.Request) error {
		if *compareMD != "" {
			reference, _, _, _, err := runner.ResolveMarkdown(*compareMD, *compareRef, req.Reference, "", "")
			if err != nil {
				return err
			}
			req.Reference = reference
		}
		if provided["sizes-csv"] {
			req.SizesCSV = *sizesCSV
		}
		if provided["threshold"] || provided["pass-threshold"] {
			req.PassOverlap = threshold / 100
			if threshold == 0 {
				req.PassOverlap = -1
			}
		}
		if provided["max-scale-dev"] {
			req.PassScale = *maxScale
			if *maxScale == 0 {
				req.PassScale = -1
			}
		}
		if provided["detail"] {
			req.Detail = *detail
		}
		if provided["zoom"] {
			req.Zoom = *zoom
		}
		if provided["worst-region"] {
			req.WorstRegion = *worst
		}
		if provided["inline-images"] {
			req.InlineImages = *inline
		}
		if provided["dpi"] {
			req.RasterDPI = *dpi
		}
		for _, entry := range []struct {
			name, text string
			target     []float64
		}{{"reference-mm", *referenceMM, req.ReferenceMM[:]}, {"candidate-mm", *candidateMM, req.CandidateMM[:]}, {"crop", *cropText, req.Crop[:]}} {
			if entry.text != "" {
				parts := strings.Split(entry.text, ",")
				if len(parts) != len(entry.target) {
					return fmt.Errorf("%s requires %d comma-separated dimensions", entry.name, len(entry.target))
				}
				for index, part := range parts {
					number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
					if err != nil {
						return fmt.Errorf("%s: %w", entry.name, err)
					}
					entry.target[index] = number
				}
			}
		}
		return nil
	}
	svc := &service{resources: *resources}
	handle := func(raw []byte) bool {
		var object map[string]any
		if e := json.Unmarshal(raw, &object); e != nil {
			writeError(out, e, 1)
			return false
		}
		if object == nil {
			writeError(out, fmt.Errorf("request must be an object"), 1)
			return false
		}
		if _, ok := object["out_dir"]; !ok && *outDir != "" {
			object["out_dir"] = *outDir
		}
		var result any
		var err error
		if command == "compare" {
			var req cf.Request
			err = json.Unmarshal(raw, &req)
			if err == nil {
				if req.Version == "" {
					req.Version = "compare-figure/1"
				}
				if req.OutDir == "" {
					req.OutDir = *outDir
				}
				err = applyComparison(&req)
				if err == nil {
					result, err = cf.ProcessRequest(ctx, req)
				}
			}
		} else {
			result, err = svc.handle(ctx, object)
		}
		if err != nil {
			failure := map[string]any{"version": "compare-figure-tools/1", "ok": false, "error": err.Error()}
			if id, ok := object["id"]; ok {
				failure["id"] = id
			}
			emit(out, failure)
			return false
		}
		if id, ok := object["id"]; ok {
			data, _ := json.Marshal(result)
			var response map[string]any
			_ = json.Unmarshal(data, &response)
			response["id"] = id
			result = response
		}
		emit(out, result)
		data, _ := json.Marshal(result)
		var response map[string]any
		_ = json.Unmarshal(data, &response)
		return response["ok"] != false
	}
	if *original != "" || *candidate != "" {
		data, _ := json.Marshal(cf.Request{Version: "compare-figure/1", Reference: *original, Candidate: *candidate, OutDir: *outDir, RasterDPI: *dpi})
		if handle(data) {
			return 0
		}
		return 2
	}
	reader := in
	if *request != "" && *request != "-" {
		file, e := os.Open(*request)
		if e != nil {
			return writeError(out, e, 2)
		}
		defer file.Close()
		reader = file
	}
	if *jsonl {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 32*1024*1024)
		failed := false
		for scanner.Scan() {
			if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
				continue
			}
			if !handle(scanner.Bytes()) {
				failed = true
			}
		}
		if e := scanner.Err(); e != nil {
			return writeError(out, e, 1)
		}
		if failed {
			return 1
		}
		return 0
	}
	data, e := io.ReadAll(reader)
	if e != nil {
		return writeError(out, e, 2)
	}
	if handle(data) {
		return 0
	}
	return 2
}
