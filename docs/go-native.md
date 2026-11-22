# Native Go runner and Go-only Skill

The supported exported runtime is Go. Python remains the development prototype and cross-language test oracle; it is not an export choice or a runtime bridge. The Go binary does not spawn Python for models, compilation, documents or image comparison.

## Standalone sizing and Markdown context

Reference sizing has no parsing-engine/index/bbox dependency. Input is a local image, optionally a nearby or explicitly selected sizes.csv and optional Markdown. Explicit `reference_mm` / `--reference-mm W,H` wins; otherwise `sizes_csv` / `--sizes-csv FILE` or nearby sizes.csv (size.csv accepted) provides file,width_mm,height_mm,page_w_mm. `# scale_px_per_pt=...` is provenance only: CSV mm are already calibrated and must not be scaled again. Without mm, validate ink aspect only and say so explicitly.

Results include reference_measurement/candidate_measurement, a full human-readable `text` receipt, metrics and normalization. Physical ink width/height and max deviation are calculated before candidate pixel rescaling. PDF size is read from its actual page. Pixel normalization is uniform width scaling plus white padding, not independent axis stretching; overlap cannot hide a wrong printed scale.

`run`, `tool` and `compare` accept `--source-md Book.md --image-ref images/Book/figure.jpg`; a local image is resolved relative to the Markdown directory. A unique local image can be inferred; multiple images require selection. Initial context comes from 20 lines before/after the selected image, with full Markdown retained for image_context and neighboring images. Encoded local paths, optional titles and spaces are supported; caller downloads external HTTP/data links first. No external book files are modified.

## Module map

| Prototype | Native package | Responsibility |
| --- | --- | --- |
| `runner.py` | `go/runner` | JSON/YAML models/extends, inputs, stage wiring, thresholds, checker, resume and delivery |
| `vector_session.py` | `go/session` | Chat/Responses/Anthropic, SSE, messages/images/reasoning, budgets, compaction, file-media transcript/checkpoint |
| `vector_tools.py`, `luadraw_harness.py`, `vector_measure.py` | `go/drawing` | 11 schemas/tools, source files, RE2 grep, crop/view/context, physical size, compare/submit and accepted source delivery |
| `luadraw_runtime.py`, Lua helper | `go/luadraw` + existing `scripts/luadraw_runtime.lua` | Actual host probes, preferred LuaLaTeX, standalone Lua→generated TikZ→XeLaTeX/pdfLaTeX |
| `luadraw_docs.py` | `go/luadraw` | Pure Go + Poppler PDF bbox index, source search/read, manual region image, warm cache |
| `tools_cli.py` | `go/app` | Native JSON/JSONL operations, image paths/base64, errors and process status |
| Development packaging | `go/distribution` | Movable resources and idempotent Go-only Skill export |
| Production comparison | root Go package | Shared `Compare`, `Evidence`, `WorstRegion`, `ProcessRequest` and image/PDF artifacts |

Runtime resource discovery checks explicit `--resource-root`, `COMPARE_FIGURE_ROOT`, then executable/working-directory ancestors. It contains no compiled-in developer path. The exported Skill includes binaries, prompts, fixed LuaDraw sources/manuals, the Lua helper and licenses. It excludes Python prototypes, tests, local configuration and private runs.

## Two modes

Automatic mode uses `run` (alias `auto`) with the user's model, base URL and key. `--api-key-env NAME`, `--api-key-file FILE` and ephemeral `--api-key KEY` are accepted; JSON runner options also accept `api_key`. The client receives credentials without storing them in metadata/transcripts. The model handles all drawing/tool iterations. Default minimum overlap is 17%.

Semi-automatic mode lets the current Agent draw and iterate without an additional model/API key. Use `luadraw-doc-search` (alias `docs`) for search/read/view, `compile` (alias `luadraw-compile`) for LuaDraw compilation, and `compare --reference REF --candidate PDF --threshold 17`. The Agent reads reference/manual/preview/comparison images and revises editable source until `pass=true` and the figure is visually satisfactory. Direct compile/compare use does not require a runner or submit call.

`compare` flags include `--threshold` / `--pass-threshold` in PERCENT, `--max-scale-dev` as a fraction (default 0.10), `--reference-mm W,H`, `--candidate-mm W,H`, `--crop L,T,R,B`, `--detail`, `--zoom`, `--worst-region` and `--inline-images`. JSON `pass_overlap` remains a ratio (default 0.17). Results add `similarity_percent` and `thresholds.{min_overlap,min_overlap_percent,max_scale_dev,overlap_pass,size_pass}` alongside `pass`, metrics and images. A valid comparison below threshold still exits 0; only an operational failure exits 2.

