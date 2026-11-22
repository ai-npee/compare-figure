package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

const SubmitReminder = `You have NOT called submit yet. Do these in order: (1) compile {path: "figure.tex"} until it succeeds, (2) call compare_figure and look at its comparison, (3) call submit with {path: "figure.tex"} (the file must match the last successful compile). Submitting before compare_figure succeeds is rejected.`
const CheckFeedback = `A vision check compared your submitted figure with the ORIGINAL image and found visible differences:
{problems}

Fix exactly these points in figure.tex (do not change anything else), call compile {path: "figure.tex"}, compare your render with the original once more, and call submit again. Sizes, positions and labels in the original are the reference.`

type Session struct {
	toolset                     Toolset
	Client                      Completer
	Transcript                  *Transcript
	System                      string
	Tuning                      map[string]any
	Messages                    []Message
	Rounds, Tools               int
	Compactions                 int
	writeErr                    error
	calibration                 float64
	measuredPrompt, lastCompact int
	accessNudged                bool
}

func NewSession(tools Toolset, client Completer, tr *Transcript, system string, tuning map[string]any, messages []Message) *Session {
	defaults := map[string]any{"max_tool_rounds": 128, "max_tokens": 16384, "context_limit": 0, "compaction_at": .85, "tool_rounds_warn_ratio": .7, "tool_rounds_grace": 20, "prune_tool_chars": 4096, "keep_images": 3}
	for k, v := range tuning {
		defaults[k] = v
	}
	if messages == nil {
		messages = []Message{{"role": "system", "content": system}}
	}
	s := &Session{toolset: tools, Client: client, Transcript: tr, System: system, Tuning: defaults, Messages: messages, calibration: 1}
	if tr != nil {
		defs := []map[string]any{}
		if tools != nil {
			defs = tools.Definitions()
		}
		if err := tr.Event(map[string]any{"t": "meta", "kind": "system", "system": system, "tools": defs}); err != nil {
			s.writeErr = err
		}
	}
	return s
}
func num(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return def
}
func (s *Session) record(m Message) {
	s.Messages = append(s.Messages, m)
	if s.Transcript != nil {
		if err := s.Transcript.Message(m); err != nil && s.writeErr == nil {
			s.writeErr = err
		}
	}
}
func (s *Session) Record(m Message) error {
	s.record(m)
	return s.writeErr
}
func (s *Session) estimated() int {
	defs := []map[string]any{}
	if s.toolset != nil {
		defs = s.toolset.Definitions()
	}
	b, _ := json.Marshal(defs)
	n := len(b)
	for _, m := range s.Messages {
		n += len(TextOf(m))
		if calls, ok := m["tool_calls"]; ok {
			b, _ := json.Marshal(calls)
			n += len(b)
		}
		for _, p := range arr(m["content"]) {
			if asMap(p)["type"] == "image_url" {
				n += 1024
			}
		}
	}
	return int(float64(max(1, n/3)) * s.calibration)
}
func (s *Session) request(ctx context.Context, choice string, maxTokens int, kind string) (Message, error) {
	est := s.estimated()
	if l := int(num(s.Tuning["context_limit"], 0)); l > 0 && maxTokens > l-est-256 {
		maxTokens = max(1, l-est-256)
	}
	defs := []map[string]any{}
	if s.toolset != nil {
		defs = s.toolset.Definitions()
	}
	m, e := s.Client.Complete(ctx, s.Messages, defs, choice, maxTokens)
	if e != nil {
		return nil, e
	}
	if m["role"] != "assistant" {
		return nil, errors.New("model API returned a non-assistant message")
	}
	s.Rounds++
	if c, ok := s.Client.(*Client); ok {
		u := c.Usage
		p := num(u["prompt_tokens"], num(u["input_tokens"], 0))
		if p > 0 && kind != "compact" {
			s.calibration = math.Max(1, p/math.Max(1, float64(est)))
			s.measuredPrompt = int(p)
		}
		if s.Transcript != nil {
			if err := s.Transcript.Event(map[string]any{"t": "usage", "round": s.Rounds, "kind": kind, "model": c.Model, "prompt_tokens": int(p), "completion_tokens": int(num(u["completion_tokens"], num(u["output_tokens"], 0))), "finish_reason": c.Finish, "duration_ms": c.Elapsed.Milliseconds()}); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}
func (s *Session) pruneImages(keep int) {
	if keep <= 0 {
		return
	}
	total := 0
	for _, m := range s.Messages {
		for _, p := range arr(m["content"]) {
			if asMap(p)["type"] == "image_url" {
				total++
			}
		}
	}
	remove := total - keep
	for _, m := range s.Messages {
		if remove <= 0 {
			break
		}
		parts, ok := m["content"].([]any)
		if !ok {
			continue
		}
		out := []any{}
		for _, v := range parts {
			p := asMap(v)
			if p["type"] == "image_url" && remove > 0 {
				out = append(out, map[string]any{"type": "text", "text": "[Earlier image omitted from context; use view_image/view_pdf to inspect it again.]"})
				remove--
			} else {
				out = append(out, v)
			}
		}
		m["content"] = out
	}
}
func (s *Session) compact(ctx context.Context) error {
	limit := int(num(s.Tuning["context_limit"], 0))
	now := max(s.estimated(), s.measuredPrompt)
	if limit <= 0 || float64(now) < float64(limit)*num(s.Tuning["compaction_at"], .85) || s.lastCompact > 0 && now < s.lastCompact*12/10 {
		return nil
	}
	if len(s.Messages) <= 2 {
		return nil
	}
	tail := min(len(s.Messages), max(2, len(s.Messages)-8))
	for tail > 2 && s.Messages[tail]["role"] == "tool" {
		tail--
	}
	chars := int(num(s.Tuning["prune_tool_chars"], 4096))
	for _, m := range s.Messages[2:tail] {
		if m["role"] == "tool" && chars > 0 {
			textRunes := []rune(TextOf(m))
			if len(textRunes) > chars {
				m["content"] = string(textRunes[:chars/2]) + "\n[tool result pruned]\n" + string(textRunes[len(textRunes)-chars/4:])
			}
		}
	}
	s.pruneImages(int(num(s.Tuning["keep_images"], 3)))
	if s.estimated() < int(float64(limit)*num(s.Tuning["compaction_at"], .85)) || tail <= 2 {
		return nil
	}
	instruction := fmt.Sprintf("=== CONTEXT COMPACTION REQUEST ===\nSummarise all work before the last %d messages as a compact but complete continuation note: preserve task, decisions, exact paths/ids/numbers, progress and remaining steps; omit verbose intermediate dumps. Respond only with the note.", len(s.Messages)-tail)
	tmp := append(append([]Message{}, s.Messages...), Message{"role": "user", "content": instruction})
	saved := s.Messages
	s.Messages = tmp
	m, e := s.request(ctx, "none", 8192, "compact")
	s.Messages = saved
	if e != nil {
		return e
	}
	summary := TextOf(m)
	if strings.TrimSpace(summary) == "" {
		return errors.New("empty compaction summary")
	}
	note := "=== COMPRESSED SESSION CONTEXT ===\n<compacted-summary>\nAutomatically generated continuation checkpoint. Treat this context as established; continue without acknowledging it.\n<summary>\n" + summary + "\n</summary>\n</compacted-summary>"
	s.Messages = append(append(append([]Message{}, saved[:2]...), Message{"role": "user", "content": note}), saved[tail:]...)
	if s.Transcript != nil {
		if err := s.Transcript.Checkpoint(s.Messages[1:]); err != nil {
			return err
		}
	}
	s.Compactions++
	s.measuredPrompt = 0
	s.lastCompact = s.estimated()
	return nil
}

var aliasNormalize = regexp.MustCompile(`[_\-\s.]`)

func resolveTool(name string, defs []map[string]any, aliases map[string]any) string {
	names := []string{}
	for _, d := range defs {
		names = append(names, fmt.Sprint(asMap(d["function"])["name"]))
	}
	candidates := []string{name}
	for _, sep := range []string{"__", "/", "."} {
		if i := strings.LastIndex(name, sep); i >= 0 {
			candidates = append(candidates, strings.TrimSpace(name[i+len(sep):]))
		}
	}
	norm := func(s string) string { return strings.ToLower(aliasNormalize.ReplaceAllString(s, "")) }
	for _, c := range candidates {
		for _, n := range names {
			if c == n {
				return n
			}
		}
	}
	for _, c := range candidates {
		for a, v := range aliases {
			if norm(c) == norm(a) {
				for _, n := range names {
					if norm(n) == norm(fmt.Sprint(v)) {
						return n
					}
				}
			}
		}
	}
	for _, c := range candidates {
		hit := ""
		for _, n := range names {
			if norm(n) == norm(c) {
				if hit != "" {
					hit = ""
					break
				}
				hit = n
			}
		}
		if hit != "" {
			return hit
		}
	}
	return name
}
func (s *Session) Run(ctx context.Context, userText string, images []string) (string, error) {
	if userText != "" || len(images) > 0 {
		parts := []any{}
		if userText != "" {
			parts = append(parts, map[string]any{"type": "text", "text": userText})
		}
		for _, p := range images {
			im, e := ImagePart(p, 0)
			if e != nil {
				return "", e
			}
			parts = append(parts, im)
		}
		if len(images) > 0 {
			s.record(Message{"role": "user", "content": parts})
		} else {
			s.record(Message{"role": "user", "content": userText})
		}
	}
	maxRounds := int(num(s.Tuning["max_tool_rounds"], 128))
	grace := int(num(s.Tuning["tool_rounds_grace"], 20))
	if grace == 0 {
		grace = 20
	}
	hard := maxRounds + grace
	toolRounds, post, reminder := 0, 0, -1
	emptyRetry := false
	for {
		if s.writeErr != nil {
			return "", s.writeErr
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if c, ok := s.Client.(*Client); ok {
			if mx := int(num(c.Options["max_images"], 0)); mx > 0 {
				s.pruneImages(mx)
			}
		}
		if e := s.compact(ctx); e != nil {
			return "", e
		}
		enabled := maxRounds <= 0 || toolRounds < hard
		if maxRounds > 0 && float64(toolRounds) >= float64(maxRounds)*num(s.Tuning["tool_rounds_warn_ratio"], .7) {
			msg := ""
			if toolRounds < maxRounds {
				msg = fmt.Sprintf("ROUND BUDGET: %d of %d tool rounds used, %d left. Use them sparingly, finish and submit your best result.", toolRounds, maxRounds, maxRounds-toolRounds)
			} else if toolRounds < hard {
				msg = fmt.Sprintf("ROUND BUDGET EXCEEDED: %d extra rounds remain; wrap up and submit your best result.", hard-toolRounds)
			}
			if msg != "" {
				u := Message{"role": "user", "content": msg}
				if reminder >= 0 {
					s.Messages[reminder] = u
				} else {
					reminder = len(s.Messages)
					s.record(u)
				}
			}
		}
		choice := "auto"
		if !enabled {
			choice = "none"
		}
		m, e := s.request(ctx, choice, int(num(s.Tuning["max_tokens"], 16384)), "")
		if e != nil {
			return "", e
		}
		calls := arr(m["tool_calls"])
		content := TextOf(m)
		c, ok := s.Client.(*Client)
		reasoning, _ := m["reasoning_content"].(string)
		if len(calls) == 0 && strings.TrimSpace(content) == "" && strings.TrimSpace(reasoning) == "" && !emptyRetry && ok && num(c.Usage["completion_tokens"], -1) == 0 {
			emptyRetry = true
			continue
		}
		if len(calls) > 0 {
			if !enabled {
				post++
				if post > 2 {
					return "", errors.New("model kept calling tools after tool-round budget exhausted")
				}
			}
			s.record(m)
			if s.writeErr != nil {
				return "", s.writeErr
			}
			defs := []map[string]any{}
			if s.toolset != nil {
				defs = s.toolset.Definitions()
			}
			evidence := []struct{ id, name, path string }{}
			terminal := false
			for _, v := range calls {
				call := asMap(v)
				fn := asMap(call["function"])
				name := fmt.Sprint(fn["name"])
				aliases := map[string]any{}
				if c, ok := s.Client.(*Client); ok {
					aliases = asMap(c.Options["tool_aliases"])
				}
				name = resolveTool(name, defs, aliases)
				args := map[string]any{}
				argstr := fmt.Sprint(fn["arguments"])
				var argErr error
				if argstr != "" {
					argErr = json.Unmarshal([]byte(argstr), &args)
				}
				result, image := "", ""
				var e error
				if argErr != nil {
					result = "TOOL ERROR: invalid tool arguments JSON: " + argErr.Error()
				} else if s.toolset == nil {
					result = "TOOL ERROR: no toolset is configured"
				} else {
					result, image, e = s.toolset.Execute(ctx, name, args)
				}
				if e != nil {
					result = "TOOL ERROR: " + e.Error()
				}
				s.Tools++
				id := fmt.Sprint(call["id"])
				accepted := name == "submit" && strings.HasPrefix(result, "SUBMITTED")
				if accepted && s.Transcript != nil {
					state := s.toolset.Snapshot()
					if state == nil {
						state = map[string]any{}
					}
					threshold := state["_accept_threshold"]
					if threshold == nil {
						threshold = state["min_overlap"]
					}
					scaleLimit := state["_scale_limit"]
					if scaleLimit == nil {
						scaleLimit = state["max_scale_dev"]
					}
					state["_accepted_threshold"] = threshold
					state["_accepted_scale_limit"] = scaleLimit
					if err := s.Transcript.Event(map[string]any{"t": "state", "state": state}); err != nil {
						return "", err
					}
				}
				s.record(Message{"role": "tool", "tool_call_id": id, "content": result})
				if s.writeErr != nil {
					return "", s.writeErr
				}
				if image != "" {
					evidence = append(evidence, struct{ id, name, path string }{id, name, image})
				}
				terminal = terminal || accepted
			}
			for _, ev := range evidence {
				im, e := ImagePart(ev.path, 0)
				if e != nil {
					continue
				}
				s.record(Message{"role": "user", "content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("Image returned by %s (tool_call_id=%s).", ev.name, ev.id)}, im}})
			}
			toolRounds++
			if terminal {
				if s.writeErr != nil {
					return "", s.writeErr
				}
				return "submitted", nil
			}
			continue
		}
		s.record(m)
		if s.writeErr != nil {
			return "", s.writeErr
		}
		if enabled && !s.accessNudged && regexp.MustCompile(`(?i)(tools? (are |is )?(not available|unavailable)|cannot (access|use).*tool|工具.*(不可用|无法))`).MatchString(content) {
			s.accessNudged = true
			s.record(Message{"role": "user", "content": "Your tools ARE available in this session. The previous reply was incorrect; call the tools now."})
			continue
		}
		if strings.TrimSpace(content) == "" {
			s.record(Message{"role": "user", "content": "Continue."})
			budget := int(num(s.Tuning["max_tokens"], 16384))
			if client, ok := s.Client.(*Client); ok && client.Finish == "length" {
				budget *= 2
			}
			m, e = s.request(ctx, "none", budget, "nudge")
			if e != nil {
				return "", e
			}
			s.record(m)
			if s.writeErr != nil {
				return "", s.writeErr
			}
			if strings.TrimSpace(TextOf(m)) == "" {
				return "", errors.New("empty final answer")
			}
		}
		return "text", nil
	}
}
