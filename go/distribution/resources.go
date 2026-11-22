package distribution

import (
	cf "compare-figure"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// FindRoot locates movable runtime resources, never a compiled-in developer path.
func FindRoot(explicit string) (string, error) {
	valid := func(path string) bool {
		for _, name := range []string{"prompts/luadraw_figure.system.md", "vendor/luadraw-3.7/luadraw/files/luadraw.sty", "scripts/luadraw_runtime.lua"} {
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
			return "", fmt.Errorf("runtime resources missing under %s", absolute)
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

type ExportOptions struct{ Root, Out, BinariesDir, Version string }
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
	// Do not leave a second runtime choice in an old or user-populated export.
	if info, e := os.Stat(out); e == nil && info.IsDir() {
		if err = filepath.WalkDir(out, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() {
				ext := strings.ToLower(filepath.Ext(path))
				if ext == ".py" || ext == ".pyc" || ext == ".pyo" {
					return fmt.Errorf("export contains a Python artifact %s; choose a clean Go-only destination", path)
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
		if strings.HasPrefix(filepath.ToSlash(to), "bin/") {
			info, e := buildinfo.ReadFile(from)
			if e != nil || !strings.HasSuffix(info.Path, "/cmd/compare-figure") {
				return fmt.Errorf("not a compare-figure Go binary: %s", from)
			}
			settings := map[string]string{}
			for _, setting := range info.Settings {
				settings[setting.Key] = setting.Value
			}
			parts := strings.Split(filepath.ToSlash(to), "/")
			platform := settings["GOOS"] + "-" + settings["GOARCH"]
			if len(parts) != 3 || parts[1] != platform {
				return fmt.Errorf("binary target %s does not match export path %s", platform, to)
			}
		}
		data, e := os.ReadFile(from)
		if e == nil {
			add(to, data, mode)
		}
		return e
	}
	for _, directory := range []string{"prompts", "vendor/luadraw-3.7"} {
		base := filepath.Join(root, filepath.FromSlash(directory))
		err = filepath.WalkDir(base, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in packaged resources: %s", path)
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
	for _, name := range []string{"scripts/luadraw_runtime.lua", "vendor/dependencies.lock.json", "LICENSE"} {
		if err = copyFile(filepath.Join(root, filepath.FromSlash(name)), name, 0644); err != nil {
			return nil, err
		}
	}
	platforms := []string{}
	if options.BinariesDir != "" {
		err = filepath.WalkDir(options.BinariesDir, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() != "compare-figure" && d.Name() != "compare-figure.exe" {
				return nil
			}
			relative, e := filepath.Rel(options.BinariesDir, path)
			if e != nil {
				return e
			}
			if filepath.Dir(relative) == "." {
				return fmt.Errorf("binary must be nested by platform, e.g. linux-amd64/compare-figure")
			}
			platforms = append(platforms, filepath.ToSlash(filepath.Dir(relative)))
			return copyFile(path, filepath.Join("bin", relative), 0755)
		})
		if err != nil {
			return nil, err
		}
	} else {
		exe, e := os.Executable()
		if e != nil {
			return nil, e
		}
		platform := runtime.GOOS + "-" + runtime.GOARCH
		name := "compare-figure"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err = copyFile(exe, filepath.Join("bin", platform, name), 0755); err != nil {
			return nil, err
		}
		platforms = append(platforms, platform)
	}
	if len(platforms) == 0 {
		return nil, fmt.Errorf("no native binaries found")
	}
	sort.Strings(platforms)
	add("SKILL.md", skill, 0644)
	add("VERSION", []byte(version+"\n"), 0644)
	add("README.md", []byte("# compare-figure Go Skill — two modes\n\nSee SKILL.md for both supported work modes. Automatic: run with the supplied model, base URL and key. Semi-automatic: the current Agent writes LuaDraw and calls luadraw-doc-search/docs, compile, compare, or the complete persistent tool CLI; no second model/key is needed. Both default to 17% overlap and a 10% size-deviation limit.\n\nSelect the native binary under bin matching the host OS/architecture. Runtime resources are executable-relative; external Lua/TeX and Poppler are detected before drawing. Packaged platforms: "+strings.Join(platforms, ", ")+".\n\nCompare/docs/compile reuse a fixed temporary output slot per working directory when --out is omitted. Save final artifacts or choose --out before the next call; parallel jobs use distinct outputs. Automatic run workspaces resume accepted results without repeated model calls. Model credentials and Python prototypes are not exported.\n"), 0644)
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
	manifest := map[string]any{"schema": "compare-figure-skill/1", "runtime": "go", "version": version, "modes": []string{"automatic", "semi-automatic"}, "default_min_overlap": .17, "default_max_scale_dev": .10, "platforms": platforms, "files": records}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err = cf.AtomicWrite(filepath.Join(out, "manifest.json"), append(data, '\n'), 0644); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "runtime": "go", "version": version, "out_dir": out, "platforms": platforms, "written": written, "unchanged": unchanged, "manifest": filepath.Join(out, "manifest.json")}, nil
}
func within(base, path string) bool {
	relative, e := filepath.Rel(base, path)
	return e == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
