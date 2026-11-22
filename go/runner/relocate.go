package runner

import (
	"path"
	"path/filepath"
	"strings"
)

// Rebase only paths owned by a recorded workspace. Use slash normalization so
// saved Windows references can be moved to Unix and vice versa.
func rebaseWorkspace(value any, oldRoot, newRoot string) any {
	oldRoot = strings.TrimSuffix(strings.ReplaceAll(oldRoot, "\\", "/"), "/")
	var visit func(any) any
	visit = func(value any) any {
		switch v := value.(type) {
		case string:
			normalized := strings.ReplaceAll(v, "\\", "/")
			if normalized == oldRoot {
				return newRoot
			}
			if strings.HasPrefix(normalized, oldRoot+"/") {
				relative := strings.TrimPrefix(normalized, oldRoot+"/")
				candidate := filepath.Join(newRoot, filepath.FromSlash(relative))
				if inside(newRoot, candidate) {
					return candidate
				}
			}
			return v
		case map[string]any:
			out := Object{}
			for key, item := range v {
				out[key] = visit(item)
			}
			return out
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = visit(item)
			}
			return out
		case []string:
			out := make([]string, len(v))
			for i, item := range v {
				out[i], _ = visit(item).(string)
			}
			return out
		default:
			return value
		}
	}
	return visit(value)
}
func recordedParent(file string) string { return path.Dir(strings.ReplaceAll(file, "\\", "/")) }
func recordedBase(file string) string   { return path.Base(strings.ReplaceAll(file, "\\", "/")) }
