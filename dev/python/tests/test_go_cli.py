import base64
import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[3]
GO = ROOT / "go"
BINARY = ROOT / "build" / "compare-figure"


def go_tool():
    configured = os.environ.get("COMPARE_FIGURE_GO")
    if configured:
        resolved = shutil.which(configured) or (configured if Path(configured).is_file() else None)
        return resolved, None
    path_go = shutil.which("go")
    candidates = []
    if path_go:
        candidates.append((path_go, None))
    gopath = Path(os.environ.get("GOPATH", Path.home() / "go"))
    for binary in (gopath / "pkg/mod").glob("golang.org/toolchain@*/bin/go"):
        root = binary.parent.parent
        version = re.search(r"go(\d+)\.(\d+)(?:\.(\d+))?", root.name)
        if version and tuple(int(part or 0) for part in version.groups()) >= (1, 25, 0):
            candidates.append((str(binary), str(root)))
    for executable, goroot in reversed(candidates):
        try:
            output = subprocess.run([executable, "version"], text=True, capture_output=True, check=True).stdout
        except (OSError, subprocess.CalledProcessError):
            continue
        match = re.search(r"go(\d+)\.(\d+)(?:\.(\d+))?", output)
        if match and tuple(int(part or 0) for part in match.groups()) >= (1, 25, 0):
            return executable, goroot
    return None, None


def build_cli():
    executable, goroot = go_tool()
    if not executable:
        raise unittest.SkipTest("Go >= 1.25 is unavailable; set COMPARE_FIGURE_GO to its executable")
    env = os.environ.copy()
    env.update({
        "PATH": f"{Path(executable).parent}:{env.get('PATH', '')}",
        "GOPATH": env.get("GOPATH", str(Path.home() / "go")),
        "GOPROXY": env.get("GOPROXY", "off"),
        "GOCACHE": str(ROOT / "build" / "go-cache"),
    })
    if goroot:
        env["GOROOT"] = goroot
    subprocess.run([executable, "build", "-o", str(BINARY), "./cmd/compare-figure"], cwd=GO, env=env, check=True)
    return BINARY


def request(reference, candidate, **extra):
    return {"version": "compare-figure/1", "reference": str(reference), "candidate": str(candidate), **extra}


