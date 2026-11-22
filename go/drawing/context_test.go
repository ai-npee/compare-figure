package drawing

import (
	"context"
	"strings"
	"testing"
)

func TestImageContextPlainLinkDefaultTarget(t *testing.T) {
	target := "images/高等数学释疑解难-工科课委会/b5c316e431a176e7f8ad0a4adeb5680738233aca15b20273f69118c9846e61f6.jpg"
	md := "前文\n[url](images/书/previous.jpg)\n图6.8\n说明文字\n[url](" + target + ")\n[Image](三维坐标与阴影D_2)\n图6.9\n[url](images/书/next.jpg)\n图6.10"
	h := testHarness(t, map[string]any{"current_img": target, "md_content": md})
	text, _, err := h.Execute(context.Background(), "image_context", map[string]any{"up": 6, "down": 6})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"image 2 of 3", target, "up 6 / down 6", "图6.9", "previous.jpg", "next.jpg"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "找不到") {
		t.Fatal(text)
	}
}

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
