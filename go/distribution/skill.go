package distribution

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// skillDocument reads the repository's single canonical Skill instruction file.
// Only version metadata changes during export; instructions are not duplicated
// in Go or reconstructed from another template.
func skillDocument(root, override string) ([]byte, string, error) {
	data, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return nil, "", fmt.Errorf("read SKILL.md: %w", err)
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return nil, "", fmt.Errorf("SKILL.md frontmatter has no closing delimiter")
	}
	end += 4
	var front map[string]any
	if err = yaml.Unmarshal([]byte(normalized[4:end]), &front); err != nil {
		return nil, "", fmt.Errorf("SKILL.md frontmatter: %w", err)
	}
	if front["name"] != "compare-figure" {
		return nil, "", fmt.Errorf("SKILL.md name must be compare-figure")
	}
	description, ok := front["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return nil, "", fmt.Errorf("SKILL.md description is required")
	}
	metadata, _ := front["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
	}
	version := strings.TrimSpace(override)
	if version == "" {
		if v, e := os.ReadFile(filepath.Join(root, "VERSION")); e == nil {
			version = strings.TrimSpace(string(v))
		} else if !os.IsNotExist(e) {
			return nil, "", e
		} else {
			version, _ = metadata["version"].(string)
		}
	}
	version = strings.TrimPrefix(version, "v")
	if !releaseVersion.MatchString(version) {
		return nil, "", fmt.Errorf("version must be SemVer, for example 0.1.0")
	}
	if dash := strings.IndexByte(version, '-'); dash >= 0 {
		pre := strings.SplitN(version[dash+1:], "+", 2)[0]
		for _, id := range strings.Split(pre, ".") {
			if len(id) > 1 && id[0] == '0' && strings.Trim(id, "0123456789") == "" {
				return nil, "", fmt.Errorf("numeric prerelease identifiers cannot have leading zeroes")
			}
		}
	}
	metadata["version"] = version
	front["metadata"] = metadata
	header, err := yaml.Marshal(front)
	if err != nil {
		return nil, "", err
	}
	return []byte("---\n" + string(header) + "---\n" + normalized[end+5:]), version, nil
}
