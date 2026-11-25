package app

import (
	"bytes"
	"compare-figure/distribution"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunEnvironmentDiscoversConfigAndExplicitModeWins(t *testing.T) {
	resources, err := distribution.FindRoot("")
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("COMPARE_FIGURE_CONFIG", "")
	file := filepath.Join(cwd, "runner.yaml")
	// Deliberately invalid configured policy proves both discovery and override.
	if err := os.WriteFile(file, []byte("drawing:\n  mode: invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if status := Run(context.Background(), []string{"run", "--environment", "--resource-root", resources}, nil, &out, &out); status == 0 {
		t.Fatalf("discovered invalid mode was ignored: %s", out.String())
	}
	out.Reset()
	if status := Run(context.Background(), []string{"run", "--environment", "--drawing-mode", "latex", "--resource-root", resources}, nil, &out, &out); status != 0 {
		t.Fatalf("explicit mode failed %d: %s", status, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result["drawing_mode"] != "latex" {
		t.Fatalf("bad explicit mode: %v %s", err, out.String())
	}
	out.Reset()
	if status := Run(context.Background(), []string{"run", "--environment", "--drawing-mode", "latex", "--config", filepath.Join(cwd, "missing.yaml"), "--resource-root", resources}, nil, &out, &out); status == 0 {
		t.Fatal("explicit mode ignored missing config")
	}
}
