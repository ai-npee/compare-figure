package drawing

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func clampCropArgs(a map[string]any) (float64, float64, float64, float64, error) {
	clamp := func(value, fallback float64) float64 {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fallback
		}
		return max(0, min(100, value))
	}
	left := clamp(number(a["left"], 0), 0)
	top := clamp(number(a["top"], 0), 0)
	right := clamp(number(a["right"], 100), 100)
	bottom := clamp(number(a["bottom"], 100), 100)
	if right <= left || bottom <= top {
		return 0, 0, 0, 0, fmt.Errorf("crop must satisfy 0 <= left < right <= 100 and 0 <= top < bottom <= 100")
	}
	return left, top, right, bottom, nil
}

var pdfPagesRE = regexp.MustCompile(`(?m)^Pages:\s*(\d+)\s*$`)
var pdfPageSizeRE = regexp.MustCompile(`(?m)^Page\s+\d+\s+size:\s*([\d.]+)\s+x\s+([\d.]+)\s+pts`)

func pdfPageInfo(ctx context.Context, path string, page int) (int, float64, float64, error) {
	cmdctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdctx, "pdfinfo", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-box", path)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("pdfinfo: %w: %s", err, strings.TrimSpace(string(data)))
	}
	text := string(data)
	pages := 0
	if match := pdfPagesRE.FindStringSubmatch(text); len(match) == 2 {
		pages, _ = strconv.Atoi(match[1])
	}
	var width, height float64
	if match := pdfPageSizeRE.FindStringSubmatch(text); len(match) == 3 {
		width, _ = strconv.ParseFloat(match[1], 64)
		height, _ = strconv.ParseFloat(match[2], 64)
		width, height = width*25.4/72, height*25.4/72
	}
	return pages, width, height, nil
}
