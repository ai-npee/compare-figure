from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any

PROJECT_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_ROOT = PROJECT_ROOT / "vendor" / "luadraw-3.7" / "luadraw"
RUNNER = PROJECT_ROOT / "scripts" / "luadraw_runtime.lua"
_PROBE_CACHE: dict[str, dict[str, Any]] = {}


def _run(argv: list[str], *, cwd: Path, env: dict[str, str], timeout: int) -> tuple[int, str]:
    try:
        result = subprocess.run(argv, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, timeout=timeout, check=False)
        return result.returncode, result.stdout
    except subprocess.TimeoutExpired as exc:
        output = exc.stdout or ""
        if isinstance(output, bytes):
            output = output.decode(errors="replace")
        return 124, str(output) + f"\nTimed out after {timeout}s"
    except OSError as exc:
        return 127, str(exc)


def _version(binary: str, env: dict[str, str]) -> str:
    exe = shutil.which(binary, path=env.get("PATH"))
    if not exe:
        return ""
    try:
        result = subprocess.run([exe, "--version"], text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=5, check=False, env=env)
        return (result.stdout.splitlines() or [""])[0].strip()
    except (OSError, subprocess.TimeoutExpired):
        return ""


def _tex_env(root: Path) -> dict[str, str]:
    env = os.environ.copy()
    files = root / "files"
    env["TEXINPUTS"] = str(files) + "//:" + env.get("TEXINPUTS", "")
    env["LUAINPUTS"] = str(files) + "//:" + env.get("LUAINPUTS", "")
    return env


def _luadraw_version(root: Path) -> str | None:
    sty = next((path for path in (root / "files" / "luadraw.sty", root / "luadraw.sty") if path.is_file()), None)
    if sty:
        with sty.open(encoding="utf-8", errors="replace") as stream:
            first_line = stream.readline()
        match = re.search(r"luadraw\.sty\s+version\s+([\d.]+)", first_line, re.I)
        if match:
            return match.group(1)
    readme = root / "README.md"
    if readme.is_file():
        match = re.search(r"Package Version:\s*\*\*([\d.]+)\*\*", readme.read_text(encoding="utf-8", errors="replace"), re.I)
        if match:
            return match.group(1)
    return None


def _find_lua_files(root: Path) -> Path | None:
    for candidate in (root / "files", root):
        if (candidate / "luadraw_graph2d.lua").is_file() and (candidate / "luadraw_base.lua").is_file():
            return candidate
    return None


def _usable_lualatex(root: Path, env: dict[str, str]) -> tuple[bool, str]:
    exe = shutil.which("lualatex", path=env.get("PATH"))
    if not exe:
        return False, "lualatex not found"
    with tempfile.TemporaryDirectory(prefix="luadraw-probe-") as td:
        work = Path(td)
        env = dict(env)
        env["TEXMFVAR"] = str(work / "texmf-var")
        env["TEXMFCACHE"] = str(work / "texmf-cache")
        env["XDG_CACHE_HOME"] = str(work / "cache")
        (work / "texmf-var").mkdir()
        (work / "texmf-cache").mkdir()
        (work / "cache").mkdir()
        probe_lines = [
            r"\documentclass[tikz,border=1pt]{standalone}",
            r"\usepackage[3d]{luadraw}",
            r"\begin{document}",
            r'\directlua{luadraw.graph_name="probe.tkz"; local g=luadraw.graph:new{window={-1,1,-1,1},bbox=false}; g:Dline(-1,1,"red"); g:Show()}',
            r"\end{document}",
        ]
        (work / "probe.tex").write_text("\n".join(probe_lines) + "\n", encoding="utf-8")
        code, log = _run([exe, "-interaction=nonstopmode", "-halt-on-error", "probe.tex"],
                         cwd=work, env=env, timeout=40)
        usable = code == 0 and (work / "probe.pdf").is_file() and (work / "probe.tkz").is_file()
        return usable, log[-8000:]