def write_pdf(path):
    content = b"0 0 0 rg 20 20 60 60 re f"
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << >> /Contents 4 0 R >>",
        b"<< /Length " + str(len(content)).encode() + b" >>\nstream\n" + content + b"\nendstream",
    ]
    data = bytearray(b"%PDF-1.4\n")
    offsets = [0]
    for index, obj in enumerate(objects, 1):
        offsets.append(len(data))
        data.extend(f"{index} 0 obj\n".encode() + obj + b"\nendobj\n")
    xref = len(data)
    data.extend(f"xref\n0 {len(offsets)}\n0000000000 65535 f \n".encode())
    for offset in offsets[1:]:
        data.extend(f"{offset:010d} 00000 n \n".encode())
    data.extend(f"trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode())
    path.write_bytes(data)


class GoCLIRegressionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cli = build_cli()

    def make_image(self, path):
        image = Image.new("RGB", (96, 80), "white")
        ImageDraw.Draw(image).rectangle((18, 12, 70, 60), fill=(30, 30, 30))
        image.save(path)
        return image

    def test_png_metrics_match_vector_metrics_and_inline_image(self):
        import sys
        sys.path.insert(0, str(ROOT / "dev" / "python"))
        from vector_metrics import compare_images

        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            ref_path, cand_path = tmp / "ref.png", tmp / "cand.png"
            ref = self.make_image(ref_path)
            ref.save(cand_path)
            payload = request(ref_path, cand_path, inline_images=True, out_dir=str(tmp / "out"))
            result = subprocess.run([str(self.cli), "--stdin"], input=json.dumps(payload), text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            actual = json.loads(result.stdout)
            expected = compare_images(ref, ref)
            self.assertTrue(actual["ok"])
            for key in ("similarity", "gray", "scale_x", "scale_y", "scale_dev", "aspect_ref", "aspect_cand", "aspect_dev", "ink_ref_w", "ink_ref_h", "ink_cand_w", "ink_cand_h", "share_ref_w", "share_ref_h", "share_cand_w", "share_cand_h"):
                self.assertAlmostEqual(actual["metrics"][key], expected[key], places=12, msg=key)
            self.assertEqual(actual["metrics"]["channels"], expected["channels"])
            self.assertEqual(len(actual["metrics"]["runs"]), len(expected["runs"]))
            for actual_run, expected_run in zip(actual["metrics"]["runs"], expected["runs"]):
                for key in ("name", "score", "dx", "dy"):
                    self.assertAlmostEqual(actual_run[key], expected_run[key], places=12, msg=key)
            self.assertIn("pass", actual)
            for key in ("dx", "dy"):
                self.assertEqual(actual["metrics"][key], expected[key])
            self.assertEqual(actual["metrics"]["size_unit"], expected["size_unit"])
            self.assertEqual(actual["images"][0]["mime"], "image/png")
            self.assertEqual(actual["images"][0]["width"], 192)
            self.assertEqual(base64.b64decode(actual["images"][0]["base64"])[:8], b"\x89PNG\r\n\x1a\n")
            self.assertEqual(actual["selected_image"], str(tmp / "out" / "comparison.png"))
            self.assertEqual(actual["crop"], [0, 0, 100, 100])
            metrics_path = Path(actual["metrics_path"])
            self.assertTrue(metrics_path.is_file())
            self.assertEqual(json.loads(metrics_path.read_text()), actual["metrics"])

    def make_asymmetric_rgba(self, size):
        image = Image.new("RGBA", size, (255, 255, 255, 0))
        draw = ImageDraw.Draw(image)
        w, h = size
        draw.polygon([(w // 8, h // 5), (w * 3 // 5, h // 9), (w * 2 // 3, h // 2), (w // 4, h * 2 // 3)], fill=(20, 90, 180, 225))
        draw.ellipse((w * 3 // 5, h // 2, w * 7 // 8, h * 4 // 5), fill=(210, 35, 70, 160))
        draw.line((w // 6, h * 4 // 5, w * 4 // 5, h * 3 // 4), fill=(15, 140, 65, 255), width=max(1, w // 35))
        return image

    def test_resized_shifted_alpha_and_mm_metrics_match_python(self):
        import sys
        sys.path.insert(0, str(ROOT / "dev" / "python"))
        from vector_metrics import compare_images

        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            ref = self.make_asymmetric_rgba((120, 90))
            reduced = ref.resize((83, 62), Image.Resampling.BICUBIC)
            cand = Image.new("RGBA", reduced.size, (255, 255, 255, 0))
            cand.alpha_composite(reduced, dest=(2, -1))
            ref_path, cand_path = tmp / "reference.png", tmp / "candidate.png"
            ref.save(ref_path)
            cand.save(cand_path)
            payload = request(ref_path, cand_path, reference_mm=[60, 45], candidate_mm=[66, 42], out_dir=str(tmp / "out"))
            run = subprocess.run([str(self.cli), "--stdin"], input=json.dumps(payload), text=True, capture_output=True)
            self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
            actual = json.loads(run.stdout)["metrics"]
            expected = compare_images(ref, cand, (60, 45, 66, 42))
            for key in ("similarity", "gray", "scale_x", "scale_y", "scale_dev", "aspect_ref", "aspect_cand", "aspect_dev", "ink_ref_w", "ink_ref_h", "ink_cand_w", "ink_cand_h", "share_ref_w", "share_ref_h", "share_cand_w", "share_cand_h"):
                self.assertAlmostEqual(actual[key], expected[key], delta=1e-6, msg=f"{key}: Go={actual[key]!r}, Python={expected[key]!r}")
            for go_value, py_value in zip(actual["channels"], expected["channels"]):
                self.assertAlmostEqual(go_value, py_value, delta=1e-6, msg=f"channel score: Go={go_value!r}, Python={py_value!r}")
            self.assertEqual(len(actual["runs"]), len(expected["runs"]))
            for go_run, py_run in zip(actual["runs"], expected["runs"]):
                self.assertEqual(go_run["name"], py_run["name"])
                self.assertAlmostEqual(go_run["score"], py_run["score"], delta=1e-6)
                self.assertLessEqual(abs(go_run["dx"] - py_run["dx"]), 1)
                self.assertLessEqual(abs(go_run["dy"] - py_run["dy"]), 1)
            for key in ("dx", "dy"):
                self.assertLessEqual(abs(actual[key] - expected[key]), 1, f"{key}: Go={actual[key]}, Python={expected[key]}")
            self.assertEqual(actual["size_unit"], "mm")
            self.assertEqual((actual["dx"], actual["dy"]), (-3, 1))

    def test_negative_pass_scale_disables_size_gate(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            ref_path = tmp / "ref.png"
            self.make_image(ref_path)
            payload = request(ref_path, ref_path, reference_mm=[10, 10], candidate_mm=[20, 20], pass_scale=-1, out_dir=str(tmp / "out"))
            result = subprocess.run([str(self.cli), "--stdin"], input=json.dumps(payload), text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            actual = json.loads(result.stdout)
            self.assertEqual(actual["metrics"]["scale_dev"], 1)
            self.assertTrue(actual["pass"])

    def test_pdf_rasterization_and_jsonl_error_continuation(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            ref = self.make_image(tmp / "ref.png")
            pdf = tmp / "candidate.pdf"
            write_pdf(pdf)
            first = request(tmp / "ref.png", pdf, raster_dpi=110, worst_region=True)
            second = {"version": "bad"}
            third = request(tmp / "ref.png", tmp / "ref.png")
            before = set(Path(tempfile.gettempdir()).glob("compare-figure-pdf-*"))
            lines = "\n".join(json.dumps(item) for item in (first, second, third)) + "\n"
            result = subprocess.run([str(self.cli), "--jsonl"], input=lines, text=True, capture_output=True)
            after = set(Path(tempfile.gettempdir()).glob("compare-figure-pdf-*"))
            self.assertEqual(result.returncode, 1)
            responses = [json.loads(line) for line in result.stdout.splitlines()]
            self.assertEqual(len(responses), 3)
            self.assertTrue(responses[0]["ok"])
            crop = responses[0]["crop"]
            self.assertEqual(crop[2] - crop[0], 50)
            self.assertEqual(crop[3] - crop[1], 50)
            self.assertIn("version", responses[1]["error"])
            self.assertTrue(responses[2]["ok"])
            self.assertGreater(responses[0]["images"][0]["width"], 0)
            self.assertEqual(after - before, set())


if __name__ == "__main__":
    unittest.main()
