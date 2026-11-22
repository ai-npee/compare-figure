package drawing

import (
	"context"
	"strings"
	"testing"
)

func TestImageContextReportsPriorCurrentNextAndWindows(t *testing.T) {
	md := "before\n![first](images/ch1/a.png)\nfirst caption\n![second](images/ch1/b.png)\nsecond caption\n![third](images/ch1/c.png)\nafter"
	h := testHarness(t, map[string]any{"current_img": "images/ch1/b.png", "md_content": md, "max_up": 1, "max_down": 1})
	text, _, err := h.Execute(context.Background(), "image_context", map[string]any{"image": "b.png", "up": 8, "down": 8})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"image 2 of 3", "images/ch1/a.png", "images/ch1/b.png", "images/ch1/c.png", "line 4", "up 1 / down 1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("context missing %q:\n%s", want, text)
		}
	}
}
