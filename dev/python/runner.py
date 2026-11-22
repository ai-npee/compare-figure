#!/usr/bin/env python3
"""Independent Python port of DocVision's production vector-figure stage."""
import argparse
import copy
import hashlib
import json
import math
import os
from pathlib import Path
import shutil
import subprocess
import sys

from PIL import Image
from vector_session import Client, Session, Transcript, SUBMIT_REMINDER, CHECK_FEEDBACK, image_part
from vector_tools import Harness
from luadraw_harness import LuaDrawHarness
from vector_measure import measure_image

ROOT = Path(__file__).resolve().parents[2]
SOURCE_REVISION = "20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7"


def prompt(name, **values):
    text = (ROOT / "prompts" / name).read_text(encoding="utf-8")
    for key, value in values.items():
        text = text.replace("{" + key + "}", str(value))
    return text


def merge(base, overrides):
    out = copy.deepcopy(base)
    for key, value in overrides.items():
        if value is None:
            out.pop(key, None)
        elif isinstance(value, dict) and isinstance(out.get(key), dict):
            out[key] = merge(out[key], value)
        else:
            out[key] = copy.deepcopy(value)
    return out


def load_config(path):
    if not path:
        return {}
    text = Path(path).read_text(encoding="utf-8")
    if Path(path).suffix.lower() in {".yaml", ".yml"}:
        try:
            import yaml
        except ImportError:
            raise ValueError("Reading YAML needs PyYAML; use a JSON configuration or install PyYAML") from None
        config = yaml.safe_load(text)
    else:
        config = json.loads(text)
    if not isinstance(config, dict):
        raise ValueError("Configuration must be an object")
    return config


def model_config(config, name=None):
    registry = config.get("models")
    if not registry:
        return {k: v for k, v in config.items() if k not in {"session", "checker_session", "figure_check", "view", "compile"}}
    name = name or config.get("latex", {}).get("drawing_model", "drawing")
    if name not in registry:
        # Source ResolveModel accepts a wire model name as well as an alias.
        name = next((key for key, value in registry.items() if value.get("model") == name), name)
    def expand(key, seen):
        if key in seen:
            raise ValueError("Model extends cycle: " + key)
        if key not in registry:
            raise ValueError("Model alias not configured: " + str(key))
        entry = registry[key]
        return merge(expand(entry["extends"], seen | {key}), entry) if entry.get("extends") else copy.deepcopy(entry)
    entry = expand(name, set())
    if name != "text" and "text" in registry:
        base = expand("text", set())
        # The host does not inherit image/context/concurrency limits from text.
        for key in ("max_images", "context_limit", "max_concurrency"):
            base.pop(key, None)
        entry = merge(base, entry)
    return entry


def make_client(config, name=None, overrides=None, timeout=180):
    entry = merge(model_config(config, name), overrides or {})
    if name and not config.get("models"):
        entry["model"] = name
    model = entry.pop("model", None) or os.environ.get("OPENAI_MODEL")
    base = entry.pop("base_url", None) or os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1")
    key_env = entry.pop("api_key_env", "OPENAI_API_KEY")
    key = entry.pop("api_key", None) or os.environ.get(key_env)
    if not model or not key:
        raise ValueError("Configure a model and an API key (environment variable " + key_env + ")")
    api_timeout = entry.pop("api_timeout", timeout) or timeout
    return Client(base, model, key, api_timeout, **entry)


def stage_settings(config):
    latex = config.get("latex", {})
    session = config.get("session", latex.get("sessions", {}).get("drawing", {}))
    checker_session = config.get("checker_session", latex.get("sessions", {}).get("checker", {}))
    checks = config.get("figure_check", latex.get("figure_check", {}))
    view = config.get("view", config.get("tools", {}).get("view", {}))
    compile_options = config.get("compile", latex.get("compile", {}))
    return session, checker_session, checks, view, compile_options


