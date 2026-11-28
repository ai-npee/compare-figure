---
name: compare-figure
description: "Create original figures or redraw references with configurable LuaDraw, native LaTeX, Asymptote, or mixed drawing policies. Shared doc-library lookup, automatic and Agent-led drawing; reference comparison defaults to 17% overlap."
license: MIT
metadata:
  version: "0.3.0"
---

# compare-figure: two work modes

The Skill package contains Go source and runtime resources, with no prebuilt executable. Before first use, the Agent checks for Go 1.25+ (`go version`); official downloads and installation instructions are https://go.dev/dl/ and https://go.dev/doc/install. From this Skill root run `go -C go build -o ../build/compare-figure ./cmd/compare-figure` (Windows: `go -C go build -o ../build/compare-figure.exe ./cmd/compare-figure`). Go builds for the current host by default; use the existing executable for subsequent calls, and rebuild after source updates. Go modules listed in go/go.mod may need downloading on the first build. Use a writable cache, for example `GOCACHE="$PWD/build/go-cache"` on POSIX systems.
Resolve the executable and resources relative to this SKILL.md, not the caller working directory. Use an absolute executable path when operating inside a book directory. The examples use compare-figure as shorthand for that full binary path. Execution is native Go; Python is not required. Lua/TeX/Poppler remain detected external tools. Asymptote modes also require a working `asy` executable; see install.md. The independent doc-library module is a fixed upstream Go dependency downloaded during the first build; no sibling checkout is required.

## Before automatic runs: discover configuration

Do this before asking the user for model, URL or key details. Automatic configuration applies to `run`/`auto` and `tools` with `operation: "run"`; standalone `compile`, `docs`, `compare`, `tool` and `environment` do not load it. `run --environment` follows the same discovery rules.

1. Check whether the request already provides `--config PATH` or JSON `config`. Otherwise check `COMPARE_FIGURE_CONFIG`.
2. If neither selects a path, inspect the process working directory for `runner.yaml`, then `runner.yml`, then `runner.json`; only then inspect the Skill/resource root in that same filename order. Deduplicate when cwd equals the root. Do not scan parent directories or HOME.
3. Select only the first match; files are not merged. Explicit/env paths that are missing, invalid or unreadable fail rather than falling back. A discovered file that cannot be parsed also fails. Relative paths inside it resolve from process cwd.
4. Do not automatically load `runner.local.yaml` or its `.yml`/`.json` variants. If one is clearly the intended private config, select it explicitly with `--config PATH` or `COMPARE_FIGURE_CONFIG=PATH`. Never read or print credentials merely to discover settings. Confirm only the config path and that expected keys exist; let the program read secret values.
5. If an existing applicable config is present, use it. Do not ask again for model/URL/key. If none exists, copy `examples/runner.yaml.example` to cwd as `runner.yaml`, then request only genuinely missing connection details. The example is documentation, not an automatically loaded config. `run.json`, `result.json`, `agent-state` and session files are runtime state, not model configuration.
6. Before any model request, run `compare-figure run --config PATH --original IMAGE --out DIR --check` (or use the discovered default) and verify success. `--check` does not contact the API or create session files. Then start the actual run. Task inputs remain explicit: `--original` or `--source-md` plus `--image-ref`, and `--out`.

`--config none` (also JSON `"config":"none"`) is a reserved value that disables file discovery while retaining CLI options, environment defaults and built-in defaults. The environment variable is second only in selecting the config file; it does not mean all environment variables override file API fields. For details and actual config keys see [configuration guide](docs/configuration.md). Preserve user-adjusted budgets when preparing a new local file; do not overwrite an existing config.

Copy `examples/runner.yaml.example` only when no reusable config exists. Actual runner.yaml/runner.json and *.local.yaml/JSON files are ignored by Git and excluded from Skill exports. Never publish real credentials.

## Drawing policy (independent of work mode and TeX engine)

Select `drawing.mode` in YAML, `--drawing-mode` in CLI, or top-level `drawing_mode` in JSON:

