import base64
import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from PIL import Image, ImageDraw
import compare_backend
from compare_backend import GoWorker
from tools_cli import ToolService

ROOT = Path(__file__).resolve().parents[1]


class ToolInterfaceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        im = Image.new("RGB", (80, 70), "white")
        ImageDraw.Draw(im).line([(5, 12), (60, 12), (60, 50)], fill="black", width=3)
        self.reference = self.root / "reference.png"
        im.save(self.reference)
        self.request = {"version": "compare-figure/1", "reference": str(self.reference), "candidate": str(self.reference),
                        "out_dir": str(self.root / "out"), "inline_images": True}

    def test_python_json_image_result_and_negative_disabled_thresholds(self):
        result = compare_backend.compare({**self.request, "pass_scale": -1, "pass_overlap": -1})
        self.assertTrue(result["ok"])
        self.assertTrue(result["pass"])
        self.assertTrue(Path(result["selected_image"]).is_file())
        self.assertEqual(base64.b64decode(result["images"][0]["base64"])[:8], b"\x89PNG\r\n\x1a\n")
        self.assertAlmostEqual(result["metrics"]["similarity"], 1)

    @unittest.skipUnless(compare_backend.binary_path(), "build Go binary first")
    def test_go_worker_reuses_process_and_survives_bad_request(self):
        with GoWorker() as worker:
            result = worker.request(self.request)
            process = worker.process.pid
            with self.assertRaises(ValueError):
                worker.request({"version": "bad"})
            again = worker.request({**self.request, "detail": "gray", "out_dir": str(self.root / "again")})
            self.assertEqual(worker.process.pid, process)
            self.assertTrue(result["ok"] and again["ok"])
            self.assertAlmostEqual(result["metrics"]["similarity"], compare_backend.compare(self.request)["metrics"]["similarity"], places=12)

    def test_jsonl_cli_is_json_only_and_keeps_serving_after_error(self):
        commands = [self.request, {"operation": "unknown", "id": "bad"}, {**self.request, "id": "next"}]
        proc = subprocess.run([sys.executable, str(ROOT / "tools_cli.py"), "--backend", "python", "--jsonl"],
                              input="\n".join(json.dumps(r) for r in commands) + "\n", text=True,
                              capture_output=True, timeout=30)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        results = [json.loads(line) for line in proc.stdout.splitlines()]
        self.assertEqual(len(results), 3)
        self.assertTrue(results[0]["ok"])
        self.assertFalse(results[1]["ok"])
        self.assertEqual(results[1]["id"], "bad")
        self.assertEqual(results[2]["id"], "next")
        self.assertTrue(results[2]["ok"])

    def test_compile_operation_returns_pdf_and_inline_preview(self):
        from luadraw_runtime import probe
        if not probe()["ready"]:
            self.skipTest("No usable LuaDraw compiler")
        source = self.root / "figure.lua"
        source.write_text('local ld=luadraw\nlocal g=ld.graph:new{window={-2,2,-1,1},bbox=false}\ng:Dline(-1,1,"red")\ng:Show()\n')
        service = ToolService(backend="python")
        self.addCleanup(service.close)
        result = service.handle({"operation": "compile", "source": str(source), "out_dir": str(self.root / "compiled"),
                                 "inline_images": True, "options": {"force_fallback": True}})
        self.assertTrue(result["ok"], result.get("error"))
        self.assertEqual(Path(result["pdf"]).read_bytes()[:4], b"%PDF")
        self.assertTrue(result["generated_tikz"])
        self.assertEqual(base64.b64decode(result["images"][0]["base64"])[:8], b"\x89PNG\r\n\x1a\n")
        source.write_text('tex.print("hand drawn TikZ")')
        bad = subprocess.run([sys.executable, str(ROOT / "tools_cli.py"), "--backend", "python"],
                             input=json.dumps({"operation": "compile", "source": str(source), "out_dir": str(self.root / "bad")}),
                             text=True, capture_output=True, timeout=30)
        self.assertEqual(bad.returncode, 2, bad.stdout + bad.stderr)
        self.assertFalse(json.loads(bad.stdout)["ok"])

    def test_documents_use_same_programmatic_image_contract(self):
        service = ToolService(backend="python")
        self.addCleanup(service.close)
        result = service.handle({"operation": "docs", "arguments": {"query": "Dpolyline", "include_image": True},
                                 "cache_dir": str(self.root / "cache"), "out_dir": str(self.root / "doc"), "inline_images": True})
        self.assertTrue(result["ok"])
        self.assertIn("Dpolyline", result["text"])
        self.assertTrue(result["images"])
        self.assertEqual(base64.b64decode(result["images"][0]["base64"])[:8], b"\x89PNG\r\n\x1a\n")


if __name__ == "__main__":
    unittest.main()
