import tempfile
import unittest
from unittest import mock
from pathlib import Path

import luadraw_runtime as runtime


FIGURE = '''local ld = luadraw
local g = ld.graph:new{window={-2, 2, -1.5, 1.5}, bbox=false}
g:Dline(-1, 1, "red,thick")
g:Dcircle(0, 1, nil, "blue")
g:Show()
'''


class LuaDrawRuntimeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = Path(__file__).resolve().parents[3] / "vendor" / "luadraw-3.7" / "luadraw"
        if not (cls.root / "files" / "luadraw.sty").is_file():
            raise unittest.SkipTest("vendored LuaDraw 3.7 is not available")

    def _source_and_work(self, temp, name):
        base = Path(temp)
        source = base / "figure.lua"
        source.write_text(FIGURE, encoding="utf-8")
        work = base / name
        return source, work

    def test_probe_checks_installed_tools_and_package(self):
        result = runtime.probe(self.root)
        self.assertEqual(result["luadraw_version"], "3.7")
        self.assertTrue(result["luadraw_files"])
        self.assertIn("lualatex_usable", result)
        self.assertIn("standalone_lua_usable", result)
        self.assertTrue(result["packages"].get("luadraw.sty"))

    def test_lualatex_direct_real_compile(self):
        status = runtime.probe(self.root)
        if not status["lualatex_usable"]:
            self.skipTest("LuaLaTeX direct route unavailable: " + status["probe_log"][-300:])
        with tempfile.TemporaryDirectory(prefix="test-luadraw-direct-") as temp:
            source, work = self._source_and_work(temp, "out")
            built = runtime.compile_figure(source, work, {"luadraw_root": self.root})
            self.assertTrue(built["ok"], built["error"] or built["logs"])
            self.assertEqual(built["engine"], "lualatex")
            self.assertTrue(Path(built["pdf"]).is_file())
            self.assertTrue(Path(built["pdf"]).read_bytes().startswith(b"%PDF"))

    def test_masked_lualatex_forces_standalone_generation_and_xelatex(self):
        with tempfile.TemporaryDirectory(prefix="test-luadraw-fallback-") as temp:
            source, work = self._source_and_work(temp, "out")
            built = runtime.compile_figure(source, work, {
                "luadraw_root": self.root,
                "mask_executables": ["lualatex"],
            })
            self.assertTrue(built["ok"], built["error"] or built["logs"])
            self.assertEqual(built["backend"], "standalone-lua+generated-TikZ")
            self.assertEqual(built["engine"], "xelatex")
            self.assertTrue(Path(built["tikz"]).is_file())
            self.assertIn("\\begin{tikzpicture}", Path(built["tikz"]).read_text(encoding="utf-8"))
            self.assertTrue(Path(built["pdf"]).is_file())

    def test_fallback_probe_tries_pdflatex_after_xelatex_failure(self):
        files = self.root / "files"
        env = runtime._tex_env(self.root)

        def fake_run(argv, *, cwd, env, timeout):
            if len(argv) > 1 and argv[1] == str(runtime.RUNNER):
                (cwd / "figure.tkz").write_text("% fixture", encoding="utf-8")
                return 0, "generated"
            engine = Path(argv[0]).name
            if engine == "xelatex":
                return 1, "synthetic XeLaTeX failure"
            if engine == "pdflatex":
                (cwd / "probe.pdf").write_bytes(b"%PDF-1.4\\n")
                return 0, "synthetic pdfLaTeX success"
            return 127, "unexpected executable"

        with mock.patch.object(runtime, "_run", side_effect=fake_run):
            ok, log, engine, lua_bin, candidates = runtime._usable_fallback(
                files, env, ["fake-lua"])
        self.assertTrue(ok, log)
        self.assertEqual(engine, "pdflatex")
        self.assertEqual([item["usable"] for item in candidates], [False, True])
        self.assertIn("synthetic XeLaTeX failure", log)

    def test_masked_xelatex_uses_real_pdflatex(self):
        with tempfile.TemporaryDirectory(prefix="test-luadraw-pdflatex-") as temp:
            source, work = self._source_and_work(temp, "out")
            built = runtime.compile_figure(source, work, {
                "luadraw_root": self.root,
                "mask_executables": ["lualatex", "xelatex"],
            })
            self.assertTrue(built["ok"], built["error"] or built["logs"])
            self.assertEqual(built["engine"], "pdflatex")
            self.assertTrue(Path(built["pdf"]).read_bytes().startswith(b"%PDF"))

    def test_direct_wrapper_uses_relative_dofile_string(self):
        wrapper = runtime._direct_wrapper("figure", "_luadraw_inputs/figure's.lua")
        self.assertIn('dofile("_luadraw_inputs/figure\'s.lua")', wrapper)
        self.assertNotIn("dofile{", wrapper)

    def test_standalone_3d_dpolyline_real_compile(self):
        figure = '''local ld = luadraw
local g = ld.graph3d:new{window3d={-2,2,-2,2,-2,2}, window={-3,3,-3,3}, viewdir={30,60}, bbox=false}
local M = ld.pt3d.M
g:Dpolyline3d({M(-1,0,0), M(0,1,1), M(1,0,0)}, "blue,line width=.8pt")
g:Show()
'''
        with tempfile.TemporaryDirectory(prefix="test-luadraw-3d-") as temp:
            base = Path(temp)
            source = base / "figure.lua"
            source.write_text(figure, encoding="utf-8")
            built = runtime.compile_figure(source, base / "out", {
                "luadraw_root": self.root,
                "engine": "fallback",
            })
            self.assertTrue(built["ok"], built["error"] or built["logs"])
            tikz = Path(built["generated_tikz"][0]).read_text(encoding="utf-8")
            self.assertIn("\\begin{tikzpicture}", tikz)
            self.assertTrue(Path(built["pdf"]).is_file())

    def test_tex_environment_cjk_math_and_source_preservation(self):
        tex = r'''\begin{luadraw}{name=figure, exec=true}
local ld = luadraw
local g = ld.graph:new{window={-2, 2, -1, 1}, bbox=false}
g:Dline(-1, 1, "red")
g:Dlabel("$\\alpha$ 中文标签", 0, {})
g:Show()
\end{luadraw}
'''
        with tempfile.TemporaryDirectory(prefix="test-luadraw-tex-") as temp:
            base = Path(temp)
            source = base / "figure.tex"
            source.write_text(tex, encoding="utf-8")
            work = base / "out"
            work.mkdir()
            sentinel = work / "figure.lua"
            sentinel.write_text("do not overwrite", encoding="utf-8")
            built = runtime.compile_figure(source, work, {
                "luadraw_root": self.root,
                "engine": "fallback",
            })
            self.assertTrue(built["ok"], built["error"] or built["logs"])
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "do not overwrite")
            self.assertEqual(Path(built["generated_lua"]).name, "_luadraw_from_tex.lua")
            self.assertIn("ctex", Path(built["wrapper"]).read_text(encoding="utf-8"))
            self.assertIn("中文标签", Path(built["tikz"]).read_text(encoding="utf-8"))

    def test_handmade_tikz_is_rejected(self):
        with tempfile.TemporaryDirectory(prefix="test-luadraw-reject-") as temp:
            base = Path(temp)
            source = base / "figure.lua"
            source.write_text("tex.print('\\\\begin{tikzpicture}')", encoding="utf-8")
            built = runtime.compile_figure(source, base / "out", {"luadraw_root": self.root})
            self.assertFalse(built["ok"])
            self.assertIn("LuaDraw is mandatory", built["error"])


if __name__ == "__main__":
    unittest.main()