| Policy | Allowed source | Bundled documentation |
|---|---|---|
| `luadraw` (default) | LuaDraw `.lua` or genuine LuaDraw environment in `.tex` | LuaDraw |
| `latex` | Native `.tex`: TikZ, PGFPlots, picture and installed TeX packages | No special-library manual; use existing TeX knowledge or configured document sources |
| `asymptote` | `.asy` or `.tex` with an `asy` environment | Asymptote |
| `mixed` | Model selects LuaDraw, Asymptote or native LaTeX per figure | Both LuaDraw and Asymptote |

Drawing prompts are file-based and share the complete workflow and user-context structure; see `docs/prompt-guide.md`. Backend policies change the permitted language, source examples, reference sources and compilation guidance without dropping visual checks, size requirements, uncertainty handling or cross-page rules. The unused historical `latex_figure.system.md`/`latex_figure.user.md` templates were removed; `latex_figurecheck.*` still drives optional visual verification. New templates apply to new sessions; a resumed session preserves its recorded system prompt.

Honor the chosen policy. Do not silently switch to a forbidden library when compilation fails. `--engine` still selects the TeX engine, not the drawing policy. Do not embed the reference raster instead of reconstructing the figure. A source file uses one selected backend; mixed does not require using all libraries together. Asymptote output is a static PDF for comparison, not an interactive WebGL delivery.

Accepted delivery preserves the actual `.lua`, `.asy` or `.tex` source and a standalone editable `submitted.tex` with `submitted.pdf`. LuaDraw retains its embedded official environment and independent Lua fallback. Asymptote TeX embeds the Asy program; compilation runs TeX, explicitly invokes `asy` on generated figure sources, then reruns TeX. A TeX editor alone needs that extra Asymptote build step; use this Skill's compile command to perform it. Native LaTeX retains its editable document. Read `source_language`, `drawing_mode`, `backend` and `engine` in the result rather than assuming LuaDraw. Keep warnings and uncertainty. Policy changes require a separate workspace or a compatible resume; do not mix an accepted drawing from a different policy into a run.

## Original drawing and standalone documentation — no reference image required

When the user wants a new diagram, use the current Agent's reasoning and file tools. Add `--drawing-mode POLICY` to compile/docs/tool/run commands when using a nondefault policy. No reference bitmap, Markdown book, image_context workspace, second model or API key is required. Consult documentation before inventing API calls:

    compare-figure document-search --mode frequent --cache-dir work/shared-doc-cache
    compare-figure document-search --query graph3d --snippet-chars 1200 --max-chars 6000 --cache-dir work/shared-doc-cache

Use list/find/grep to discover real files, read exact constructors/methods/examples, and view a PDF page/crop only when helpful. Write source allowed by the drawing policy, compile it with `--drawing-mode`, and inspect the generated PDF/preview. For a new drawing without a reference, judge the user-requested geometry, labels and style; no reference comparison score or submit acceptance is available. Deliver editable TeX and original language source plus PDF after compilation succeeds. Run compare only if the user supplies a reference.

The documentation command is also an independent machine interface:

    compare-figure document-search --stdin --cache-dir work/shared-doc-cache
    compare-figure document-search --request examples/luadraw-doc.json.example
    compare-figure document-search --jsonl --cache-dir work/shared-doc-cache

Supply a bare JSON tool object (`{"mode":"search","query":"Dpolyline3d"}`) or an envelope (`{"operation":"docs","arguments":{"mode":"search","query":"Dpolyline3d"},"cache_dir":"work/shared-doc-cache"}`). JSONL keeps one local process and index objects alive across requests; it is not a model session or a network server. A bad line returns a JSON error and subsequent lines continue. Prefer one shared writable cache directory across agent sessions; use separate --out directories for parallel PDF previews. The existing `tools --jsonl` envelope interface remains available. See docs/luadraw-doc.md for calling contracts, Go API, dependencies and cache controls. The generic wrapper uses the independent doc-library Go module; it provides source-qualified results from the manuals allowed by the drawing policy. Use `--drawing-mode mixed --doc-source asymptote` or JSON `source:"asymptote"` to select a source. `luadraw-doc` and the old JSON docs envelope remain compatibility entrances. See docs/doc-library-compatibility.md for differences and cache-layout changes.

## Documentation navigation and reusable paragraphs