def _usable_fallback(files: Path, env: dict[str, str], lua_bins: list[str]) -> tuple[bool, str, str | None, str | None, list[dict[str, Any]]]:
    tex_engines = [(name, shutil.which(name, path=env.get("PATH")))
                   for name in ("xelatex", "pdflatex")]
    tex_engines = [(name, path) for name, path in tex_engines if path]
    candidates: list[dict[str, Any]] = []
    all_logs: list[str] = []
    if not lua_bins or not tex_engines:
        return False, "standalone Lua or XeLaTeX/pdfLaTeX not found", None, None, candidates
    for lua_bin in lua_bins:
        with tempfile.TemporaryDirectory(prefix="luadraw-fallback-probe-") as td:
            work = Path(td)
            source = work / "probe.lua"
            source.write_text('local ld=luadraw; local g=ld.graph:new{window={-1,1,-1,1},bbox=false}; g:Dline(-1,1,"red"); g:Show()\n', encoding="utf-8")
            code, lua_log = _run([lua_bin, str(RUNNER), str(files), str(source), str(work)], cwd=work, env=env, timeout=25)
            tikz = work / "figure.tkz"
            if code != 0 or not tikz.is_file():
                for engine_name, engine_path in tex_engines:
                    candidates.append({"lua": lua_bin, "engine": engine_name, "usable": False,
                                       "returncode": code, "log": lua_log[-6000:]})
                all_logs.append(f"{lua_bin} LuaDraw generation failed:\\n{lua_log[-6000:]}")
                continue
            probe_lines = [
                r"\documentclass[tikz,border=1pt]{standalone}",
                r"\usepackage{tikz}",
                r"\begin{document}",
                r"\input{figure.tkz}",
                r"\end{document}",
            ]
            (work / "probe.tex").write_text("\n".join(probe_lines) + "\n", encoding="utf-8")
            for engine_name, engine_path in tex_engines:
                pdf = work / "probe.pdf"
                if pdf.exists():
                    pdf.unlink()
                tex_code, tex_log = _run([engine_path, "-interaction=nonstopmode", "-halt-on-error", "probe.tex"],
                                         cwd=work, env=env, timeout=40)
                usable = tex_code == 0 and pdf.is_file()
                candidates.append({"lua": lua_bin, "engine": engine_name, "usable": usable,
                                   "returncode": tex_code, "log": tex_log[-6000:]})
                if not usable:
                    all_logs.append(f"{lua_bin}+{engine_name} failed:\\n{tex_log[-6000:]}")
    selected = next((c for c in candidates if c["usable"]), None)
    return (selected is not None, "\\n".join(all_logs)[-12000:],
            selected["engine"] if selected else None, selected["lua"] if selected else None, candidates)


def _probe_key(root: Path, files: Path | None, env: dict[str, str]) -> str:
    relevant = ("PATH", "TEXINPUTS", "LUAINPUTS", "TEXMFVAR", "TEXMFCACHE", "XDG_CACHE_HOME")
    parts = [str(root), *(f"{key}={env.get(key, '')}" for key in relevant)]
    for name in ("lualatex", "texlua", "lua5.4", "lua", "xelatex", "pdflatex", "kpsewhich"):
        path = shutil.which(name, path=env.get("PATH"))
        parts.append(f"{name}:{path}")
        if path:
            stat = Path(path).stat()
            parts.append(f"{stat.st_mtime_ns}:{stat.st_size}")
    if files:
        for name in ("luadraw.sty", "luadraw_base.lua", "luadraw_graph2d.lua", "luadraw_graph3d.lua"):
            path = files / name
            if path.exists():
                stat = path.stat()
                parts.append(f"{name}:{stat.st_mtime_ns}:{stat.st_size}")
    for path in (RUNNER,):
        if path.exists():
            stat = path.stat()
            parts.append(f"{path}:{stat.st_mtime_ns}:{stat.st_size}")
    return hashlib.sha256("\\0".join(parts).encode()).hexdigest()


