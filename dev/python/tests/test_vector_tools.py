import json
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from PIL import Image

import vector_tools


class VectorToolContractTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.ref = self.root / "ref.png"
        Image.new("RGBA", (80, 60), (0, 0, 0, 0)).save(self.ref)
        self.h = vector_tools.Harness(self.root / "work", self.ref, md_content="a\n![x](images/a.png)\nb")

    def tearDown(self):
        self.tmp.cleanup()

    def test_schemas_and_hidden_submit_parameters(self):
        defs = self.h.definitions()
        self.assertEqual([d["function"]["name"] for d in defs], ["write_file", "edit_file", "read_file", "grep", "compile", "view_pdf", "compare_figure", "submit", "image_context", "view_image"])
        schemas = {d["function"]["name"]: d["function"] for d in defs}
        self.assertEqual(set(schemas["submit"]["parameters"]["properties"]), {"path", "code", "merges"})
        self.assertNotIn("accept_as_is", json.dumps(schemas["submit"]))
        self.assertNotIn('"note"', json.dumps(schemas["submit"]))
        self.assertEqual(schemas["submit"]["parameters"]["properties"]["merges"]["type"], "string")
        self.assertIn("Mounts: work(rw)", schemas["read_file"]["description"])
        self.assertIn("Mounts: work(rw)", schemas["view_pdf"]["description"])
        self.assertEqual(self.h.min_overlap, .17)
        self.assertEqual(vector_tools.Harness(self.root/"w2", self.ref, threshold=50).min_overlap, .5)

    def test_standalone_wrapper_hoists_setup_lines(self):
        body="\\usetikzlibrary{arrows.meta}\n\\usepackage{xcolor}\n\\begin{tikzpicture}\\draw (0,0)--(1,1);\\end{tikzpicture}"
        out=vector_tools.build_standalone(body, True)
        self.assertIn("\\usepackage{ctex}",out)
        self.assertIn("\\usetikzlibrary{arrows.meta}",out)
        self.assertIn("\\usepackage{xcolor}",out)
        self.assertIn("\\begin{tikzpicture}",out)
        self.assertIn("border=6pt",out)

    def test_file_mount_paths_write_edit_read_grep(self):
        self.h.execute("write_file", {"path":"work:figure.tex", "content":"alpha\nbeta\n"})
        self.assertTrue((self.root/"work"/"figure.tex").exists())
        self.assertIn("越界",self.h.execute("write_file", {"path":"../escape.tex", "content":"x"})[0])
        self.h.execute("edit_file", {"path":"/work/figure.tex", "find":"alpha", "replace":"omega"})
        self.assertIn("omega",self.h.execute("read_file", {"path":"work:figure.tex"})[0])
        self.assertIn("figure.tex:1:omega",self.h.execute("grep", {"pattern":"omega"})[0])

    def test_read_window_caps_at_400_and_edit_append_adds_newline(self):
        data="".join(f"line-{i}\n" for i in range(1,405))
        self.h.execute("write_file", {"path":"window.tex", "content":data})
        text,_=self.h.execute("read_file", {"path":"window.tex", "start_line":1, "end_line":404})
        self.assertTrue(text.startswith("[lines 1-400 of 405]"))
        self.assertIn("400: line-400",text)
        self.assertNotIn("401: line-401",text)
        self.h.execute("write_file", {"path":"append.tex", "content":"tail"})
        self.h.execute("edit_file", {"path":"append.tex", "append":True, "replace":"next"})
        self.assertEqual((self.root/"work"/"append.tex").read_text(),"tail\nnext")

    def test_grep_uses_re2_unicode_and_invalid_literal_fallback(self):
        self.h.execute("write_file", {"path":"unicode.tex", "content":"数学分析\nroman"})
        self.assertIn("unicode.tex:1:数学分析",self.h.execute("grep", {"pattern":r"\p{Han}+"})[0])
        self.h.execute("write_file", {"path":"literal.tex", "content":"a[b\nother"})
        self.assertIn("literal.tex:1:a[b",self.h.execute("grep", {"pattern":"["})[0])

    def test_compile_collects_merges_but_submit_merges_executor_ignored(self):
        self.h.execute("write_file", {"path":"figure.tex", "content":"\\begin{tikzpicture}\\draw (0,0)--(1,1);\\end{tikzpicture}"})
        self.h.execute("compile", {"path":"figure.tex", "merges":["images/a.png"]})
        self.assertEqual(self.h.state["merges"],["images/a.png"])
        self.assertEqual(next(d for d in self.h.definitions() if d["function"]["name"]=="submit")["function"]["parameters"]["properties"]["merges"]["type"],"string")

    def test_compare_scores_full_before_crop_and_sets_compare_called(self):
        self.h.state.update(compile_ok=True,last_pdf=str(self.root/"candidate.pdf"));(self.root/"candidate.pdf").write_bytes(b"fake")
        metric={"similarity":.8,"channels":[.8,.7,.9],"gray":.75,"dx":2,"dy":-1,"runs":[],"scale_x":1,"scale_y":1,"scale_dev":0,"size_unit":"aspect","aspect_ref":1,"aspect_cand":1,"ink_ref_w":10,"ink_ref_h":10,"ink_cand_w":10,"ink_cand_h":10,"share_ref_w":.5,"share_ref_h":.5,"share_cand_w":.5,"share_cand_h":.5}
        with patch.object(self.h,"rasterize",return_value=self.root/"render.png"),patch.object(vector_tools.Image,"open",side_effect=[Image.new("RGBA",(80,60),(0,0,0,0)),Image.new("RGB",(60,80))]),patch.object(self.h,"_pdf_info",return_value=(1,20,30)),patch.object(vector_tools.vector_metrics,"compare_images",return_value=metric) as cmp,patch.object(vector_tools.vector_metrics,"make_overlay",return_value=Image.new("RGB",(100,100),"white")):
            text,path=self.h.execute("compare_figure",{"left":10,"top":10,"right":50,"bottom":50,"zoom":120})
        self.assertTrue(self.h.state["compare_called"],text)
        self.assertEqual(cmp.call_args.args[0].mode,"RGBA")
        self.assertEqual(cmp.call_args.kwargs["canvas_mm"],(0.0,0.0,20,30))
        self.assertEqual(self.h.state["best_overlap"],.8)
        self.assertIn("Ink size:",text)
        self.assertTrue(Path(path).exists())

    def test_reference_mm_pair_and_same_attempt_best_metrics(self):
        self.h.options["reference_mm"]=(34,18)
        self.h.state.update(compile_ok=True,last_pdf=str(self.root/"cand.pdf"));(self.root/"cand.pdf").write_bytes(b"pdf")
        base={"channels":[.5]*3,"gray":.5,"dx":0,"dy":0,"runs":[],"scale_x":1,"scale_y":1,"size_unit":"mm","aspect_ref":1,"aspect_cand":1,"ink_ref_w":10,"ink_ref_h":10,"ink_cand_w":11,"ink_cand_h":10,"share_ref_w":.5,"share_ref_h":.5,"share_cand_w":.5,"share_cand_h":.5}
        one=dict(base,similarity=.6,scale_dev=.4,ink_cand_w=14);two=dict(base,similarity=.9,scale_dev=.1,ink_cand_w=11)
        with patch.object(self.h,"rasterize",return_value=self.root/"x.png"),patch.object(vector_tools.Image,"open",side_effect=[Image.new("RGB",(40,40))]*4),patch.object(self.h,"_pdf_info",return_value=(1,40,22)),patch.object(vector_tools.vector_metrics,"compare_images",side_effect=[one,two]) as cmp,patch.object(vector_tools.vector_metrics,"make_overlay",return_value=Image.new("RGB",(40,40))):
            self.h.execute("compare_figure",{});self.h.execute("compare_figure",{})
        self.assertEqual(cmp.call_args.kwargs["canvas_mm"],(34.,18.,40,22))
        self.assertEqual(self.h.state["best_overlap"],.9)
        self.assertEqual(self.h.state["ink_cand_w_at_best"],11)
        self.assertEqual(self.h.state["scale_dev_at_best"],.1)

    def test_view_pdf_rerenders_for_crop_and_reports_budget_and_size(self):
        (self.root/"work"/"sample.pdf").write_bytes(b"pdf");self.h.options.update(view_pdf_max=1,warn_ratio=1)
        with patch.object(self.h,"_pdf_render",return_value=Image.new("RGB",(400,200),"white")) as render,patch.object(self.h,"_pdf_info",return_value=(1,50,25)):
            text,path=self.h.execute("view_pdf",{"path":"sample.pdf","left":50,"right":100,"zoom":100})
            second,_=self.h.execute("view_pdf",{"path":"sample.pdf","left":50,"right":100,"zoom":100})
        self.assertEqual(render.call_args.args[2],400)
        self.assertIn("this crop 25.0mm x 25.0mm",text)
        self.assertIn("VIEW BUDGET",text)
        self.assertIn("VIEW BUDGET SPENT",second)
        with Image.open(path) as im:self.assertEqual(im.width,100)

    def test_view_image_subject_resolution_measurement_and_budget(self):
        images=self.root/"images";subject=images/"book";subject.mkdir(parents=True)
        Image.new("RGBA",(16,12),(0,0,0,0)).save(subject/"local.png")
        self.h.images_dir=images;self.h.current_img="images/book/main.png";self.h.options.update(view_image_max=1,warn_ratio=1)
        text,path=self.h.execute("view_image",{"path":"images/book/local.png","zoom":32})
        self.assertIn("ORIGINAL BITMAP: 16x12px, aspect 1.33:1.",text)
        self.assertIn("VIEW BUDGET",text)
        self.assertTrue(Path(path).exists())

    def test_image_context_unique_basename_positive_window_and_neighbor_delta(self):
        self.h.options["md_content"]="top\n![a](images/x/a.png)\ntext\n![b](images/y/b.png)\nend"
        text,_=self.h.execute("image_context",{"image":"images/y/b.png","up":0,"down":-4})
        self.assertIn("up 10 / down 10",text)
        self.assertIn("-2 lines from this image",text)
        self.h.options["md_content"]="![a](images/x/a.png)\n![b](images/y/a.png)"
        text,_=self.h.execute("image_context",{"image":"a.png"})
        self.assertIn("找不到图片",text)

    def test_live_threshold_zero_ignored_and_negative_disables_gate(self):
        self.h.options["live_threshold"]=lambda:.5
        self.h.execute("submit",{"code":"body"})
        self.assertEqual(self.h.min_overlap,.5)
        self.h.options["live_threshold"]=lambda:0
        self.h.execute("submit",{"code":"body"})
        self.assertEqual(self.h.min_overlap,.5)
        self.h.options["live_threshold"]=lambda:-1
        self.h.execute("submit",{"code":"body"})
        self.assertEqual(self.h.min_overlap,-1)

    def test_submit_is_state_only_deliver_writes_flat_result_and_preview(self):
        self.h.state.update(compare_called=True,compile_ok=True,last_code="body",last_pdf=str(self.root/"work"/"standalone.pdf"),best_overlap=.9)
        pdf=Path(self.h.state["last_pdf"]);pdf.write_bytes(b"pdf")
        text,_=self.h.execute("submit",{"code":"body"})
        self.assertIn("SUBMITTED",text)
        self.assertFalse((self.root/"work"/"submitted.pdf").exists())
        def fake_rasterize(pdf,prefix):
            path=Path(prefix).with_suffix(".png");path.write_bytes(b"png");return path
        with patch.object(self.h,"rasterize",side_effect=fake_rasterize):
            result=self.h.deliver()
        self.assertIn("best_overlap",result)
        self.assertTrue((self.root/"work"/"result.json").exists())
        self.assertIn("预览/submitted.png",result["preview"])

    def test_measure_hint_and_rasterize_api(self):
        self.assertEqual(self.h.measure_hint(),"ORIGINAL BITMAP: 80x60px, aspect 1.33:1.")
        with patch.object(vector_tools.subprocess,"run") as run:
            run.side_effect=lambda cmd,**kw:Path(str(cmd[-1])+".png").touch()
            p=self.h.rasterize(self.ref,self.root/"preview")
        self.assertEqual(p.name,"preview.png")
        self.assertIn("-r",run.call_args.args[0])

    def test_deliver_recompiles_final_body_and_keeps_pdf_when_preview_fails(self):
        self.h.state.update(submitted=True,final_code="body",last_pdf="")
        def compile_final(cmd,**kwargs):
            (self.root/"work"/"standalone.pdf").write_bytes(b"valid pdf placeholder")
            return __import__("subprocess").CompletedProcess(cmd,0,stdout="")
        with patch.object(vector_tools.subprocess,"run",side_effect=compile_final),patch.object(self.h,"rasterize",side_effect=RuntimeError("no renderer")):
            result=self.h.deliver()
        self.assertTrue((self.root/"work"/"submitted.pdf").exists())
        self.assertIn("warning",result)
        self.assertTrue(json.loads((self.root/"work"/"result.json").read_text())["submitted"])

    @unittest.skipUnless(shutil.which("xelatex") and shutil.which("pdftoppm"),"local XeLaTeX/Poppler required")
    def test_real_xelatex_compile_with_promoted_preamble(self):
        code=r"\usetikzlibrary{arrows.meta}"+"\n"+r"\begin{tikzpicture}\draw (0,0)--(1,1);\end{tikzpicture}"
        self.h.execute("write_file",{"path":"figure.tex","content":code})
        text,_=self.h.execute("compile",{"path":"figure.tex"})
        self.assertIn("COMPILE OK",text)
        self.assertTrue(Path(self.h.state["last_pdf"]).exists())
        self.assertIn("\\usetikzlibrary{arrows.meta}",(self.root/"work"/"standalone.tex").read_text())


if __name__ == "__main__":
    unittest.main()
