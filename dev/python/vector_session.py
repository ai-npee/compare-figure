"""Standalone transport/session adapters for the DocVision vector-stage port.

Stage prompts/tools are ported separately; this module replaces the host session
service. Source revision and precise compatibility scope are in docs/migration.md.
"""
import base64
import copy
from datetime import datetime, timezone
import hashlib
import io
import json
import math
import mimetypes
from pathlib import Path
import re
import time
import urllib.error
import urllib.request

from PIL import Image

COMPACT_MARKER = "=== COMPRESSED SESSION CONTEXT"
SUBMIT_REMINDER = ('You have NOT called submit yet. Do these in order: (1) compile {path: "figure.tex"} until it succeeds, '
                   '(2) call compare_figure and look at its comparison, (3) call submit with {path: "figure.tex"} '
                   '(the file must match the last successful compile). Submitting before compare_figure succeeds is rejected.')
CHECK_FEEDBACK = ('A vision check compared your submitted figure with the ORIGINAL image and found visible differences:\n'
                  '{problems}\n\nFix exactly these points in figure.tex (do not change anything else), '
                  'call compile {path: "figure.tex"}, compare your render with the original once more, '
                  'and call submit again. Sizes, positions and labels in the original are the reference.')


def image_part(path, max_side=0):
    path = Path(path)
    mime = mimetypes.guess_type(str(path))[0] or "image/png"
    data = path.read_bytes()
    if max_side:
        with Image.open(io.BytesIO(data)) as im:
            if max(im.size) > max_side:
                im.thumbnail((max_side, max_side), Image.Resampling.LANCZOS)
                out = io.BytesIO()
                im.convert("RGB").save(out, format="JPEG", quality=90)
                data, mime = out.getvalue(), "image/jpeg"
    return {"type": "image_url", "image_url": {"url": "data:" + mime + ";base64," + base64.b64encode(data).decode()}}


def text_of(message):
    content = message.get("content") or ""
    if isinstance(content, str):
        return content
    return "\n".join(p.get("text", "") for p in content if p.get("type") in {"text", "input_text", "output_text"})


