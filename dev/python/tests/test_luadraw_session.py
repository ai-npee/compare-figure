import contextlib
import io
import json
from pathlib import Path
import shutil
import tempfile
import unittest

from PIL import Image
from luadraw_harness import LuaDrawHarness
from luadraw_runtime import compile_figure, probe
from vector_session import Session, Transcript

ROOT = Path(__file__).resolve().parents[1]
SOURCE = '''local ld = luadraw
local g = ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}
g:Dline(-1,1,"red,thick")
g:Dcircle(0,1,nil,"blue")
g:Show()
'''


def call(name, arguments=None, ident="one"):
    return {"id": ident, "type": "function", "function": {"name": name, "arguments": json.dumps(arguments or {})}}


class Scripted:
    model, options, usage, finish = "fake", {}, {}, "tool_calls"
    def __init__(self, calls):
        self.calls = iter(calls)
        self.calls_seen = 0
    def complete(self, messages, tools=None, tool_choice="auto", max_tokens=16384):
        self.calls_seen += 1
        return {"role": "assistant", "content": "", "tool_calls": [next(self.calls)]}


class LuaDrawSessionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.environment = probe()
        if not cls.environment["ready"]:
            raise unittest.SkipTest("No usable LuaDraw toolchain")

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        source = self.root / "reference.lua"
        source.write_text(SOURCE)
        built = compile_figure(source, self.root / "producer", {"engine": "auto"})
        self.assertTrue(built["ok"], built.get("error"))
        import subprocess
        subprocess.run(["pdftoppm", "-png", "-singlefile", "-r", "110", built["pdf"], str(self.root / "reference")], capture_output=True, check=True)
        self.reference = self.root / "reference.png"
        self.h = LuaDrawHarness(self.root / "session", self.reference, threshold=90, max_scale_dev=-1, comparison_backend="auto")
        self.addCleanup(self.h.close)

    def test_luadraw_tool_policy_doc_lookup_compile_compare_submit_delivery(self):
        definitions = self.h.definitions()
        self.assertEqual(len(definitions), 11)
        self.assertIn("luadraw_doc", [d["function"]["name"] for d in definitions])
        model = Scripted([
            call("luadraw_doc", {"query": "Dcircle", "include_image": True}),
            call("write_file", {"path": "figure.lua", "content": SOURCE}),
            call("compile", {"path": "figure.lua"}),
            call("view_pdf", {"path": "standalone.pdf", "zoom": 600}),
            call("compare_figure", {"detail": "gray", "worst_region": True, "zoom": 600}),
            call("submit", {"path": "figure.lua"}),
        ])
        session = Session(self.h, model, Transcript(self.h.root / "session.jsonl"), "Use LuaDraw")
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(session.run("draw", [self.reference]), "submitted")
        result = self.h.deliver()
        self.assertEqual(result["source_language"], "luadraw")
        self.assertEqual(Path(result["editable_source"]).read_text(), SOURCE)
        self.assertTrue(Path(result["submitted_pdf"]).is_file())
        self.assertGreaterEqual(result["best_overlap"], .9)
        self.assertEqual(self.h.state["source_language"], "luadraw")
        text = (self.h.root / "session.jsonl").read_text()
        self.assertIn("luadraw_doc", text)
        self.assertNotIn("data:image", text)

    def test_pure_tikz_and_changed_lua_cannot_be_submitted(self):
        self.h.execute("write_file", {"path": "figure.tex", "content": r"\begin{tikzpicture}\draw (0,0)--(1,1);\end{tikzpicture}"})
        receipt, _ = self.h.execute("compile", {"path": "figure.tex"})
        self.assertIn("COMPILE FAILED", receipt)
        self.assertFalse(self.h.state["compile_ok"])
        self.h.execute("write_file", {"path": "figure.lua", "content": SOURCE})
        self.assertIn("COMPILE OK", self.h.execute("compile", {})[0])
        self.h.execute("compare_figure", {})
        self.h.execute("edit_file", {"path": "figure.lua", "find": "red,thick", "replace": "black,thick"})
        self.assertIn("REJECTED", self.h.execute("submit", {})[0])

    def test_visual_checker_can_run_a_multiturn_luadraw_revision(self):
        import runner
        self.h.execute("write_file", {"path": "figure.lua", "content": SOURCE})
        self.h.execute("compile", {})
        self.h.execute("compare_figure", {})
        self.assertTrue(self.h.execute("submit", {})[0].startswith("SUBMITTED"))
        revised = SOURCE + "-- reviewed diagram\n"
        client = Scripted([call("write_file", {"path": "figure.lua", "content": revised}),
                           call("compile", {}), call("compare_figure", {}), call("submit", {})])
        drawing = Session(self.h, client, Transcript(self.h.root / "revision.jsonl"), "Use LuaDraw")
        checker = Scripted([call("submit", {"report": {"status": "issues", "issues": "Review the diagram"}}),
                            call("submit", {"report": {"status": "pass"}})])
        with contextlib.redirect_stdout(io.StringIO()):
            warnings = runner.verify_figure(self.h, drawing, self.reference, checker, {}, {"max_rounds": 2}, "Chinese")
        self.assertEqual(warnings, [])
        self.assertEqual(client.calls_seen, 4)
        self.assertTrue(self.h.state["submitted"])
        self.assertEqual(self.h.state["final_code"], revised)
        self.assertEqual(Path(self.h.deliver()["editable_source"]).read_text(), revised)

    def test_delivery_uses_accepted_source_after_later_unsubmitted_draft(self):
        self.h.execute("write_file", {"path": "figure.lua", "content": SOURCE})
        self.h.execute("compile", {})
        self.h.execute("compare_figure", {})
        self.assertTrue(self.h.execute("submit", {})[0].startswith("SUBMITTED"))
        self.h.execute("write_file", {"path": "figure.lua", "content": SOURCE.replace('red,thick', 'black,thick')})
        self.h.execute("compile", {})
        result = self.h.deliver()
        self.assertEqual(Path(result["editable_source"]).read_text(), SOURCE)
        self.assertGreaterEqual(result["best_overlap"], .9)
        self.assertTrue(Path(result["submitted_tex"]).is_file())


if __name__ == "__main__":
    unittest.main()
