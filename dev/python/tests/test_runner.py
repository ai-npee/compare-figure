import contextlib
import copy
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest.mock import patch

from PIL import Image
import runner
from vector_session import Client, Session, Transcript, SUBMIT_REMINDER, image_part, resolve_tool, text_of
from vector_measure import measure_image

BODY = r"""\begin{tikzpicture}
\draw[thick] (0,0) rectangle (3,2);
\draw[->] (0,0) -- (3,2);
\node at (1.5,0.5) {$A$};
\end{tikzpicture}
"""


def call(name, args=None, ident="test"):
    return {"id": ident, "type": "function", "function": {"name": name, "arguments": json.dumps(args or {})}}


def response(*calls):
    return {"role": "assistant", "content": None, "tool_calls": list(calls)}


class Scripted:
    model = "fake-model"
    options = {}
    usage = {}
    finish = "stop"
    elapsed = 0

    def __init__(self, messages):
        self.responses = iter(messages)
        self.requests = []

    def complete(self, messages, tools=None, tool_choice="auto", max_tokens=16384):
        self.requests.append((copy.deepcopy(messages), tools, tool_choice, max_tokens))
        return next(self.responses)


class RunnerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.reference = self.root / "reference.png"
        Image.new("RGB", (400, 300), "white").save(self.reference)

    def harness(self, **options):
        return runner.Harness(self.root / "work", self.reference, **options)

    def test_prompts_are_unmodified_upstream_files(self):
        manifest = json.loads((runner.ROOT / "docs/upstream.json").read_text())
        self.assertTrue(manifest["revision"].startswith("20faca6"))
        for name in ("latex_figure.system.md", "latex_figure.user.md", "latex_figurecheck.system.md", "latex_figurecheck.user.md"):
            self.assertEqual(hashlib.sha256((runner.ROOT / "prompts" / name).read_bytes()).hexdigest(),
                             manifest["files"]["go/internal/prompts/templates/" + name])
        self.assertIn("Write ONLY the body", runner.prompt("latex_figure.system.md"))
        self.assertIn("{MAX_ROUNDS}", runner.prompt("latex_figure.system.md"))
        rendered = runner.prompt("latex_figure.system.md", MAX_ROUNDS=128, OUTPUT_LANG="Chinese")
        self.assertNotIn("{MAX_ROUNDS}", rendered)
        self.assertIn("Max 128 tool rounds", rendered)

    def test_file_media_transcript_and_interrupted_resume(self):
        transcript = Transcript(self.root / "session.jsonl")
        user = {"role": "user", "content": [{"type": "text", "text": "context"}, image_part(self.reference)]}
        transcript.message(user)
        transcript.message(response(call("submit", {"path": "figure.tex"}, "unfinished")))
        messages, stats = transcript.load("fresh system")
        self.assertEqual(messages[0]["content"], "fresh system")
        self.assertEqual(messages[1], user)
        self.assertEqual(messages[-1]["tool_call_id"], "unfinished")
        self.assertIn("TOOL INTERRUPTED", messages[-1]["content"])
        self.assertNotIn("base64", transcript.path.read_text())
        again, _ = transcript.load("fresh system")
        self.assertEqual(again, messages)
        self.assertEqual(stats["submit_attempts"], 1)

    def test_all_batch_tool_results_precede_image_feedback(self):
        h = self.harness(images_dir=self.root)
        client = Scripted([response(call("view_image", {"path": "reference.png"}, "one"), call("image_context", ident="two")),
                           {"role": "assistant", "content": "done"}])
        transcript = Transcript(h.root / "session.jsonl")
        session = Session(h, client, transcript, "test")
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(session.run("Draw", [self.reference]), "text")
        sent = client.requests[-1][0]
        pos = next(i for i, m in enumerate(sent) if m.get("tool_calls"))
        self.assertEqual([m["role"] for m in sent[pos+1:pos+4]], ["tool", "tool", "user"])
        self.assertEqual([m["tool_call_id"] for m in sent[pos+1:pos+3]], ["one", "two"])
        self.assertEqual(len(client.requests[0][1]), 10)

    def test_resume_continues_without_injected_user_or_reference(self):
        h = self.harness()
        transcript = Transcript(h.root / "session.jsonl")
        transcript.message({"role": "user", "content": "old task"})
        transcript.message(response(call("read_file", {"path": "figure.tex"}, "one")))
        transcript.message({"role": "tool", "tool_call_id": "one", "content": "existing code"})
        messages, _ = transcript.load("new system")
        client = Scripted([{"role": "assistant", "content": "done"}])
        session = Session(h, client, transcript, "new system", messages=messages)
        session.run()
        self.assertEqual(client.requests[0][0], messages[:-1])
        self.assertEqual(client.requests[0][0][-1]["role"], "tool")

    def test_soft_round_limit_keeps_tools_declared_and_then_sets_none(self):
        h = self.harness()
        client = Scripted([response(call("image_context", ident="one")), response(call("image_context", ident="two")),
                           {"role": "assistant", "content": "budget used"}])
        session = Session(h, client, Transcript(h.root / "session.jsonl"), "test",
                          {"max_tool_rounds": 1, "tool_rounds_grace": 1})
        with contextlib.redirect_stdout(io.StringIO()):
            session.run("draw")
        self.assertEqual([r[2] for r in client.requests], ["auto", "auto", "none"])
        self.assertTrue(all(r[1] for r in client.requests))
        self.assertTrue(any("ROUND BUDGET EXCEEDED" in text_of(m) for m in client.requests[1][0]))

    def test_alias_resolution_matches_source(self):
        definitions = self.harness().definitions()
        self.assertEqual(resolve_tool("mcp__docvision__read_file", definitions), "read_file")
        self.assertEqual(resolve_tool("ReadFile", definitions), "read_file")
        self.assertEqual(resolve_tool("Read", definitions, {"Read": "read_file"}), "read_file")
        self.assertEqual(resolve_tool("Bash", definitions), "Bash")

    def test_compaction_keeps_task_and_tool_boundary_and_replays_images(self):
        h = self.harness()
        transcript = Transcript(h.root / "session.jsonl")
        client = Scripted([{"role": "assistant", "content": "summary preserving paths and code"}])
        session = Session(h, client, transcript, "system", {"context_limit": 300, "prune_tool_chars": -1, "keep_images": -1})
        session.record({"role": "user", "content": [{"type": "text", "text": "original task"}, image_part(self.reference)]})
        for n in range(8):
            session.record(response(call("read_file", {"path": "figure.tex"}, str(n))))
            session.record({"role": "tool", "tool_call_id": str(n), "content": "old context " * 200})
        session.maybe_compact()
        restored, _ = transcript.load("system")
        self.assertEqual(restored, session.messages)
        self.assertEqual(session.compactions, 1)
        self.assertIsNone(client.requests[0][1])
        self.assertNotIn("data:image", transcript.path.read_text())

    def test_cli_check_no_outputs_and_resume_threshold_persistence(self):
        out = self.root / "run"
        common = ["--out", str(out), "--model", "fake", "--max-turns", "1"]
        client = Scripted([{"role": "assistant", "content": "not submitted"}] * 6)
        with patch.dict(os.environ, {"OPENAI_API_KEY": "fake-key"}), patch.object(runner, "Client", return_value=client), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.main(common + ["--original", str(self.reference), "--check"]), 0)
            self.assertFalse(out.exists())
            self.assertEqual(runner.main(common + ["--original", str(self.reference)]), 2)
            self.assertEqual(runner.main(common + ["--resume", "--pass-threshold", "95"]), 2)
            self.assertEqual(runner.main(common + ["--resume"]), 2)
        meta = json.loads((out / "run.json").read_text())
        self.assertEqual(meta["fig_min_overlap"], .95)
        self.assertEqual(meta["options"]["max_scale_dev"], .10)
        self.assertEqual(meta["options"]["accept_min_submits"], 8)
        self.assertEqual(meta["options"]["view_image_max"], 30)
        self.assertEqual(meta["options"]["view_pdf_max"], 25)
        self.assertNotIn("fake-key", (out / "session.jsonl").read_text())
        self.assertEqual(client.requests[1][0][-1]["content"], SUBMIT_REMINDER.replace("figure.tex", "figure.lua").replace("the file must match", "the LuaDraw source must match"))

    def test_checkpoint_resets_stats_and_last_submit_receipt_controls_credit(self):
        transcript = Transcript(self.root / "session.jsonl")
        transcript.message(response(call("submit", {"code": "old"}, "old")))
        transcript.message({"role": "tool", "tool_call_id": "old", "content": "SUBMITTED old"})
        snapshot = [{"role": "user", "content": "task"}, response(call("submit", {"code": "new"}, "new")),
                    {"role": "tool", "tool_call_id": "new", "content": "REJECTED current"}]
        transcript.event({"t": "checkpoint", "messages": snapshot})
        messages, stats = transcript.load("system")
        self.assertEqual(stats["rounds"], 1)
        self.assertEqual(stats["tools"], 1)
        self.assertEqual(stats["submit_attempts"], 1)
        self.assertFalse(stats["accepted"])
        self.assertEqual(json.loads(stats["submit_args"])["code"], "new")

    def test_accepted_state_survives_interruption_before_delivery(self):
        h = self.harness()
        h.state.update(submitted=True, final_code=BODY, last_pdf=str(h.root / "standalone.pdf"),
                       best_overlap=.6, scale_dev_at_best=.04)
        transcript = Transcript(h.root / "session.jsonl")
        transcript.message(response(call("submit", {"code": BODY})))
        transcript.event({"t": "state", "state": {**h.state, "_accepted_threshold": .17, "_accepted_scale_limit": .1}})
        transcript.message({"role": "tool", "tool_call_id": "test", "content": "SUBMITTED. accepted"})
        messages, stats = transcript.load("fresh system")
        fresh = runner.Harness(h.root, self.reference)
        self.assertTrue(runner.restore_submission(fresh, stats, .17, .1))
        self.assertEqual(fresh.state["final_code"], BODY)
        self.assertEqual(fresh.state["last_pdf"], "")
        self.assertFalse(runner.restore_submission(runner.Harness(h.root, self.reference), stats, .9, .1))

    def test_resume_credit_rechecks_current_threshold(self):
        for file in ("submitted.tex", "submitted.pdf"):
            (self.root / file).write_text("artifact")
        (self.root / "result.json").write_text(json.dumps({"best_overlap": .3, "scale_dev": .05}))
        self.assertIsNotNone(runner.resume_credit(self.root, .17, .1, True))
        self.assertIsNone(runner.resume_credit(self.root, .5, .1, True))
        self.assertIsNone(runner.resume_credit(self.root, .17, .01, True))
        self.assertIsNone(runner.resume_credit(self.root, .17, .1, False))

    def test_optional_visual_checker_roundtrip_and_transport_fallback(self):
        class DrawSession:
            def __init__(inner, state):
                inner.state, inner.feedback = state, []
            def run(inner, text):
                inner.feedback.append(text)
                inner.state["final_code"] = "fixed code"
        class Submitted:
            root = self.root
            state = {"last_pdf": "submitted.pdf", "final_code": "old code", "submitted": True}
            def rasterize(inner, pdf, prefix):
                return self.reference
        h = Submitted()
        drawing = DrawSession(h.state)
        checker = Scripted([
            response(call("submit", {"report": {"status": "issues", "issues": "Missing label A"}}, "c1")),
            response(call("submit", {"report": {"status": "pass"}}, "c2")),
        ])
        with contextlib.redirect_stdout(io.StringIO()):
            warnings = runner.verify_figure(h, drawing, self.reference, checker, {"max_tool_rounds": 4}, {"max_rounds": 2}, "Chinese")
        self.assertEqual(warnings, [])
        self.assertEqual(len(drawing.feedback), 1)
        self.assertIn("Missing label A", drawing.feedback[0])
        self.assertIn('compile {path: "figure.tex"}', drawing.feedback[0])
        self.assertEqual(len(checker.requests[0][0][-1]["content"]), 3)
        class Broken(Scripted):
            def complete(inner, *args, **kwargs):
                raise RuntimeError("offline")
        warnings = runner.verify_figure(h, drawing, self.reference, Broken([]), {}, {}, "Chinese")
        self.assertIn("delivering current version", warnings[0])

    def test_mineru_measurement_and_explicit_original_size(self):
        part = self.root / "part"
        folder = part / "images"
        folder.mkdir(parents=True)
        target = folder / "sample.png"
        Image.new("RGB", (400, 200), "white").save(target)
        (part / "content_list.json").write_text(json.dumps([{"img_path": "images/sample.png", "bbox": [0, 0, 144, 90], "page_idx": 0}]))
        (part / "layout.json").write_text(json.dumps({"pdf_info": [{"page_size": [600, 800]}]}))
        m = measure_image(target)
        self.assertAlmostEqual(m.width_mm, 25.4)
        self.assertAlmostEqual(m.height_mm, 12.7)
        self.assertIn("ORIGINAL FIGURE SIZE", m.hint())
        copy = self.root / "sample.png"
        shutil.copyfile(target, copy)
        self.assertAlmostEqual(measure_image(copy, self.root).width_mm, m.width_mm)
        self.assertAlmostEqual(measure_image(copy, reference_mm=(30, 20)).height_mm, 20)
        self.assertIn("this crop is", m.crop(0, 0, 50, 50).crop_hint())

    def test_model_config_extends_and_limit_inheritance(self):
        config = {"models": {"text": {"api_key": "secret", "max_images": 9, "max_tokens": 1000},
                             "base": {"model": "vision", "base_url": "http://localhost/v1", "request_body": {"a": 1}},
                             "draw": {"extends": "base", "request_body": {"b": 2}}}}
        entry = runner.model_config(config, "draw")
        self.assertEqual(entry["request_body"], {"a": 1, "b": 2})
        self.assertEqual(entry["api_key"], "secret")
        self.assertNotIn("max_images", entry)

    def test_three_api_protocols_and_chat_stream_assembly(self):
        received = []
        class Handler(BaseHTTPRequestHandler):
            def do_POST(inner):
                data = json.loads(inner.rfile.read(int(inner.headers["Content-Length"])))
                received.append((inner.path, data))
                if "/no-stream/" in inner.path and data.get("stream"):
                    inner.send_response(400)
                    inner.end_headers()
                    inner.wfile.write(b"stream is not supported")
                    return
                if "/bad-request/" in inner.path:
                    inner.send_response(400)
                    inner.end_headers()
                    inner.wfile.write(b"malformed transcript")
                    return
                if inner.path.endswith("/responses"):
                    result = {"output": [{"type": "function_call", "call_id": "r1", "name": "image_context", "arguments": "{}"}], "usage": {"input_tokens": 10}}
                elif inner.path.endswith("/messages"):
                    result = {"content": [{"type": "tool_use", "id": "a1", "name": "image_context", "input": {}}]}
                elif data.get("stream"):
                    chunks = [
                        {"choices": [{"delta": {"tool_calls": [{"index": 0, "id": "s1", "function": {"name": "image_context", "arguments": "{"}}]}}]},
                        {"choices": [{"delta": {"tool_calls": [{"index": 0, "function": {"arguments": "}"}}]}, "finish_reason": "tool_calls"}]},
                        {"choices": [], "usage": {"prompt_tokens": 15}}]
                    raw = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
                    inner.send_response(200)
                    inner.send_header("Content-Type", "text/event-stream")
                    inner.end_headers()
                    inner.wfile.write(raw.encode())
                    return
                else:
                    result = {"choices": [{"message": response(call("image_context")), "finish_reason": "tool_calls"}]}
                raw = json.dumps(result).encode()
                inner.send_response(200)
                inner.send_header("Content-Type", "application/json")
                inner.end_headers()
                inner.wfile.write(raw)
            def log_message(*args):
                pass
        server = HTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(thread.join)
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        messages = [{"role": "system", "content": "system"}, {"role": "user", "content": [{"type": "text", "text": "draw"}, image_part(self.reference)]}]
        definitions = self.harness().definitions()
        for api, stream in (("openai", False), ("openai", True), ("responses", False), ("anthropic", False)):
            client = Client(f"http://127.0.0.1:{server.server_port}/v1", "model", "fake", api=api, stream=stream)
            result = client.complete(messages, definitions)
            self.assertEqual(result["tool_calls"][0]["function"]["name"], "image_context")
            self.assertEqual(json.loads(result["tool_calls"][0]["function"]["arguments"]), {})
            if api == "responses":
                self.assertIn("input", received[-1][1])
            elif api == "anthropic":
                self.assertEqual(received[-1][1]["messages"][0]["content"][1]["type"], "image")
        for api in ("openai", "responses", "anthropic"):
            client = Client(f"http://127.0.0.1:{server.server_port}/v1/no-stream", "model", "fake", api=api, stream=True)
            start = len(received)
            self.assertIn("tool_calls", client.complete(messages, definitions))
            self.assertEqual([item[1]["stream"] for item in received[start:]], [True, False])
        bad = Client(f"http://127.0.0.1:{server.server_port}/v1/bad-request", "model", "fake", stream=True)
        start = len(received)
        with self.assertRaisesRegex(RuntimeError, "HTTP 400"):
            bad.complete(messages, definitions)
        self.assertEqual(len(received) - start, 1)

    def test_anthropic_native_stream_reassembles_thinking_and_tools(self):
        events = [
            {"type": "message_start", "message": {"usage": {"input_tokens": 12}}},
            {"type": "content_block_start", "index": 0, "content_block": {"type": "thinking", "thinking": ""}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "inspect"}},
            {"type": "content_block_stop", "index": 0},
            {"type": "content_block_start", "index": 1, "content_block": {"type": "tool_use", "id": "tool", "name": "image_context", "input": {}}},
            {"type": "content_block_delta", "index": 1, "delta": {"type": "input_json_delta", "partial_json": "{\"up\":20}"}},
            {"type": "content_block_stop", "index": 1},
            {"type": "message_delta", "delta": {"stop_reason": "tool_use"}, "usage": {"output_tokens": 5}},
            {"type": "message_stop"},
        ]
        raw = io.BytesIO("".join("data: " + json.dumps(e) + "\n\n" for e in events).encode())
        result = Client("http://localhost", "model", "fake")._stream(raw, "anthropic")
        self.assertEqual(result["content"][0]["thinking"], "inspect")
        self.assertEqual(result["content"][1]["input"], {"up": 20})
        self.assertEqual(result["usage"], {"input_tokens": 12, "output_tokens": 5})

    @unittest.skipUnless(all(shutil.which(x) for x in ("xelatex", "pdfinfo", "pdftoppm")), "needs TeX and Poppler")
    def test_cli_delivery_and_resume_after_accepted_submit_without_model_call(self):
        SOURCE = 'local ld=luadraw\nlocal g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}\ng:Dline(-1,1,"red,thick")\ng:Dcircle(0,1,nil,"blue")\ng:Show()\n'
        from luadraw_runtime import compile_figure
        lua = self.root / "reference.lua"
        lua.write_text(SOURCE)
        built = compile_figure(lua, self.root / "reference-luadraw", {"engine": "auto"})
        self.assertTrue(built["ok"], built.get("error"))
        producer = self.harness(max_scale_dev=-1)
        reference = producer.rasterize(Path(built["pdf"]), self.root / "original")
        target = self.root / "cli"
        args = ["--original", str(reference), "--out", str(target), "--model", "fake-model", "--pass-threshold", "90", "--max-scale-dev", "-1"]
        model = Scripted([response(call("write_file", {"path": "figure.lua", "content": SOURCE}, "w"),
                                   call("compile", ident="c"), call("compare_figure", ident="cmp"),
                                   call("submit", {"path": "figure.lua"}, "s"))])
        with patch.dict(os.environ, {"OPENAI_API_KEY": "fake-key"}), patch.object(runner, "Client", return_value=model), contextlib.redirect_stdout(io.StringIO()):
            with patch.object(runner.LuaDrawHarness, "deliver", side_effect=RuntimeError("simulated delivery interruption")):
                with self.assertRaisesRegex(RuntimeError, "simulated delivery"):
                    runner.main(args)
        self.assertFalse((target / "result.json").exists())
        no_more_model = Scripted([])
        resume = ["--out", str(target), "--model", "fake-model", "--resume"]
        with patch.dict(os.environ, {"OPENAI_API_KEY": "fake-key"}), patch.object(runner, "Client", return_value=no_more_model), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.main(resume), 0)
            self.assertEqual(runner.main(resume), 0)
        self.assertEqual(no_more_model.requests, [])
        result = json.loads((target / "result.json").read_text())
        self.assertGreaterEqual(result["best_overlap"], .9)
        self.assertTrue((target / "submitted.pdf").exists())

    @unittest.skipUnless(all(shutil.which(x) for x in ("xelatex", "pdfinfo", "pdftoppm")), "needs TeX and Poppler")
    def test_real_source_body_compile_compare_submit_delivery(self):
        producer = self.harness(max_scale_dev=-1)
        producer.execute("write_file", {"path": "figure.tex", "content": BODY})
        text, _ = producer.execute("compile", {"path": "figure.tex"})
        self.assertIn("COMPILE OK", text)
        reference = producer.rasterize(Path(producer.state["last_pdf"]), self.root / "rendered")
        h = runner.Harness(self.root / "draw", reference, threshold=90, max_scale_dev=-1)
        client = Scripted([
            response(call("write_file", {"path": "figure.tex", "content": BODY})),
            response(call("compile", {"path": "figure.tex"})),
            response(call("view_pdf", {"path": "standalone.pdf", "left": 20, "right": 80, "zoom": 900})),
            response(call("compare_figure", {"worst_region": True, "detail": "gray", "zoom": 500})),
            response(call("submit", {"path": "figure.tex"})),
        ])
        session = Session(h, client, Transcript(h.root / "session.jsonl"), runner.prompt("latex_figure.system.md", MAX_ROUNDS=128, OUTPUT_LANG="Chinese"))
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(session.run("draw", [reference]), "submitted")
        result = h.deliver()
        self.assertTrue((h.root / "submitted.pdf").exists())
        self.assertIn("\\documentclass", (h.root / "submitted.tex").read_text())
        self.assertEqual((h.root / "submitted.body.tex").read_text(), BODY)
        self.assertTrue(h.state["submitted"])
        self.assertGreaterEqual(h.state["best_overlap"], .9)


if __name__ == "__main__":
    unittest.main()