def probe(luadraw_root: str | Path | None = None) -> dict[str, Any]:
    root = Path(luadraw_root).expanduser().resolve() if luadraw_root else DEFAULT_ROOT.resolve()
    files = _find_lua_files(root)
    env = _tex_env(root)
    key = _probe_key(root, files, env)
    if key in _PROBE_CACHE:
        return dict(_PROBE_CACHE[key])
    versions = {name: _version(name, env) for name in
                ("lualatex", "texlua", "lua5.4", "lua", "xelatex", "pdflatex", "kpsewhich")}
    package_paths: dict[str, str] = {}
    kpse = shutil.which("kpsewhich", path=env.get("PATH"))
    if kpse and files:
        for name in ("luadraw.sty", "luadraw_graph2d.lua", "luadraw_graph3d.lua"):
            code, out = _run([kpse, name], cwd=root, env=env, timeout=8)
            package_paths[name] = out.strip().splitlines()[-1] if code == 0 and out.strip() else ""
    lualatex_ok, lualatex_log = _usable_lualatex(root, env) if files and (root / "files" / "luadraw.sty").is_file() else (False, "LuaDraw package files unavailable")
    lua_bins = [path for name in ("texlua", "lua5.4", "lua")
                if (path := shutil.which(name, path=env.get("PATH")))]
    standalone_ok, fallback_log, fallback_engine, fallback_lua, fallback_candidates = (
        _usable_fallback(files, env, lua_bins) if files else
        (False, "LuaDraw modules unavailable", None, None, []))
    lua_status = {path: any(candidate["lua"] == path and candidate["usable"]
                            for candidate in fallback_candidates) for path in lua_bins}
    engine_status = {name: any(candidate["engine"] == name and candidate["usable"]
                               for candidate in fallback_candidates)
                     for name in ("xelatex", "pdflatex")}
    detected_version = _luadraw_version(root)
    if not standalone_ok and fallback_log:
        lualatex_log += "\\nStandalone fallback probe: " + fallback_log[-6000:]
    preferred = "lualatex" if lualatex_ok else ("standalone-lua+" + fallback_engine if standalone_ok and fallback_engine else "unavailable")
    ready = lualatex_ok or standalone_ok
    result = {
        "ok": ready,
        "ready": ready,
        "selected_backend": preferred,
        "luadraw_root": str(root),
        "luadraw_files": str(files) if files else None,
        "luadraw_version": detected_version,
        "executables": {
            name: {
                "path": shutil.which(name, path=env.get("PATH")),
                "version": versions[name],
                "available": bool(versions[name]),
                "usable": {
                    "lualatex": lualatex_ok,
                    "texlua": lua_status.get(shutil.which("texlua", path=env.get("PATH")), False),
                    "lua5.4": lua_status.get(shutil.which("lua5.4", path=env.get("PATH")), False),
                    "lua": lua_status.get(shutil.which("lua", path=env.get("PATH")), False),
                    "xelatex": engine_status["xelatex"],
                    "pdflatex": engine_status["pdflatex"],
                    "kpsewhich": bool(package_paths.get("luadraw.sty")),
                }.get(name, False),
            } for name in versions
        },
        "packages": package_paths,
        "lualatex_usable": lualatex_ok,
        "standalone_lua_usable": standalone_ok,
        "lua_modules_usable": any(lua_status.values()),
        "fallback_engine": fallback_engine,
        "fallback_lua": fallback_lua,
        "fallback_candidates": fallback_candidates,
        "selected_strategy": preferred,
        "probe_log": lualatex_log[-8000:],
    }
    _PROBE_CACHE[key] = dict(result)
    return result


def _direct_wrapper(basename: str, relative_source: str, package_lines: str = "") -> str:
    lua_source = json.dumps(relative_source)
    lines = [
        r"\documentclass[tikz,border=2pt]{standalone}",
        *package_lines.splitlines(),
        r"\usepackage[3d]{luadraw}",
        r"\begin{document}",
        f"\\directlua{{ld=luadraw; Z=luadraw.cpx.Z; M=luadraw.pt3d.M; pt3d=luadraw.pt3d; luadraw.graph_name={json.dumps(basename + '.tkz')}; dofile({lua_source})}}",
        r"\end{document}",
    ]
    return "\n".join(lines) + "\n"


def _has_luadraw_usage(source: str) -> bool:
    # Require an actual LuaDraw graph construction and rendering call, not handmade TikZ.
    return bool(re.search(r"(?:luadraw|\bld)\.graph(?:3d)?\s*:\s*new", source) and
                re.search(r"\b\w+\s*:\s*(?:Show|Save)\s*\(", source))


