package comparefigure

import (
	"fmt"
	"strings"
)

// ComparisonText preserves the production compare_figure receipt verbatim.
func ComparisonText(result Result, detail string) string {
	if result.Metrics == nil {
		return "Comparison unavailable."
	}
	m := result.Metrics
	size := "Ink size: not measurable (no ink in one of the images)."
	if m.InkRefW > 0 && m.InkCandW > 0 {
		if m.SizeUnit == "mm" {
			size = fmt.Sprintf("Ink size: canvas share w %.0f%%/%.0f%%, h %.0f%%/%.0f%% (original/candidate); physical ink %.1fmm x %.1fmm vs %.1fmm x %.1fmm; width x%.3f, height x%.3f, max deviation %.1f%% (measured in mm BEFORE the comparison rescales the candidate, so it does catch a wrong printed size; fix the coordinates/canvas rather than the stroke width). Ink aspect ratio %.2f:1 vs %.2f:1.", m.ShareRefW*100, m.ShareCandW*100, m.ShareRefH*100, m.ShareCandH*100, m.InkRefW, m.InkRefH, m.InkCandW, m.InkCandH, m.ScaleX, m.ScaleY, m.ScaleDev*100, m.AspectRef, m.AspectCandidate)
		} else {
			size = fmt.Sprintf("Ink size: canvas share w %.0f%%/%.0f%%, h %.0f%%/%.0f%% (original/candidate); physical size unknown (image or PDF page size not measurable), so only the aspect ratio is checked: %.2f:1 vs %.2f:1, deviation %.1f%% >? limit. Ink bbox %dx%dpx vs %dx%dpx is NOT comparable (render DPI differs) — do not aim at it. Fix the coordinates/canvas rather than the stroke width.", m.ShareRefW*100, m.ShareCandW*100, m.ShareRefH*100, m.ShareCandH*100, m.AspectRef, m.AspectCandidate, m.AspectDev*100, int(m.InkRefW), int(m.InkRefH), int(m.InkCandW), int(m.InkCandH))
		}
	}
	runs := make([]string, 0, len(m.Runs))
	for _, run := range m.Runs {
		runs = append(runs, fmt.Sprintf("%s %.2f%% @(%d,%d)", run.Name, run.Score*100, run.DX, run.DY))
	}
	if detail == "" {
		detail = "all"
	}
	crop := result.Crop
	if len(crop) != 4 {
		crop = []float64{0, 0, 100, 100}
	}
	width, height := 0, 0
	if len(result.Images) > 0 {
		width, height = result.Images[0].Width, result.Images[0].Height
	}
	return fmt.Sprintf("Soft-ink overlap: %.2f%% (R %.2f%%, G %.2f%%, B %.2f%%, gray %.2f%%). %s Estimated candidate translation applied: dx=%d px, dy=%d px. Alignment starts: %s. Detail=%s, crop=%.1f%%,%.1f%%-%.1f%%,%.1f%%, output=%dx%dpx. Overlay key: white=shared ink, magenta=original-only ink, green=candidate-only ink, black=background.", m.Similarity*100, m.Channels[0]*100, m.Channels[1]*100, m.Channels[2]*100, m.Gray*100, size, m.DX, m.DY, strings.Join(runs, "; "), detail, crop[0], crop[1], crop[2], crop[3], width, height)
}