For LuaDraw surface meshes and intersecting surfaces, consult the [surface rendering reference](docs/luadraw-surface-rendering.md) before treating `Dscene3d` as a universal default. It records tested `Dfacet`/`Dmixfacet` alternatives and the limits of implicit clipping; it does not mandate one method for every figure. Bundled documentation sources are declared with the standard doc-library configuration in `docs/document-sources/<drawing-mode>.yaml`; source roots resolve relative to that YAML. The configured `luadraw` source includes both the unchanged official library and the explicitly allowed project reference in one index, cache namespace and frequency ranking. Omit `source` to search all sources configured for the drawing mode, or use `source:"luadraw"` for LuaDraw and `source:"asymptote"` for Asymptote. Read with the exact returned source and document path: configured LuaDraw paths are relative to the package root, such as `vendor/luadraw-3.7/luadraw/doc/luadraw-doc-en.pdf` and `docs/luadraw-surface-rendering.md`. Project notes remain project-maintained, not upstream documentation or a general rendering fix.

`document_search` is the knowledge-base tool; `luadraw_doc` remains an execution alias. Use `mode:list` with a relative `doc` directory to discover actual files (optionally `recursive:true`); `mode:find` searches file names/globs; `mode:grep` searches literal text (`regex:true` opts into a regular expression). Follow `\\input`/`\\include` references using these modes and `mode:read`; do not guess manual section filenames. Access stays within the selected configured document roots; `source` selects luadraw/asymptote in mixed mode.

`mode:read` accepts source files with 1-based start_line/end_line, or a PDF doc (also accepted in query) with page for text. `mode:view` displays a PDF whole page or a percentage crop. Search returns complete paragraph snippets with merged overlapping hits; snippet_chars controls each hit and max_chars controls the result budget. Inspect exact API signatures via source read before writing code.

