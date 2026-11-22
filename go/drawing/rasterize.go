package drawing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Rasterize renders the first PDF page using the session's configured Poppler
// executable and DPI, returning the generated PNG path.
func (h *Harness) Rasterize(ctx context.Context, pdf, prefix string) (string, error) {
	abs, err := filepath.Abs(prefix)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return "", err
	}
	command := stringValue(h.Options["raster_command"], "pdftoppm")
	dpi := int(number(h.Options["raster_dpi"], 110))
	tmp, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(tmp, command, "-png", "-r", strconv.Itoa(dpi), "-singlefile", pdf, abs)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", command, err, strings.TrimSpace(string(data)))
	}
	path := abs + ".png"
	if _, err = os.Stat(path); err != nil {
		return "", fmt.Errorf("%s did not produce %s", command, path)
	}
	return path, nil
}

func (h *Harness) rasterizeAtDPI(ctx context.Context, pdf, prefix string) (string, error) {
	return h.Rasterize(ctx, pdf, prefix)
}