def compile_figure(source: str | Path, workdir: str | Path,
                   options: dict[str, Any] | None = None) -> dict[str, Any]:
    options = dict(options or {})
    src = Path(source).expanduser().resolve()
    work = Path(workdir).expanduser().resolve()
    result: dict[str, Any] = {"ok": False, "pdf": None, "path": None, "wrapper": None,
                              "engine": None, "backend": None, "generated_tikz": [], "tikz": None,
                              "logs": [], "error": None, "luadraw_version": None, "generated_lua": None}
    try:
        if not src.is_file():
            raise FileNotFoundError(f"LuaDraw source not found: {src}")
        raw = src.read_text(encoding="utf-8")
        work.mkdir(parents=True, exist_ok=True)
        if src.suffix.lower() == ".tex":
            env_blocks = re.findall(r"\\begin\s*\{(luadraw|luadrawenv)\}(?:\{[^{}]*\})?(.*?)\\end\s*\{\1\}", raw, re.S)
            code_blocks = re.findall(r"\\begin\s*\{(luacode\*?)\}(.*?)\\end\s*\{\1\}", raw, re.S)
            blocks = [body for _, body in env_blocks] or [body for _, body in code_blocks]
            if not blocks:
                raise ValueError("figure.tex must contain a luadraw environment or luacode* LuaDraw source")
            if len(blocks) != 1:
                raise ValueError("figure.tex currently requires exactly one executable LuaDraw environment")
            lua_text = "\n".join(blocks).replace(r"\shortcuts", "")
            if not _has_luadraw_usage(lua_text):
                raise ValueError("LuaDraw is mandatory: figure.tex has no graph construction and Show/Save call")
            lua_source = work / "_luadraw_from_tex.lua"
            lua_source.write_text(lua_text, encoding="utf-8")
        elif src.suffix.lower() == ".lua":
            if not _has_luadraw_usage(raw):
                raise ValueError("LuaDraw is mandatory: figure.lua must construct a LuaDraw graph and call Show/Save")
            staged_dir = work / "_luadraw_inputs"
            staged_dir.mkdir(parents=True, exist_ok=True)
            lua_source = staged_dir / src.name
            if src != lua_source:
                shutil.copy2(src, lua_source)
        else:
            raise ValueError("source must be figure.lua or figure.tex")
        work.mkdir(parents=True, exist_ok=True)
        root = Path(options.get("luadraw_root") or DEFAULT_ROOT).expanduser().resolve()
        info = probe(root)
        result["luadraw_version"] = info["luadraw_version"]
        if not info["luadraw_files"]:
            raise RuntimeError(f"LuaDraw 3.7 modules not found under {root}")
        if not info["lualatex_usable"]:
            result["logs"].append({"engine": "lualatex", "stage": "capability-probe", "returncode": 1, "log": info["probe_log"][-9000:]})
        if not info["standalone_lua_usable"]:
            result["logs"].append({"engine": "standalone-lua+tex", "stage": "capability-probe", "returncode": 1, "log": info["probe_log"][-9000:]})
        timeout = int(options.get("compile_timeout", options.get("timeout", 120)))
        env = _tex_env(root)
        cache = work / ".luadraw-cache"
        for dirname in ("texmf-var", "texmf-cache", "xdg"):
            (cache / dirname).mkdir(parents=True, exist_ok=True)
        env["TEXMFVAR"] = str(cache / "texmf-var")
        env["TEXMFCACHE"] = str(cache / "texmf-cache")
        env["XDG_CACHE_HOME"] = str(cache / "xdg")
        basename = str(options.get("output_name", "figure"))
        if not re.fullmatch(r"[A-Za-z0-9_-]+", basename):
            raise ValueError("output_name may contain only letters, digits, '_' and '-'")
        pdf = work / f"{basename}.pdf"
        wrapper = work / f"{basename}_luadraw_wrapper.tex"
        output_tikz = work / f"{basename}.tkz"
        if any(path.resolve().parent != work for path in (pdf, output_tikz, wrapper)):
            raise ValueError("output artifacts must remain inside workdir")
        for path in (pdf, output_tikz):
            if path == src:
                raise ValueError(f"refusing to remove source file {path}")
            if path.exists():
                path.unlink()
        relative_source = lua_source.relative_to(work).as_posix()
        contains_cjk = bool(re.search(r"[\u2e80-\u9fff]", raw))
        extra_packages = options.get("tex_packages", options.get("packages", []))
        if isinstance(extra_packages, str):
            extra_packages = [extra_packages]
        if not isinstance(extra_packages, (list, tuple)) or any(not re.fullmatch(r"[A-Za-z0-9_-]+", str(p)) for p in extra_packages):
            raise ValueError("tex_packages must be a list of package names")
        packages = list(dict.fromkeys((["ctex"] if contains_cjk else []) + [str(p) for p in extra_packages]))
        package_lines = "".join(f"\\usepackage{{{name}}}\n" for name in packages)
        package_lines += str(options.get("preamble", "")) + ("\n" if options.get("preamble") else "")
        result.update({"pdf": str(pdf), "path": str(pdf), "wrapper": str(wrapper), "generated_lua": str(lua_source)})
        masked = set(options.get("mask_executables", []))
        requested_engine = str(options.get("engine", "auto")).lower()
        if requested_engine not in {"auto", "lualatex", "xelatex", "pdflatex", "fallback"}:
            raise ValueError("engine must be auto, lualatex, xelatex, pdflatex, or fallback")
        direct_ok = (info["lualatex_usable"] and "lualatex" not in masked and
                     requested_engine not in {"xelatex", "pdflatex", "fallback"} and
                     not options.get("force_fallback"))
        if direct_ok:
            wrapper.write_text(_direct_wrapper(basename, relative_source, package_lines), encoding="utf-8")
            engine = shutil.which("lualatex", path=env.get("PATH"))
            code, log = _run([engine, "-interaction=nonstopmode", "-halt-on-error", "-jobname=" + basename,
                              wrapper.name], cwd=work, env=env, timeout=timeout)
            result["logs"].append({"engine": "lualatex", "returncode": code, "log": log[-12000:]})
            if code == 0 and pdf.is_file():
                direct_tikz = output_tikz
                if direct_tikz.is_file():
                    result["tikz"] = str(direct_tikz)
                    result["generated_tikz"] = [str(direct_tikz)]
                result.update(ok=True, engine="lualatex", backend="luadraw.sty")
                return result
        candidates = [candidate for candidate in info["fallback_candidates"]
                      if candidate["usable"] and Path(candidate["lua"]).name not in masked
                      and candidate["engine"] not in masked
                      and (requested_engine not in {"xelatex", "pdflatex"} or candidate["engine"] == requested_engine)]
        engine_preference = ([requested_engine] if requested_engine in {"xelatex", "pdflatex"}
                             else [info.get("fallback_engine"), "xelatex", "pdflatex"])
        engine_rank = {name: index for index, name in enumerate(dict.fromkeys(n for n in engine_preference if n))}
        candidates.sort(key=lambda candidate: (engine_rank.get(candidate["engine"], 99),
                                                Path(candidate["lua"]).name))
        if not candidates:
            raise RuntimeError("no LuaDraw fallback Lua/TeX engine combination passed the real probe")
        runner = RUNNER
        tex_success = False
        last_error = "LuaDraw fallback failed"
        for candidate in candidates:
            lua_bin = candidate["lua"]
            if output_tikz.exists():
                output_tikz.unlink()
            code, log = _run([lua_bin, str(runner), str(info["luadraw_files"]), str(lua_source), str(work), basename],
                             cwd=work, env=env, timeout=timeout)
            result["logs"].append({"engine": Path(lua_bin).name, "stage": "LuaDraw-to-TikZ", "returncode": code, "log": log[-12000:]})
            if code != 0 or not output_tikz.is_file():
                last_error = "standalone LuaDraw TikZ generation failed"
                continue
            result["tikz"] = str(output_tikz)
            result["generated_tikz"] = [str(output_tikz)]
            wrapper.write_text("\\documentclass[tikz,border=2pt]{standalone}\n\\usepackage{tikz}\n" + package_lines +
                               "\\begin{document}\n\\input{" + output_tikz.name + "}\n\\end{document}\n", encoding="utf-8")
            tex_engine = candidate["engine"]
            exe = shutil.which(tex_engine, path=env.get("PATH"))
            if pdf.exists():
                pdf.unlink()
            code, log = _run([exe, "-interaction=nonstopmode", "-halt-on-error", "-jobname=" + basename,
                              wrapper.name], cwd=work, env=env, timeout=timeout)
            result["logs"].append({"engine": tex_engine, "returncode": code, "log": log[-12000:]})
            if code == 0 and pdf.is_file():
                result.update(ok=True, engine=tex_engine, backend="standalone-lua+generated-TikZ")
                return result
            last_error = f"{tex_engine} failed to compile generated LuaDraw TikZ"
        raise RuntimeError(last_error)
    except Exception as exc:
        failed_stage = next((entry for entry in reversed(result["logs"]) if entry.get("returncode", 0) != 0), None)
        if failed_stage and failed_stage.get("log"):
            result["error"] = f"{exc}\nLast stage ({failed_stage.get('stage') or failed_stage.get('engine')}) log:\n{failed_stage['log'][-9000:]}"
        else:
            result["error"] = str(exc)
        return result