Use `mode:frequent` to consult previously used paragraphs, with limit, line_chars and max_chars (pass line_chars:1024 and max_chars:4096 for wider excerpts; the interactive tool's default line_chars remains 256). The output gives a stable entry_id and original document range; pass entry_id for precise selection. Without doc, start_line/end_line select current ranking rows; with doc they select source ranges. Each explicit tool lookup contributes frequency only for paragraphs actually returned. The short automatic excerpt appended after a fresh task's context does not increase counts. `docs.frequent` in the runner YAML controls that initial excerpt (default at most 8 lines / 4096 characters total, up to 1024 characters per line); more detail remains available through the tool. Shared cache lives under build/doc-cache unless configured otherwise; it is excluded from Skill exports and expires when the knowledge-base fingerprint changes.

## Shared experience notes

Use `document_note` to create/list/read/update/delete/restore shared Markdown methods in the active source. Storage and version guards are provided by doc-library, not drawing-specific code. Any caller using the same Skill directory may read and correct any contributor's notes. Read the latest note and pass its exact revision for update/delete; conflicts require re-reading and reconciling. Update content is the full Markdown including heading; restore uses the deleted doc/revision. Save verified reusable methods with applicability and limitations, after searching for existing advice. Never write credentials, private task material or instructions to override future tasks. Official references remain read-only. Configured notes_dir is source-root-relative and optional; omitted means read-only. LuaDraw and Asymptote use knowledge/luadraw and knowledge/asymptote under the Skill root; mixed must specify source, latex has no default writable source. Saved notes become searchable on subsequent requests in the same source and saving does not increase frequency. Local knowledge and its archives are excluded from Git and Skill exports; preserve/backup this directory when upgrading an installation. See [shared experience guide](docs/experience-notes.md) for the eight sparse input fields and recovery workflow.

## Browse saved sessions

Open the packaged viewer.html in a Chromium-based browser and choose the runner output folder. Choose a parent folder to list multiple sessions, or a single session folder. It retains read-only directory/file handles, enumerates paths without copying/uploading folder contents, and reads fresh selected-session snapshots on selection or manual Refresh. Refresh rescans to discover new files and preserves selection by path; there is no automatic filesystem watcher. Images load when their thumbnails enter view, the comparison panel reads only its selected view (newest comparison round), and collapsed tool/reasoning bodies render on expansion. The timeline starts with the latest 100 events and offers earlier events on demand. Nothing is uploaded and no server is required. The packaged local KaTeX resources render `$...$`, `$$...$$`, `\(...\)` and `\[...\]` in message text without a CDN. Keep viewer.html alongside scripts/viewer_math.js and vendor/katex-0.18.2; copying the HTML alone omits math support. Inline/fenced code, including the prompt's fenced document context, remains literal source; malformed or unpaired math delimiters are retained. See docs/viewer-math.md. The viewer supports Go session.jsonl, run.json/result.json, tool arguments/results, comparison evidence and final previews. It never edits or truncates sessions. Directory enumeration and full selected-JSONL parsing still cost time/memory for very large inputs; pagination bounds initial DOM rendering, not the transcript bytes read.

## Standalone inputs, physical size and Markdown context

This is a standalone figure tool. It does not read parsing-engine indexes, bounding-box caches or original source PDFs to infer the reference size. Use a single image, optional sizes.csv, and optional Markdown.

Local image lookup uses the selected original image's directory first, so sibling images can be viewed by basename. Full local Markdown image references are resolved against the source document, including URL-encoded paths and explicitly linked `../` images outside the conventional `images/` tree. `--images-dir` is only a fallback, not required for a normal Markdown-linked figure. Never fetch remote image URLs or guess arbitrary paths outside the configured roots or explicit Markdown references.

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
- Follow the user's requested mode. If the user asks the current Agent to draw and has not supplied another model, use semi-automatic mode. Both work modes use the selected drawing policy and the same native scoring.

Run compare-figure environment --drawing-mode POLICY first to discover the actually working compiler route. Prefer usable LuaLaTeX; otherwise use the tested standalone Lua/texlua → generated TikZ → XeLaTeX/pdfLaTeX route. Executable existence alone does not prove usability.

## Mode 1 — automatic runner

The model must support image input and tool calls. The Agent starts one run with the supplied model/base URL and a key environment variable or key file:

    compare-figure run --original reference.png --context-file context.md --out work/auto --model MODEL --base-url BASE_URL --api-key-env FIGURE_MODEL_KEY

Alternatively use --api-key-file /path/to/key.txt; direct --api-key is supported. JSON input accepts api_key as an ephemeral option. Credentials are used only by the model client, not saved in run metadata/transcripts or exported Skill resources. --api openai|responses|anthropic selects the provider protocol; --config accepts JSON/YAML models and session settings.

Default thresholds: min overlap 0.17 (17%), max size deviation 0.10. --pass-threshold uses PERCENT, e.g. --pass-threshold 17. The hidden best-effort submit route remains available only after its configured real attempt minimum; do not preempt normal refinement.

Repeating matching inputs resumes automatically; run --out work/auto --resume reuses saved inputs. An accepted completed result still meeting current gates returns existing artifacts without another model request. The runner handles documentation, edits, compilation, full-image comparison, submission, optional visual checker and accepted-source delivery. Read result.json and surface warnings/uncertain/merges to the user.

## Mode 2 — Agent-led semi-automatic drawing

The current Agent performs the drawing and iteration. A typical CLI loop is:

1. Inspect the reference and surrounding context. Identify geometry, dimensions, labels, line styles, layering and any cross-page continuation. Preserve uncertain marks rather than inventing missing labels.
2. Search the selected library documentation before inventing methods:

       compare-figure document-search Dpolyline3d

   Search returns text with source/document/PDF page or source-line locations without an image by default; bbox is opt-in via `include_bbox:true` (CLI `--include-bbox=true`). Omit include_bbox to inherit the startup default, or pass explicit false to override it. Model text omits hash/cache metadata, PDF point coordinates and page dimensions. Decide whether a page image is needed. Use `docs --mode view --doc vendor/luadraw-3.7/luadraw/doc/luadraw-doc-en.pdf --page 91` to inspect the located whole page; optionally pass --left/--top/--right/--bottom percentages and --zoom for a region. Opt-in `bbox_pct` coordinates are percentages and can be used directly for a view crop; PDF geometry remains available internally. `docs --mode read --doc vendor/luadraw-3.7/luadraw/files/luadraw_base.lua --start-line 1 --end-line 40` reads source text. Explicit --include-image may attach a search crop when requested, independently of bbox output; do not request images for every lookup automatically.
3. Write editable figure.lua (luadraw), figure.asy (asymptote), or figure.tex (latex/mixed). Use real geometry, including actual 3D constructions when needed. Pure hand-written TikZ is allowed in latex/mixed and rejected in luadraw/asymptote. Generated artifacts do not replace the accepted source. Embedding the reference raster is not redrawing.
4. Compile, inspect the returned preview/PDF, and compare against the original:

       compare-figure compile --source figure.lua --out work/compiled
       compare-figure compare --reference reference.png --candidate work/compiled/figure.pdf --threshold 17

   Read the actual pdf path from compile's JSON; the example assumes source name figure.lua. compare returns similarity_percent, metrics, thresholds, pass and images. Default size limit remains 0.10; inspect thresholds.overlap_pass and thresholds.size_pass when pass=false. --reference-mm W,H supplies known printed size; --max-scale-dev adjusts the size gate. --threshold is PERCENT; JSON pass_overlap is a ratio (0.17).
5. Open the returned comparison image. Correct visible geometry/labels/topology/style and rerun compile/compare. --detail, --crop L,T,R,B, --zoom and --worst-region select visual evidence after full-image scoring. A crop does not raise the full-image score. A 17% score is an acceptance floor, not a guarantee of semantic correctness.
6. Once pass=true and visual inspection is satisfactory, stop and deliver the editable drawing source, matching PDF/preview, comparison image and result JSON. In this direct loop no automatic runner or submit call is required. Copy scratch artifacts to a chosen final output directory before another scratch call. Never claim an unsupported or failed rendering is accepted.

### All 11 tools as native CLI calls

For the original complete tool contracts and persistent compile/compare/submit state, use tool (alias agent-tool):

    compare-figure tool --workspace work/agent --original reference.png --name write_file --args '{"path":"figure.lua","content":"...LuaDraw program..."}'
    compare-figure tool --workspace work/agent --name document_search --args '{"query":"Dpolyline3d"}'
    compare-figure tool --workspace work/agent --name compile --args '{"path":"figure.lua"}'
    compare-figure tool --workspace work/agent --name compare_figure --args '{"detail":"gray"}'
    compare-figure tool --workspace work/agent --name submit --args '{"path":"figure.lua"}'
    compare-figure tool --workspace work/agent --name deliver

Use --args-file for complex source/arguments to avoid shell escaping. This path exposes write_file, edit_file, read_file, grep, compile, view_pdf, compare_figure, submit, image_context, view_image and document_search (legacy luadraw_doc alias) without a model. The workspace snapshot persists between commands. Initial --original binds the task; subsequent commands reuse it. This path keeps the original submission gates and accepted-source delivery; the direct compile/compare loop above is sufficient when the Agent manages its own final output.

## Outputs and process contract

- compare/docs/compile without --out reuse one deterministic temporary output slot per current working directory and operation. comparison.png, metrics.json and result.json are overwritten on the next comparison; docs uses 预览/document.png. Repeating a normal call does not accumulate timestamp directories or previews.
- The returned JSON paths are authoritative. --out chooses an explicit location when outputs must be retained. Parallel tasks must use different explicit --out directories. COMPARE_FIGURE_OUTPUT_ROOT relocates the reusable scratch area; no automatic sweeping of user directories occurs.
- Full automatic and tool workspaces retain their deliberate transcript/editable/accepted artifacts. They are not the disposable compare/docs/compile slots.
- tools --request FILE, tools --stdin and tools --jsonl provide the same operations programmatically. inline_images=true (CLI --inline-images) embeds base64; otherwise open images[].path. stdout is JSON only for tool results.
- A successful comparison exits 0 even when pass=false: the command computed a valid result. Read pass/thresholds rather than inferring acceptance from exit status. Operational errors exit 2 (a JSONL stream with failed requests exits 1 at EOF).
- Preserve provenance/licenses, actual compiler backend, warnings, uncertain labels and merges. Do not change external book Markdown; cross-page merge application belongs to the caller.