All 11 drawing tools are callable via `tool` / `agent-tool --workspace DIR --name NAME --args JSON` (or `--args-file`). First invocation binds `--original`; persistent state lets separate CLI invocations compile→compare→submit→deliver without a model. `--name deliver` writes accepted-source outputs. This stateful path retains original submission gates; the direct manual loop leaves final artifact management to the Agent.

## Output lifetime

`compare`, `docs`, `compile` and their JSON-service operations reuse one fixed temporary output slot per current working directory and operation when `out_dir` / `--out` is absent. No per-request output directory is created. The base is a user-specific directory under the OS temporary root; `COMPARE_FIGURE_OUTPUT_ROOT` overrides it. The response contains the exact absolute location.

Comparison overwrites comparison.png, metrics.json and result.json. Document search/view publishes one 预览/document.png and removes only its newly generated intermediate render. Compile reuses its source-named artifacts. Save final files before the next call or give an explicit output directory; independent parallel jobs must choose distinct outputs. Explicit automatic or stateful-tool workspaces deliberately retain their source/state/transcript artifacts. This policy does not automatically sweep user files or old releases' temporary directories.

## Run

```bash
build/compare-figure environment
build/compare-figure run \
  --original /path/to/reference.png \
  --context-file /path/to/context.md \
  --out runs/figure-001 --config /path/to/model.json

build/compare-figure run --out runs/figure-001 --resume --config /path/to/model.json
```

Model configuration accepts the prototype's JSON or YAML model registry and `extends` structure, the three wire APIs, key environment names, session/checker settings, tool view settings and compile options. Keys are not copied into run metadata or Skill exports. A new task can also be supplied as a JSON object via `run --request file.json` / `run --stdin`; field names use snake_case.

`run --check` checks the input and real compiler route without a model key, without an API request and without creating the session directory. `environment` reports executable/package/version facts and actual candidate usability. This host now passes real LuaLaTeX compilation. Both probes and builds point TEXMFVAR, TEXMFCACHE and XDG_CACHE_HOME at the same writable private cache root; the old separated-cache setup caused a false fontloader failure. The fallback wrapper preloads the same TikZ runtime libraries as LuaDraw.sty, including arrows.meta, patterns, plotmarks and decorations.markings. Other machines are probed independently.

The default gates remain overlap 0.17, size deviation 0.10 and hidden best-effort acceptance after 8 real submit attempts. The image is scored before evidence crops/zoom. Unknown physical dimensions use ink aspect instead of mismatched-DPI pixel widths. The model must use LuaDraw, compile, view, compare and submit; final artifacts use the exact accepted editable source.

## Repeatability and resume

- Repeating the same input command against the same output directory resumes automatically. Input fingerprints prevent an unrelated task from silently using an old workspace.
- A completed result that still meets current gates returns existing artifacts before constructing a model client. No model key or second model request is required.
- An accepted state recorded before a delivery interruption is recoverable; accepted editable source can be recompiled without calling the drawing model again.
- Initialization failure does not lose the initial task/image on retry. Interrupted tools are repaired when loading the transcript. A single workspace admits one live runner; dead local process locks can be recovered.
- The version-3 metadata/transcript shape remains compatible with the Python prototype. Current system/tool definitions are reinserted on continuation; no duplicate opening task or original image is added to an already-started session.
- Per-image `run.json.fig_min_overlap` is re-read on submission. Zero does not overwrite the existing gate; negative values disable it. Raising gates can require further drawing work.
- Exports and metadata use same-directory replacement and skip unchanged file content. Repeating an export preserves an identical manifest and unchanged files.

## Native JSON tools

```bash
build/compare-figure tools --request request.json
build/compare-figure tools --jsonl
build/compare-figure compare --stdin < comparison.json
```

`tools` supports `compare`, `docs`, `compile`, `environment` and `run`. One JSON object produces one response; `--jsonl` keeps caches in a live service and continues after a bad request. stdout contains JSON only. A one-shot failed request exits 2; a JSONL stream that contained failures exits 1 after EOF. Images use `{path,mime,width,height,base64?}`, with encoded bytes when `inline_images:true`. Omitted `out_dir` reuses the fixed per-working-directory operation slot described above.

```json
{"operation":"docs","arguments":{"query":"Dpolyline3d","include_image":true},"inline_images":true}
```

```json
{"operation":"compile","source":"/path/to/figure.lua","options":{"engine":"auto"},"include_image":true}
```

The Go document implementation uses `pdftotext -bbox`, `pdfinfo` and `pdftoppm` directly, then Go image code for crops/zoom. It does not invoke the prototype or require PyMuPDF. Search returns official snippets and PDF document/page/bbox locations as text, without an image by default. The model chooses whether to call `mode:view` with that doc/page for a whole page, or add percentage `left/top/right/bottom` and `zoom` for a region; search bbox units are PDF points, not percentages. Explicit `include_image:true` remains an opt-in search crop. `mode:read` exposes source line windows.

