// Package render selects the permitted drawing language independently of the TeX engine.
package render

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"compare-figure/luadraw"
)

func NormalizeMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "luadraw"
	}
	switch mode {
	case "luadraw", "mixed", "latex", "asymptote":
		return mode, nil
	}
	return "", fmt.Errorf("invalid drawing_mode %q: expected luadraw, mixed, latex, or asymptote", mode)
}
func Mode(options map[string]any) (string, error) {
	return NormalizeMode(str(options, "drawing_mode", ""))
}
func DefaultSource(mode string) string {
	switch mode {
	case "asymptote":
		return "figure.asy"
	case "mixed", "latex":
		return "figure.tex"
	}
	return "figure.lua"
}
func SourceDescription(mode string) string {
	switch mode {
	case "asymptote":
		return "Asymptote .asy source or a .tex document containing an asy environment"
	case "latex":
		return "native LaTeX .tex source using TikZ, PGFPlots, picture or other installed TeX packages"
	case "mixed":
		return "editable LuaDraw .lua, Asymptote .asy, or LaTeX .tex; choose the most suitable drawing library"
	}
	return "LuaDraw .lua or .tex containing a genuine LuaDraw environment"
}
func str(m map[string]any, k, def string) string {
	if v, ok := m[k].(string); ok && v != "" {
		return v
	}
	return def
}
func failed(message string) map[string]any {
	return map[string]any{"ok": false, "error": message, "logs": []any{}, "generated_tikz": []string{}}
}

var luaEnv = regexp.MustCompile(`\\begin\s*\{(?:luadraw\*?|luacode\*?)\}|\\usepackage(?:\[[^]]*\])?\{luadraw\}`)
var asyEnv = regexp.MustCompile(`\\begin\s*\{asy\}`)

// Language identifies the source language and enforces the configured policy before invoking a compiler.
func Language(path, code, mode string) (string, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	lang := ""
	switch strings.ToLower(filepath.Ext(path)) {
	case ".lua":
		lang = "luadraw"
	case ".asy":
		lang = "asymptote"
	case ".tex":
		hasLua, hasAsy := luaEnv.MatchString(code), asyEnv.MatchString(code)
		if hasLua && hasAsy {
			return "", fmt.Errorf("one figure cannot combine LuaDraw and Asymptote environments; select a single source backend")
		}
		if hasLua {
			lang = "luadraw"
		} else if hasAsy {
			lang = "asymptote"
		} else {
			lang = "latex"
		}
	default:
		return "", fmt.Errorf("drawing source must end in .lua, .asy or .tex")
	}
	if mode != "mixed" && mode != lang {
		return "", fmt.Errorf("drawing mode %s rejects %s source; expected %s", mode, lang, SourceDescription(mode))
	}
	return lang, nil
}

func Probe(ctx context.Context, root, mode string) (map[string]any, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return nil, err
	}
	if mode == "luadraw" {
		r, e := luadraw.Probe(ctx, filepath.Join(root, "vendor", "luadraw-3.7", "luadraw"))
		if r != nil {
			r["drawing_mode"] = mode
		}
		return r, e
	}
	tools := map[string]any{}
	texOK := false
	for _, n := range []string{"lualatex", "xelatex", "pdflatex", "asy"} {
		p, _ := exec.LookPath(n)
		usable := false
		log := ""
		if p != "" {
			c, cancel := context.WithTimeout(ctx, 8*time.Second)
			b, e := exec.CommandContext(c, p, "--version").CombinedOutput()
			cancel()
			usable = e == nil
			log = string(b)
			if len(log) > 2000 {
				log = log[:2000]
			}
		}
		tools[n] = map[string]any{"path": p, "usable": usable, "version": log}
		if n != "asy" && usable {
			texOK = true
		}
	}
	asyOK := tools["asy"].(map[string]any)["usable"] == true
	usable := texOK
	if mode == "asymptote" {
		usable = texOK && asyOK
	}
	return map[string]any{"drawing_mode": mode, "usable": usable, "tools": tools, "latex_usable": texOK, "asymptote_usable": asyOK}, nil
}

