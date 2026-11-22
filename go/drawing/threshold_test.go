package drawing

import (
	"strings"
	"testing"
	"time"
)

func TestExplicitZeroThresholdAndAttemptLimitDisableGates(t *testing.T) {
	h := testHarness(t, map[string]any{"accept_min_submits": 0, "max_scale_dev": -1.0})
	zero, err := New(h.root, h.reference, "", 0, time.Second, map[string]any{"accept_min_submits": 0, "max_scale_dev": -1.0})
	if err != nil {
		t.Fatal(err)
	}
	if zero.MinOverlap != 0 || int(number(zero.Options["accept_min_submits"], -1)) != 0 {
		t.Fatalf("explicit zero overwritten: threshold=%v options=%#v", zero.MinOverlap, zero.Options)
	}
	zero.State["last_code"] = "local g=luadraw\n"
	zero.State["compile_ok"] = true
	zero.State["compare_called"] = true
	zero.State["best_overlap"] = .01
	zero.State["scale_dev_at_best"] = .5
	zero.State["size_unit_at_best"] = "aspect"
	zero.State["aspect_ref_at_best"] = 1
	zero.State["aspect_cand_at_best"] = 1.5
	text, err := zero.submit(map[string]any{"code": "local g=luadraw\n", "accept_as_is": true, "note": "known mismatch, no more changes possible"})
	if err != nil || !strings.HasPrefix(text, "SUBMITTED as best-effort") {
		t.Fatalf("explicit zero should enable immediate best effort: %s %v", text, err)
	}
	if zero.Snapshot()["_accepted_threshold"] != float64(0) {
		t.Fatalf("accepted threshold not saved: %#v", zero.Snapshot())
	}
}
