from __future__ import annotations

import builtins
import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import fitz

from luadraw_docs import LuaDrawDocs, main


def make_root(base: Path) -> Path:
    root = base / "luadraw"
    (root / "doc/src").mkdir(parents=True)
    (root / "files").mkdir()
    (root / "examples").mkdir()
    (root / "doc/src/luadraw-doc-en.tex").write_text("\\def\\version{3.7}\n\\section{Dscene3d}\n", encoding="utf-8")
    (root / "files/luadraw_scene3d.lua").write_text("function Dscene3d(scene)\n  return scene\nend\n", encoding="utf-8")
    (root / "examples/demo.tex").write_text("\\begin{luadraw}\nDscene3d()\n\\end{luadraw}\n", encoding="utf-8")
    pdf = fitz.open()
    page = pdf.new_page(width=600, height=800)
    page.insert_text((90, 120), "Dscene3d method creates a 3D scene", fontsize=16)
    page.insert_text((90, 145), "LuaDraw 3D graphics", fontsize=12)
    pdf.save(root / "doc/luadraw-doc-en.pdf")
    pdf.close()
    return root


class LuaDrawDocsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.base = Path(self.temp.name)
        self.root = make_root(self.base)
        self.cache = self.base / "cache"
        self.out = self.base / "out"

    def tearDown(self):
        self.temp.cleanup()

    def test_schema_query_only_search_read_and_local_image(self):
        tool = LuaDrawDocs(self.root, self.cache)
        schema = tool.definition()
        self.assertEqual(schema["function"]["name"], "luadraw_doc")
        self.assertNotIn("mode", schema["function"]["parameters"].get("required", []))
        text, image = tool.execute({"query": "Dscene3d", "limit": 10, "include_image": True}, self.out)
        self.assertIn("LuaDraw 3.7", text)
        self.assertIn("index_sha256", text)
        self.assertIn("schema 2", text)
        self.assertIn("p.", text)
        self.assertIn("bbox=", text)
        self.assertTrue(image and Path(image).is_file())
        read, no_image = tool.execute({"mode": "read", "doc": "files/luadraw_scene3d.lua", "start_line": 2, "end_line": 2}, self.out)
        self.assertIn("2:   return scene", read)
        self.assertIsNone(no_image)

    def test_view_percent_crop_and_path_boundary(self):
        tool = LuaDrawDocs(self.root, self.cache)
        text, image = tool.execute({"mode": "view", "doc": "doc/luadraw-doc-en.pdf", "page": 1, "left": 5, "top": 5, "right": 70, "bottom": 50, "zoom": 600}, self.out)
        self.assertIn("page 1/1", text)
        self.assertTrue(Path(image).is_file())
        with self.assertRaisesRegex(ValueError, "inside"):
            tool.execute({"mode": "view", "doc": "../../outside.pdf", "page": 1}, self.out)
        with self.assertRaisesRegex(ValueError, "out of range"):
            tool.execute({"mode": "view", "doc": "doc/luadraw-doc-en.pdf", "page": 2}, self.out)

    def test_warm_cache_skips_extraction_and_source_change_rebuilds(self):
        first = LuaDrawDocs(self.root, self.cache)
        first._get_index()
        cache_path = self.cache / "luadraw-doc-index-v2.json"
        cached = json.loads(cache_path.read_text(encoding="utf-8"))
        self.assertEqual(cached["schema"], 2)
        self.assertEqual(cached["version"], "3.7")
        self.assertEqual(len(cached["key"]), 64)

        second = LuaDrawDocs(self.root, self.cache)
        with mock.patch.object(second, "_build", side_effect=AssertionError("warm cache must not extract docs")):
            second.execute({"query": "Dscene3d", "include_image": False}, self.out)
        source = self.root / "files/luadraw_scene3d.lua"
        source.write_text(source.read_text(encoding="utf-8") + "-- changed\n", encoding="utf-8")
        with mock.patch.object(second, "_build", wraps=second._build) as rebuild:
            rebuilt = second._get_index()
            rebuild.assert_called_once()
        self.assertNotEqual(rebuilt["key"], cached["key"])

    def test_poppler_fallback_indexes_bbox_and_renders_crop(self):
        real_import = builtins.__import__
        def without_fitz(name, *args, **kwargs):
            if name == "fitz":
                raise ImportError("test fallback")
            return real_import(name, *args, **kwargs)
        with mock.patch("builtins.__import__", side_effect=without_fitz):
            tool = LuaDrawDocs(self.root, self.cache)
        text, image = tool.execute({"query": "Dscene3d", "include_image": True}, self.out)
        self.assertIn("bbox=", text)
        self.assertTrue(Path(image).is_file())

    def test_jsonl_cli_reuses_one_tool(self):
        request = json.dumps({"mode": "read", "doc": "files/luadraw_scene3d.lua", "start_line": 1, "end_line": 1}) + "\n"
        with mock.patch("sys.stdin", io.StringIO(request)), contextlib.redirect_stdout(io.StringIO()) as stdout:
            code = main(["--root", str(self.root), "--cache-dir", str(self.cache), "--jsonl"])
        self.assertEqual(code, 0)
        result = json.loads(stdout.getvalue().strip())
        self.assertIn("function Dscene3d", result["text"])
        self.assertIsNone(result["image_path"])


if __name__ == "__main__":
    unittest.main()
