package distribution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		"docs/luadraw-surface-rendering.md",
		"docs/document-sources/luadraw.yaml",
		"docs/document-sources/mixed.yaml",
		"docs/document-sources/asymptote.yaml",
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

func TestExportIncludesOfflineKaTeXResourcesAndProvenance(t *testing.T) {
	root, err := FindRoot("")
	if err != nil {
		t.Skip(err)
	}
	out := filepath.Join(t.TempDir(), "export")
	if _, err = Export(context.Background(), ExportOptions{Root: root, Out: out}); err != nil {
		t.Fatal(err)
	}
	resourceRoot := filepath.Join(root, "vendor", "katex-0.18.2")
	manifestBytes, err := os.ReadFile(filepath.Join(resourceRoot, "RESOURCE-MANIFEST.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Project       string `json:"project"`
		Version       string `json:"version"`
		TarballSHA256 string `json:"tarball_sha256"`
		Files         []struct {
			Path         string `json:"path"`
			UpstreamPath string `json:"upstream_path"`
			SHA256       string `json:"sha256"`
			Size         int64  `json:"size_bytes"`
			License      string `json:"license"`
		} `json:"files"`
	}
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Project != "KaTeX" || manifest.Version != "0.18.2" || manifest.TarballSHA256 != "a71df261a7080b53a4be44ea1b40e8e73e84cdd5d3311239310568e76437c018" {
		t.Fatalf("unexpected KaTeX provenance: %+v", manifest)
	}
	seen := map[string]bool{}
	for _, item := range manifest.Files {
		if item.Path == "" || filepath.IsAbs(item.Path) || strings.Contains(filepath.ToSlash(item.Path), "..") || seen[item.Path] {
			t.Fatalf("invalid or duplicate KaTeX manifest path %q", item.Path)
		}
		seen[item.Path] = true
		source, e := os.ReadFile(filepath.Join(resourceRoot, filepath.FromSlash(item.Path)))
		if e != nil {
			t.Fatal(item.Path, e)
		}
		copy, e := os.ReadFile(filepath.Join(out, "vendor", "katex-0.18.2", filepath.FromSlash(item.Path)))
		if e != nil || !bytes.Equal(copy, source) {
			t.Fatalf("exported KaTeX resource mismatch for %s: %v", item.Path, e)
		}
		sum := sha256.Sum256(source)
		if item.Size != int64(len(source)) || item.SHA256 != hex.EncodeToString(sum[:]) || item.License != "MIT" || item.UpstreamPath == "" {
			t.Fatalf("KaTeX manifest entry does not verify for %s", item.Path)
		}
	}
	for _, required := range []string{"katex.min.js", "katex.min.css", "LICENSE"} {
		if !seen[required] {
			t.Fatalf("missing required KaTeX resource %s", required)
		}
	}
	if !seen["fonts/KaTeX_Main-Regular.woff2"] {
		t.Fatal("missing CSS font resource")
	}
	css, err := os.ReadFile(filepath.Join(resourceRoot, "katex.min.css"))
	if err != nil {
		t.Fatal(err)
	}
	fontURL := regexp.MustCompile(`url\(["']?([^"')]+)["']?\)`).FindAllStringSubmatch(string(css), -1)
	for _, match := range fontURL {
		if strings.HasPrefix(match[1], "fonts/") && !seen[filepath.ToSlash(filepath.Clean(match[1]))] {
			t.Fatalf("CSS font URL is missing from resource manifest: %s", match[1])
		}
	}
	for _, name := range []string{"docs/katex-upstream.json", "docs/viewer-math.md", "docs/prompt-guide.md", "scripts/viewer_math.js"} {
		source, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if e != nil {
			t.Fatal(e)
		}
		copy, e := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if e != nil || !bytes.Equal(copy, source) {
			t.Fatalf("exported provenance/document mismatch for %s: %v", name, e)
		}
	}
	if !bytes.Equal(manifestBytes, mustRead(t, filepath.Join(out, "vendor", "katex-0.18.2", "RESOURCE-MANIFEST.json"))) {
		t.Fatal("exported KaTeX resource manifest bytes differ")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
