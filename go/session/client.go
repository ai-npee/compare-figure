package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message = map[string]any

type Completer interface {
	Complete(context.Context, []Message, []map[string]any, string, int) (Message, error)
}
type Toolset interface {
	Definitions() []map[string]any
	Execute(context.Context, string, map[string]any) (string, string, error)
	Snapshot() map[string]any
}

type Client struct {
	BaseURL, Model, Key string
	Options             map[string]any
	Usage               map[string]any
	Finish              string
	Elapsed             time.Duration
	Partial             func(string)
	http                *http.Client
}

func NewClient(baseURL, model, key string, timeout time.Duration, options map[string]any) *Client {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	opts := map[string]any{}
	for k, v := range options {
		opts[k] = v
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Model: model, Key: key, Options: opts, http: &http.Client{Timeout: timeout}}
}
func (c *Client) api() string {
	a, _ := c.Options["api"].(string)
	if a == "" {
		return "openai"
	}
	return a
}
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
func arr(v any) []any { a, _ := v.([]any); return a }
func text(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		out := []string{}
		for _, p := range x {
			if s, ok := asMap(p)["text"].(string); ok {
				out = append(out, s)
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}
func TextOf(v any) string {
	if m, ok := v.(map[string]any); ok {
		return text(m["content"])
	}
	return text(v)
}

func responsesContent(role string, value any) []any {
	kind := "input_text"
	if role == "assistant" {
		kind = "output_text"
	}
	if s, ok := value.(string); ok {
		return []any{map[string]any{"type": kind, "text": s}}
	}
	out := []any{}
	for _, v := range arr(value) {
		p := asMap(v)
		switch p["type"] {
		case "text":
			out = append(out, map[string]any{"type": kind, "text": p["text"]})
		case "image_url":
			out = append(out, map[string]any{"type": "input_image", "image_url": asMap(p["image_url"])["url"]})
		case "input_text", "output_text", "input_image":
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"type": kind, "text": ""})
	}
	return out
}
func anthropicContent(role string, value any) []any {
	if s, ok := value.(string); ok {
		return []any{map[string]any{"type": "text", "text": s}}
	}
	out := []any{}
	for _, v := range arr(value) {
		p := asMap(v)
		switch p["type"] {
		case "text":
			out = append(out, map[string]any{"type": "text", "text": p["text"]})
		case "image_url":
			u := fmt.Sprint(asMap(p["image_url"])["url"])
			head, data, ok := strings.Cut(u, ",")
			if ok && strings.HasPrefix(head, "data:") {
				out = append(out, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": strings.TrimSuffix(strings.TrimPrefix(head, "data:"), ";base64"), "data": data}})
			} else {
				out = append(out, map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": u}})
			}
		}
	}
	if len(out) == 0 && role != "assistant" {
		out = append(out, map[string]any{"type": "text", "text": ""})
	}
	return out
}
func cleanMessages(messages []Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		x := Message{}
		for k, v := range m {
			if !strings.HasPrefix(k, "_") {
				x[k] = v
			}
		}
		out = append(out, x)
	}
	return out
}
func (c *Client) Complete(ctx context.Context, messages []Message, tools []map[string]any, choice string, maxTokens int) (Message, error) {
	api := c.api()
	stream, _ := c.Options["stream"].(bool)
	if maxTokens <= 0 {
		maxTokens = 16384
	}
	body := map[string]any{"model": c.Model, "stream": stream}
	for _, k := range []string{"temperature"} {
		if v, ok := c.Options[k]; ok {
			body[k] = v
		}
	}
	endpoint := "/chat/completions"
	switch api {
	case "openai":
		body["messages"] = cleanMessages(messages)
		body["max_tokens"] = maxTokens
		for _, k := range []string{"thinking", "reasoning_effort", "tool_stream", "user"} {
			if v, ok := c.Options[k]; ok {
				body[k] = v
			}
		}
		if stream {
			body["stream_options"] = map[string]any{"include_usage": true}
		}
		if len(tools) > 0 {
			body["tools"] = tools
			body["tool_choice"] = choice
		}
	case "responses":
		endpoint = "/responses"
		input := []any{}
		for _, m := range messages {
			role := fmt.Sprint(m["role"])
			if role == "system" {
				body["instructions"] = text(m["content"])
				continue
			}
			if role == "assistant" && len(arr(m["_response_items"])) > 0 {
				input = append(input, arr(m["_response_items"])...)
				continue
			}
			if role == "tool" {
				input = append(input, map[string]any{"type": "function_call_output", "call_id": m["tool_call_id"], "output": text(m["content"])})
				continue
			}
			if role == "assistant" {
				for _, v := range arr(m["tool_calls"]) {
					tc := asMap(v)
					fn := asMap(tc["function"])
					input = append(input, map[string]any{"type": "function_call", "call_id": tc["id"], "name": fn["name"], "arguments": fn["arguments"]})
				}
			}
			input = append(input, map[string]any{"role": role, "content": responsesContent(role, m["content"])})
		}
		body["input"] = input
		body["max_output_tokens"] = maxTokens
		body["store"] = false
		body["include"] = []string{"reasoning.encrypted_content"}
		if v, ok := c.Options["thinking"]; ok {
			body["reasoning"] = v
		}
		if effort, ok := c.Options["reasoning_effort"]; ok {
			body["reasoning"] = map[string]any{"effort": effort}
		}
		if len(tools) > 0 {
			ts := []any{}
			for _, d := range tools {
				fn := asMap(d["function"])
				// Keep optional parameters optional unless the tool explicitly opts into strict mode.
				strict, _ := fn["strict"].(bool)
				ts = append(ts, map[string]any{"type": "function", "name": fn["name"], "description": fn["description"], "parameters": fn["parameters"], "strict": strict})
			}
			body["tools"] = ts
			body["tool_choice"] = choice
		}
	case "anthropic":
		endpoint = "/messages"
		wire := []any{}
		system := []string{}
		for _, m := range messages {
			role := fmt.Sprint(m["role"])
			if role == "system" {
				system = append(system, text(m["content"]))
				continue
			}
			orig := role
			if role == "tool" {
				role = "user"
			}
			var content any
			if orig == "tool" {
				content = []any{map[string]any{"type": "tool_result", "tool_use_id": m["tool_call_id"], "content": text(m["content"])}}
			} else if orig == "assistant" && len(arr(m["_anthropic_blocks"])) > 0 {
				content = arr(m["_anthropic_blocks"])
			} else {
				content = anthropicContent(orig, m["content"])
				if orig == "assistant" {
					blocks := arr(content)
					for _, v := range arr(m["tool_calls"]) {
						tc := asMap(v)
						fn := asMap(tc["function"])
						var args any
						if json.Unmarshal([]byte(fmt.Sprint(fn["arguments"])), &args) != nil {
							args = map[string]any{}
						}
						blocks = append(blocks, map[string]any{"type": "tool_use", "id": tc["id"], "name": fn["name"], "input": args})
					}
					content = blocks
				}
			}
			if len(wire) > 0 && asMap(wire[len(wire)-1])["role"] == role {
				prev := asMap(wire[len(wire)-1])
				prev["content"] = append(arr(prev["content"]), arr(content)...)
			} else {
				wire = append(wire, map[string]any{"role": role, "content": content})
			}
		}
		body["messages"] = wire
		body["system"] = strings.Join(system, "\n")
		body["max_tokens"] = maxTokens
		if len(tools) > 0 {
			ts := []any{}
			for _, d := range tools {
				fn := asMap(d["function"])
				ts = append(ts, map[string]any{"name": fn["name"], "description": fn["description"], "input_schema": fn["parameters"]})
			}
			body["tools"] = ts
			if choice == "none" {
				body["tool_choice"] = map[string]any{"type": "none"}
			}
		}
		if v, ok := c.Options["thinking"]; ok {
			body["thinking"] = v
		}
		if v, ok := c.Options["tool_stream"]; ok {
			body["tool_stream"] = v
		}
	default:
		return nil, fmt.Errorf("unknown api protocol %q", api)
	}
	for k, v := range asMap(c.Options["request_body"]) {
		body[k] = v
	}
	started := time.Now()
	maxAttempts := int(num(c.Options["api_max_retries"], 3))
	fallbackDone := false
	for attempt := 0; maxAttempts <= 0 || attempt < maxAttempts; attempt++ {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if api == "anthropic" {
			req.Header.Set("x-api-key", c.Key)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+c.Key)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if maxAttempts <= 0 || attempt < maxAttempts-1 {
				if e := waitRetry(ctx, attempt); e != nil {
					return nil, e
				}
				continue
			}
			return nil, err
		}
		ct := resp.Header.Get("Content-Type")
		var data []byte
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			data, err = io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		} else {
			data, err = io.ReadAll(resp.Body)
		}
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			fallbackHint := strings.ToLower(string(data))
			streamHint := strings.Contains(fallbackHint, "stream") || strings.Contains(fallbackHint, "unsupported") || strings.Contains(fallbackHint, "not support")
			if stream && !fallbackDone && (streamHint || resp.StatusCode == 404 || resp.StatusCode == 405 || resp.StatusCode == 415 || resp.StatusCode == 422) {
				fallbackDone = true
				body["stream"] = false
				delete(body, "stream_options")
				attempt--
				continue
			}
			if transientStatus(resp.StatusCode) && (maxAttempts <= 0 || attempt < maxAttempts-1) {
				if e := waitRetry(ctx, attempt); e != nil {
					return nil, e
				}
				continue
			}
			return nil, fmt.Errorf("Model API HTTP %d; check configured protocol/endpoint/model/credentials", resp.StatusCode)
		}
		c.Elapsed = time.Since(started)
		if strings.Contains(ct, "text/event-stream") {
			switch api {
			case "openai":
				data, err = c.parseChatSSE(data)
			case "responses":
				data, err = parseResponsesSSE(data)
			case "anthropic":
				data, err = parseAnthropicSSE(data)
			}
			if err != nil {
				if stream && !fallbackDone && strings.Contains(strings.ToLower(err.Error()), "stream") {
					fallbackDone = true
					body["stream"] = false
					delete(body, "stream_options")
					attempt--
					continue
				}
				return nil, err
			}
		}
		return c.decode(api, data)
	}
	return nil, errors.New("stream fallback exhausted")
}
func (c *Client) decode(api string, b []byte) (Message, error) {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	c.Usage = asMap(v["usage"])
	switch api {
	case "openai":
		cs := arr(v["choices"])
		if len(cs) == 0 {
			return nil, errors.New("empty choices")
		}
		ch := asMap(cs[0])
		c.Finish = fmt.Sprint(ch["finish_reason"])
		m := asMap(ch["message"])
		m["role"] = "assistant"
		return m, nil
	case "responses":
		c.Finish = fmt.Sprint(v["status"])
		items := arr(v["output"])
		parts := []string{}
		calls := []any{}
		for _, it := range items {
			x := asMap(it)
			switch x["type"] {
			case "message":
				for _, pv := range arr(x["content"]) {
					p := asMap(pv)
					if p["type"] == "output_text" {
						parts = append(parts, fmt.Sprint(p["text"]))
					}
				}
			case "function_call":
				calls = append(calls, map[string]any{"id": x["call_id"], "type": "function", "function": map[string]any{"name": x["name"], "arguments": x["arguments"]}})
			}
		}
		m := Message{"role": "assistant", "content": strings.Join(parts, "\n"), "_response_items": items}
		if len(calls) > 0 {
			m["tool_calls"] = calls
		}
		return m, nil
	case "anthropic":
		c.Finish = fmt.Sprint(v["stop_reason"])
		blocks := arr(v["content"])
		parts := []string{}
		calls := []any{}
		for _, bv := range blocks {
			x := asMap(bv)
			if x["type"] == "text" {
				parts = append(parts, fmt.Sprint(x["text"]))
			}
			if x["type"] == "tool_use" {
				a, _ := json.Marshal(x["input"])
				calls = append(calls, map[string]any{"id": x["id"], "type": "function", "function": map[string]any{"name": x["name"], "arguments": string(a)}})
			}
		}
		m := Message{"role": "assistant", "content": strings.Join(parts, "\n"), "_anthropic_blocks": blocks}
		if len(calls) > 0 {
			m["tool_calls"] = calls
		}
		return m, nil
	}
	return nil, errors.New("invalid protocol")
}
func transientStatus(code int) bool {
	switch code {
	case 408, 429, 500, 502, 503, 504, 529:
		return true
	}
	return false
}
func retryDelay(attempt int) time.Duration {
	return time.Duration(min(1<<min(attempt+1, 5), 30)) * time.Second
}
func waitRetry(ctx context.Context, attempt int) error {
	delay := retryDelay(attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func float(v any) float64 { n, _ := v.(float64); return n }
func (c *Client) parseChatSSE(b []byte) ([]byte, error) {
	m := Message{"role": "assistant", "content": ""}
	calls := map[int]map[string]any{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 64<<20)
	for sc.Scan() {
		s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "data:"))
		if s == "[DONE]" {
			break
		}
		if !strings.HasPrefix(strings.TrimSpace(sc.Text()), "data:") {
			continue
		}
		var e map[string]any
		if json.Unmarshal([]byte(s), &e) != nil {
			continue
		}
		if e["usage"] != nil {
			c.Usage = asMap(e["usage"])
		}
		for _, cv := range arr(e["choices"]) {
			ch := asMap(cv)
			if ch["finish_reason"] != nil {
				c.Finish = fmt.Sprint(ch["finish_reason"])
			}
			d := asMap(ch["delta"])
			if p, ok := d["content"].(string); ok {
				m["content"] = fmt.Sprint(m["content"]) + p
			}
			if p, ok := d["reasoning_content"].(string); ok {
				m["reasoning_content"] = fmt.Sprint(m["reasoning_content"]) + p
			}
			for _, tv := range arr(d["tool_calls"]) {
				t := asMap(tv)
				i := int(float(t["index"]))
				out := calls[i]
				if out == nil {
					out = map[string]any{"id": "", "type": "function", "function": map[string]any{"name": "", "arguments": ""}}
					calls[i] = out
				}
				if id, ok := t["id"].(string); ok && id != "" {
					out["id"] = id
				}
				fn := asMap(out["function"])
				df := asMap(t["function"])
				for _, k := range []string{"name", "arguments"} {
					if fragment, ok := df[k].(string); ok {
						fn[k] = fmt.Sprint(fn[k]) + fragment
					}
				}
			}
		}
	}
	cs := []any{}
	for i := 0; i < len(calls); i++ {
		if calls[i] != nil {
			cs = append(cs, calls[i])
		}
	}
	if len(cs) > 0 {
		m["tool_calls"] = cs
	}
	return json.Marshal(map[string]any{"choices": []any{map[string]any{"message": m, "finish_reason": c.Finish}}, "usage": c.Usage})
}
func parseResponsesSSE(b []byte) ([]byte, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var e map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &e) != nil {
			continue
		}
		switch e["type"] {
		case "response.completed", "response.incomplete":
			return json.Marshal(e["response"])
		case "response.failed":
			return nil, errors.New("responses stream failed")
		}
	}
	return nil, errors.New("responses stream ended before completion")
}
func parseAnthropicSSE(b []byte) ([]byte, error) {
	blocks := map[int]map[string]any{}
	usage := map[string]any{}
	finish := ""
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var e map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &e) != nil {
			continue
		}
		switch e["type"] {
		case "message_start":
			usage = asMap(asMap(e["message"])["usage"])
		case "content_block_start":
			blocks[int(float(e["index"]))] = asMap(asMap(e["content_block"]))
		case "content_block_delta":
			idx := int(float(e["index"]))
			d := asMap(e["delta"])
			block := blocks[idx]
			key, src := "", ""
			switch d["type"] {
			case "text_delta":
				key, src = "text", "text"
			case "thinking_delta":
				key, src = "thinking", "thinking"
			case "signature_delta":
				key, src = "signature", "signature"
			case "input_json_delta":
				key, src = "_json", "partial_json"
			}
			if fragment, ok := d[src].(string); key != "" && ok {
				prior, _ := block[key].(string)
				block[key] = prior + fragment
			}
		case "content_block_stop":
			block := blocks[int(float(e["index"]))]
			if raw, ok := block["_json"].(string); ok {
				var v any
				if json.Unmarshal([]byte(raw), &v) == nil {
					block["input"] = v
				}
				delete(block, "_json")
			}
		case "message_delta":
			finish = fmt.Sprint(asMap(e["delta"])["stop_reason"])
		case "message_stop":
			out := []any{}
			for i := 0; i < len(blocks); i++ {
				if blocks[i] != nil {
					out = append(out, blocks[i])
				}
			}
			return json.Marshal(map[string]any{"content": out, "usage": usage, "stop_reason": finish})
		case "error":
			return nil, errors.New("anthropic stream failed")
		}
	}
	return nil, errors.New("anthropic stream ended before completion")
}