func Compile(ctx context.Context, source, workdir string, options map[string]any) (map[string]any, error) {
	mode, err := Mode(options)
	if err != nil {
		return failed(err.Error()), nil
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return failed(err.Error()), nil
	}
	lang, err := Language(source, string(raw), mode)
	if err != nil {
		return failed(err.Error()), nil
	}
	if lang == "luadraw" {
		r, e := luadraw.Compile(ctx, source, workdir, options)
		if r != nil {
			r["source_language"] = lang
			r["drawing_mode"] = mode
		}
		return r, e
	}
	result := failed("")
	result["source_language"] = lang
	result["drawing_mode"] = mode
	result["backend"] = lang
	src, err := filepath.Abs(source)
	if err != nil {
		return failed(err.Error()), nil
	}
	work, err := filepath.Abs(workdir)
	if err != nil {
		return failed(err.Error()), nil
	}
	if err = os.MkdirAll(work, 0700); err != nil {
		return failed(err.Error()), nil
	}
	name := str(options, "output_name", "figure")
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(name) {
		return failed("invalid output_name"), nil
	}
	// Reuse a private deterministic compilation slot. Parallel tasks must use
	// different workdirs, as with the existing standalone compile interface.
	stage := filepath.Join(work, ".render-"+name)
	if info, e := os.Lstat(stage); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return failed("render output directory must not be a symlink"), nil
	}
	if err = os.MkdirAll(stage, 0700); err != nil {
		return failed(err.Error()), nil
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return failed(err.Error()), nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		candidate := filepath.Join(stage, entry.Name())
		// Only direct files in this renderer-owned directory may be removed;
		// retain the input itself when recompiling a generated wrapper.
		if filepath.Dir(candidate) != stage || candidate == src {
			continue
		}
		if err = os.Remove(candidate); err != nil {
			return failed(err.Error()), nil
		}
	}
	pdf := filepath.Join(stage, name+".pdf")
	result["pdf"] = pdf
	result["path"] = pdf
	seconds := 120
	for _, k := range []string{"timeout", "compile_timeout"} {
		switch v := options[k].(type) {
		case int:
			if v > 0 {
				seconds = v
			}
		case float64:
			if v > 0 {
				seconds = int(v)
			}
		}
	}
	env := append([]string{}, os.Environ()...)
	asyHome := filepath.Join(work, ".asy-cache")
	if err = os.MkdirAll(asyHome, 0700); err != nil {
		return failed(err.Error()), nil
	}
	env = setEnv(env, "ASYMPTOTE_HOME", asyHome)
	env = setEnv(env, "ASYMPTOTE_DIR", filepath.Dir(src)+string(os.PathListSeparator)+os.Getenv("ASYMPTOTE_DIR"))
	// Use an isolated, consistent TeX cache and search local source dependencies.
	cache := filepath.Join(work, ".tex-cache")
	if err = os.MkdirAll(cache, 0700); err != nil {
		return failed(err.Error()), nil
	}
	for k, v := range map[string]string{"TEXMFVAR": cache, "TEXMFCACHE": cache, "TEXINPUTS": filepath.Dir(src) + string(filepath.Separator) + "//" + string(os.PathListSeparator) + os.Getenv("TEXINPUTS")} {
		env = setEnv(env, k, v)
	}
	if root := str(options, "resource_root", ""); root != "" {
		files := filepath.Join(root, "vendor", "luadraw-3.7", "luadraw", "files")
		env = setEnv(env, "TEXINPUTS", files+"//"+string(os.PathListSeparator)+envValue(env, "TEXINPUTS"))
	}
	logs := []any{}
	execute := func(bin string, args ...string) bool {
		c, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(c, bin, args...)
		cmd.Dir = stage
		cmd.Env = env
		b, e := cmd.CombinedOutput()
		log := string(b)
		if len(log) > 12000 {
			log = log[len(log)-12000:]
		}
		code := 0
		if e != nil {
			code = 1
			if x, ok := e.(*exec.ExitError); ok {
				code = x.ExitCode()
			}
			log += "\n" + e.Error()
		}
		logs = append(logs, map[string]any{"engine": bin, "returncode": code, "log": log})
		result["logs"] = logs
		if e != nil {
			result["error"] = fmt.Sprintf("%s failed:\n%s", bin, log)
			return false
		}
		return true
	}
	engine := str(options, "engine", "auto")
	if engine == "auto" {
		engine = ""
		for _, n := range []string{"lualatex", "xelatex", "pdflatex"} {
			if p, _ := exec.LookPath(n); p != "" {
				engine = n
				break
			}
		}
	}
	if !map[string]bool{"lualatex": true, "xelatex": true, "pdflatex": true}[engine] {
		result["error"] = "a usable lualatex, xelatex or pdflatex engine is required"
		return result, nil
	}
	result["engine"] = engine
	if lang == "asymptote" && strings.EqualFold(filepath.Ext(src), ".asy") {
		asy := filepath.Join(stage, name+".asy")
		if err = os.WriteFile(asy, raw, 0600); err != nil {
			return failed(err.Error()), nil
		}
		if !execute("asy", "-noV", "-f", "pdf", "-tex", engine, "-o", name, filepath.Base(asy)) {
			return result, nil
		}
		result["wrapper"] = asy
	} else {
		code := string(raw)
		if !strings.Contains(code, `\documentclass`) {
			code = wrapTeX(code, options, lang)
		}
		tex := filepath.Join(stage, name+".tex")
		if err = os.WriteFile(tex, []byte(code), 0600); err != nil {
			return failed(err.Error()), nil
		}
		result["wrapper"] = tex
		texRun := func() bool {
			return execute(engine, "-no-shell-escape", "-interaction=nonstopmode", "-halt-on-error", "-jobname="+name, tex)
		}
		if !texRun() {
			return result, nil
		}
		if lang == "asymptote" {
			files, e := filepath.Glob(filepath.Join(stage, name+"-*.asy"))
			if e != nil {
				return failed(e.Error()), nil
			}
			if len(files) == 0 {
				result["error"] = "TeX produced no Asymptote figure sources; use the asymptote package and an asy environment"
				return result, nil
			}
			for _, f := range files {
				if !execute("asy", "-noV", "-f", "pdf", "-tex", engine, filepath.Base(f)) {
					return result, nil
				}
			}
			if !texRun() {
				return result, nil
			}
		}
	}
	if info, e := os.Stat(pdf); e != nil || info.Size() == 0 {
		result["error"] = "compiler did not produce a nonempty PDF"
		return result, nil
	}
	result["ok"] = true
	result["error"] = nil
	return result, nil
}
func envValue(env []string, key string) string {
	for _, v := range env {
		if strings.HasPrefix(v, key+"=") {
			return strings.TrimPrefix(v, key+"=")
		}
	}
	return ""
}
func setEnv(env []string, k, v string) []string {
	r := []string{}
	for _, x := range env {
		if !strings.HasPrefix(x, k+"=") {
			r = append(r, x)
		}
	}
	return append(r, k+"="+v)
}
func wrapTeX(body string, options map[string]any, lang string) string {
	preamble := "\\usepackage{tikz,pgfplots}\n\\pgfplotsset{compat=1.18}\n"
	if lang == "asymptote" {
		preamble = "\\usepackage[inline]{asymptote}\n"
	}
	if regexp.MustCompile(`[\x{2e80}-\x{9fff}]`).MatchString(body) {
		preamble += "\\usepackage{ctex}\n"
	}
	for _, key := range []string{"tex_packages", "packages"} {
		switch list := options[key].(type) {
		case []any:
			for _, p := range list {
				if s, ok := p.(string); ok && regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(s) {
					preamble += "\\usepackage{" + s + "}\n"
				}
			}
		case []string:
			for _, p := range list {
				if regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(p) {
					preamble += "\\usepackage{" + p + "}\n"
				}
			}
		}
	}
	return "\\documentclass[border=2pt]{standalone}\n" + preamble + str(options, "preamble", "") + "\n\\begin{document}\n" + body + "\n\\end{document}\n"
}

// SelfContainedTeX retains the accepted editable language in the delivery document.
func SelfContainedTeX(code string, options map[string]any, sourceIsTeX bool, language string) string {
	if language == "luadraw" {
		return luadraw.SelfContainedTeX(code, options, sourceIsTeX)
	}
	if sourceIsTeX && strings.Contains(code, `\documentclass`) {
		return code
	}
	if language == "asymptote" && !sourceIsTeX {
		code = "\\begin{asy}\n" + code + "\n\\end{asy}"
	}
	return wrapTeX(code, options, language)
}
