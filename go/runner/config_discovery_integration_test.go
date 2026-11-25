package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCheckUsesDiscoveredConfiguration(t *testing.T) {
	resources, reference := realFixture(t)
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("COMPARE_FIGURE_CONFIG", "")
	path := filepath.Join(cwd, "runner.yaml")
	if err := os.WriteFile(path, []byte("drawing:\n  mode: latex\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), Options{Original: reference, Out: filepath.Join(t.TempDir(), "out"), ResourceRoot: resources, Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if result["config_path"] != path || result["api_contacted"] != false || Obj(result, "environment")["drawing_mode"] != "latex" {
		t.Fatal(result)
	}
}
