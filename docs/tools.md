# JSON tools and LuaDraw runtime

> 当前正式运行与Skill导出使用原生Go，参见 [Go接口与导出](go-native.md)。下文Python命令保留作开发原型/历史对照，不作为Skill导出选择；比较JSON契约继续兼容。

## Stable process interface

`tools_cli.py` is the Python entry for `compare`, `docs`, `compile` and `environment`. One-shot reads a JSON object from stdin or `--request file.json`; `--jsonl` keeps one service process alive and emits exactly one JSON object per input line. stdout is protocol only; generated images live under `out_dir` (a system temporary directory when omitted). `inline_images:true` adds the encoded bytes for adapters that attach images directly to a model.

```bash
conda run -n base python dev/python/tools_cli.py --request request.json
conda run -n base python dev/python/tools_cli.py --jsonl --backend auto
```

`auto` reuses a Go JSONL subprocess when `build/compare-figure` or `COMPARE_FIGURE_BIN` is available, otherwise uses the production Python metric. `--backend go` requires a usable binary; it does not silently switch to Python on error. JSONL streams continue after a per-request error. Single-request errors use exit 2.

Comparison request:

```json
{
  "version": "compare-figure/1",
  "operation": "compare",
  "reference": "/path/to/reference.png",
  "candidate": "/path/to/drawing.pdf",
  "reference_mm": [40, 30],
  "detail": "all",
  "worst_region": false,
  "crop": [0, 0, 100, 100],
  "zoom": 1200,
  "raster_dpi": 110,
  "pass_overlap": 0.17,
  "pass_scale": 0.1,
  "inline_images": true
}
```

`candidate` accepts an image or PDF; PDF rendering uses Poppler. Sizes are measured before normalization. If original physical dimensions are unknown, omit `reference_mm` and the size gate uses ink aspect. `crop`/`worst_region`/`zoom` only select returned evidence, never the scored image. Zero gate values mean defaults; negative values disable a gate. Paths are resolved relative to the CLI working directory; absolute paths are recommended for subprocess adapters.

Response includes `ok`, `version`, `metrics` (raw `similarity` in 0–1, channels, gray, dx/dy, alignment runs, mm/aspect size data), `pass`, `selected_image`, `out_dir` and `images`:

```json
{
  "images": [{
    "path": "/tmp/compare-figure-.../comparison.png",
    "mime": "image/png",
    "width": 1200,
    "height": 900,
    "base64": "...when requested..."
  }]
}
```

The Go binary exposes the same comparison request directly:

```bash
build/compare-figure --stdin < request.json
build/compare-figure --request request.json
build/compare-figure --jsonl
build/compare-figure --original reference.png --pdf candidate.pdf --out runs/comparison
```

The Go library exports `Compare`, `Evidence`, `WorstRegion`, `Request`, `Result`, `Metrics` and image helpers. Build with Go 1.25+:

```bash
cd go
GOCACHE="$PWD/../build/go-cache" go test ./...
GOCACHE="$PWD/../build/go-cache" go vet ./...
GOCACHE="$PWD/../build/go-cache" go build -o ../build/compare-figure ./cmd/compare-figure
```

On this host the system Go is older than the module requirement; an already cached Go 1.25.11 toolchain was used. No network download is required for the built binary. The Python test helper can discover a suitable cached toolchain or use `COMPARE_FIGURE_GO=/path/to/go`.

## LuaDraw document tool

The active model gets one `luadraw_doc` tool with three modes. Search mode defaults when `query` is supplied:

```json
{"operation":"docs","arguments":{"query":"Dpolyline3d","limit":5,"include_image":true},"inline_images":true}
```

It returns relevant official tutorial/source/example excerpts with locations and a manual page/crop image. Subsequent detail inspection remains the same tool:

```json
{"operation":"docs","arguments":{"mode":"view","doc":"doc/luadraw-doc-en.pdf","page":118,"left":5,"top":20,"right":95,"bottom":65,"zoom":1400}}
```

Use the actual `doc`/page returned by search; the example page is illustrative, not a hardcoded function location. `mode:read` reads indexed source files with `start_line`/`end_line`. Document paths are restricted to the vendored library, never arbitrary host paths. PyMuPDF is preferred; Poppler text/bbox and rendering provide a fallback. Search cache fingerprints are tied to the pinned distribution and file metadata, avoiding repeated full-PDF extraction in a warm service. Session tools share a build-directory index across figures.

## LuaDraw and environment selection

LuaDraw **3.7** is bundled byte-for-byte from the adjacent YanTrace lock. The remembered `0.3.6` is exam-zh. Official manuals and source are included; original LPPL notice and file hashes are preserved under `vendor/`.

```bash
conda run -n base python dev/python/runner.py --environment
```

This needs no model credentials and reports executables, versions, package locations, real usability and chosen strategy. `--check` also validates session inputs before any model API request. An executable merely existing is insufficient: LuaLaTeX can fail due to missing packages or an unwritable font cache.

An external program can compile without a model session in the same JSON interface:

```json
{"operation":"compile","source":"/path/to/figure.lua","options":{"engine":"auto"},"include_image":true,"inline_images":true}
```

The result includes `ok`, `pdf`, `wrapper`, `generated_lua`, `generated_tikz`, actual `backend`/`engine`, compile logs and the preview `images` array. `out_dir` defaults to an isolated temporary directory. Set `include_image:false` when only editable/vector artifacts are needed.

Compiler selection follows this order:

1. Prefer actual usable LuaLaTeX with LuaDraw.
2. Otherwise run the **same editable LuaDraw program** under standalone Lua/texlua, preserve generated TikZ, and compile with a usable XeLaTeX or pdfLaTeX chain.
3. If no route works, report the detected reason and compile logs. Do not replace it with hand-written TikZ or claim success from an untested engine.

Reference: locally available DocVision `origin/feat/skills` `f0c00e9`, particularly `skills/kernel/internal/texcheck/precompile.go`, its tests and `skills/docs/luadraw/tools/luadraw-expand.py`. Source branches remain untouched.

The primary model source is `figure.lua`; `compile` detects/executes LuaDraw, produces `standalone.pdf`, and reports actual engine/backend. The delivered editable source is `submitted.lua`, not generated TikZ. A LuaDraw-containing TeX body is also supported for official examples. Pure hand-written TikZ is rejected by this stage. The original upstream TikZ prompts remain immutable provenance fixtures; the active prompts are the explicitly new `luadraw_figure.*` templates required by this change.

The wrapper and generated TikZ are included with the accepted editable Lua source. Rebuilding requires the same pinned LuaDraw distribution and detected compile route. Keep all generated dependencies with the TeX wrapper if moving it.
