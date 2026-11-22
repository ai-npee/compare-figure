package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeCreditRelocatesSavedWindowsArtifactPaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"submitted.pdf", "submitted.tex", "submitted.lua"} {
		if e := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	old := `C:\old\workspace`
	result := Object{"submitted": true, "source_language": "luadraw", "best_overlap": .95, "scale_dev": .01, "submitted_pdf": old + `\submitted.pdf`, "submitted_tex": old + `\submitted.tex`, "editable_source": old + `\submitted.lua`, "generated_lua": old + `\_luadraw_inputs\submitted.lua`}
	if e := atomicJSON(filepath.Join(root, "result.json"), result); e != nil {
		t.Fatal(e)
	}
	credit := ResumeCredit(root, .9, .1, true)
	if credit == nil {
		t.Fatal("relocated accepted result was not reusable")
	}
	for _, key := range []string{"submitted_pdf", "submitted_tex", "editable_source", "generated_lua"} {
		if !strings.HasPrefix(Str(credit, key, ""), root+string(os.PathSeparator)) {
			t.Fatal(key, credit)
		}
	}
}
func TestRelocationKeepsExternalBookReferencesAndRejectsTraversal(t *testing.T) {
	old := "/old/run"
	value := Object{"pdf": old + "/standalone.pdf", "book": "/books/source.md", "unsafe": old + "/../../outside.pdf"}
	result := rebaseWorkspace(value, old, t.TempDir()).(map[string]any)
	if result["book"] != "/books/source.md" || result["unsafe"] != old+"/../../outside.pdf" {
		t.Fatal(result)
	}
}
