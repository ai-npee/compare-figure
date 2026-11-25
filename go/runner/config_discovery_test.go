package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDiscoveryPriorityAndOptOut(t *testing.T) {
	cwd, root, other := t.TempDir(), t.TempDir(), t.TempDir()
	t.Chdir(cwd)
	t.Setenv("COMPARE_FIGURE_CONFIG", "")
	write := func(dir, name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// Local credential/test files must be explicitly selected, never guessed.
	write(cwd, "runner.local.yaml")
	path, err := ResolveConfigPath("", root)
	if err != nil || path != "" {
		t.Fatalf("unexpected local selection: %q %v", path, err)
	}
	global := write(root, "runner.yaml")
	path, err = ResolveConfigPath("", root)
	if err != nil || path != global {
		t.Fatalf("resource default: %q %v", path, err)
	}
	json := write(cwd, "runner.json")
	path, err = ResolveConfigPath("", root)
	if err != nil || path != json {
		t.Fatalf("cwd first: %q %v", path, err)
	}
	yml := write(cwd, "runner.yml")
	path, err = ResolveConfigPath("", root)
	if err != nil || path != yml {
		t.Fatalf("yml preference: %q %v", path, err)
	}
	yaml := write(cwd, "runner.yaml")
	path, err = ResolveConfigPath("", root)
	if err != nil || path != yaml {
		t.Fatalf("yaml preference: %q %v", path, err)
	}
	env := write(other, "shared.json")
	t.Setenv("COMPARE_FIGURE_CONFIG", env)
	path, err = ResolveConfigPath("", root)
	if err != nil || path != env {
		t.Fatalf("env override: %q %v", path, err)
	}
	path, err = ResolveConfigPath(json, root)
	if err != nil || path != json {
		t.Fatalf("explicit override: %q %v", path, err)
	}
	path, err = ResolveConfigPath("none", root)
	if err != nil || path != "" {
		t.Fatalf("explicit none: %q %v", path, err)
	}
	t.Setenv("COMPARE_FIGURE_CONFIG", "none")
	path, err = ResolveConfigPath("", root)
	if err != nil || path != "" {
		t.Fatalf("env none: %q %v", path, err)
	}
	t.Setenv("COMPARE_FIGURE_CONFIG", filepath.Join(other, "missing.yaml"))
	if _, err = ResolveConfigPath("", root); err == nil {
		t.Fatal("missing env silently fell back")
	}
	if _, err = ResolveConfigPath(other, root); err == nil {
		t.Fatal("directory accepted as config")
	}
}

func TestSelectedMalformedDefaultDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("COMPARE_FIGURE_CONFIG", "")
	path := filepath.Join(dir, "runner.yaml")
	if err := os.WriteFile(path, []byte("drawing: [broken"), 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := ResolveConfigPath("", t.TempDir())
	if err != nil || selected != path {
		t.Fatal(selected, err)
	}
	if _, err := LoadConfig(selected); err == nil {
		t.Fatal("malformed discovered config silently ignored")
	}
}
