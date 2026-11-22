package drawing

import (
	"strings"
	"testing"
)

func TestMeasureHintPrefersPreservedOriginalMeasurement(t *testing.T) {
	h := testHarness(t, map[string]any{"measurement": map[string]any{"width_mm": 30.0, "height_mm": 15.0, "page_w_mm": 100.0, "pixels_w": 600, "pixels_h": 300, "dpi": 508, "found": true, "aspect_str": "2.00:1"}})
	if hint := h.MeasureHint(); !strings.Contains(hint, "about 30% of the page width") || !strings.Contains(hint, "508 dpi") {
		t.Fatalf("original measurement not retained: %s", hint)
	}
}