def atomic_json(path, value):
    # A same-directory replacement prevents partial metadata after interruption.
    path = Path(path)
    temp = path.with_suffix(path.suffix + ".tmp")
    temp.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")
    temp.replace(path)


class CheckerTools:
    def __init__(self, root):
        self.root = root
        self.state = {"submitted": False, "status": "", "issues": ""}

    def definitions(self):
        return [{"type": "function", "function": {
            "name": "submit", "description": "Declare the comparison of the redrawn figure with the original final. Only call after a clean compile.",
            "parameters": {"type": "object", "properties": {
                "notes": {"type": "string", "description": "Optional short handover notes."},
                "report": {"type": "object", "description": "REQUIRED verdict. status=pass when the redrawn figure faithfully reproduces the original's content and layout; status=issues when it does not, with report.issues listing each visible discrepancy (what is wrong and where).",
                           "properties": {"status": {"type": "string", "enum": ["pass", "issues"]},
                                          "issues": {"type": "string", "description": "Concrete cls/manual/format problems found (empty for pass)."},
                                          "suggestions": {"type": "string", "description": "Concrete suggestions for the cls/manual (empty for pass)."}},
                           "required": ["status"]}}}}}]

    def execute(self, name, args):
        if name != "submit":
            return "TOOL ERROR: unknown tool " + name, None
        if not isinstance(args.get("report"), dict):
            return "REJECTED: submit requires a 'report' object (work report / 工作汇报). Call submit again with report.status = pass or issues.", None
        self.state.update(submitted=True, **args["report"])
        return "SUBMITTED.", None


def verify_figure(harness, session, original, checker_client, tuning, checks, language):
    rounds = max(1, int(checks.get("max_rounds", 2)))
    tools = CheckerTools(harness.root)
    checker = Session(tools, checker_client, Transcript(harness.root / "figure_check.jsonl"),
                      prompt("latex_figurecheck.system.md", MAX_ROUNDS=tuning.get("max_tool_rounds", 50), OUTPUT_LANG=language), tuning)
    warnings = []
    for number in range(1, rounds + 1):
        try:
            png = harness.rasterize(Path(harness.state["last_pdf"]), harness.root / "预览" / f"check_{number}")
            if not png:
                png = harness.root / "预览" / f"check_{number}.png"
            user = prompt("latex_figurecheck.user.md", ORIGINAL=Path(original).name, REDRAWN=Path(png).name)
            if number > 1:
                user = (f"This is round {number} of the same comparison, after the drawing session tried to fix the problems you listed. "
                        "Compare the two images again and submit your verdict.")
            # Source checker sends two images, original first; resize to 1280.
            checker.record({"role": "user", "content": [{"type": "text", "text": user}, image_part(original, 1280), image_part(png, 1280)]})
            checker.run()
        except (ValueError, OSError, RuntimeError, KeyError, subprocess.SubprocessError) as error:
            warnings.append("Figure check unavailable; delivering current version: " + str(error))
            break
        if not tools.state["submitted"]:
            warnings.append("Figure checker did not submit a verdict; delivering current version")
            break
        if tools.state.get("status") != "issues":
            break
        problems = tools.state.get("issues") or "the checker reported visible differences but gave no description"
        if number >= rounds:
            warnings.append("Figure check round limit reached: " + problems)
            break
        before = harness.state["final_code"]
        accepted_before = copy.deepcopy(harness.state) if harness.state.get("source_language") == "luadraw" else None
        if accepted_before is not None:
            # A previous submit must not terminate the new correction run after
            # its first tool call. Keep the accepted version for fail-open delivery.
            harness.state["submitted"] = False
        # Source permits further tools after a prior accepted submit, on the same session.
        try:
            feedback = CHECK_FEEDBACK.replace("{problems}", problems)
            if harness.state.get("source_language") == "luadraw":
                feedback = feedback.replace("figure.tex", harness.state.get("source_path", "figure.lua"))
                feedback += "\nKeep using LuaDraw; consult luadraw_doc for the affected construction."
            session.run(feedback)
        except (ValueError, OSError, RuntimeError) as error:
            if accepted_before is not None:
                harness.state.update(accepted_before)
            warnings.append("Figure correction failed; delivering current version: " + str(error))
            break
        if accepted_before is not None and not harness.state["submitted"]:
            harness.state.update(accepted_before)
            warnings.append("Drawing correction did not submit an accepted revision; delivering previous accepted version")
            break
        if harness.state["final_code"] == before:
            warnings.append("Drawing session made no change after figure check")
            break
    return warnings


