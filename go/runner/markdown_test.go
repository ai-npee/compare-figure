package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownImagePathsIncludesAllLocalReferences(t *testing.T) {
	md, images, root := writeMarkdownFixture(t, "![](images/book/target.png)\n[see](../shared%20image.png)\n![](other/sibling.png)\n![](missing.png)\n![](https://example.com/image.png)\n")
	local := filepath.Join(images, "target.png")
	outside := filepath.Join(root, "shared image.png")
	other := filepath.Join(filepath.Dir(md), "other", "sibling.png")
	if err := os.MkdirAll(filepath.Dir(other), 0755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{local, outside, other} {
		if err := os.WriteFile(p, []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := MarkdownImagePaths(md, images)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"images/book/target.png": local, "../shared image.png": outside, "other/sibling.png": other} {
		if got[key] != want {
			t.Fatalf("%s=%v, want %s", key, got[key], want)
		}
	}
	if len(got) != 3 {
		t.Fatalf("unexpected local references: %v", got)
	}
}

func writeMarkdownFixture(t *testing.T, content string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	mdDir := filepath.Join(root, "book")
	images := filepath.Join(mdDir, "images", "book")
	if err := os.MkdirAll(images, 0o755); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(mdDir, "source.md")
	if err := os.WriteFile(md, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return md, images, root
}

func TestResolveMarkdownPercentEscapedAngleReferenceAndContext(t *testing.T) {
	md, images, _ := writeMarkdownFixture(t, "before\n![](<images/book/图%20一.jpg> \"title\")\nafter\n")
	image := filepath.Join(images, "图 一.jpg")
	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	gotOriginal, gotRef, gotImages, gotContext, err := ResolveMarkdown(md, "images/book/图 一.jpg", "", "", "extra")
	if err != nil {
		t.Fatal(err)
	}
	if gotOriginal != image || gotRef != "images/book/图%20一.jpg" || gotImages != filepath.Join(filepath.Dir(md), "images") {
		t.Fatalf("unexpected resolution: original=%q ref=%q images=%q", gotOriginal, gotRef, gotImages)
	}
	if !strings.Contains(gotContext, "extra") || !strings.Contains(gotContext, "before") || !strings.Contains(gotContext, "after") {
		t.Fatalf("missing context: %q", gotContext)
	}
}

func TestResolveMarkdownDuplicateBasenameRequiresCompleteReference(t *testing.T) {
	root := t.TempDir()
	mdDir := filepath.Join(root, "book")
	for _, dir := range []string{filepath.Join(mdDir, "images", "a"), filepath.Join(mdDir, "images", "b")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "图.jpg"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	md := filepath.Join(mdDir, "source.md")
	if err := os.WriteFile(md, []byte("![](images/a/图.jpg)\n![](images/b/图.jpg)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := ResolveMarkdown(md, "图.jpg", "", "", ""); err == nil || !strings.Contains(err.Error(), "duplicate basename") {
		t.Fatalf("expected duplicate basename error, got %v", err)
	}
	original := filepath.Join(mdDir, "images", "b", "图.jpg")
	gotOriginal, gotRef, _, _, err := ResolveMarkdown(md, "", original, "", "")
	if err != nil || gotOriginal != original || gotRef != "images/b/图.jpg" {
		t.Fatalf("original matching failed: %q %q %v", gotOriginal, gotRef, err)
	}
}

func TestResolveMarkdownExternalSelectionAndAutoAmbiguity(t *testing.T) {
	md, images, _ := writeMarkdownFixture(t, "![](https://example.test/a.jpg)\n![](images/book/a.jpg)\n")
	if err := os.WriteFile(filepath.Join(images, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := ResolveMarkdown(md, "https://example.test/a.jpg", "", "", ""); err == nil || !strings.Contains(err.Error(), "external URL") {
		t.Fatalf("expected external URL error, got %v", err)
	}
	if _, _, _, _, err := ResolveMarkdown(md, "", "", "", ""); err != nil {
		t.Fatalf("external image should be ignored for automatic local selection: %v", err)
	}
}

func TestResolveMarkdownDirectPassthroughWithoutSource(t *testing.T) {
	gotOriginal, gotRef, gotImages, gotContext, err := ResolveMarkdown("", "ref", "original", "images", "context")
	if err != nil || gotOriginal != "original" || gotRef != "ref" || gotImages != "images" || gotContext != "context" {
		t.Fatalf("unexpected passthrough: %q %q %q %q %v", gotOriginal, gotRef, gotImages, gotContext, err)
	}
}
