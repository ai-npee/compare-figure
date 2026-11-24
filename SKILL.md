---
name: compare-figure
description: "Create original LuaDraw figures or redraw and compare references. Provides standalone CLI/JSON/JSONL documentation lookup with shared frequency caching, plus automatic and Agent-led drawing tools; reference comparison defaults to 17% overlap."
license: MIT
metadata:
  version: "0.2.2"
---

# compare-figure: two work modes

The Skill package contains Go source and runtime resources, with no prebuilt executable. Before first use, the Agent checks for Go 1.25+ (`go version`); official downloads and installation instructions are https://go.dev/dl/ and https://go.dev/doc/install. From this Skill root run `go -C go build -o ../build/compare-figure ./cmd/compare-figure` (Windows: `go -C go build -o ../build/compare-figure.exe ./cmd/compare-figure`). Go builds for the current host by default; use the existing executable for subsequent calls, and rebuild after source updates. Go modules listed in go/go.mod may need downloading on the first build. Use a writable cache, for example `GOCACHE="$PWD/build/go-cache"` on POSIX systems.
Resolve the executable and resources relative to this SKILL.md, not the caller working directory. Use an absolute executable path when operating inside a book directory. The examples use compare-figure as shorthand for that full binary path. Execution is native Go; Python is not required. Lua/TeX/Poppler remain detected external tools.

Copy `examples/runner.yaml.example` to `runner.yaml` for a commented automatic-mode configuration; actual runner.yaml/runner.json and *.local.yaml/JSON files are ignored by Git and excluded from Skill exports. Preserve user-adjusted session/view/compile/figure_check budgets when creating local configurations. Use environment variables for keys where practical and never publish real credentials.

The accepted delivery includes a self-contained submitted.tex with the exact LuaDraw program embedded in the official luadraw environment, plus submitted.pdf rebuilt from that TeX. Prefer this editable_tex/submitted_tex as the user's single-file TeX deliverable; editable_source/editable_lua also retain the accepted Lua for compatibility. No input Lua/TikZ sidecar is needed. `compile --source submitted.tex --engine lualatex` uses the official environment; `--engine xelatex` or `--engine pdflatex` extracts and runs the same embedded Lua with Lua/texlua before compiling generated TikZ. A TeX engine is still required for the PDF stage.

## Original drawing and standalone documentation — no reference image required

When the user wants a new LuaDraw diagram, use the current Agent's reasoning and file tools. No reference bitmap, Markdown book, image_context workspace, second model or API key is required. Consult documentation before inventing API calls:

    compare-figure luadraw-doc --mode frequent --cache-dir work/shared-doc-cache
    compare-figure luadraw-doc --query graph3d --snippet-chars 1200 --max-chars 6000 --cache-dir work/shared-doc-cache

Use list/find/grep to discover real files, read exact constructors/methods/examples, and view a PDF page/crop only when helpful. Write figure.lua or a self-contained TeX luadraw environment, compile it, and inspect the generated PDF/preview. For a new drawing without a reference, judge the user-requested geometry, labels and style; no reference comparison score or submit acceptance is available. Deliver editable TeX/Lua and PDF after compilation succeeds. Run compare only if the user supplies a reference.

The documentation command is also an independent machine interface:

    compare-figure luadraw-doc --stdin --cache-dir work/shared-doc-cache
    compare-figure luadraw-doc --request examples/luadraw-doc.json.example
    compare-figure luadraw-doc --jsonl --cache-dir work/shared-doc-cache

Supply a bare JSON tool object (`{"mode":"search","query":"Dpolyline3d"}`) or an envelope (`{"operation":"docs","arguments":{"mode":"search","query":"Dpolyline3d"},"cache_dir":"work/shared-doc-cache"}`). JSONL keeps one local process and index objects alive across requests; it is not a model session or a network server. A bad line returns a JSON error and subsequent lines continue. Prefer one shared writable cache directory across agent sessions; use separate --out directories for parallel PDF previews. The existing `tools --jsonl` envelope interface remains available. See docs/luadraw-doc.md for calling contracts, Go API, dependencies and cache controls. This library indexes the bundled LuaDraw manuals/sources, not arbitrary LuaLaTeX documentation.

