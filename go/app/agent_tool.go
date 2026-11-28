package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cf "compare-figure"
	"compare-figure/distribution"
	"compare-figure/drawing"
	"compare-figure/measurement"
	"compare-figure/render"
	"compare-figure/runner"
)

const agentStateName = ".compare-figure-agent.json"

type agentRecord struct {
	Version      int            `json:"version"`
	Reference    string         `json:"reference"`
	ReferenceSHA string         `json:"reference_sha256"`
	Context      string         `json:"context,omitempty"`
	SourceMD     string         `json:"source_md,omitempty"`
	Threshold    float64        `json:"threshold"`
	Options      map[string]any `json:"options"`
	Snapshot     map[string]any `json:"snapshot"`
}

// agentToolCLI executes one deterministic, model-free drawing harness tool call.
func agentToolCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("agent-tool", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	workspaceArg := fs.String("workspace", "", "workspace directory (created if needed)")
	originalArg := fs.String("original", "", "reference image (required on first call)")
	name := fs.String("name", "", "tool name")
	argsJSON := fs.String("args", "", "tool arguments as JSON object; '-' reads stdin")
	argsFile := fs.String("args-file", "", "read tool arguments JSON object from file")
	contextText := fs.String("context", "", "source context")
	sourceMD := fs.String("source-md", "", "Markdown source file for image context")
	imageRef := fs.String("image-ref", "", "reference name for the original image")
	imagesDir := fs.String("images-dir", "", "source image directory")
	sizesCSV := fs.String("sizes-csv", "", "reference sizes.csv (auto-detected beside image)")
	refMM := fs.String("reference-mm", "", "reference width,height in mm")
	threshold := fs.Float64("threshold", 17, "overlap threshold percent (0-100)")
	maxScale := fs.Float64("max-scale-dev", .1, "maximum size deviation")
	resourceRoot := fs.String("resource-root", "", "Go runtime resource root")
	luaDrawRoot := fs.String("luadraw-root", "", "LuaDraw distribution root")
	drawingMode := fs.String("drawing-mode", "", "luadraw/mixed/latex/asymptote")
	timeout := fs.Duration("timeout", 180*time.Second, "tool timeout")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			_, _ = io.WriteString(out, "Usage: compare-figure tool --workspace DIR --name TOOL [--original IMAGE on first call] [--args JSON | --args-file FILE] [--timeout 180s]\n")
			return 0
		}
		return agentError(out, err, 2)
	}
	if fs.NArg() != 0 {
		return agentError(out, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")), 2)
	}
	provided := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if math.IsNaN(*threshold) || math.IsInf(*threshold, 0) || *threshold < 0 || *threshold > 100 {
		return agentError(out, fmt.Errorf("--threshold must be a finite percentage from 0 to 100"), 2)
	}
	if math.IsNaN(*maxScale) || math.IsInf(*maxScale, 0) {
		return agentError(out, fmt.Errorf("--max-scale-dev must be finite"), 2)
	}
	if *workspaceArg == "" {
		return agentError(out, fmt.Errorf("--workspace DIR is required"), 2)
	}
	workspace, err := filepath.Abs(*workspaceArg)
	if err != nil {
		return agentError(out, err, 2)
	}
	if err = os.MkdirAll(workspace, 0755); err != nil {
		return agentError(out, err, 2)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return agentError(out, err, 2)
	}
	st, err := os.Stat(workspace)
	if err != nil || !st.IsDir() {
		return agentError(out, fmt.Errorf("--workspace must be an existing directory"), 2)
	}
	release, err := runner.LockWorkspace(workspace)
	if err != nil {
		return agentError(out, err, 2)
	}
	defer release()

	callArgs, err := readAgentArgs(*argsJSON, *argsFile, in)
	if err != nil {
		return agentError(out, err, 2)
	}
	statePath := filepath.Join(workspace, agentStateName)
	if info, e := os.Lstat(statePath); e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return agentError(out, fmt.Errorf("agent state must be a regular file"), 2)
	} else if e != nil && !os.IsNotExist(e) {
		return agentError(out, e, 2)
	}
	record, err := loadAgentRecord(statePath)
	if err != nil && !os.IsNotExist(err) {
		return agentError(out, err, 2)
	}
	first := os.IsNotExist(err)
	selectedMode := ""
	if first || provided["drawing-mode"] {
		selectedMode, err = render.NormalizeMode(*drawingMode)
		if err != nil {
			return agentError(out, err, 2)
		}
	}
	if !first && provided["drawing-mode"] {
		savedMode, modeErr := render.NormalizeMode(runner.Str(record.Options, "drawing_mode", "luadraw"))
		if modeErr != nil {
			return agentError(out, fmt.Errorf("stored drawing mode: %w", modeErr), 2)
		}
		if selectedMode != savedMode {
			return agentError(out, fmt.Errorf("workspace uses drawing mode %s; requested mode %s cannot reuse this state; choose a new --workspace", savedMode, selectedMode), 2)
		}
	}
	if first && *sourceMD != "" {
		var resolveErr error
		*originalArg, *imageRef, *imagesDir, *contextText, resolveErr = runner.ResolveMarkdown(*sourceMD, *imageRef, *originalArg, *imagesDir, *contextText)
		if resolveErr != nil {
			return agentError(out, resolveErr, 2)
		}
		provided["image-ref"], provided["images-dir"], provided["context"] = true, true, true
	}
	if first {
		if *originalArg == "" {
			return agentError(out, fmt.Errorf("--original IMAGE is required on the first workspace call"), 2)
		}
		reference, e := filepath.Abs(*originalArg)
		if e != nil {
			return agentError(out, e, 2)
		}
		refHash, e := hashAgentFile(reference)
		if e != nil {
			return agentError(out, e, 2)
		}
		if _, e = cf.DecodeImage(reference); e != nil {
			return agentError(out, fmt.Errorf("invalid reference image: %w", e), 2)
		}
		resources, e := distribution.FindRoot(*resourceRoot)
		if e != nil {
			return agentError(out, e, 2)
		}
		imageName := "images/" + filepath.Base(reference)
		imageDirectory := filepath.Dir(reference)
		if *imagesDir != "" {
			imageDirectory, e = filepath.Abs(*imagesDir)
			if e != nil {
				return agentError(out, e, 2)
			}
		}
		options := map[string]any{"max_scale_dev": *maxScale, "drawing_mode": selectedMode, "resource_root": resources, "images_dir": imageDirectory, "image_source_dir": filepath.Dir(reference), "current_img": imageName, "md_content": *contextText + "\n![](" + imageName + ")\n"}
		if *sourceMD != "" {
			mdPath, e := filepath.Abs(*sourceMD)
			if e != nil {
				return agentError(out, e, 2)
			}
			options["image_markdown_dir"] = filepath.Dir(mdPath)
			paths, e := runner.MarkdownImagePaths(mdPath, imageDirectory)
			if e != nil {
				return agentError(out, e, 2)
			}
			options["image_references"] = paths
		}
		if *refMM != "" {
			dimensions, e := parseReferenceMM(*refMM)
			if e != nil {
				return agentError(out, e, 2)
			}
			options["reference_mm"] = dimensions
		}
		if *luaDrawRoot != "" {
			options["luadraw_root"] = *luaDrawRoot
		} else {
			options["luadraw_root"] = filepath.Join(resources, "vendor", "luadraw-3.7", "luadraw")
		}
		record = &agentRecord{Version: 1, Reference: reference, ReferenceSHA: refHash, Context: *contextText, Threshold: *threshold / 100, Options: options}
	} else {
		if record.Version != 1 || record.Snapshot == nil {
			return agentError(out, fmt.Errorf("unsupported or incomplete agent state"), 2)
		}
		if *originalArg != "" {
			given, e := filepath.Abs(*originalArg)
			if e != nil {
				return agentError(out, e, 2)
			}
			digest, e := hashAgentFile(given)
			if e != nil {
				return agentError(out, e, 2)
			}
			if given != record.Reference || digest != record.ReferenceSHA {
				return agentError(out, fmt.Errorf("workspace is bound to a different reference image"), 2)
			}
		}
	}
	if record.Options == nil {
		record.Options = map[string]any{}
	}
	if first || provided["drawing-mode"] {
		record.Options["drawing_mode"] = selectedMode
	} else if _, ok := record.Options["drawing_mode"]; !ok {
		record.Options["drawing_mode"] = "luadraw"
	}
	if provided["threshold"] {
		record.Threshold = *threshold / 100
	}
	if provided["context"] {
		record.Context = *contextText
	}
	if provided["max-scale-dev"] {
		record.Options["max_scale_dev"] = *maxScale
	}
	if provided["images-dir"] {
		directory := *imagesDir
		if directory == "" {
			directory = filepath.Dir(record.Reference)
		}
		directory, err = filepath.Abs(directory)
		if err != nil {
			return agentError(out, err, 2)
		}
		record.Options["images_dir"] = directory
	}
	if provided["reference-mm"] {
		if *refMM == "" {
			delete(record.Options, "reference_mm")
		} else {
			dimensions, e := parseReferenceMM(*refMM)
			if e != nil {
				return agentError(out, e, 2)
			}
			record.Options["reference_mm"] = dimensions
		}
	}
	if provided["image-ref"] {
		value := *imageRef
		if value == "" {
			value = filepath.Base(record.Reference)
		}
		record.Options["current_img"] = value
	}
	if provided["source-md"] {
		if *sourceMD == "" {
			record.SourceMD = ""
		} else {
			path, e := filepath.Abs(*sourceMD)
			if e != nil {
				return agentError(out, e, 2)
			}
			content, e := os.ReadFile(path)
			if e != nil {
				return agentError(out, e, 2)
			}
			record.SourceMD = path
			record.Options["md_content"] = string(content)
		}
	}
	if provided["resource-root"] {
		resources, e := distribution.FindRoot(*resourceRoot)
		if e != nil {
			return agentError(out, e, 2)
		}
		record.Options["resource_root"] = resources
		if !provided["luadraw-root"] {
			record.Options["luadraw_root"] = filepath.Join(resources, "vendor", "luadraw-3.7", "luadraw")
		}
	}
	if provided["luadraw-root"] {
		record.Options["luadraw_root"] = *luaDrawRoot
	}
	if record.SourceMD == "" {
		imageName := fmt.Sprint(record.Options["current_img"])
		record.Options["md_content"] = record.Context + "\n![](" + markdownImagePath(imageName) + ")\n"
	}
	currentHash, err := hashAgentFile(record.Reference)
	if err != nil {
		return agentError(out, err, 2)
	}
	if currentHash != record.ReferenceSHA {
		return agentError(out, fmt.Errorf("bound reference image changed since the workspace was initialized"), 2)
	}
	if *name == "" {
		return agentError(out, fmt.Errorf("--name TOOL is required"), 2)
	}
	if *name != "deliver" && !knownAgentTool(*name) {
		return agentError(out, fmt.Errorf("unknown tool %q", *name), 2)
	}
	if provided["sizes-csv"] {
		record.Options["sizes_csv"] = *sizesCSV
	}
	if first || provided["reference-mm"] || provided["sizes-csv"] {
		mm := []float64{}
		switch values := record.Options["reference_mm"].(type) {
		case []float64:
			mm = values
		case []any:
			for _, value := range values {
				if n, ok := value.(float64); ok {
					mm = append(mm, n)
				}
			}
		}
		csvPath, _ := record.Options["sizes_csv"].(string)
		measured, e := measurement.Measure(record.Reference, "", csvPath, mm)
		if e != nil {
			return agentError(out, e, 2)
		}
		raw, _ := json.Marshal(measured)
		saved := map[string]any{}
		_ = json.Unmarshal(raw, &saved)
		record.Options["measurement"] = saved
		if measured.Found {
			record.Options["reference_mm"] = []float64{measured.WidthMM, measured.HeightMM}
		}
	}
	h, err := drawing.New(workspace, record.Reference, record.Context, record.Threshold, *timeout, record.Options)
	if err != nil {
		return agentError(out, err, 2)
	}
	if !first {
		if err = h.Restore(record.Snapshot); err != nil {
			return agentError(out, err, 2)
		}
	}
	response := map[string]any{"ok": true, "text": "", "images": []cf.ImageArtifact{}, "state": h.Snapshot()}
	if *name == "deliver" {
		result, e := h.Deliver(ctx)
		if e != nil {
			response["ok"] = false
			response["text"] = e.Error()
		} else {
			data, _ := json.Marshal(result)
			response["text"] = string(data)
			if preview, ok := result["preview"].(string); ok && preview != "" {
				artifact, imageErr := cf.ImageArtifactFor(preview, true)
				if imageErr != nil {
					response["ok"] = false
					response["text"] = imageErr.Error()
				} else {
					response["images"] = []cf.ImageArtifact{artifact}
				}
			}
		}
	} else {
		text, image, execErr := h.Execute(ctx, *name, callArgs)
		if execErr != nil {
			response["ok"] = false
			text = execErr.Error()
		}
		if badAgentResult(text) {
			response["ok"] = false
		}
		response["text"] = text
		if image != "" {
			artifact, imageErr := cf.ImageArtifactFor(image, true)
			if imageErr != nil {
				response["ok"] = false
				response["text"] = text + "\nImage artifact error: " + imageErr.Error()
			} else {
				response["images"] = []cf.ImageArtifact{artifact}
			}
		}
	}
	response["state"] = h.Snapshot()
	record.Snapshot = h.Snapshot()
	if err = saveAgentRecord(statePath, record); err != nil {
		return agentError(out, err, 2)
	}
	emit(out, response)
	if response["ok"] == false {
		return 2
	}
	return 0
}

