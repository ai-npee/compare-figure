package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveConfigPath selects one configuration; files are never implicitly merged.
// "none" deliberately disables file discovery while retaining CLI/env defaults.
func ResolveConfigPath(explicit, resources string) (string, error) {
	selected := strings.TrimSpace(explicit)
	if selected == "none" {
		return "", nil
	}
	if selected == "" {
		selected = strings.TrimSpace(os.Getenv("COMPARE_FIGURE_CONFIG"))
	}
	if selected == "none" {
		return "", nil
	}
	if selected != "" {
		absolute, err := filepath.Abs(selected)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return "", fmt.Errorf("configuration %s: %w", absolute, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("configuration must be a regular file: %s", absolute)
		}
		return absolute, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, directory := range []string{cwd, resources} {
		if directory == "" {
			continue
		}
		directory, err = filepath.Abs(directory)
		if err != nil {
			return "", err
		}
		if seen[directory] {
			continue
		}
		seen[directory] = true
		for _, name := range []string{"runner.yaml", "runner.yml", "runner.json"} {
			candidate := filepath.Join(directory, name)
			info, err := os.Stat(candidate)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return "", fmt.Errorf("configuration %s: %w", candidate, err)
			}
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("configuration must be a regular file: %s", candidate)
			}
			return candidate, nil
		}
	}
	return "", nil
}
