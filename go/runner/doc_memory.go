package runner

import (
	"compare-figure/luadraw"
	"context"
	"strings"
)

// documentationMemory is optional reference material, added only to a fresh
// task's opening context; it never changes the input fingerprint or resume log.
func documentationMemory(ctx context.Context, config, options Object) (string, error) {
	memory := Obj(Obj(config, "docs"), "frequent")
	if !Bool(memory, "prompt_enabled", true) {
		return "", nil
	}
	maxChars := min(2048, max(0, int(Num(memory, "prompt_max_chars", 2048))))
	lines := min(8, max(0, int(Num(memory, "prompt_lines", 8))))
	lineChars := min(512, max(16, int(Num(memory, "line_chars", 256))))
	if maxChars == 0 || lines == 0 {
		return "", nil
	}
	docs, err := luadraw.NewDocs(Str(options, "luadraw_root", ""), Str(options, "doc_cache_dir", ""))
	if err != nil {
		return "", err
	}
	text, err := docs.FrequentSnippet(ctx, lines, lineChars, maxChars)
	if err != nil {
		return "", err
	}
	return boundedDocumentationMemory(text, lines, lineChars, maxChars), nil
}

func boundedDocumentationMemory(text string, lines, lineChars, maxChars int) string {
	if strings.TrimSpace(text) == "" || lines <= 0 || lineChars <= 0 || maxChars <= 0 {
		return ""
	}
	var result []string
	remaining := maxChars
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(result) >= lines || remaining <= 0 {
			break
		}
		chars := []rune(line)
		limit := min(lineChars, remaining)
		if len(chars) > limit {
			chars = chars[:limit]
		}
		result = append(result, string(chars))
		remaining -= len(chars) + 1
	}
	return strings.Join(result, "\n")
}