## Documentation navigation and reusable paragraphs

`luadraw_doc` is the single knowledge-base tool. Use `mode:list` with a relative `doc` directory to discover actual files (optionally `recursive:true`); `mode:find` searches file names/globs; `mode:grep` searches literal text (`regex:true` opts into a regular expression). Follow `\\input`/`\\include` references using these modes and `mode:read`; do not guess manual section filenames. Access stays within the bundled LuaDraw root.

`mode:read` accepts source files with 1-based start_line/end_line, or a PDF doc (also accepted in query) with page for text. `mode:view` displays a PDF whole page or a percentage crop. Search returns complete paragraph snippets with merged overlapping hits; snippet_chars controls each hit and max_chars controls the result budget. Inspect exact API signatures via source read before writing code.

Use `mode:frequent` to consult previously used paragraphs, with limit, line_chars and max_chars (default 8 entries, 256 characters per line, 4096 total). The output gives a stable entry_id and original document range; pass entry_id for precise selection. Without doc, start_line/end_line select current ranking rows; with doc they select source ranges. Each explicit tool lookup contributes frequency only for paragraphs actually returned. The short automatic excerpt appended after a fresh task's context does not increase counts. `docs.frequent` in the runner YAML controls that initial excerpt (default at most 8 lines / 2048 characters); more detail remains available through the tool. Shared cache lives under build/doc-cache unless configured otherwise; it is excluded from Skill exports and expires when the knowledge-base fingerprint changes.

## Browse saved sessions

Open the packaged viewer.html in a Chromium-based browser and choose the runner output folder. Choose a parent folder to list multiple sessions, or a single session folder. It retains read-only directory/file handles, enumerates paths without copying/uploading folder contents, and reads fresh selected-session snapshots on selection or manual Refresh. Refresh rescans to discover new files and preserves selection by path; there is no automatic filesystem watcher. Images load when their thumbnails enter view, the comparison panel reads only its selected view (newest comparison round), and collapsed tool/reasoning bodies render on expansion. The timeline starts with the latest 100 events and offers earlier events on demand. Nothing is uploaded and no server is required. The viewer supports Go session.jsonl, run.json/result.json, tool arguments/results, comparison evidence and final previews. It never edits or truncates sessions. Directory enumeration and full selected-JSONL parsing still cost time/memory for very large inputs; pagination bounds initial DOM rendering, not the transcript bytes read.

## Standalone inputs, physical size and Markdown context

This is a standalone figure tool. It does not read parsing-engine indexes, bounding-box caches or original source PDFs to infer the reference size. Use a single image, optional sizes.csv, and optional Markdown.

For the common directory Book.md plus images/Book/figure.jpg and images/Book/sizes.csv, select an image with:

    compare-figure run --source-md Book.md --image-ref images/Book/figure.jpg --out work/auto --model MODEL --base-url URL --api-key-env KEY_ENV
    compare-figure tool --workspace work/agent --source-md Book.md --image-ref images/Book/figure.jpg --name image_context
    compare-figure compare --source-md Book.md --image-ref images/Book/figure.jpg --candidate redrawn.pdf

Image links are resolved relative to the Markdown file. The selected image's surrounding 20 lines provide initial context; full Markdown remains available through image_context for neighbors/continuations. Multiple local images require an explicit image-ref or original path. Local URL-encoded links and paths containing spaces are supported; external HTTP/data images must be downloaded by the caller first. The original Markdown is never modified.

Physical size precedence: explicit --reference-mm W,H, then explicit --sizes-csv FILE or auto-discovered sizes.csv (also size.csv) beside/above the selected image. The CSV header is file,width_mm,height_mm,page_w_mm, optionally preceded by # scale_px_per_pt=1.1361. Millimetre values are already calibrated: do not divide them by scale_px_per_pt again. Keep both width and height from the CSV and retain page_w_mm as page-width context. Duplicate basenames require an unambiguous relative path.

