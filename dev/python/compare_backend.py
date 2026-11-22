"""JSON comparison interface with optional persistent Go worker.

The Go worker is reused across tool calls; Python production metrics remain a
portable fallback when the binary has not been built. No legacy scoring is used.
"""
import base64
import json
import os
from pathlib import Path
import queue
import shutil
import subprocess
import tempfile
import threading

ROOT = Path(__file__).resolve().parents[2]


def binary_path(explicit=None):
    if explicit:
        path = shutil.which(str(explicit)) or str(explicit)
        if not Path(path).is_file():
            raise FileNotFoundError("Go comparator not found: " + str(explicit))
        return str(Path(path).resolve())
    configured = os.environ.get("COMPARE_FIGURE_BIN")
    if configured:
        return binary_path(configured)
    for path in (ROOT / "build/compare-figure", ROOT / "build/compare-figure.exe", ROOT / "go/build/compare-figure"):
        if path.is_file():
            return str(path)
    return shutil.which("compare-figure")


class GoWorker:
    """Serial JSONL transport, bounded requests, one process per session."""
    def __init__(self, executable=None, timeout=180):
        self.executable = binary_path(executable)
        if not self.executable:
            raise FileNotFoundError("Build Go comparator first, or select the Python backend")
        self.timeout, self.process = timeout, None
        self.responses = None
        self.lock = threading.Lock()

    @staticmethod
    def _read_responses(process, responses):
        try:
            for line in process.stdout:
                responses.put(line)
        finally:
            responses.put(None)

    def request(self, request):
        with self.lock:
            if self.process is None or self.process.poll() is not None:
                self.process = subprocess.Popen([self.executable, "--jsonl"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                                stderr=subprocess.DEVNULL, text=True, bufsize=1)
                self.responses = queue.Queue()
                threading.Thread(target=self._read_responses, args=(self.process, self.responses), daemon=True).start()
            self.process.stdin.write(json.dumps(request, ensure_ascii=False) + "\n")
            self.process.stdin.flush()
            try:
                line = self.responses.get(timeout=self.timeout)
            except queue.Empty:
                self.close()
                raise TimeoutError("Go comparison request timed out") from None
            if not line:
                self.close()
                raise RuntimeError("Go comparison worker stopped without a response")
            result = json.loads(line)
            if result.get("error") or result.get("ok") is False:
                raise ValueError("Go comparator: " + str(result.get("error", result)))
            return result

    def close(self):
        process, self.process = self.process, None
        if process and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=3)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        if process:
            for stream in (process.stdin, process.stdout):
                if stream:
                    stream.close()

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()


def compare(request, worker=None):
    """Return compare-figure/1 JSON with image paths or inline content."""
    if request.get("version", "compare-figure/1") != "compare-figure/1":
        raise ValueError("version must be compare-figure/1")
    request = {"version": "compare-figure/1", **request}
    if worker:
        return worker.request(request)
    from PIL import Image
    import vector_metrics as metrics
    out = Path(request.get("out_dir") or tempfile.mkdtemp(prefix="compare-figure-")).resolve()
    out.mkdir(parents=True, exist_ok=True)
    reference = Path(request["reference"]).resolve()
    candidate = Path(request["candidate"]).resolve()
    refmm = list(request.get("reference_mm") or (0, 0))
    candmm = list(request.get("candidate_mm") or (0, 0))
    if candidate.suffix.lower() == ".pdf":
        import re
        info = subprocess.run(["pdfinfo", str(candidate)], text=True, capture_output=True, check=True, timeout=30).stdout
        size = re.search(r"(?m)^Page size:\s*([\d.]+) x ([\d.]+) pts", info)
        if size and not all(candmm):
            candmm = [float(size[1]) * 25.4 / 72, float(size[2]) * 25.4 / 72]
        prefix = out / "candidate"
        subprocess.run(["pdftoppm", "-png", "-singlefile", "-r", str(request.get("raster_dpi", 110)),
                        str(candidate), str(prefix)], capture_output=True, check=True, timeout=60)
        candidate = prefix.with_suffix(".png")
    with Image.open(reference) as source:
        ref = source.copy()
    with Image.open(candidate) as source:
        cand = source.copy()
    result = metrics.compare_images(ref, cand, (*refmm, *candmm))
    crop = request.get("crop") or [0, 0, 100, 100]
    if request.get("worst_region") and not request.get("crop"):
        crop = list(metrics.worst_region(ref, cand))
    left, top, right, bottom = crop
    if not 0 <= left < right <= 100 or not 0 <= top < bottom <= 100:
        raise ValueError("Invalid percentage crop")
    detail = request.get("detail", "all")
    if detail not in {"all", "gray", "r", "g", "b", "rg", "rb", "gb"}:
        raise ValueError("Invalid detail")
    overlay = metrics.make_overlay(ref, cand, result["dx"], result["dy"], detail)
    w, h = overlay.size
    overlay = overlay.crop((int(left*w/100+.5), int(top*h/100+.5), max(1, int(right*w/100+.5)), max(1, int(bottom*h/100+.5))))
    zoom = int(request.get("zoom", 0))
    if zoom < 0 or zoom > 6000:
        raise ValueError("zoom must be between 0 and 6000")
    if zoom and overlay.width != zoom:
        import numpy as np
        height = max(1, int(overlay.height * zoom / overlay.width + .5))
        overlay = Image.fromarray(metrics._catmull_rom_resize(np.asarray(overlay.convert("RGB")), zoom, height))
    selected = out / "comparison.png"
    overlay.save(selected)
    image = {"path": str(selected), "mime": "image/png", "width": overlay.width, "height": overlay.height}
    if request.get("inline_images"):
        image["base64"] = base64.b64encode(selected.read_bytes()).decode()
    pass_overlap, pass_scale = request.get("pass_overlap", .17) or .17, request.get("pass_scale", .1) or .1
    passed = ((pass_overlap < 0 or result["similarity"] >= pass_overlap)
              and (pass_scale < 0 or result["scale_dev"] <= pass_scale))
    response = {"version": "compare-figure/1", "ok": True, "backend": "python", "metrics": result, "pass": passed,
                "out_dir": str(out), "selected_image": str(selected), "images": [image], "crop": crop}
    (out / "metrics.json").write_text(json.dumps(response, ensure_ascii=False, indent=2), encoding="utf-8")
    return response
