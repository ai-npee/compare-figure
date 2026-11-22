"""LuaDraw policy and documentation/comparison adapters over migrated tools."""
import copy
import json
from pathlib import Path
import shutil

from PIL import Image

from compare_backend import GoWorker, binary_path, compare
from vector_tools import Harness as ProductionHarness, _safe


class LuaDrawHarness(ProductionHarness):
    def __init__(self, *args, **options):
        super().__init__(*args, **options)
        self.docs = None
        self.worker = None
        backend = options.get("comparison_backend", "auto")
        executable = options.get("compare_binary")
        if backend == "go" or (backend == "auto" and binary_path(executable)):
            self.worker = GoWorker(executable, timeout=self.timeout)
        self.state.update(source_path="figure.lua", source_language="luadraw", drawing_backend="", generated_tex="")

    def __del__(self):
        try:
            self.close()
        except Exception:
            pass

    def close(self):
        if self.worker:
            self.worker.close()

    def definitions(self):
        from luadraw_docs import LuaDrawDocs
        definitions = super().definitions()
        for tool in definitions:
            fn = tool["function"]
            if fn["name"] == "write_file":
                fn["description"] = "Write editable LuaDraw .lua source or a LaTeX .tex body containing a luadraw environment into your workspace. Full content replaces the file. Also allows .cls .sty .md. Use edit_file for small changes."
            elif fn["name"] == "compile":
                fn["description"] = ("Compile an editable LuaDraw program (default figure.lua), or a .tex body containing a genuine luadraw environment. "
                    "Do not pass code inline. The host prefers tested LuaLaTeX; if unavailable it evaluates the SAME LuaDraw program using standalone Lua/texlua, "
                    "then compiles generated TikZ with XeLaTeX/pdfLaTeX. Pure hand-written TikZ and reference-raster embedding are rejected. "
                    "Inspect standalone.pdf with view_pdf and compare_figure before submit. The receipt reports the actual backend and errors.")
                fn["parameters"]["properties"]["path"]["description"] = "Workspace-relative LuaDraw .lua program (default figure.lua), or .tex LuaDraw-environment body."
            elif fn["name"] == "submit":
                fn["description"] = "Submit the exact editable LuaDraw source that was successfully compiled and compared. Use {path:'figure.lua'}; never submit generated TikZ as the editable source."
                fn["parameters"]["properties"]["path"]["description"] = "Workspace-relative editable LuaDraw source; must equal the last successful compile."
                fn["parameters"]["properties"]["code"]["description"] = "Final LuaDraw source inline, only when not using path."
        if self.docs is None:
            self.docs = LuaDrawDocs(root=self.options.get("luadraw_root"), cache_dir=self.options.get("doc_cache_dir") or Path(__file__).resolve().parents[2] / "build/doc-index")
        definitions.append(self.docs.definition())
        return definitions

    def _tool_luadraw_doc(self, args):
        if self.docs is None:
            self.definitions()
        target = self.root / "预览" / "luadraw-doc"
        target.mkdir(parents=True, exist_ok=True)
        return self.docs.execute(args, target)

    def _tool_write_file(self, args):
        path = str(args.get("path", ""))
        if Path(path).suffix.lower() == ".lua":
            target = _safe(self.root, path)
            target.parent.mkdir(parents=True, exist_ok=True)
            content = args.get("content", "")
            if not isinstance(content, str):
                raise ValueError("content must be a string")
            target.write_text(content, encoding="utf-8")
            return f"WROTE {path} ({len(content.encode())} bytes)", None
        return super()._tool_write_file(args)

    def _tool_compile(self, args):
        from luadraw_runtime import compile_figure
        rel = str(args.get("path") or "figure.lua")
        source = _safe(self.root, rel)
        if not source.is_file():
            return "NOT FOUND: " + rel + " (write editable LuaDraw source first)", None
        code = source.read_text(encoding="utf-8")
        for item in args.get("merges", []) if isinstance(args.get("merges", []), list) else []:
            if isinstance(item, str) and item.strip():
                self.state["merges"].append(item.strip())
        self.state.update(compile_ok=False, last_code=code, last_pdf="", compare_called=False,
                          best_overlap=0.0, scale_dev_at_best=0.0, source_path=rel)
        result = compile_figure(source, self.root, self.options)
        if not result.get("ok"):
            error = result.get("error") or result.get("logs") or "Unknown LuaDraw compile error"
            self.state["compile_err"] = str(error)
            return "COMPILE FAILED. Fix the LuaDraw program and compile again.\n" + str(error)[-12000:], None
        pdf = Path(result["pdf"])
        canonical = self.root / "standalone.pdf"
        if pdf.resolve() != canonical.resolve():
            shutil.copyfile(pdf, canonical)
        wrapper = result.get("wrapper") or result.get("tex") or result.get("tex_path")
        self.state.update(compile_ok=True, compile_err="", last_pdf=str(canonical),
                          drawing_backend=result.get("backend", ""), drawing_engine=result.get("engine", ""),
                          generated_tex=str(wrapper or ""), generated_tikz=result.get("generated_tikz", []),
                          luadraw_version=result.get("luadraw_version", "3.7"))
        pages, width, height = self._pdf_info(canonical)
        return (f"COMPILE OK. Artifact: standalone.pdf ({pages} page), {width:.1f}mm x {height:.1f}mm. "
                f"LuaDraw {self.state['luadraw_version']}; backend={self.state['drawing_backend']}; engine={self.state['drawing_engine']}. "
                'Inspect with view_pdf {path:"standalone.pdf",page:1}, then compare_figure.'), None

    def _tool_submit(self, args):
        args = dict(args)
        if not args.get("path") and not args.get("code"):
            args["path"] = self.state.get("source_path", "figure.lua")
        receipt, image = super()._tool_submit(args)
        if receipt.startswith("SUBMITTED"):
            self.state["accepted_snapshot"] = copy.deepcopy({k: v for k, v in self.state.items() if k != "accepted_snapshot"})
        return receipt, image

    def _tool_compare_figure(self, args):
        explicit = str(args.get("pdf_path", "")).strip()
        if not explicit and (not self.state["compile_ok"] or not self.state["last_pdf"]):
            return "Comparison unavailable: compile a LuaDraw figure successfully first, or provide pdf_path.", None
        pdf = _safe(self.root, explicit) if explicit else Path(self.state["last_pdf"])
        if not pdf.is_file():
            return "Comparison unavailable: PDF not found.", None
        round_number = int(self.state.get("compare_round", 0)) + 1
        out = self.root / "compare" / f"round_{round_number:04d}"
        request = {"version": "compare-figure/1", "reference": str(self.reference), "candidate": str(pdf),
                   "reference_mm": list(self.options.get("reference_mm") or (0, 0)),
                   "detail": args.get("detail", "all"), "zoom": args.get("zoom", 0),
                   "worst_region": args.get("worst_region", False), "out_dir": str(out),
                   "raster_dpi": self.options.get("raster_dpi", 110),
                   "pass_overlap": self.min_overlap, "pass_scale": self.max_scale_dev}
        if any(k in args for k in ("left", "top", "right", "bottom")):
            request["crop"] = [float(args.get(k, d)) for k, d in (("left", 0), ("top", 0), ("right", 100), ("bottom", 100))]
        response = compare(request, self.worker)
        metrics = response["metrics"]
        self.state.update(compare_called=True, compare_round=round_number)
        self._last_metrics = metrics
        if not self.state["best_overlap"] or metrics["similarity"] > self.state["best_overlap"]:
            mapping = {"similarity": "best_overlap", "scale_dev": "scale_dev_at_best", "scale_x": "best_scale_x", "scale_y": "best_scale_y",
                       "size_unit": "size_unit_at_best", "aspect_ref": "aspect_ref_at_best", "aspect_cand": "aspect_cand_at_best",
                       "ink_ref_w": "ink_ref_w_at_best", "ink_ref_h": "ink_ref_h_at_best", "ink_cand_w": "ink_cand_w_at_best", "ink_cand_h": "ink_cand_h_at_best"}
            for key, dest in mapping.items():
                self.state[dest] = metrics.get(key, "" if key == "size_unit" else 0)
        text = {"backend": "go" if self.worker else "python", "scope": "full-image score; crop is evidence only",
                "overlay_key": {"white": "shared ink", "magenta": "original-only ink", "green": "candidate-only ink", "black": "background"},
                "size_guidance": ("Physical ink measured in mm before normalization; fix geometry/canvas, not stroke width." if metrics.get("size_unit") == "mm" else "Physical dimensions unknown; only ink aspect ratio is checked. Pixel dimensions at different render DPI are not comparable."),
                "metrics": metrics, "crop": response.get("crop"), "images": response.get("images"),
                "threshold": {"overlap": self.min_overlap, "size_deviation": self.max_scale_dev}}
        return json.dumps(text, ensure_ascii=False), response["selected_image"]

    def deliver(self):
        if not self.state["submitted"]:
            raise ValueError("No accepted LuaDraw source")
        accepted = self.state.get("accepted_snapshot")
        if accepted and accepted.get("final_code") == self.state["final_code"]:
            self.state.update(copy.deepcopy(accepted))
        code = self.state["final_code"]
        suffix = Path(self.state.get("source_path", "figure.lua")).suffix
        final_source = self.root / ("submitted.lua" if suffix == ".lua" else "submitted.body.tex")
        final_source.write_text(code, encoding="utf-8")
        # Always rebuild the accepted editable version: a checker may have
        # compiled a later draft without submitting it, or files changed since.
        from luadraw_runtime import compile_figure
        compiled = compile_figure(final_source, self.root, self.options)
        if not compiled.get("ok"):
            raise RuntimeError("Accepted LuaDraw source could not be rebuilt: " + str(compiled.get("error")))
        source_pdf = Path(compiled["pdf"])
        generated = Path(compiled.get("wrapper") or compiled.get("tex") or compiled.get("tex_path"))
        self.state["generated_tikz"] = compiled.get("generated_tikz", [])
        self.state["drawing_engine"] = compiled.get("engine", "")
        self.state["drawing_backend"] = compiled.get("backend", "")
        pdf, tex = self.root / "submitted.pdf", self.root / "submitted.tex"
        if source_pdf.resolve() != pdf.resolve():
            shutil.copyfile(source_pdf, pdf)
        if generated.resolve() != tex.resolve():
            shutil.copyfile(generated, tex)
        warnings = []
        try:
            preview = self.rasterize(pdf, self.root / "预览/submitted")
        except Exception as error:
            preview = None
            warnings.append("Preview failed; vector PDF retained: " + str(error))
        result = {"submitted": True, "source_language": "luadraw", "luadraw_version": self.state.get("luadraw_version", "3.7"),
                  "backend": self.state.get("drawing_backend"), "engine": self.state.get("drawing_engine"),
                  "editable_source": str(final_source), "submitted_tex": str(tex), "submitted_pdf": str(pdf),
                  "preview": str(preview) if preview else None, "merges": self.state["merges"], "uncertain": self.state["uncertain"],
                  "best_overlap": self.state["best_overlap"], "scale_dev": self.state["scale_dev_at_best"],
                  "accepted_as_is": self.state["accepted_as_is"], "accept_note": self.state["accept_note"],
                  "submit_attempts": self.state["submit_attempts"], "warnings": warnings,
                  "generated_tikz": self.state.get("generated_tikz", []),
                  "generated_lua": compiled.get("generated_lua")}
        temp = self.root / "result.json.tmp"
        temp.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        temp.replace(self.root / "result.json")
        return result