class Transcript:
    """Append-only source-compatible t=meta/msg/usage JSONL with file media."""
    def __init__(self, path):
        self.path = Path(path).resolve()
        self.media = self.path.parent / "media"
        self.media.mkdir(parents=True, exist_ok=True)

    def event(self, event):
        event = {**event, "ts": datetime.now(timezone.utc).isoformat(timespec="milliseconds")}
        with self.path.open("a", encoding="utf-8") as out:
            out.write(json.dumps(event, ensure_ascii=False) + "\n")
            out.flush()

    def meta(self, system, definitions, model, label="tikz"):
        self.event({"t": "meta", "kind": "system", "session_label": label, "model": model,
                    "system": system, "system_sha": hashlib.sha256(system.encode()).hexdigest(),
                    "tools": [d["function"] for d in definitions]})

    def message(self, message):
        if message["role"] == "system":
            return
        event = {"t": "msg", "role": message["role"], "text": text_of(message)}
        for key in ("tool_calls", "tool_call_id", "reasoning_content", "_response_items", "_anthropic_blocks"):
            if key in message:
                event[key] = message[key]
        images = []
        if isinstance(message.get("content"), list):
            for part in message["content"]:
                if part.get("type") != "image_url":
                    continue
                url = part["image_url"]["url"]
                if not url.startswith("data:"):
                    raise ValueError("Transcript accepts only inline image data")
                header, encoded = url.split(",", 1)
                data = base64.b64decode(encoded, validate=True)
                ext = mimetypes.guess_extension(header[5:].split(";")[0]) or ".png"
                name = hashlib.sha256(data).hexdigest()[:24] + ext
                target = self.media / name
                if not target.exists():
                    target.write_bytes(data)
                images.append("file://media/" + name)
        if images:
            event["images"] = images
        event["h"] = hashlib.sha256(json.dumps(event, sort_keys=True, ensure_ascii=False).encode()).hexdigest()[:12]
        self.event(event)

    def load(self, system):
        messages = [{"role": "system", "content": system}]
        stats = {"rounds": 0, "tools": 0, "submit_attempts": 0, "accepted": False}
        pending = {}
        last_state = None
        if not self.path.exists():
            return messages, stats
        for number, line in enumerate(self.path.read_text(encoding="utf-8").splitlines(), 1):
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                raise ValueError(f"Invalid transcript line {number}; repair the truncated line before resuming") from None
            kind = event.get("t")
            if kind == "checkpoint":
                restored = copy.deepcopy(event["messages"])
                for message in restored:
                    if isinstance(message.get("content"), list):
                        for part in message["content"]:
                            if part.get("type") == "image_url" and part["image_url"]["url"].startswith("file://"):
                                target = (self.path.parent / part["image_url"]["url"][7:]).resolve()
                                if not target.is_relative_to(self.path.parent):
                                    raise ValueError("Checkpoint media escapes session directory")
                                part["image_url"] = image_part(target)["image_url"]
                messages = [{"role": "system", "content": system}] + restored
                pending = {}
                last_state = None
                continue
            if kind == "state":
                last_state = event.get("state")
                continue
            if kind == "usage":
                continue
            if event.get("type") == "message":  # first Python prototype
                msg = event["message"]
                if msg["role"] == "system":
                    continue
            elif kind == "msg":
                msg = {"role": event["role"], "content": event.get("text", "")}
                for key in ("tool_calls", "tool_call_id", "reasoning_content", "_response_items", "_anthropic_blocks"):
                    if key in event:
                        msg[key] = event[key]
                if event.get("images"):
                    parts = [{"type": "text", "text": msg["content"]}] if msg["content"] else []
                    for ref in event["images"]:
                        if not ref.startswith("file://"):
                            raise ValueError("Invalid transcript media reference")
                        target = (self.path.parent / ref[7:]).resolve()
                        if not target.is_relative_to(self.path.parent):
                            raise ValueError("Transcript media escapes session directory")
                        parts.append(image_part(target))
                    msg["content"] = parts
            else:
                continue
            messages.append(msg)
        # Like LoadTranscriptStats, count only the retained replay span, not
        # the pre-compaction history or auxiliary usage/summary requests.
        stats = {"rounds": 0, "tools": 0, "submit_attempts": 0, "accepted": False}
        pending, submit_calls = {}, {}
        for msg in messages:
            if msg["role"] == "assistant":
                stats["rounds"] += 1
                for call in msg.get("tool_calls") or []:
                    pending[call["id"]] = call
                    if call.get("function", {}).get("name") == "submit":
                        submit_calls[call["id"]] = call
                        stats["submit_attempts"] += 1
            elif msg["role"] == "tool":
                stats["tools"] += 1
                pending.pop(msg["tool_call_id"], None)
                if msg["tool_call_id"] in submit_calls:
                    call = submit_calls[msg["tool_call_id"]]
                    stats["submit_args"] = call["function"]["arguments"]
                    stats["accepted"] = text_of(msg).strip().startswith("SUBMITTED")
        if stats["accepted"]:
            stats["submitted_state"] = last_state
        for ident in pending:
            msg = {"role": "tool", "tool_call_id": ident,
                   "content": "TOOL INTERRUPTED: no result was recorded before the previous process stopped. Check the workspace before retrying."}
            self.message(msg)
            messages.append(msg)
        return messages, stats


