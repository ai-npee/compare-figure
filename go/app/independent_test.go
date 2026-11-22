package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneMarkdownCSVCompareAndToolContext(t *testing.T) {
	book := t.TempDir()
	images := filepath.Join(book, "images", "书名")
	if err := os.MkdirAll(images, 0755); err != nil {
		t.Fatal(err)
	}
	reference := writeAgentTestImage(t, images, "figure.png")
	csv := filepath.Join(images, "sizes.csv")
	if err := os.WriteFile(csv, []byte("# scale_px_per_pt=1.1361\nfile,width_mm,height_mm,page_w_mm\nfigure.png,86.63,103.71,143.93\n"), 0600); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(book, "书名.md")
	if err := os.WriteFile(md, []byte("# Geometry\nOriginal context here\n![](images/书名/figure.png)\nAdditional explanation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "comparison")
	result := callNative(t, "compare", "--source-md", md, "--image-ref", "images/书名/figure.png", "--candidate", reference, "--candidate-mm", "86.63,103.71", "--out", out)
	measured := result["reference_measurement"].(map[string]any)
	if measured["source"] != "sizes_csv" || measured["width_mm"] != 86.63 || measured["page_w_mm"] != 143.93 || result["pass"] != true || !strings.Contains(result["text"].(string), "physical ink") {
		t.Fatal(result)
	}
	workspace := filepath.Join(t.TempDir(), "agent")
	contextResult := callNative(t, "tool", "--workspace", workspace, "--source-md", md, "--image-ref", "images/书名/figure.png", "--name", "image_context")
	if !strings.Contains(contextResult["text"].(string), "Original context here") {
		t.Fatal(contextResult)
	}
	record, err := loadAgentRecord(filepath.Join(workspace, agentStateName))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(record.Options["measurement"])
	if !bytes.Contains(encoded, []byte("sizes_csv")) {
		t.Fatal(string(encoded))
	}
	request := map[string]any{"operation": "compare", "source_md": md, "image_ref": "images/书名/figure.png", "candidate": reference, "candidate_mm": []float64{86.63, 103.71}, "out_dir": out}
	response, err := (&service{}).handle(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(response)
	if !bytes.Contains(data, []byte("physical ink")) {
		t.Fatal(string(data))
	}
}
