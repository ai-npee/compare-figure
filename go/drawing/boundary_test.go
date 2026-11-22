package drawing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceSymlinkBoundaryAndDynamicThreshold(t *testing.T) {
	h := testHarness(t, nil)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(h.root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := h.safe("escape/new/draft.lua"); err == nil {
		t.Fatal("nonexistent descendant through outside symlink must be rejected")
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("needle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if text, _, err := h.Execute(t.Context(), "read_file", map[string]any{"path": "escape/secret.txt"}); err != nil || !strings.HasPrefix(text, "ERROR: 路径越界") || strings.Contains(text, "needle") {
		t.Fatalf("symlink target escaped: %q %v", text, err)
	}
	live := 0.0
	h.Options["live_threshold"] = func() float64 { return live }
	if got := h.liveThreshold(); got != .17 {
		t.Fatalf("zero live threshold must retain configured default: %v", got)
	}
	live = -1
	if got := h.liveThreshold(); got != -1 {
		t.Fatalf("negative live threshold must disable overlap gate: %v", got)
	}
}

func TestViewBudgetWarnsThenReportsSpent(t *testing.T) {
	h := testHarness(t, map[string]any{"view_pdf_max": 2, "warn_ratio": .7})
	if got := h.budgetNote("view_pdf", "drawing.pdf#1", 1, 2); got != "" {
		t.Fatalf("early budget note: %q", got)
	}
	if got := h.budgetNote("view_pdf", "drawing.pdf#1", 2, 2); !strings.Contains(got, "VIEW BUDGET:") {
		t.Fatalf("warning note: %q", got)
	}
	if got := h.budgetNote("view_pdf", "drawing.pdf#1", 3, 2); !strings.Contains(got, "VIEW BUDGET SPENT") {
		t.Fatalf("spent note: %q", got)
	}
}
