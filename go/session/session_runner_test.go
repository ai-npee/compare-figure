package session

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type scriptedCompleter struct {
	replies []Message
	seen    [][]Message
	choices []string
}

func (c *scriptedCompleter) Complete(_ context.Context, messages []Message, _ []map[string]any, choice string, _ int) (Message, error) {
	snap := append([]Message(nil), messages...)
	c.seen = append(c.seen, snap)
	c.choices = append(c.choices, choice)
	m := c.replies[0]
	c.replies = c.replies[1:]
	return m, nil
}

type testTools struct {
	calls []string
	state map[string]any
}

func (t *testTools) Definitions() []map[string]any {
	return []map[string]any{{"type": "function", "function": map[string]any{"name": "write_file"}}, {"type": "function", "function": map[string]any{"name": "submit"}}}
}
func (t *testTools) Execute(_ context.Context, name string, args map[string]any) (string, string, error) {
	t.calls = append(t.calls, name)
	if name == "submit" {
		return "SUBMITTED accepted", "", nil
	}
	return "ok", "", nil
}
func (t *testTools) Snapshot() map[string]any { return t.state }
func tc(id, name string, args any) any {
	b, _ := json.Marshal(args)
	return map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(b)}}
}
func TestToolBatchAndAcceptedStatePersistence(t *testing.T) {
	dir := t.TempDir()
	tr, e := NewTranscript(filepath.Join(dir, "s.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	tools := &testTools{state: map[string]any{"submitted": true, "_accept_threshold": .17, "_scale_limit": .1}}
	model := &scriptedCompleter{replies: []Message{{"role": "assistant", "content": "", "tool_calls": []any{tc("a", "write_file", map[string]any{"path": "a"}), tc("b", "submit", map[string]any{"code": "ok"})}}, {"role": "assistant", "content": "done"}}}
	s := NewSession(tools, model, tr, "system", map[string]any{"max_tool_rounds": 4}, nil)
	result, e := s.Run(context.Background(), "go", nil)
	if e != nil || result != "submitted" {
		t.Fatalf("result=%s err=%v", result, e)
	}
	if len(tools.calls) != 2 || tools.calls[0] != "write_file" || tools.calls[1] != "submit" {
		t.Fatalf("tool order: %#v", tools.calls)
	}
	raw, e := os.ReadFile(tr.path)
	if e != nil {
		t.Fatal(e)
	}
	if !contains(string(raw), `"_accepted_threshold":0.17`) || !contains(string(raw), `"_accepted_scale_limit":0.1`) {
		t.Fatalf("accepted thresholds missing: %s", raw)
	}
	_, stats, e := tr.Load("system")
	if e != nil {
		t.Fatal(e)
	}
	if stats["accepted"] != true || stats["submit_args"] != `{"code":"ok"}` || stats["submit_attempts"] != 1 {
		t.Fatalf("resume stats %#v", stats)
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
func TestImagePartWidthZeroAndResize(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "image.png")
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	img := image.NewRGBA(image.Rect(0, 0, 300, 120))
	if e = png.Encode(f, img); e != nil {
		t.Fatal(e)
	}
	f.Close()
	raw, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	native, e := ImagePart(p, 0)
	if e != nil {
		t.Fatal(e)
	}
	scaled, e := ImagePart(p, 128)
	if e != nil {
		t.Fatal(e)
	}
	unpack := func(part map[string]any) []byte {
		u := asMap(part["image_url"])["url"].(string)
		_, payload, _ := strings.Cut(u, ",")
		b, _ := base64.StdEncoding.DecodeString(payload)
		return b
	}
	if string(unpack(native)) != string(raw) {
		t.Fatal("width=0 changed image bytes")
	}
	decoded, _, e := image.Decode(bytes.NewReader(unpack(scaled)))
	if e != nil {
		t.Fatal(e)
	}
	if max(decoded.Bounds().Dx(), decoded.Bounds().Dy()) != 128 {
		t.Fatalf("scaled dimensions %v", decoded.Bounds())
	}
}