The comparison JSON includes reference_measurement/candidate_measurement (canvas mm, source and metadata path), text (full Soft-ink overlap/physical ink/aspect/alignment receipt), metrics and normalization. PDF candidate canvas mm come from the actual PDF page. Physical ink is measured BEFORE candidate fitting: multiplying printed size by two must fail the size gate even when overlap remains excellent. Pixel fitting uniformly rescales candidate width to reference width and white-pads the common canvas; it never independently stretches width and height. When mm are unavailable, the receipt explicitly states aspect-only validation; do not claim absolute printed size is checked. Default overlap 17% and size deviation 10% remain unchanged.

The Agent-facing compare_figure tool returns the original production Soft-ink overlap / Ink size / alignment / crop / overlay receipt plus its image. Do not wrap that text in a custom metrics JSON or add canvas/source/normalization prose. Structured CLI/JSON consumers can inspect separate measurement fields. Comparison reports measurements; the original submit_figure enforces configured overlap/size limits and retains its original SUBMITTED / REJECTED / best-effort behavior. Do not introduce additional acceptance rules.

## Choose the mode

- Automatic mode: the user supplies a vision/tool model, base URL and API key. The Agent invokes run and the runner owns drawing, all tool calls, iteration and accepted delivery. Default overlap is 17%.
- Semi-automatic mode: the current Agent draws the figure itself, calls documentation/compile/compare tools, reads returned images and iterates. No second drawing model or model key is needed. Default overlap is also 17%. Do not invoke run or request model credentials in this mode.
- Follow the user's requested mode. If the user asks the current Agent to draw and has not supplied another model, use semi-automatic mode. Both modes use LuaDraw source and the same native scoring.

Run compare-figure environment first to discover the actually working compiler route. Prefer usable LuaLaTeX; otherwise use the tested standalone Lua/texlua → generated TikZ → XeLaTeX/pdfLaTeX route. Executable existence alone does not prove usability.

## Mode 1 — automatic runner

The model must support image input and tool calls. The Agent starts one run with the supplied model/base URL and a key environment variable or key file:

    compare-figure run --original reference.png --context-file context.md --out work/auto --model MODEL --base-url BASE_URL --api-key-env FIGURE_MODEL_KEY

Alternatively use --api-key-file /path/to/key.txt; direct --api-key is supported. JSON input accepts api_key as an ephemeral option. Credentials are used only by the model client, not saved in run metadata/transcripts or exported Skill resources. --api openai|responses|anthropic selects the provider protocol; --config accepts JSON/YAML models and session settings.

Default thresholds: min overlap 0.17 (17%), max size deviation 0.10. --pass-threshold uses PERCENT, e.g. --pass-threshold 17. The hidden best-effort submit route remains available only after its configured real attempt minimum; do not preempt normal refinement.

Repeating matching inputs resumes automatically; run --out work/auto --resume reuses saved inputs. An accepted completed result still meeting current gates returns existing artifacts without another model request. The runner handles documentation, edits, compilation, full-image comparison, submission, optional visual checker and accepted-source delivery. Read result.json and surface warnings/uncertain/merges to the user.

## Mode 2 — Agent-led semi-automatic drawing

The current Agent performs the drawing and iteration. A typical CLI loop is:

1. Inspect the reference and surrounding context. Identify geometry, dimensions, labels, line styles, layering and any cross-page continuation. Preserve uncertain marks rather than inventing missing labels.
2. Search LuaDraw usage before inventing methods:

       compare-figure luadraw-doc-search Dpolyline3d

   Search returns text plus official PDF document/page/bbox locations without an image by default. Decide whether a page image is needed. Use `docs --mode view --doc doc/luadraw-doc-en.pdf --page 91` to inspect the located whole page; optionally pass --left/--top/--right/--bottom percentages and --zoom for a region. Search bbox coordinates are PDF points, not percentages. `docs --mode read --doc files/luadraw_base.lua --start-line 1 --end-line 40` reads source text. Explicit --include-image may attach a search crop when requested; do not request images for every lookup automatically.