## Documentation navigation and frequency memory

The single `luadraw_doc` tool supports `list`, `find`, `grep`, `search`, `read`, `view`, and `frequent`. All filesystem access stays within the fixed library root. List direct children (`recursive:true` for descendants), find actual names/globs, and grep literal text (`regex:true` explicitly). `doc` scopes directory/file searches; `limit`, `offset`, and `max_chars` bound navigation results. Read source via 1-based line ranges, capped at 400 lines. Read a PDF as text with doc/query + page, or a page-qualified `doc/luadraw-doc-en.pdf#page=14`. `view` alone requires rendering; search stays text-only unless include_image is explicitly requested.

Search canonicalizes complete source paragraphs and PDF paragraph blocks, merges spatially adjacent same-column PDF hits, deduplicates before pagination, and preserves exact signature locations. `snippet_chars` defaults to 1200 (64–8000), `max_chars` defaults to 6000 (64–32000). Each result provides a source range or PDF page/block range/bbox. Use a precise read for larger context.

`frequent` returns at most 8 ranked paragraphs by default, 256 characters per complete output line and 4096 total; adjust `limit`, `line_chars`, `max_chars`. Its stable short `entry_id` selects a particular paragraph even as ranks change. Without doc, start_line/end_line select ranking rows from a fixed snapshot; with doc, they filter original ranges. Only entries actually emitted by an explicit model lookup increase counts. `FrequentSnippet`, used for the automatic initial user-context excerpt, is read-only and never increments counters. YAML docs.frequent controls that initial excerpt, hard-capped at 8 lines / 2048 characters; zero budgets or prompt_enabled:false disable it.

The shared cache defaults to the Skill root's build/doc-cache (COMPARE_FIGURE_CACHE or explicit cache_dir overrides). It records document paragraphs, IDs and counts only, uses bounded atomic cross-process updates, and invalidates on library fingerprint changes. It is excluded from source exports. Future per-book/global frequency design is recorded as a DocVision note; the standalone tool currently has one shared index.

## Editable TeX delivery and fallback

Delivery includes `submitted.tex` with the accepted program embedded in `\\begin{luadraw}{name=submitted}`. It references no input Lua/TikZ sidecar and is recompiled to produce the delivered PDF. `editable_tex` identifies this file; `editable_source`/`editable_lua` retain the original accepted Lua for API compatibility. Accepted metrics and uncertainty belong to the same accepted source.

```bash
build/compare-figure compile --source submitted.tex --out work/direct --engine lualatex
build/compare-figure compile --source submitted.tex --out work/fallback --engine xelatex
```

With LuaLaTeX, the official environment generates its own intermediates. Without it, the Go compiler extracts the embedded Lua, runs standalone Lua/texlua to generate TikZ, then compiles through XeLaTeX/pdfLaTeX. Lua alone does not generate a finished PDF; a working TeX backend and Poppler remain required. The TeX file can be copied alone into a fresh working directory when LuaDraw and its TeX dependencies are installed or exposed through the Skill compiler.

## Build and export

Root SKILL.md is the single canonical instruction source; VERSION now defaults to 0.2.0. Releases contain production Go sources with go.mod/go.sum and runtime resources, not prebuilt executables. Source-only manifest schema v2 records the distribution and required Go version plus SHA256 hashes. No packaged platform matrix is needed.

The target machine's Agent checks Go 1.25+ and builds once from the Skill root. Official Go downloads: https://go.dev/dl/ ; installation: https://go.dev/doc/install . The Go toolchain builds for the current host unless GOOS/GOARCH are explicitly set. Initial module downloads may require network access.

```bash
# From the Skill root; use an .exe output name on Windows
GOCACHE="$PWD/build/go-cache" go -C go build -o ../build/compare-figure ./cmd/compare-figure
build/compare-figure environment

# Source-only export; the script compiles a local export tool but does not package it
./scripts/export_skill.sh
./scripts/export_skill.sh --version 0.2.0 --go /path/to/go
```

The package includes canonical Skill/VERSION, source, fixed prompts/vendor/manuals/licenses, Lua helper, commented examples/runner.yaml.example and the local folder-based viewer.html. Python prototypes and tests remain development-only under dev/python. Actual runner.yaml/runner.json/*.local.yaml or JSON files, keys, runs, caches and compiled binaries are excluded. Copy the example to runner.yaml, edit privately, and pass --config runner.yaml. JSON remains accepted for compatibility.

The export script resolves its own repository root regardless of caller directory. Different release versions are preserved; unknown destination files are rejected, not silently removed. VERSION, manifest.version and Skill metadata.version agree. Exported sources can build independently of the checkout and locate their resources relative to the Skill root.

The native `build --platforms GOOS/GOARCH` command remains available for optional cross-compilation. Cross-compilation is distinct from execution on the target OS; report actual runs separately.
