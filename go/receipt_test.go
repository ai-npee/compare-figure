package comparefigure

import "testing"

func TestProductionReceiptExact(t *testing.T) {
	m := Metrics{Similarity: .1474, Channels: [3]float64{.1474, .1474, .1474}, Gray: .1438, SizeUnit: "mm", ShareRefW: .98, ShareCandW: .82, ShareRefH: .97, ShareCandH: .89, InkRefW: 45.4, InkRefH: 54.8, InkCandW: 44.7, InkCandH: 53.9, ScaleX: .984, ScaleY: .983, ScaleDev: .017, AspectRef: .83, AspectCandidate: .83, DX: -20, DY: -37, Runs: []AlignmentRun{{Name: "origin", Score: .1474, DX: 33, DY: 15}, {Name: "centroid", Score: .1474, DX: 33, DY: 15}}}
	result := Result{Metrics: &m, Crop: []float64{0, 0, 100, 100}, Images: []ImageArtifact{{Width: 1080, Height: 1312}}}
	want := "Soft-ink overlap: 14.74% (R 14.74%, G 14.74%, B 14.74%, gray 14.38%). Ink size: canvas share w 98%/82%, h 97%/89% (original/candidate); physical ink 45.4mm x 54.8mm vs 44.7mm x 53.9mm; width x0.984, height x0.983, max deviation 1.7% (measured in mm BEFORE the comparison rescales the candidate, so it does catch a wrong printed size; fix the coordinates/canvas rather than the stroke width). Ink aspect ratio 0.83:1 vs 0.83:1. Estimated candidate translation applied: dx=-20 px, dy=-37 px. Alignment starts: origin 14.74% @(33,15); centroid 14.74% @(33,15). Detail=all, crop=0.0%,0.0%-100.0%,100.0%, output=1080x1312px. Overlay key: white=shared ink, magenta=original-only ink, green=candidate-only ink, black=background."
	if got := ComparisonText(result, "all"); got != want {
		t.Fatalf("receipt differs from production\ngot %s\nwant %s", got, want)
	}
}
