package distribution

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExportIncludesAsymptoteAndPreservesLuaDrawResources(t *testing.T) {
	root, err := FindRoot("")
	if err != nil {
		t.Skip(err)
	}
	out := filepath.Join(t.TempDir(), "export")
	if _, err = Export(context.Background(), ExportOptions{Root: root, Out: out}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"docs/asymptote-upstream.json",
		"docs/drawing-modes.md",
		"docs/doc-library-compatibility.md",
		"vendor/asymptote-3.16/RESOURCE-MANIFEST.json",
		"vendor/asymptote-3.16/LICENSE",
		"vendor/asymptote-3.16/LICENSE.LESSER",
		"vendor/asymptote-3.16/doc/asymptote.pdf",
		"vendor/asymptote-3.16/doc/asymptote.texi",
		"vendor/asymptote-3.16/examples/triangle.asy",
	} {
		source, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if e != nil {
			t.Fatal(name, e)
		}
		got, e := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if e != nil || !bytes.Equal(got, source) {
			t.Fatalf("exported resource mismatch for %s: %v", name, e)
		}
	}
	luaRoot := filepath.Join(root, "vendor", "luadraw-3.7")
	if err = filepath.WalkDir(luaRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		source, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		got, e := os.ReadFile(filepath.Join(out, rel))
		if e != nil {
			return e
		}
		if !bytes.Equal(got, source) {
			return fmt.Errorf("LuaDraw resource changed in export: %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalSkillVersionAndBody(t *testing.T) {
	root := t.TempDir()
	source := "---\nname: compare-figure\ndescription: Figure redraw\nmetadata:\n  version: \"0.1.0\"\n---\n\n# Mode 1\nKeep original tool receipt.\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("0.2.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, version, err := skillDocument(root, "")
	if err != nil || version != "0.2.0" {
		t.Fatal(version, err)
	}
	if !strings.HasSuffix(string(first), "\n\n# Mode 1\nKeep original tool receipt.\n") {
		t.Fatal(string(first))
	}
	overridden, version, err := skillDocument(root, "v1.2.3-rc.1+build.4")
	if err != nil || version != "1.2.3-rc.1+build.4" {
		t.Fatal(version, err)
	}
	var header map[string]any
	if err := yaml.Unmarshal([]byte(strings.Split(string(overridden), "---")[1]), &header); err != nil {
		t.Fatal(err)
	}
	if header["metadata"].(map[string]any)["version"] != version {
		t.Fatal(header)
	}
	again, _, _ := skillDocument(root, "v1.2.3-rc.1+build.4")
	if string(again) != string(overridden) {
		t.Fatal("non-idempotent Skill generation")
	}
	for _, bad := range []string{"../1.2.3", "1.2", "01.2.3", "1.2.3-01", "x"} {
		if _, _, err := skillDocument(root, bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if string(after) != source {
		t.Fatal("source Skill changed")
	}
}

func TestSkillFrontmatterRequired(t *testing.T) {
	root := t.TempDir()
	for _, invalid := range []string{"# No frontmatter\n", "---\nname: other\ndescription: test\n---\n", "---\nname: compare-figure\n---\n"} {
		if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := skillDocument(root, "0.1.0"); err == nil {
			t.Fatal("accepted invalid Skill", invalid)
		}
	}
}
