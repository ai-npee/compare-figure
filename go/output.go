package comparefigure

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultOutput reuses one scratch slot per operation and working directory.
// It never makes a new directory for each request. Callers needing a history
// must select OutDir explicitly and copy artifacts before their next call.
func DefaultOutput(operation string) (string, error) {
	switch operation {
	case "compare", "docs", "compile", "view":
	default:
		return "", fmt.Errorf("unknown scratch operation %q", operation)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if resolved, e := filepath.EvalSymlinks(wd); e == nil {
		wd = resolved
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	root := os.Getenv("COMPARE_FIGURE_OUTPUT_ROOT")
	if root == "" {
		home, _ := os.UserHomeDir()
		identity := sha256.Sum256([]byte(home))
		root = filepath.Join(os.TempDir(), "compare-figure-"+hex.EncodeToString(identity[:6]), "latest")
	}
	sum := sha256.Sum256([]byte(wd))
	return filepath.Abs(filepath.Join(root, hex.EncodeToString(sum[:8]), operation))
}