class Client:
    """OpenAI Chat/Responses and Anthropic adapters, without a model SDK."""
    def __init__(self, base_url, model, api_key, timeout=180, **options):
        self.base_url, self.model, self.api_key, self.timeout = base_url.rstrip("/"), model, api_key, timeout
        self.options = options
        self.usage = {}
        self.finish = ""
        self.elapsed = 0
        self.partial = None

    def _request(self, path, body, headers=None):
        retries = self.options.get("api_max_retries", 3)
        attempt = 0
        while True:
            request = urllib.request.Request(self.base_url + path, data=json.dumps(body).encode(), headers={
                "Content-Type": "application/json", "Authorization": "Bearer " + self.api_key, **(headers or {})})
            try:
                started = time.monotonic()
                with urllib.request.urlopen(request, timeout=self.timeout) as response:
                    if "text/event-stream" in response.headers.get("Content-Type", ""):
                        result = self._stream(response, self.options.get("api", "openai"))
                    else:
                        result = json.load(response)
                self.elapsed = time.monotonic() - started
                return result
            except urllib.error.HTTPError as exc:
                detail = exc.read(16384).decode("utf-8", errors="replace").lower()
                stream_hint = any(word in detail for word in ("stream", "unsupported", "not support"))
                if body.get("stream") and (stream_hint or exc.code in {404, 405, 415, 422}):
                    body = {**body, "stream": False}
                    body.pop("stream_options", None)
                    continue
                if exc.code not in {408, 429, 500, 502, 503, 504, 529} or (retries > 0 and attempt >= retries - 1):
                    raise RuntimeError(f"Model API HTTP {exc.code}; check configured protocol/endpoint/model/credentials") from None
            except RuntimeError as exc:
                if body.get("stream") and "stream" in str(exc).lower():
                    body = {**body, "stream": False}
                    body.pop("stream_options", None)
                    continue
                raise
            except (urllib.error.URLError, TimeoutError, ConnectionError):
                if retries > 0 and attempt >= retries - 1:
                    raise RuntimeError("Model API connection failed after retries") from None
            attempt += 1
            time.sleep(min(2 ** min(attempt, 5), 30))

    def _stream(self, response, api):
        assembled = {"role": "assistant", "content": ""}
        calls, usage, anthropic_blocks = {}, {}, {}
        for raw in response:
            line = raw.decode("utf-8").strip()
            if not line.startswith("data:"):
                continue
            payload = line[5:].strip()
            if payload == "[DONE]":
                break
            event = json.loads(payload)
            if api == "anthropic":
                kind = event.get("type", "")
                if kind == "message_start":
                    usage.update(event.get("message", {}).get("usage", {}))
                elif kind == "content_block_start":
                    anthropic_blocks[event["index"]] = copy.deepcopy(event["content_block"])
                elif kind == "content_block_delta":
                    delta = event.get("delta", {})
                    block = anthropic_blocks[event["index"]]
                    if delta.get("type") == "input_json_delta":
                        block["_json"] = block.get("_json", "") + delta.get("partial_json", "")
                    else:
                        key = {"text_delta": "text", "thinking_delta": "thinking", "signature_delta": "signature"}.get(delta.get("type"))
                        if key:
                            block[key] = block.get(key, "") + delta.get(key, "")
                            if self.partial:
                                self.partial(block[key])
                elif kind == "content_block_stop":
                    block = anthropic_blocks[event["index"]]
                    if "_json" in block:
                        block["input"] = json.loads(block.pop("_json"))
                elif kind == "message_delta":
                    usage.update(event.get("usage", {}))
                    self.finish = event.get("delta", {}).get("stop_reason", self.finish)
                elif kind == "message_stop":
                    return {"content": [anthropic_blocks[k] for k in sorted(anthropic_blocks)], "usage": usage, "stop_reason": self.finish}
                elif kind == "error":
                    raise RuntimeError("Anthropic stream failed")
                continue
            if api == "responses":
                if event.get("type") in {"response.completed", "response.incomplete"}:
                    return event["response"]
                if event.get("type") == "response.failed":
                    raise RuntimeError("Responses API stream failed")
                if self.partial and event.get("type") == "response.output_text.delta":
                    self.partial(event.get("delta", ""))
                continue
            usage = event.get("usage") or usage
            for choice in event.get("choices", []):
                self.finish = choice.get("finish_reason") or self.finish
                delta = choice.get("delta", {})
                for key in ("content", "reasoning_content"):
                    if delta.get(key):
                        assembled[key] = assembled.get(key, "") + delta[key]
                        if self.partial:
                            self.partial(assembled[key])
                for call in delta.get("tool_calls", []):
                    out = calls.setdefault(call["index"], {"id": "", "type": "function", "function": {"name": "", "arguments": ""}})
                    if call.get("id"):
                        out["id"] = call["id"]
                    for key in ("name", "arguments"):
                        out["function"][key] += call.get("function", {}).get(key, "")
        if api in {"responses", "anthropic"}:
            raise RuntimeError(api + " stream ended before completion")
        if calls:
            assembled["tool_calls"] = [calls[k] for k in sorted(calls)]
        return {"choices": [{"message": assembled, "finish_reason": self.finish}], "usage": usage}

    def complete(self, messages, tools=None, tool_choice="auto", max_tokens=16384, **extra):
        api = self.options.get("api", "openai")
        clean = [{k: v for k, v in msg.items() if not k.startswith("_")} for msg in messages]
        body = {"model": self.model, "stream": self.options.get("stream", True)}
        temperature = self.options.get("temperature")
        if temperature is not None:
            body["temperature"] = temperature
        for key in ("thinking", "reasoning_effort", "tool_stream"):
            if key in self.options:
                body[key] = self.options[key]
        if api == "responses":
            inputs = []
            for msg in messages:
                if msg["role"] == "system":
                    body["instructions"] = text_of(msg)
                    continue
                if msg["role"] == "tool":
                    inputs.append({"type": "function_call_output", "call_id": msg["tool_call_id"], "output": text_of(msg)})
                elif msg.get("_response_items"):
                    inputs.extend(msg["_response_items"])
                elif msg["role"] == "assistant" and msg.get("tool_calls"):
                    if text_of(msg):
                        inputs.append({"role": "assistant", "content": [{"type": "output_text", "text": text_of(msg)}]})
                    for call in msg["tool_calls"]:
                        inputs.append({"type": "function_call", "call_id": call["id"], **call["function"]})
                else:
                    parts = []
                    if isinstance(msg.get("content"), list):
                        for part in msg["content"]:
                            if part["type"] == "image_url":
                                parts.append({"type": "input_image", "image_url": part["image_url"]["url"]})
                            elif part["type"] == "text":
                                parts.append({"type": "input_text", "text": part["text"]})
                    else:
                        parts = [{"type": "output_text" if msg["role"] == "assistant" else "input_text", "text": text_of(msg)}]
                    inputs.append({"role": msg["role"], "content": parts})
            body.update(input=inputs, max_output_tokens=max_tokens, store=False, include=["reasoning.encrypted_content"])
            body.pop("tool_stream", None)
            body.pop("thinking", None)
            body.pop("reasoning_effort", None)
            if tools:
                body.update(tools=[{"type": "function", **d["function"]} for d in tools], tool_choice=tool_choice)
            body.update(self.options.get("request_body", {}))
            if self.options.get("thinking") is not None:
                body["reasoning"] = self.options["thinking"]
            if self.options.get("reasoning_effort"):
                body["reasoning"] = {"effort": self.options["reasoning_effort"]}
            result = self._request("/responses", body)
            items = result.get("output", [])
            text, calls = [], []
            for item in items:
                if item.get("type") == "message":
                    text.extend(p.get("text", "") for p in item.get("content", []) if p.get("type") == "output_text")
                elif item.get("type") == "function_call":
                    calls.append({"type": "function", "id": item["call_id"], "function": {"name": item["name"], "arguments": item["arguments"]}})
            message = {"role": "assistant", "content": "\n".join(text), "_response_items": items}
            if calls:
                message["tool_calls"] = calls
            self.usage = result.get("usage") or {}
            self.finish = result.get("status", "")
        elif api == "anthropic":
            wire = []
            for msg in messages:
                if msg["role"] == "system":
                    continue
                role = "user" if msg["role"] == "tool" else msg["role"]
                blocks = []
                if msg["role"] == "tool":
                    blocks = [{"type": "tool_result", "tool_use_id": msg["tool_call_id"], "content": text_of(msg)}]
                elif msg.get("_anthropic_blocks"):
                    blocks = msg["_anthropic_blocks"]
                else:
                    if text_of(msg):
                        blocks.append({"type": "text", "text": text_of(msg)})
                    if isinstance(msg.get("content"), list):
                        for part in msg["content"]:
                            if part["type"] == "image_url":
                                header, data = part["image_url"]["url"].split(",", 1)
                                blocks.append({"type": "image", "source": {"type": "base64", "media_type": header[5:].split(";")[0], "data": data}})
                    for call in msg.get("tool_calls") or []:
                        blocks.append({"type": "tool_use", "id": call["id"], "name": call["function"]["name"], "input": json.loads(call["function"]["arguments"])})
                if wire and wire[-1]["role"] == role:
                    wire[-1]["content"].extend(blocks)
                else:
                    wire.append({"role": role, "content": blocks})
            body.update(messages=wire, system="\n".join(text_of(m) for m in messages if m["role"] == "system"), max_tokens=max_tokens)
            body.pop("reasoning_effort", None)
            if tools:
                body["tools"] = [{"name": d["function"]["name"], "description": d["function"]["description"], "input_schema": d["function"]["parameters"]} for d in tools]
                body["tool_choice"] = {"type": "none" if tool_choice == "none" else "auto"}
            body.update(self.options.get("request_body", {}))
            result = self._request("/messages", body, {"x-api-key": self.api_key, "anthropic-version": "2023-06-01"})
            blocks = result.get("content", [])
            message = {"role": "assistant", "content": "\n".join(b["text"] for b in blocks if b["type"] == "text"), "_anthropic_blocks": blocks}
            calls = [{"id": b["id"], "type": "function", "function": {"name": b["name"], "arguments": json.dumps(b["input"], ensure_ascii=False)}} for b in blocks if b["type"] == "tool_use"]
            if calls:
                message["tool_calls"] = calls
            self.usage = result.get("usage") or {}
            self.finish = result.get("stop_reason", "")
        elif api == "openai":
            body.update(messages=clean, max_tokens=max_tokens)
            if tools:
                body.update(tools=tools, tool_choice=tool_choice)
            if body["stream"]:
                body["stream_options"] = {"include_usage": True}
            if self.options.get("user"):
                body["user"] = self.options["user"]
            body.update(self.options.get("request_body", {}))
            result = self._request("/chat/completions", body)
            choice = result["choices"][0]
            message = choice["message"]
            self.usage = result.get("usage") or {}
            self.finish = choice.get("finish_reason", "")
        else:
            raise ValueError("api must be openai, responses or anthropic")
        return message


