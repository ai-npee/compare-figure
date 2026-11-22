package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSemiAutomaticCompileRepeatsInFixedSlot(t *testing.T) {
	t.Setenv("COMPARE_FIGURE_OUTPUT_ROOT", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	source := filepath.Join(t.TempDir(), "figure.lua")
	program := "local ld=luadraw\nlocal g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}\ng:Dline(-1,1,\"red,thick\")\ng:Show()\n"
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	first := callNative(t, "compile", "--source", source)
	if first["ok"] != true {
		t.Fatal(first)
	}
	before := fileCount(t, first["out_dir"].(string))
	next := callNative(t, "luadraw-compile", source)
	if next["pdf"] != first["pdf"] || next["out_dir"] != first["out_dir"] {
		t.Fatal(first, next)
	}
	if fileCount(t, first["out_dir"].(string)) != before {
		t.Fatal("compile output grew between same-source runs")
	}
	images := next["images"].([]any)
	if len(images) != 1 {
		t.Fatal(next)
	}
}
