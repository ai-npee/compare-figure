#!/usr/bin/env python3
"""Versioned JSON/image CLI. Operations: compare, docs, compile, environment.

Use --jsonl for persistent request/response calls. stdout contains only JSON.
"""
import argparse
import base64
import json
from pathlib import Path
import sys
import tempfile

from compare_backend import GoWorker, binary_path, compare


class ToolService:
    def __init__(self, backend="auto", executable=None):
        self.backend, self.docs = backend, None
        self.worker = GoWorker(executable) if backend == "go" or (backend == "auto" and binary_path(executable)) else None

    def handle(self, request):
        operation = request.get("operation", "compare")
        if operation == "compare":
            return compare(request, self.worker)
        if operation == "compile":
            from luadraw_runtime import compile_figure
            from PIL import Image
            import subprocess
            out = Path(request.get("out_dir") or tempfile.mkdtemp(prefix="luadraw-compile-")).resolve()
            result = compile_figure(Path(request["source"]), out, request.get("options", {}))
            result.update(version="compare-figure-tools/1", operation="compile", out_dir=str(out), images=[])
            if result.get("ok") and request.get("include_image", True):
                prefix = out / "预览" / "compiled"
                prefix.parent.mkdir(parents=True, exist_ok=True)
                subprocess.run(["pdftoppm", "-png", "-singlefile", "-r", str(request.get("raster_dpi", 110)),
                                result["pdf"], str(prefix)], check=True, capture_output=True, timeout=60)
                path = prefix.with_suffix(".png")
                with Image.open(path) as image:
                    width, height = image.size
                artifact = {"path": str(path), "mime": "image/png", "width": width, "height": height}
                if request.get("inline_images"):
                    artifact["base64"] = base64.b64encode(path.read_bytes()).decode()
                result["images"].append(artifact)
            return result
        if operation == "environment":
            from luadraw_runtime import probe
            return {"version": "compare-figure-tools/1", "ok": True, "environment": probe(request.get("luadraw_root"))}
        if operation == "docs":
            from luadraw_docs import LuaDrawDocs
            if self.docs is None:
                self.docs = LuaDrawDocs(root=request.get("luadraw_root"), cache_dir=request.get("cache_dir"))
            out = Path(request.get("out_dir") or tempfile.mkdtemp(prefix="luadraw-doc-")).resolve()
            out.mkdir(parents=True, exist_ok=True)
            text, image = self.docs.execute(request.get("arguments", request), out)
            images = []
            if image:
                from PIL import Image
                path = Path(image)
                with Image.open(path) as im:
                    w, h = im.size
                item = {"path": str(path), "mime": "image/png", "width": w, "height": h}
                if request.get("inline_images"):
                    item["base64"] = base64.b64encode(path.read_bytes()).decode()
                images.append(item)
            return {"version": "compare-figure-tools/1", "ok": True, "text": text, "images": images, "out_dir": str(out)}
        raise ValueError("Unknown operation: " + str(operation))

    def close(self):
        if self.worker:
            self.worker.close()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--request", default="-", help="JSON file or - for stdin")
    parser.add_argument("--jsonl", action="store_true", help="Serve one JSON request/response per line")
    parser.add_argument("--backend", choices=["auto", "python", "go"], default="auto")
    parser.add_argument("--go-binary")
    args = parser.parse_args(argv)
    service = ToolService(args.backend, args.go_binary)
    failed = False
    try:
        stream = sys.stdin if args.request == "-" else Path(args.request).open(encoding="utf-8")
        try:
            items = stream if args.jsonl else [stream.read()]
            for raw in items:
                if not raw.strip():
                    continue
                request = None
                try:
                    request = json.loads(raw)
                    if not isinstance(request, dict):
                        raise ValueError("Request must be an object")
                    result = service.handle(request)
                    if result.get("ok") is False:
                        failed = True
                except Exception as exc:
                    failed = True
                    result = {"version": "compare-figure-tools/1", "ok": False, "error": str(exc)}
                if isinstance(request, dict) and "id" in request:
                    result["id"] = request["id"]
                print(json.dumps(result, ensure_ascii=False, allow_nan=False), flush=True)
        finally:
            if stream is not sys.stdin:
                stream.close()
    finally:
        service.close()
    return 0 if args.jsonl or not failed else 2


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, RuntimeError) as error:
        print(json.dumps({"version": "compare-figure-tools/1", "ok": False, "error": str(error)}), flush=True)
        raise SystemExit(2)
