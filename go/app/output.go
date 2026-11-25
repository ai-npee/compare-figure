package app

import (
	cf "compare-figure"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// retainDocumentImage publishes one preview per output slot, then removes only
// the uniquely named render created inside that exact request directory.
func retainDocumentImage(target, generated string) (string, error) {
	directory, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	source, err := filepath.Abs(generated)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(directory, source)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("document preview escaped output directory")
	}
	stable := filepath.Join(directory, "预览", "document.png")
	if source == stable {
		return stable, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	if err = cf.AtomicWrite(stable, data, 0644); err != nil {
		return "", err
	}
	// The library returns a newly created private image path for this request.
	if err = os.Remove(source); err != nil {
		return "", err
	}
	return stable, nil
}
