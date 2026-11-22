package session

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type Transcript struct{ path, dir string }

func NewTranscript(path string) (*Transcript, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	d := filepath.Dir(p)
	if e = os.MkdirAll(filepath.Join(d, "media"), 0755); e != nil {
		return nil, e
	}
	return &Transcript{p, d}, nil
}
func (t *Transcript) Event(m map[string]any) error {
	if t == nil {
		return nil
	}
	m["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	f, e := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(m); e != nil {
		return e
	}
	return f.Sync()
}
func (t *Transcript) Message(m Message) error {
	if t == nil {
		return nil
	}
	ev := map[string]any{"t": "msg", "role": m["role"], "text": TextOf(m)}
	for _, k := range []string{"tool_calls", "tool_call_id", "reasoning_content", "_response_items", "_anthropic_blocks"} {
		if v, ok := m[k]; ok {
			ev[k] = v
		}
	}
	if parts, ok := m["content"].([]any); ok {
		images := []string{}
		for _, p := range parts {
			x := asMap(p)
			if x["type"] != "image_url" {
				continue
			}
			u := asMap(x["image_url"])
			raw, _ := u["url"].(string)
			head, data, ok := strings.Cut(raw, ",")
			if !ok || !strings.HasPrefix(head, "data:") {
				continue
			}
			b, e := base64.StdEncoding.DecodeString(data)
			if e != nil {
				continue
			}
			mt := strings.TrimSuffix(strings.TrimPrefix(head, "data:"), ";base64")
			ext, _ := mime.ExtensionsByType(mt)
			suffix := ".png"
			if len(ext) > 0 {
				suffix = ext[0]
			}
			sum := sha256.Sum256(b)
			name := hex.EncodeToString(sum[:12]) + suffix
			if e := os.WriteFile(filepath.Join(t.dir, "media", name), b, 0644); e != nil {
				return e
			}
			images = append(images, "file://media/"+name)
		}
		if len(images) > 0 {
			ev["images"] = images
		}
	}
	raw, _ := json.Marshal(ev)
	sum := sha256.Sum256(raw)
	ev["h"] = hex.EncodeToString(sum[:6])
	return t.Event(ev)
}
func (t *Transcript) Load(system string) ([]Message, map[string]any, error) {
	msgs := []Message{{"role": "system", "content": system}}
	stats := map[string]any{"rounds": 0, "tools": 0, "submit_attempts": 0, "accepted": false}
	f, e := os.Open(t.path)
	if os.IsNotExist(e) {
		return msgs, stats, nil
	}
	if e != nil {
		return nil, nil, e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 64<<20)
	var state any
	for n := 1; scan.Scan(); n++ {
		var ev map[string]any
		if e = json.Unmarshal(scan.Bytes(), &ev); e != nil {
			return nil, nil, fmt.Errorf("invalid transcript line %d: %w", n, e)
		}
		if ev["type"] == "message" {
			m := asMap(ev["message"])
			if m["role"] != "system" {
				t.restoreImages(m)
				msgs = append(msgs, m)
			}
			continue
		}
		switch ev["t"] {
		case "checkpoint":
			msgs = []Message{{"role": "system", "content": system}}
			state = nil
			for _, v := range arr(ev["messages"]) {
				m := asMap(v)
				t.restoreImages(m)
				msgs = append(msgs, m)
			}
		case "state":
			state = ev["state"]
		case "msg":
			m := Message{"role": ev["role"], "content": ev["text"]}
			for _, k := range []string{"tool_calls", "tool_call_id", "reasoning_content", "_response_items", "_anthropic_blocks"} {
				if v, ok := ev[k]; ok {
					m[k] = v
				}
			}
			images := arr(ev["images"])
			if len(images) > 0 {
				parts := []any{}
				if TextOf(m) != "" {
					parts = append(parts, map[string]any{"type": "text", "text": TextOf(m)})
				}
				for _, v := range images {
					ref, _ := v.(string)
					p, e := t.mediaPath(ref)
					if e != nil {
						return nil, nil, e
					}
					part, e := ImagePart(p, 0)
					if e != nil {
						return nil, nil, e
					}
					parts = append(parts, part)
				}
				m["content"] = parts
			}
			msgs = append(msgs, m)
		}
	}
	if e = scan.Err(); e != nil {
		return nil, nil, e
	}
	pending := map[string]bool{}
	submit := map[string]string{}
	accepted := false
	for _, m := range msgs {
		if m["role"] == "assistant" {
			stats["rounds"] = int(stats["rounds"].(int)) + 1
			for _, v := range arr(m["tool_calls"]) {
				c := asMap(v)
				id := fmt.Sprint(c["id"])
				pending[id] = true
				if asMap(c["function"])["name"] == "submit" {
					submit[id] = fmt.Sprint(asMap(c["function"])["arguments"])
					stats["submit_attempts"] = int(stats["submit_attempts"].(int)) + 1
				}
			}
		} else if m["role"] == "tool" {
			stats["tools"] = int(stats["tools"].(int)) + 1
			id := fmt.Sprint(m["tool_call_id"])
			delete(pending, id)
			if args, ok := submit[id]; ok {
				stats["submit_args"] = args
				accepted = strings.HasPrefix(TextOf(m), "SUBMITTED")
			}
		}
	}
	stats["accepted"] = accepted
	if accepted {
		stats["submitted_state"] = state
	}
	for id := range pending {
		m := Message{"role": "tool", "tool_call_id": id, "content": "TOOL INTERRUPTED: no result was recorded before the previous process stopped. Check the workspace before retrying."}
		if err := t.Message(m); err != nil {
			return nil, nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, stats, nil
}
func (t *Transcript) mediaPath(ref string) (string, error) {
	if !strings.HasPrefix(ref, "file://media/") {
		return "", fmt.Errorf("invalid transcript media reference %q", ref)
	}
	p, e := filepath.Abs(filepath.Join(t.dir, strings.TrimPrefix(ref, "file://")))
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(t.dir, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("transcript media escapes session directory")
	}
	return p, nil
}
func (t *Transcript) restoreImages(m Message) {
	parts, ok := m["content"].([]any)
	if !ok {
		return
	}
	for _, v := range parts {
		x := asMap(v)
		if x["type"] != "image_url" {
			continue
		}
		im := asMap(x["image_url"])
		s, _ := im["url"].(string)
		u, e := url.Parse(s)
		if e == nil && u.Scheme == "file" {
			if p, e := t.mediaPath(s); e == nil {
				if b, e := os.ReadFile(p); e == nil {
					mt := mime.TypeByExtension(filepath.Ext(p))
					if mt == "" {
						mt = "image/png"
					}
					im["url"] = "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b)
				}
			}
		}
	}
}
func (t *Transcript) Checkpoint(ms []Message) error {
	out := make([]Message, len(ms))
	copy(out, ms)
	for _, m := range out {
		parts, ok := m["content"].([]any)
		if !ok {
			continue
		}
		for _, v := range parts {
			x := asMap(v)
			if x["type"] != "image_url" {
				continue
			}
			im := asMap(x["image_url"])
			s, _ := im["url"].(string)
			h, b64, ok := strings.Cut(s, ",")
			if !ok {
				continue
			}
			data, e := base64.StdEncoding.DecodeString(b64)
			if e != nil {
				continue
			}
			mt := strings.TrimSuffix(strings.TrimPrefix(h, "data:"), ";base64")
			ext, _ := mime.ExtensionsByType(mt)
			suffix := ".png"
			if len(ext) > 0 {
				suffix = ext[0]
			}
			sum := sha256.Sum256(data)
			name := hex.EncodeToString(sum[:12]) + suffix
			if e := os.WriteFile(filepath.Join(t.dir, "media", name), data, 0644); e != nil {
				return e
			}
			im["url"] = "file://media/" + name
		}
	}
	return t.Event(map[string]any{"t": "checkpoint", "messages": out})
}
func ImagePart(path string, width int) (map[string]any, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	mt := mime.TypeByExtension(filepath.Ext(path))
	if mt == "" {
		mt = "image/png"
	}
	if width > 0 {
		img, _, err := image.Decode(bytes.NewReader(b))
		if err == nil && max(img.Bounds().Dx(), img.Bounds().Dy()) > width {
			w, h := img.Bounds().Dx(), img.Bounds().Dy()
			scale := float64(width) / float64(max(w, h))
			dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
			draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
			var out strings.Builder
			if err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90}); err != nil {
				return nil, err
			}
			b = []byte(out.String())
			mt = "image/jpeg"
		}
	}
	return map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b)}}, nil
}