def normalized(name):
    return re.sub(r"[_\-\s.]", "", name).lower()


def resolve_tool(name, definitions, aliases=None):
    names = [d["function"]["name"] for d in definitions]
    candidates = [name.strip()]
    for sep in ("__", "/", "."):
        if sep in name:
            candidates.append(name.rsplit(sep, 1)[-1].strip())
    for cand in candidates:
        if cand in names:
            return cand
    for cand in candidates:
        for alias, target in (aliases or {}).items():
            if normalized(cand) == normalized(alias):
                hits = [n for n in names if normalized(n) == normalized(target)]
                if len(hits) == 1:
                    return hits[0]
    for cand in candidates:
        hits = [n for n in names if normalized(n) == normalized(cand)]
        if len(hits) == 1:
            return hits[0]
    return name


class Session:
    def __init__(self, harness, client, transcript, system, tuning=None, messages=None):
        self.harness, self.client = harness, client
        self.transcript = transcript
        self.definitions = harness.definitions()
        self.messages = messages or [{"role": "system", "content": system}]
        self.tuning = {"max_tool_rounds": 128, "max_tokens": 16384, "context_limit": 0,
                       "compaction_at": .85, "tool_rounds_warn_ratio": .7, "tool_rounds_grace": 20,
                       "prune_tool_chars": 4096, "keep_images": 3, **(tuning or {})}
        self.rounds, self.tools, self.compactions = 0, 0, 0
        self.last_compact_tokens = 0
        self.measured_prompt = 0
        self.calibration = 1.0
        self.access_nudged = False
        self.transcript.meta(system, self.definitions, getattr(client, "model", "unknown"))

    def record(self, message):
        self.messages.append(message)
        self.transcript.message(message)

    def estimated_tokens(self):
        text = len(json.dumps(self.definitions, ensure_ascii=False))
        images = 0
        for msg in self.messages:
            text += len(text_of(msg)) + len(msg.get("reasoning_content", ""))
            text += len(json.dumps(msg.get("tool_calls") or []))
            if isinstance(msg.get("content"), list):
                images += sum(p.get("type") == "image_url" for p in msg["content"])
        # Host token estimator is model-specific; calibrate against actual vendor usage.
        return max(1, int((text / 3 + images * 1024) * self.calibration))

    def request(self, tools=True, choice="auto", max_tokens=None, messages=None, kind=""):
        estimate = self.estimated_tokens()
        limit = self.tuning["context_limit"]
        budget = max_tokens or self.tuning["max_tokens"]
        if limit > 0:
            budget = max(1, min(budget, limit - estimate - 256))
        message = self.client.complete(messages or self.messages, self.definitions if tools else None, choice, budget)
        usage = getattr(self.client, "usage", {})
        prompt = usage.get("prompt_tokens", usage.get("input_tokens", 0))
        if prompt and kind != "compact":
            uncalibrated = estimate / self.calibration
            self.calibration = max(1, prompt / max(1, uncalibrated))
            self.measured_prompt = prompt
        self.rounds += 1
        self.transcript.event({"t": "usage", "round": self.rounds, "kind": kind,
                               "model": getattr(self.client, "model", "unknown"),
                               "prompt_tokens": prompt, "completion_tokens": usage.get("completion_tokens", usage.get("output_tokens", 0)),
                               "finish_reason": getattr(self.client, "finish", ""),
                               "duration_ms": round(getattr(self.client, "elapsed", 0) * 1000)})
        if message.get("role") != "assistant":
            raise ValueError("Model API returned a non-assistant message")
        return message

    def prune_images(self, count):
        if count <= 0:
            return
        total = sum(sum(p.get("type") == "image_url" for p in m["content"]) for m in self.messages if isinstance(m.get("content"), list))
        remove = total - count
        for msg in self.messages:
            if remove <= 0:
                break
            if not isinstance(msg.get("content"), list):
                continue
            parts = []
            for part in msg["content"]:
                if part.get("type") == "image_url" and remove > 0:
                    parts.append({"type": "text", "text": "[Earlier image omitted from context; use view_image/view_pdf to inspect it again.]"})
                    remove -= 1
                else:
                    parts.append(part)
            msg["content"] = parts

    def maybe_compact(self):
        limit = self.tuning["context_limit"]
        now = max(self.estimated_tokens(), self.measured_prompt)
        if limit <= 0 or now < limit * self.tuning["compaction_at"]:
            return
        if self.last_compact_tokens and now < self.last_compact_tokens * 1.2:
            return
        tail_start = max(2, len(self.messages) - 8)
        while tail_start > 2 and self.messages[tail_start]["role"] == "tool":
            tail_start -= 1
        chars = self.tuning["prune_tool_chars"]
        if chars == 0:
            chars = 4096
        if chars > 0:
            for msg in self.messages[2:tail_start]:
                if msg["role"] == "tool" and len(text_of(msg)) > chars:
                    text = text_of(msg)
                    msg["content"] = text[:chars // 2] + "\n[tool result pruned]\n" + text[-chars // 4:]
        keep = self.tuning["keep_images"] or 3
        self.prune_images(keep)
        if self.estimated_tokens() < limit * self.tuning["compaction_at"]:
            return
        if tail_start <= 2:
            return
        tail = copy.deepcopy(self.messages[tail_start:])
        instruction = "\n".join([
            "=== CONTEXT COMPACTION REQUEST ===",
            "Summarise the work done so far in this session as a COMPACT but COMPLETE continuation note",
            f"covering everything up to (but NOT including) the last {len(tail)} messages.", "Rules:",
            "- Keep: the task definition, all confirmed decisions, final artefacts (the exact code, commands, file paths, ids, numbers already produced), current progress and the remaining steps.",
            "- Drop: intermediate tool dumps, failed drafts, verbose reasoning, duplicate content.",
            "- Keep exact strings (code, identifiers, paths) that are still referenced later.",
            "- Do NOT summarise or repeat the last messages: they stay in the conversation as-is.",
            "Respond with the note only, no preamble."])
        summary = text_of(self.request(tools=False, max_tokens=8192, messages=self.messages + [{"role": "user", "content": instruction}], kind="compact"))
        if not summary.strip():
            raise RuntimeError("Empty compaction summary")
        note = (COMPACT_MARKER + " ===\n<compacted-summary>\n"
                "This is an automatically generated checkpoint condensing an earlier span of the conversation to free up context. "
                "Treat the captured context as established background and build on it without restating it. "
                "Continue the task directly from the messages that follow, without acknowledging this checkpoint.\n\n"
                "<summary>\n" + summary + "\n</summary>\n</compacted-summary>")
        self.messages = self.messages[:2] + [{"role": "user", "content": note}] + tail
        self.transcript.event({"t": "checkpoint", "messages": self._checkpoint_messages(self.messages[1:])})
        self.compactions += 1
        self.measured_prompt = 0
        self.last_compact_tokens = self.estimated_tokens()

    def _checkpoint_messages(self, messages):
        # Checkpoints keep file refs on disk too; use message records for all media.
        # The snapshot contains text/tool data and references to content-addressed media.
        result = copy.deepcopy(messages)
        for msg in result:
            if isinstance(msg.get("content"), list):
                for part in msg["content"]:
                    if part.get("type") == "image_url":
                        url = part["image_url"]["url"]
                        header, payload = url.split(",", 1)
                        data = base64.b64decode(payload)
                        ext = mimetypes.guess_extension(header[5:].split(";")[0]) or ".png"
                        name = hashlib.sha256(data).hexdigest()[:24] + ext
                        path = self.transcript.media / name
                        if not path.exists():
                            path.write_bytes(data)
                        part["image_url"]["url"] = "file://media/" + name
        return result

    def run(self, user_text="", images=None):
        if user_text or images:
            content = ([{"type": "text", "text": user_text}] if user_text else []) + [image_part(p) for p in (images or [])]
            self.record({"role": "user", "content": content if images else user_text})
        tuning = self.tuning
        maximum = tuning["max_tool_rounds"]
        grace = tuning["tool_rounds_grace"]
        grace = 20 if grace == 0 else max(0, grace)
        hard_stop = maximum + grace
        warning = math.ceil(maximum * tuning["tool_rounds_warn_ratio"])
        tool_rounds, post_budget, reminder_idx, empty_retry = 0, 0, -1, False
        while True:
            self.prune_images(getattr(self.client, "options", {}).get("max_images", 0))
            self.maybe_compact()
            if maximum > 0 and tool_rounds >= warning:
                text = ""
                if tool_rounds < maximum:
                    text = f"ROUND BUDGET: {tool_rounds} of {maximum} tool rounds used, {maximum-tool_rounds} left. Use them sparingly, finish the work and call submit with your best result."
                elif tool_rounds < hard_stop:
                    text = f"ROUND BUDGET EXCEEDED: the soft limit of {maximum} tool rounds is used up. You have {hard_stop-tool_rounds} extra rounds left; after that no more tool calls are possible, so wrap up now and submit your best result."
                if text:
                    reminder = {"role": "user", "content": text}
                    if 0 <= reminder_idx < len(self.messages):
                        self.messages[reminder_idx] = reminder
                    else:
                        reminder_idx = len(self.messages)
                        self.record(reminder)
            enabled = maximum <= 0 or tool_rounds < hard_stop
            message = self.request(choice="auto" if enabled else "none")
            calls = message.get("tool_calls") or []
            content = text_of(message)
            usage = getattr(self.client, "usage", {})
            if not calls and not content.strip() and not message.get("reasoning_content") and not empty_retry and usage.get("completion_tokens", -1) == 0:
                empty_retry = True
                continue
            if calls:
                if not enabled:
                    post_budget += 1
                    if post_budget > 2:
                        raise RuntimeError("model kept calling tools after the tool-round budget was exhausted (2 extra rounds)")
                for call in calls:
                    call["type"] = "function"
                self.record(message)
                evidence, terminal = [], False
                for call in calls:
                    name = resolve_tool(call["function"]["name"], self.definitions, getattr(self.client, "options", {}).get("tool_aliases"))
                    try:
                        arguments = json.loads(call["function"]["arguments"] or "{}")
                        text, image = self.harness.execute(name, arguments)
                    except (ValueError, TypeError, KeyError, OSError) as error:
                        text, image = "TOOL ERROR: " + str(error), None
                    self.tools += 1
                    if name == "submit" and text.startswith("SUBMITTED"):
                        saved = copy.deepcopy(self.harness.state)
                        saved["_accepted_threshold"] = getattr(self.harness, "min_overlap", 0)
                        saved["_accepted_scale_limit"] = getattr(self.harness, "max_scale_dev", 0)
                        self.transcript.event({"t": "state", "state": saved})
                    self.record({"role": "tool", "tool_call_id": call["id"], "content": text})
                    if image:
                        evidence.append((call["id"], name, image))
                    terminal = terminal or (name == "submit" and text.startswith("SUBMITTED"))
                    print(f"round={self.rounds} tool={name}: {text[:160]}", flush=True)
                for ident, name, image in evidence:
                    self.record({"role": "user", "content": [{"type": "text", "text": f"Image returned by {name} (tool_call_id={ident})."}, image_part(image)]})
                tool_rounds += 1
                if terminal:
                    return "submitted"
                continue
            self.record(message)
            if enabled and not self.access_nudged and re.search(r"(?:tools? (?:are |is )?(?:not available|unavailable)|cannot (?:access|use).*tool|工具.*(?:不可用|无法))", content, re.I):
                self.access_nudged = True
                names = ", ".join(d["function"]["name"] for d in self.definitions[:12])
                self.record({"role": "user", "content": "Your tools ARE available in this session: " + names + ". The previous reply claimed they were not — that is incorrect; nothing ran because you did not call them. Call the tools now (for a conversion session start with read_file on work:manual.md, then read the chapter markdown)."})
                continue
            if not content.strip():
                self.record({"role": "user", "content": "Continue."})
                message = self.request(choice="none", max_tokens=tuning["max_tokens"] * (2 if getattr(self.client, "finish", "") == "length" else 1), kind="nudge")
                self.record(message)
                if not text_of(message).strip():
                    raise RuntimeError("empty final answer")
            return "text"