func knownAgentTool(name string) bool {
	switch name {
	case "write_file", "edit_file", "read_file", "grep", "compile", "view_pdf", "compare_figure", "submit", "image_context", "view_image", "luadraw_doc", "document_search", "document_note":
		return true
	default:
		return false
	}
}

func markdownImagePath(reference string) string {
	reference = filepath.ToSlash(strings.TrimPrefix(reference, "./"))
	if strings.HasPrefix(reference, "images/") {
		return reference
	}
	return "images/" + reference
}

func badAgentResult(text string) bool {
	trimmed := strings.TrimSpace(text)
	for _, prefix := range []string{"REJECTED:", "ERROR:", "ERROR ", "TOOL ERROR:", "OUT OF RANGE:", "COMPILE FAILED.", "COMPILEERROR:", "NOT FOUND:", "Unknown tool:"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func readAgentArgs(raw, path string, in io.Reader) (map[string]any, error) {
	if raw != "" && path != "" {
		return nil, fmt.Errorf("use either --args or --args-file, not both")
	}
	var data []byte
	var err error
	switch {
	case path != "":
		data, err = os.ReadFile(path)
	case raw == "-":
		data, err = io.ReadAll(in)
	case raw != "":
		data = []byte(raw)
	default:
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("tool arguments must be a JSON object: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("tool arguments must be a JSON object")
	}
	return result, nil
}

func loadAgentRecord(path string) (*agentRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record agentRecord
	if err = json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("invalid agent state: %w", err)
	}
	return &record, nil
}

func saveAgentRecord(path string, record *agentRecord) error {
	if filepath.Base(path) != agentStateName || !filepath.IsAbs(path) {
		return fmt.Errorf("invalid state file target")
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("agent state must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return cf.AtomicWrite(path, append(data, '\n'), 0600)
}

func hashAgentFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read reference image: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func parseReferenceMM(value string) ([]float64, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("--reference-mm requires WIDTH,HEIGHT")
	}
	var result []float64
	for _, part := range parts {
		n, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
			return nil, fmt.Errorf("invalid --reference-mm value %q", value)
		}
		result = append(result, n)
	}
	return result, nil
}

func agentError(out io.Writer, err error, code int) int {
	emit(out, map[string]any{"ok": false, "text": err.Error(), "images": []cf.ImageArtifact{}, "state": nil})
	return code
}
