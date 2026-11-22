"""Development-only cross-language runner recovery oracle. Never exported."""
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from luadraw_harness import LuaDrawHarness
from luadraw_runtime import compile_figure, probe
from vector_session import Session, Transcript
from test_go_cli import build_cli

ROOT = Path(__file__).resolve().parents[3]
SOURCE = '''local ld=luadraw
local g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}
g:Dline(-1,1,"red,thick")
g:Dcircle(0,1,nil,"blue")
g:Show()
'''


class Scripted:
    model, options, usage, finish = "prototype-fixture", {}, {}, "tool_calls"
    def complete(self, *args, **kwargs):
        calls = [("write_file", {"path": "figure.lua", "content": SOURCE}), ("compile", {}),
                 ("compare_figure", {"detail": "gray"}), ("submit", {"path": "figure.lua"})]
        return {"role": "assistant", "content": "", "tool_calls": [
            {"id": str(index), "type": "function", "function": {"name": name, "arguments": json.dumps(arguments)}}
            for index, (name, arguments) in enumerate(calls)]}


class NativeRunnerPrototypeCompatibility(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not probe()["ready"]:
            raise unittest.SkipTest("No usable real LuaDraw toolchain")
        cls.binary = build_cli()

    def test_native_runner_recovers_accepted_python_transcript_without_api(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            source = base / "reference.lua"
            source.write_text(SOURCE)
            compiled = compile_figure(source, base / "producer", {"force_fallback": True})
            self.assertTrue(compiled["ok"], compiled.get("error"))
            run = base / "python-workspace"
            reference = run / "media/original.png"
            reference.parent.mkdir(parents=True)
            subprocess.run(["pdftoppm", "-singlefile", "-png", "-r", "110", compiled["pdf"],
                            str(reference.with_suffix(""))], check=True, capture_output=True)
            options = {"max_scale_dev": -1, "accept_min_submits": 8, "engine": "auto", "raster_dpi": 110,
                       "reference_mm": [0, 0], "md_content": "![](images/original.png)",
                       "current_img": "images/original.png", "images_dir": str(reference.parent)}
            metadata = {"version": 3, "reference": "media/original.png", "reference_sha256": hashlib.sha256(reference.read_bytes()).hexdigest(),
                        "context": "Cross-language synthetic fixture", "options": options, "fig_min_overlap": .9}
            (run / "run.json").write_text(json.dumps(metadata))
            tools = LuaDrawHarness(run, reference, threshold=90, comparison_backend="python", **options)
            self.addCleanup(tools.close)
            session = Session(tools, Scripted(), Transcript(run / "session.jsonl"), "Prototype fixture")
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(session.run("Draw", [reference]), "submitted")
            self.assertFalse((run / "result.json").exists())
            env = {**os.environ, "OPENAI_API_KEY": "", "OPENAI_MODEL": "", "COMPARE_FIGURE_ROOT": str(ROOT)}
            proc = subprocess.run([str(self.binary), "run", "--out", str(run), "--resume"],
                                  text=True, capture_output=True, env=env, timeout=90)
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            result = json.loads(proc.stdout)
            self.assertTrue(result["submitted"])
            self.assertFalse(result["api_contacted"])
            self.assertEqual(Path(result["editable_source"]).read_text(), SOURCE)
            self.assertTrue(Path(result["submitted_pdf"]).read_bytes().startswith(b"%PDF"))
            self.assertGreaterEqual(result["best_overlap"], .9)
            # Repeating an explicit resume needs neither source prototype nor a model.
            before = (run / "result.json").read_bytes()
            again = subprocess.run([str(self.binary), "run", "--out", str(run), "--resume"],
                                   text=True, capture_output=True, env=env, timeout=90)
            self.assertEqual(again.returncode, 0, again.stdout + again.stderr)
            self.assertFalse(json.loads(again.stdout)["api_contacted"])
            self.assertEqual(before, (run / "result.json").read_bytes())


if __name__ == "__main__":
    unittest.main()