def resume_credit(root, threshold, max_scale_dev, accepted):
    result_path = root / "result.json"
    if not accepted or not result_path.exists():
        return None
    result = json.loads(result_path.read_text(encoding="utf-8"))
    overlap = result.get("best_overlap", result.get("similarity_percent", 0) / 100)
    scale = result.get("scale_dev", result.get("scale_dev_at_best", 0))
    if overlap > 0:
        qualifies = (threshold <= 0 or overlap >= threshold) and (max_scale_dev <= 0 or scale <= max_scale_dev)
    else:
        qualifies = threshold <= 0 and max_scale_dev <= 0
    if qualifies and all((root / p).is_file() for p in ("submitted.tex", "submitted.pdf")):
        if result.get("source_language") == "luadraw" and not Path(result.get("editable_source", root / "submitted.lua")).is_file():
            return None
        return result
    return None


def restore_submission(harness, stats, threshold, max_scale_dev):
    saved = stats.get("submitted_state")
    if not stats.get("accepted") or not isinstance(saved, dict) or not saved.get("final_code", "").strip():
        return False
    overlap, size = saved.get("best_overlap", 0), saved.get("scale_dev_at_best", 0)
    passed = (threshold <= 0 or overlap >= threshold) and (max_scale_dev <= 0 or size <= max_scale_dev)
    same_best_effort = (saved.get("accepted_as_is") and saved.get("_accepted_threshold") == threshold
                        and saved.get("_accepted_scale_limit") == max_scale_dev)
    if not passed and not same_best_effort:
        return False
    state = {k: v for k, v in saved.items() if not k.startswith("_")}
    if state.get("last_pdf"):
        pdf = Path(state["last_pdf"]).resolve()
        if not pdf.is_relative_to(harness.root):
            raise ValueError("Saved submission PDF escapes the workspace")
        if not pdf.exists():
            state["last_pdf"] = ""  # deliver re-compiles the accepted final_code, as in tikz.go.
    harness.state.update(state)
    return True


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--original", type=Path)
    parser.add_argument("--context", default="")
    parser.add_argument("--context-file", type=Path)
    parser.add_argument("--source-md", type=Path, help="Full Markdown for expandable neighbour-image context")
    parser.add_argument("--image-ref", help="Current image reference exactly as in the Markdown")
    parser.add_argument("--images-dir", type=Path, help="Image root (usually the Markdown's images/ directory)")
    parser.add_argument("--reference-mm", nargs=2, type=float, metavar=("WIDTH", "HEIGHT"), help="Original printed canvas size, mm; omit to compare aspect ratio")
    parser.add_argument("--out", type=Path, help="Required for a drawing session")
    parser.add_argument("--config", type=Path, help="JSON or DocVision-compatible YAML model/stage configuration")
    parser.add_argument("--base-url")
    parser.add_argument("--model")
    parser.add_argument("--api", choices=["openai", "responses", "anthropic"])
    parser.add_argument("--api-key-env")
    parser.add_argument("--pass-threshold", type=float, help="Overlap percent; source default 17, <=0 disables")
    parser.add_argument("--max-scale-dev", type=float, help="Size deviation fraction; default 0.10, <0 disables")
    parser.add_argument("--accept-min-submits", type=int, help="Attempts before best-effort acceptance; source default 8")
    parser.add_argument("--max-turns", "--max-tool-rounds", type=int, dest="max_turns", help="Soft tool-round budget per Run; 0/negative unlimited")
    parser.add_argument("--max-tokens", type=int)
    parser.add_argument("--timeout", type=int, default=180)
    parser.add_argument("--engine", choices=["auto", "lualatex", "xelatex", "pdflatex"], help="LuaLaTeX preferred; other engines use standalone LuaDraw preprocessing")
    parser.add_argument("--luadraw-root", type=Path, help="Pinned LuaDraw distribution root")
    parser.add_argument("--comparison-backend", choices=["auto", "python", "go"], default=None)
    parser.add_argument("--compare-binary", type=Path)
    parser.add_argument("--environment", action="store_true", help="Print toolchain JSON without a model key or session output")
    parser.add_argument("--output-lang", default=None)
    parser.add_argument("--figure-check", action="store_true", help="Enable optional visual checker and same-session corrections")
    parser.add_argument("--checker-model")
    parser.add_argument("--resume", action="store_true")
    parser.add_argument("--check", action="store_true", help="Preflight only; no API calls or output creation")
    args = parser.parse_args(argv)
    config = load_config(args.config)
    if args.environment:
        from luadraw_runtime import probe
        info = probe(args.luadraw_root or config.get("luadraw_root"))
        print(json.dumps(info, ensure_ascii=False, indent=2))
        return 0 if info.get("ready") else 2
    if not args.out:
        parser.error("--out is required for a drawing session")
    tuning, checker_tuning, checks, view, compile_options = stage_settings(config)
    tuning, checks = copy.deepcopy(tuning), copy.deepcopy(checks)
    overrides = {key: getattr(args, key) for key in ("base_url", "api", "api_key_env") if getattr(args, key) is not None}
    client = make_client(config, args.model, overrides, args.timeout)
    if args.max_turns is not None:
        tuning["max_tool_rounds"] = args.max_turns
    tuning.setdefault("max_tool_rounds", 128)
    if args.max_tokens is not None:
        tuning["max_tokens"] = args.max_tokens
    for key in ("context_limit", "max_tokens"):
        value = client.options.get(key, 0)
        if value > 0:
            tuning[key] = min(tuning.get(key, value) or value, value)
    engine = args.engine or compile_options.get("engine", "auto")
    for program in ("pdfinfo", "pdftoppm"):
        if not shutil.which(program):
            parser.error("Missing dependency: " + program)
    root = args.out.resolve()
    metadata = root / "run.json"
    language = args.output_lang or config.get("output_lang", config.get("options", {}).get("output_language", "Chinese"))
    overlap = checks.get("min_overlap", .17) or .17
    scale = checks.get("max_scale_dev", .10) or .10
    attempts = checks.get("accept_min_submits", 8) or 8
    if args.resume:
        meta = json.loads(metadata.read_text(encoding="utf-8"))
        if meta.get("version") != 3:
            parser.error("This directory predates the LuaDraw stage; start a new output directory")
        reference = root / meta["reference"]
        if not reference.resolve().is_relative_to(root) or hashlib.sha256(reference.read_bytes()).hexdigest() != meta["reference_sha256"]:
            parser.error("Stored reference changed or escapes the output directory")
        if args.original or args.context or args.context_file or args.source_md or args.image_ref or args.images_dir or args.reference_mm:
            parser.error("Resume reuses stored inputs; omit image/context/source flags")
        context, options = meta["context"], meta["options"]
        overlap, scale, attempts = meta["fig_min_overlap"], options["max_scale_dev"], options["accept_min_submits"]
    else:
        if not args.original:
            parser.error("--original is required for a new session")
        if root.exists() and (not root.is_dir() or any(root.iterdir())):
            parser.error("New session output must be an empty directory")
        reference = args.original.resolve()
        context = args.context
        if args.context_file:
            context += "\n" + args.context_file.read_text(encoding="utf-8")
        md_content = args.source_md.read_text(encoding="utf-8") if args.source_md else ""
        current_img = args.image_ref or ("images/" + reference.name)
        if not md_content:
            # Standalone input adapter provides a one-image document; source tool stays unchanged.
            md_content = context + "\n\n![](" + current_img + ")\n"
        images_dir = args.images_dir.resolve() if args.images_dir else (args.source_md.resolve().parent / "images" if args.source_md else reference.parent)
        parse_root = config.get("parse_root", config.get("paths", {}).get("mineru_output", ""))
        measurement = measure_image(reference, parse_root, args.reference_mm)
        context_options = config.get("tools", {}).get("context", {})
        options = {"md_content": md_content, "current_img": current_img, "images_dir": str(images_dir),
                   "max_up": config.get("max_up", context_options.get("max_up", 50)) or 50,
                   "max_down": config.get("max_down", context_options.get("max_down", 50)) or 50,
                   "view_image_max": (view.get("image_max", 30) or 30) if view.get("image_max", 0) >= 0 else 0,
                   "view_pdf_max": (view.get("pdf_max", 25) or 25) if view.get("pdf_max", 0) >= 0 else 0,
                   "warn_ratio": view.get("warn_ratio", .7), "max_scale_dev": scale, "accept_min_submits": attempts,
                   "engine": engine, "compile_timeout": compile_options.get("timeout", 120) or 120,
                   "raster_dpi": compile_options.get("raster_dpi", 110),
                   "raster_command": compile_options.get("raster_command", "pdftoppm"),
                   "reference_mm": [measurement.width_mm, measurement.height_mm],
                   "measurement": measurement.to_dict(), "parse_root": str(parse_root)}
    if args.pass_threshold is not None:
        overlap = args.pass_threshold / 100
    if args.max_scale_dev is not None:
        scale = args.max_scale_dev
    if args.accept_min_submits is not None:
        attempts = args.accept_min_submits
    if not all(math.isfinite(v) for v in (overlap, scale)) or overlap > 1:
        parser.error("Overlap must be finite and <=100%; size deviation must be finite")
    if args.timeout <= 0 or tuning.get("max_tokens", 16384) <= 0:
        parser.error("Timeout and output token budget must be positive")
    if options["reference_mm"] and any(not math.isfinite(v) or v < 0 for v in options["reference_mm"]):
        parser.error("Original dimensions must be finite nonnegative millimetres")
    with Image.open(reference) as image:
        image.verify()
    from luadraw_runtime import probe
    luadraw_root = args.luadraw_root or config.get("luadraw_root", options.get("luadraw_root"))
    environment = probe(luadraw_root)
    if not environment.get("ready"):
        parser.error("No usable LuaDraw toolchain: " + str(environment.get("probe_log", environment)))
    if args.check:
        print(json.dumps({"ok": True, "environment": environment, "api_contacted": False}, ensure_ascii=False, indent=2))
        return 0
    root.mkdir(parents=True, exist_ok=True)
    if not args.resume:
        media = root / "media"
        media.mkdir(exist_ok=True)
        stored = media / ("original" + reference.suffix.lower())
        shutil.copyfile(reference, stored)
        reference = stored
        meta = {"version": 3, "source_revision": SOURCE_REVISION, "reference": str(reference.relative_to(root)),
                "reference_sha256": hashlib.sha256(reference.read_bytes()).hexdigest(), "context": context,
                "options": options, "fig_min_overlap": overlap}
    options.update(max_scale_dev=scale, accept_min_submits=attempts, engine=engine)
    options["luadraw_root"] = str((args.luadraw_root or Path(config.get("luadraw_root", options.get("luadraw_root") or ROOT / "vendor/luadraw-3.7/luadraw"))).resolve())
    options["comparison_backend"] = args.comparison_backend or config.get("comparison_backend", options.get("comparison_backend", "auto"))
    options["compare_binary"] = str(args.compare_binary.resolve()) if args.compare_binary else config.get("compare_binary", options.get("compare_binary"))
    meta.update(options=options, fig_min_overlap=overlap)
    atomic_json(metadata, meta)
    def live_threshold():
        return json.loads(metadata.read_text(encoding="utf-8")).get("fig_min_overlap")
    harness = LuaDrawHarness(root, reference, context, overlap * 100, args.timeout, **options, live_threshold=live_threshold)
    system = prompt("luadraw_figure.system.md", MAX_ROUNDS=tuning["max_tool_rounds"], OUTPUT_LANG=language,
                    LUADRAW_VERSION=environment.get("luadraw_version") or "unknown", ENVIRONMENT=json.dumps(environment, ensure_ascii=False))
    transcript = Transcript(root / "session.jsonl")
    messages, stats = transcript.load(system) if args.resume else (None, {"rounds": 0, "tools": 0, "submit_attempts": 0, "accepted": False})
    resumed_submission = False
    if args.resume:
        credit = resume_credit(root, overlap, scale, stats["accepted"])
        if credit:
            print("Previously accepted figure still meets the current thresholds; delivering existing artifacts without a model call.")
            harness.close()
            return 0
        resumed_submission = restore_submission(harness, stats, overlap, scale)
        if not resumed_submission and stats.get("submit_args"):
            # Source replaySubmit retries the last completed submit against
            # fresh compile/compare state; rejection resumes the drawing session.
            harness.execute("submit", json.loads(stats["submit_args"]))
    session = Session(harness, client, transcript, system, tuning, messages)
    session.rounds, session.tools = stats["rounds"], stats["tools"]
    if not args.resume:
        size_hint = harness.measure_hint()
        image_max, pdf_max = options["view_image_max"], options["view_pdf_max"]
        budget = (f"View budget: {image_max} looks at the original image, {pdf_max} at any one page of your compiled PDF." if image_max > 0
                  else f"View budget: unlimited looks at the original image, {pdf_max} at any one page of your compiled PDF.")
        initial = prompt("luadraw_figure.user.md", CONTEXT=context[:4000], ORIGINAL_SIZE=size_hint, VIEW_BUDGET=budget)
    else:
        initial = ""  # Source resumeUserText: no injected user text or repeated image.
    if not resumed_submission:
        session.run(initial, [reference] if not args.resume else None)
        if not harness.state["submitted"]:
            session.run(SUBMIT_REMINDER.replace("figure.tex", "figure.lua").replace("the file must match", "the LuaDraw source must match"))
    if not harness.state["submitted"]:
        transcript.event({"t": "stop", "reason": "模型未提交最终图形", "fallback": True})
        print("No accepted submission. Original retained; workspace and transcript kept for --resume.")
        harness.close()
        return 2
    warnings = []
    if args.figure_check or checks.get("enabled", False):
        checker_name = args.checker_model or checks.get("model") or ("verifier" if config.get("models") else None)
        try:
            checker_client = make_client(config, checker_name, timeout=args.timeout) if checker_name else client
        except ValueError:
            checker_client = client
        checker_tuning = {"max_tool_rounds": 50, **checker_tuning}
        warnings = verify_figure(harness, session, reference, checker_client, checker_tuning, checks, language)
    result = harness.deliver()
    if result.get("warning"):
        warnings.append(result["warning"])
    result.update(source_revision=SOURCE_REVISION, warnings=list(result.get("warnings", [])) + warnings)
    atomic_json(root / "result.json", result)
    transcript.event({"t": "delivered", "result": result})
    print("Submitted: " + str(root / "submitted.pdf"))
    for warning in warnings:
        print(warning, file=sys.stderr)
    harness.close()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, OSError, RuntimeError, KeyError, TypeError) as error:
        print("runner: " + str(error), file=sys.stderr)
        raise SystemExit(1)
