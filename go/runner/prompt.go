package runner

import (
	"bufio"
	"compare-figure/render"
	"compare-figure/session"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// These are template variables, not arbitrary TeX/math brace groups.
var promptVariables = []string{"MAX_ROUNDS", "OUTPUT_LANG", "LUADRAW_VERSION", "ENVIRONMENT", "CONTEXT", "CONTEXT_FENCE", "ORIGINAL_SIZE", "VIEW_BUDGET", "DRAWING_MODE", "SOURCE_DESCRIPTION", "SOURCE_PATH", "ORIGINAL", "REDRAWN", "ROUND", "PROBLEMS", "SOURCE_LANGUAGE"}

func promptText(root, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "prompts", name))
	if err != nil {
		return "", fmt.Errorf("load prompt %s: %w", name, err)
	}
	return string(data), nil
}

func interpolatePrompt(text string, values Object) (string, error) {
	// Validate only the template itself. Inserted context and environment facts
	// are opaque data, even when they contain a reserved placeholder literally.
	for _, key := range promptVariables {
		if strings.Contains(text, "{"+key+"}") {
			if _, ok := values[key]; !ok {
				return "", fmt.Errorf("missing prompt variable {%s}", key)
			}
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, 2*len(keys))
	for _, key := range keys {
		pairs = append(pairs, "{"+key+"}", fmt.Sprint(values[key]))
	}
	// Replacer scans once; it never expands placeholders inside replacement data.
	return strings.NewReplacer(pairs...).Replace(text), nil
}

// Prompt reads an on-disk template and expands variables in one nonrecursive pass.
func Prompt(root, name string, values Object) (string, error) {
	text, err := promptText(root, name)
	if err != nil {
		return "", err
	}
	text, err = interpolatePrompt(text, values)
	if err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	return text, nil
}

func drawingPromptValues(mode string, values Object) Object {
	out := Clone(values)
	if out == nil {
		out = Object{}
	}
	out["DRAWING_MODE"] = mode
	out["SOURCE_DESCRIPTION"] = render.SourceDescription(mode)
	out["SOURCE_PATH"] = render.DefaultSource(mode)
	context := Str(out, "CONTEXT", "")
	longest, run := 0, 0
	for _, char := range context {
		if char == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	out["CONTEXT_FENCE"] = strings.Repeat("`", max(3, longest+1))
	return out
}

// DrawingPrompts loads the complete mode policy/workflow and the shared user
// template. All long-form instructions live in prompts/, not Go string literals.
func DrawingPrompts(resources, mode string, variables Object) (system, user string, err error) {
	mode, err = render.NormalizeMode(mode)
	if err != nil {
		return "", "", err
	}
	values := drawingPromptValues(mode, variables)
	name := "luadraw_figure.system.md"
	if mode != "luadraw" {
		name = map[string]string{"mixed": "mixed_figure.policy.md", "latex": "native_latex_figure.policy.md", "asymptote": "asymptote_figure.policy.md"}[mode]
	}
	system, err = promptText(resources, name)
	if err != nil {
		return "", "", err
	}
	if mode != "luadraw" {
		workflow, e := promptText(resources, "drawing_figure.system.md")
		if e != nil {
			return "", "", e
		}
		system += "\n" + workflow
	}
	system, err = interpolatePrompt(system, values)
	if err != nil {
		return "", "", fmt.Errorf("drawing mode %s system: %w", mode, err)
	}
	user, err = Prompt(resources, "luadraw_figure.user.md", values)
	return system, user, err
}

// historicalDrawingSystem preserves the first recorded system policy on resume.
// Transcript.Load restores messages/checkpoints but does not load meta.system.
func historicalDrawingSystem(path string, messages []session.Message) (string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 4096), 64<<20)
	for scan.Scan() {
		var event Object
		if err := json.Unmarshal(scan.Bytes(), &event); err != nil {
			return "", err
		}
		if event["t"] == "meta" && event["kind"] == "system" {
			if text := Str(event, "system", ""); text != "" {
				return text, nil
			}
		}
		if event["t"] == "msg" && event["role"] == "system" {
			if text := Str(event, "text", ""); text != "" {
				return text, nil
			}
		}
		if event["type"] == "message" {
			message := Obj(event, "message")
			if message["role"] == "system" && session.TextOf(message) != "" {
				return session.TextOf(message), nil
			}
		}
	}
	if err := scan.Err(); err != nil {
		return "", err
	}
	for _, message := range messages {
		if message["role"] == "system" && session.TextOf(message) != "" {
			return session.TextOf(message), nil
		}
	}
	return "", nil
}

func withDrawingSystem(messages []session.Message, system string) []session.Message {
	out := []session.Message{{"role": "system", "content": system}}
	for _, message := range messages {
		if message["role"] != "system" {
			out = append(out, message)
		}
	}
	return out
}
