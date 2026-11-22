package app

import (
	"bytes"
	cf "compare-figure"
	"compare-figure/distribution"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func buildCLI(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(errOut)
	resource := fs.String("resource-root", "", "source resource root")
	target := fs.String("out", "", "platform binary directory")
	platformList := fs.String("platforms", "linux/amd64,linux/arm64,windows/amd64,darwin/amd64,darwin/arm64", "comma-separated GOOS/GOARCH")
	compiler := fs.String("go", "", "Go 1.25+ executable")
	if e := fs.Parse(args); e != nil {
		return writeError(out, e, 2)
	}
	root, e := distribution.FindRoot(*resource)
	if e != nil {
		return writeError(out, e, 2)
	}
	source := filepath.Join(root, "go")
	if _, e = os.Stat(filepath.Join(source, "go.mod")); e != nil {
		return writeError(out, fmt.Errorf("build requires development Go sources; installed Skills contain ready native binaries"), 2)
	}
	if *target == "" {
		*target = filepath.Join(root, "build", "platforms")
	}
	directory, e := filepath.Abs(*target)
	if e != nil {
		return writeError(out, e, 2)
	}
	binary := *compiler
	if binary == "" {
		binary = os.Getenv("COMPARE_FIGURE_GO")
	}
	if binary == "" {
		binary = "go"
	}
	resolved, e := exec.LookPath(binary)
	if e != nil {
		return writeError(out, e, 2)
	}
	records := []map[string]any{}
	for _, platform := range strings.Split(*platformList, ",") {
		parts := strings.Split(strings.TrimSpace(platform), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return writeError(out, fmt.Errorf("platform must be GOOS/GOARCH: %s", platform), 2)
		}
		for _, part := range parts {
			for _, r := range part {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
					return writeError(out, fmt.Errorf("invalid platform %s", platform), 2)
				}
			}
		}
		name := "compare-figure"
		if parts[0] == "windows" {
			name += ".exe"
		}
		artifact := filepath.Join(directory, parts[0]+"-"+parts[1], name)
		if e = os.MkdirAll(filepath.Dir(artifact), 0755); e != nil {
			return writeError(out, e, 2)
		}
		command := exec.CommandContext(ctx, resolved, "build", "-trimpath", "-ldflags=-s -w -buildid=", "-o", artifact, "./cmd/compare-figure")
		command.Dir = source
		command.Env = environment(os.Environ(), map[string]string{"GOOS": parts[0], "GOARCH": parts[1], "CGO_ENABLED": "0", "GOCACHE": filepath.Join(root, "build", "go-cache")})
		var log bytes.Buffer
		command.Stdout = io.MultiWriter(errOut, &log)
		command.Stderr = io.MultiWriter(errOut, &log)
		if e = command.Run(); e != nil {
			return writeError(out, fmt.Errorf("build %s: %w: %s", platform, e, log.String()), 2)
		}
		records = append(records, map[string]any{"platform": platform, "path": artifact, "compiled": true, "executed": false})
	}
	result := map[string]any{"schema": "compare-figure-build/1", "ok": true, "runtime": "go", "binaries": records}
	data, _ := json.MarshalIndent(result, "", "  ")
	if e = cf.AtomicWrite(filepath.Join(directory, "build.json"), append(data, '\n'), 0644); e != nil {
		return writeError(out, e, 2)
	}
	emit(out, result)
	return 0
}
func environment(base []string, overrides map[string]string) []string {
	result := []string{}
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := overrides[key]; !ok {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}
