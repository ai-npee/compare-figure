package distribution

import (
	cf "compare-figure"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FindRoot locates runtime resources from the canonical Skill file and runtime tree.
func FindRoot(explicit string) (string, error) {
	valid := func(path string) bool {
		for _, name := range []string{"SKILL.md", "prompts/luadraw_figure.system.md", "vendor/luadraw-3.7/luadraw/files/luadraw.sty", "scripts/luadraw_runtime.lua"} {
			if info, err := os.Stat(filepath.Join(path, filepath.FromSlash(name))); err != nil || info.IsDir() {
				return false
			}
		}
		return true
	}
	if explicit == "" {
		explicit = os.Getenv("COMPARE_FIGURE_ROOT")
	}
	if explicit != "" {
		absolute, e := filepath.Abs(explicit)
		if e != nil {
			return "", e
		}
		if !valid(absolute) {
			return "", fmt.Errorf("canonical SKILL.md or runtime resources missing under %s", absolute)
		}
		return absolute, nil
	}
	starts := []string{}
	if exe, err := os.Executable(); err == nil {
		if real, e := filepath.EvalSymlinks(exe); e == nil {
			exe = real
		}
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, e := os.Getwd(); e == nil {
		starts = append(starts, wd)
	}
	visited := map[string]bool{}
	for _, start := range starts {
		for n, p := 0, start; n < 8; n++ {
			if visited[p] {
				break
			}
			visited[p] = true
			if valid(p) {
				return p, nil
			}
			if valid(filepath.Join(p, "resources")) {
				return filepath.Join(p, "resources"), nil
			}
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			p = parent
		}
	}
	return "", fmt.Errorf("runtime resources not found; pass --resource-root or set COMPARE_FIGURE_ROOT")
}

type ExportOptions struct{ Root, Out, Version string }
type FileRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

func Export(ctx context.Context, options ExportOptions) (map[string]any, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return nil, err
	}
	skill, version, err := skillDocument(root, options.Version)
	if err != nil {
		return nil, err
	}
	if options.Out == "" {
		return nil, fmt.Errorf("export-skill requires --out")
	}
	out, err := filepath.Abs(options.Out)
	if err != nil {
		return nil, err
	}
	if out == root || within(out, root) || within(filepath.Join(root, "vendor"), out) || within(filepath.Join(root, "prompts"), out) || within(filepath.Join(root, "scripts"), out) {
		return nil, fmt.Errorf("export directory must be separate from source resources")
	}
	if info, e := os.Stat(out); e == nil && info.IsDir() {
		if err = filepath.WalkDir(out, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			relative, _ := filepath.Rel(out, path)
			parts := strings.Split(filepath.ToSlash(relative), "/")
			for _, part := range parts {
				if part == "bin" || part == "build" || part == "deps" || part == ".deps" || part == "testdata" {
					return fmt.Errorf("source-only export destination contains a forbidden path %s", path)
				}
			}
			if !d.IsDir() {
				name := strings.ToLower(d.Name())
				if strings.HasSuffix(name, ".py") || strings.HasSuffix(name, ".pyc") || strings.HasSuffix(name, ".pyo") || strings.HasSuffix(name, "_test.go") {
					return fmt.Errorf("source-only export destination contains a development artifact %s", path)
				}
				if d.Name() == "compare-figure" || d.Name() == "compare-figure.exe" {
					return fmt.Errorf("source-only export destination contains a native binary %s", path)
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	files := map[string][]byte{}
	modes := map[string]os.FileMode{}
	add := func(relative string, data []byte, mode os.FileMode) {
		files[filepath.ToSlash(relative)] = data
		modes[filepath.ToSlash(relative)] = mode
	}
	copyFile := func(from, to string, mode os.FileMode) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, e := os.ReadFile(from)
		if e == nil {
			add(to, data, mode)
		}
		return e
	}
	for _, directory := range []string{"prompts", "vendor/luadraw-3.7", "vendor/asymptote-3.16", "vendor/katex-0.18.2", "go"} {
		base := filepath.Join(root, filepath.FromSlash(directory))
		err = filepath.WalkDir(base, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				if path != base && (d.Name() == "build" || d.Name() == "deps" || d.Name() == ".deps" || d.Name() == "testdata") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in packaged resources: %s", path)
			}
			if directory == "go" {
				name := d.Name()
				if strings.HasSuffix(name, "_test.go") {
					return nil
				}
				if filepath.Ext(name) != ".go" && name != "go.mod" && name != "go.sum" {
					return nil
				}
			}
			relative, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			return copyFile(path, relative, 0644)
		})
		if err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"scripts/luadraw_runtime.lua", "scripts/export_skill.sh", "vendor/dependencies.lock.json", "LICENSE", "install.md", "viewer.html", "docs/viewer-upstream.json", "docs/luadraw-doc.md", "docs/drawing-modes.md", "docs/doc-library-compatibility.md", "docs/asymptote-upstream.json", "docs/katex-upstream.json", "docs/prompt-guide.md", "docs/viewer-math.md", "scripts/viewer_math.js", "examples/luadraw-doc.json.example"} {
		mode := os.FileMode(0644)
		if name == "scripts/export_skill.sh" {
			mode = 0755
		}
		if err = copyFile(filepath.Join(root, filepath.FromSlash(name)), name, mode); err != nil {
			return nil, err
		}
	}
	if err = copyFile(filepath.Join(root, "examples", "runner.yaml.example"), "examples/runner.yaml.example", 0644); err != nil {
		return nil, fmt.Errorf("copy redacted runner example: %w", err)
	}
	add("SKILL.md", skill, 0644)
	add("VERSION", []byte(version+"\n"), 0644)
	add("README.md", []byte("# compare-figure source Skill\n\nThis export contains production Go source only; it includes no compiled executable. Install Go 1.25+ from the official download page: https://go.dev/dl/. From this directory build the native executable with:\n\n    go -C go build -o ../build/compare-figure ./cmd/compare-figure\n\nCreate the build directory first if needed. Open viewer.html to browse saved sessions. Bundled reference resources include LuaDraw 3.7 and Asymptote 3.16 documentation and source examples; the provenance and license notices are retained beside those resources. Runtime resources are discovered from the canonical SKILL.md and resource tree.\n"), 0644)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	records := make([]FileRecord, 0, len(files))
	written, unchanged := 0, 0
	for _, name := range names {
		target := filepath.Join(out, filepath.FromSlash(name))
		data := files[name]
		sum := sha256.Sum256(data)
		records = append(records, FileRecord{name, hex.EncodeToString(sum[:]), len(data)})
		old, e := os.ReadFile(target)
		if e == nil && string(old) == string(data) {
			unchanged++
		} else {
			written++
		}
		if err = cf.AtomicWrite(target, data, modes[name]); err != nil {
			return nil, err
		}
		if err = os.Chmod(target, modes[name]); err != nil {
			return nil, err
		}
	}
	manifest := map[string]any{"schema": "compare-figure-skill/2", "distribution": "source-only", "runtime": "go", "go_version": "1.25+", "version": version, "modes": []string{"automatic", "semi-automatic"}, "default_min_overlap": .17, "default_max_scale_dev": .10, "files": records}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err = cf.AtomicWrite(filepath.Join(out, "manifest.json"), append(data, '\n'), 0644); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "runtime": "go", "distribution": "source-only", "version": version, "out_dir": out, "written": written, "unchanged": unchanged, "manifest": filepath.Join(out, "manifest.json")}, nil
}
func within(base, path string) bool {
	relative, e := filepath.Rel(base, path)
	return e == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