3. Write editable figure.lua with the Agent's own file tools. Use real LuaDraw geometry, including actual 3D constructions when needed. A single genuine LuaDraw environment in TeX is also accepted. Generated TikZ is a backend artifact; pure hand-written TikZ and embedded reference rasters are rejected as main source.
4. Compile, inspect the returned preview/PDF, and compare against the original:

       compare-figure compile --source figure.lua --out work/compiled
       compare-figure compare --reference reference.png --candidate work/compiled/figure.pdf --threshold 17

   Read the actual pdf path from compile's JSON; the example assumes source name figure.lua. compare returns similarity_percent, metrics, thresholds, pass and images. Default size limit remains 0.10; inspect thresholds.overlap_pass and thresholds.size_pass when pass=false. --reference-mm W,H supplies known printed size; --max-scale-dev adjusts the size gate. --threshold is PERCENT; JSON pass_overlap is a ratio (0.17).
5. Open the returned comparison image. Correct visible geometry/labels/topology/style and rerun compile/compare. --detail, --crop L,T,R,B, --zoom and --worst-region select visual evidence after full-image scoring. A crop does not raise the full-image score. A 17% score is an acceptance floor, not a guarantee of semantic correctness.
6. Once pass=true and visual inspection is satisfactory, stop and deliver the editable LuaDraw source, matching PDF/preview, comparison image and result JSON. In this direct loop no automatic runner or submit call is required. Copy scratch artifacts to a chosen final output directory before another scratch call. Never claim an unsupported or failed rendering is accepted.

### All 11 tools as native CLI calls

For the original complete tool contracts and persistent compile/compare/submit state, use tool (alias agent-tool):

    compare-figure tool --workspace work/agent --original reference.png --name write_file --args '{"path":"figure.lua","content":"...LuaDraw program..."}'
    compare-figure tool --workspace work/agent --name luadraw_doc --args '{"query":"Dpolyline3d"}'
    compare-figure tool --workspace work/agent --name compile --args '{"path":"figure.lua"}'
    compare-figure tool --workspace work/agent --name compare_figure --args '{"detail":"gray"}'
    compare-figure tool --workspace work/agent --name submit --args '{"path":"figure.lua"}'
    compare-figure tool --workspace work/agent --name deliver

Use --args-file for complex source/arguments to avoid shell escaping. This path exposes write_file, edit_file, read_file, grep, compile, view_pdf, compare_figure, submit, image_context, view_image and luadraw_doc without a model. The workspace snapshot persists between commands. Initial --original binds the task; subsequent commands reuse it. This path keeps the original submission gates and accepted-source delivery; the direct compile/compare loop above is sufficient when the Agent manages its own final output.

## Outputs and process contract

- compare/docs/compile without --out reuse one deterministic temporary output slot per current working directory and operation. comparison.png, metrics.json and result.json are overwritten on the next comparison; docs uses 预览/document.png. Repeating a normal call does not accumulate timestamp directories or previews.
- The returned JSON paths are authoritative. --out chooses an explicit location when outputs must be retained. Parallel tasks must use different explicit --out directories. COMPARE_FIGURE_OUTPUT_ROOT relocates the reusable scratch area; no automatic sweeping of user directories occurs.
- Full automatic and tool workspaces retain their deliberate transcript/editable/accepted artifacts. They are not the disposable compare/docs/compile slots.
- tools --request FILE, tools --stdin and tools --jsonl provide the same operations programmatically. inline_images=true (CLI --inline-images) embeds base64; otherwise open images[].path. stdout is JSON only for tool results.
- A successful comparison exits 0 even when pass=false: the command computed a valid result. Read pass/thresholds rather than inferring acceptance from exit status. Operational errors exit 2 (a JSONL stream with failed requests exits 1 at EOF).
- Preserve provenance/licenses, actual compiler backend, warnings, uncertain labels and merges. Do not change external book Markdown; cross-page merge application belongs to the caller.
