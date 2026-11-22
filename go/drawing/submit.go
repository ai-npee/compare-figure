package drawing

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func (h *Harness) submit(a map[string]any) (string, error) {
	threshold := h.liveThreshold()
	h.MinOverlap = threshold
	code := stringValue(a["code"], "")
	path := strings.TrimSpace(stringValue(a["path"], ""))
	if path != "" {
		p, err := h.safe(path)
		if err != nil {
			return "", err
		}
		data, err := readFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			return "NOT FOUND: " + path + "（先 write_file 写入最终代码）", nil
		}
		if err != nil {
			return "", err
		}
		code = string(data)
	}
	if strings.TrimSpace(code) == "" {
		return "ERROR: code 为空（或 path 文件不存在）", nil
	}
	h.State["submit_attempts"] = int(number(h.State["submit_attempts"], 0)) + 1
	if !truth(h.State["compare_called"]) {
		return "REJECTED: call compare_figure at least once after a successful compile, inspect its comparison, and then submit.", nil
	}
	overlap, deviation := number(h.State["best_overlap"], 0), number(h.State["scale_dev_at_best"], 0)
	attempts, minimum := int(number(h.State["submit_attempts"], 0)), int(number(h.Options["accept_min_submits"], 8))
	bestEffort, note := truth(a["accept_as_is"]), strings.TrimSpace(stringValue(a["note"], ""))
	why := []string{}
	if threshold > 0 && overlap < threshold {
		why = append(why, fmt.Sprintf("软墨重叠 %.2f%% < 门槛 %.2f%%（差 %.2f 个百分点）", overlap*100, threshold*100, (threshold-overlap)*100))
	}
	limit := h.MaxScaleDev
	if limit > 0 && deviation > limit {
		switch h.State["size_unit_at_best"] {
		case "mm":
			why = append(why, fmt.Sprintf("墨迹尺寸偏差 %.1f%% > 上限 %.1f%%（宽 %.1fmm → %.1fmm = 原图的 %.2f 倍；高 %.1fmm → %.1fmm = %.2f 倍）", deviation*100, limit*100, number(h.State["ink_ref_w_at_best"], 0), number(h.State["ink_cand_w_at_best"], 0), number(h.State["best_scale_x"], 0), number(h.State["ink_ref_h_at_best"], 0), number(h.State["ink_cand_h_at_best"], 0), number(h.State["best_scale_y"], 0)))
		case "aspect":
			why = append(why, fmt.Sprintf("墨迹宽高比偏差 %.1f%% > 上限 %.1f%%（原图 %.2f:1，你画的是 %.2f:1；物理尺寸查不到，只能比形状）", deviation*100, limit*100, number(h.State["aspect_ref_at_best"], 0), number(h.State["aspect_cand_at_best"], 0)))
		default:
			why = append(why, fmt.Sprintf("宽高尺寸偏差 %.1f%% > 上限 %.1f%%（宽的 %.2f 倍、高的 %.2f 倍）", deviation*100, limit*100, number(h.State["best_scale_x"], 0), number(h.State["best_scale_y"], 0)))
		}
	}
	if bestEffort {
		if minimum > 0 && attempts < minimum {
			return "REJECTED: " + fallbackWhy(why) + fmt.Sprintf("。这张图目前只交过 %d 次 submit，先把图改好再交。", attempts-1), nil
		}
		if note == "" {
			return "REJECTED: accept_as_is needs a note saying what could not be matched and why it is as good as it gets.", nil
		}
	} else if len(why) > 0 {
		message := "REJECTED: " + strings.Join(why, "；") + "。先改图再 compile + compare_figure 重新提交。"
		if minimum <= 0 || attempts >= minimum {
			message += fmt.Sprintf("这张图已经交过 %d 次：如果确实只能做到这儿，可以 submit {accept_as_is:true, note:\"差在哪里、为什么到不了\"} 交这一版（记为尽力还原，之后不会自动重画）。", attempts)
		}
		return message, nil
	}
	if !truth(h.State["compile_ok"]) || strings.TrimSpace(code) != strings.TrimSpace(stringValue(h.State["last_code"], "")) {
		return "REJECTED: the submitted code differs from the last successful compile. Call compile {path} with this exact file first.", nil
	}
	h.State["submitted"], h.State["final_code"] = true, code
	h.State["uncertain"] = strings.Contains(code, "[?]")
	h.State["accepted_as_is"], h.State["accept_note"] = bestEffort, ""
	if bestEffort {
		h.State["accept_note"] = note
	}
	snapshot := cloneMap(h.State)
	snapshot["_accepted_threshold"], snapshot["_accepted_scale_limit"] = threshold, limit
	h.State["accepted_snapshot"] = snapshot
	if bestEffort {
		return fmt.Sprintf("SUBMITTED as best-effort (overlap %.2f%%, size deviation %.1f%%): recorded so this figure is not redrawn automatically. %s", overlap*100, deviation*100, note), nil
	}
	return fmt.Sprintf("SUBMITTED. The figure is accepted (overlap %.2f%%, size deviation %.1f%%).", overlap*100, deviation*100), nil
}

var readFile = func(path string) ([]byte, error) { return os.ReadFile(path) }
