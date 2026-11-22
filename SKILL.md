---
name: compare-figure
description: "Redraw and compare figures with LuaDraw, using either an automatic model runner or Agent-led CLI tools. Supports images, sizes.csv and Markdown context; default overlap is 17%."
license: MIT
metadata:
  version: "0.1.2"
---

# compare-figure: two work modes

In an exported Skill, select bin/<host-os>-<host-arch>/compare-figure (Windows: compare-figure.exe); packaged targets are listed in manifest.json. The default release contains only Linux amd64; for other hosts, build the needed target from the source checkout with `scripts/export_skill.sh --platforms GOOS/GOARCH --out-dir dist/GOOS-GOARCH`. In a source checkout, use build/compare-figure. If it is missing, build from the Skill root with Go 1.25+: `go -C go build -o ../build/compare-figure ./cmd/compare-figure`. Resolve the executable and resources relative to this SKILL.md, not the caller working directory. Use an absolute executable path when operating inside a book directory.
The examples use compare-figure as shorthand for that full binary path. Installed execution is Go-only; Python and Go compilers are not needed. Lua/TeX/Poppler are detected external tools.

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

   This returns text, page/bbox evidence and a relevant official PDF image in the same call. Read that image. Use docs --mode read --doc files/luadraw_base.lua --start-line 1 --end-line 40 for source, or docs --mode view --doc doc/luadraw-doc-en.pdf --page 91 with optional --left/--top/--right/--bottom percentages and --zoom for a manual crop.
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
