package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveMarkdownPlainImageLinks(t *testing.T) {
	dir := t.TempDir()
	images := filepath.Join(dir, "images", "Book")
	if e := os.MkdirAll(images, 0700); e != nil {
		t.Fatal(e)
	}
	image := filepath.Join(images, "figure.jpg")
	if e := os.WriteFile(image, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	md := filepath.Join(dir, "Book.md")
	if e := os.WriteFile(md, []byte("[notes](notes.md)\n[url](images/Book/figure.jpg)\n图6.9\n[Image](descriptive prose)\n"), 0600); e != nil {
		t.Fatal(e)
	}
	original, ref, _, context, e := ResolveMarkdown(md, "images/Book/figure.jpg", "", "", "")
	if e != nil {
		t.Fatal(e)
	}
	if original != image || ref != "images/Book/figure.jpg" || !strings.Contains(context, "图6.9") {
		t.Fatalf("incorrect resolution %s %s %s", original, ref, context)
	}
	refs, _ := parseMarkdownImages("![normal](a.png) [url](b.jpg) [manual](manual.md)")
	if len(refs) != 2 || refs[0].raw != "a.png" || refs[1].raw != "b.jpg" {
		t.Fatalf("wrong image refs %#v", refs)
	}
}
