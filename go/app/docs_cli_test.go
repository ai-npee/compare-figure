package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/distribution"
)

func docsTestResources(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"pdftotext", "pdfinfo", "pdftoppm"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("Poppler unavailable: %s", name)
		}
	}
	root, err := distribution.FindRoot("")
	if err != nil {
		t.Skipf("LuaDraw resources unavailable: %v", err)
	}
	return root
}

func runDocsCLI(t *testing.T, args []string, input string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := Run(context.Background(), append([]string{"luadraw-doc"}, args...), strings.NewReader(input), &out, &out)
	return code, out.String()
}

func decodeDocsResult(t *testing.T, output string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON result: %v: %s", err, output)
	}
	return result
}

func TestLuaDrawDocsCLIJSONArgsEnvelopeAndCache(t *testing.T) {
	resources := docsTestResources(t)
	cache := filepath.Join(t.TempDir(), "chosen-cache")
	requests := `{"mode":"search","query":"Dpolyline3d","limit":1}` + "\n" +
		`{"operation":"docs","arguments":{"mode":"read","doc":"files/luadraw_base.lua","start_line":1,"end_line":2},"cache_dir":""}`
	code, output := runDocsCLI(t, []string{"--jsonl", "--output-version=1", "--resource-root", resources, "--cache-dir", cache}, requests)
	if code != 0 {
		t.Fatalf("CLI returned %d: %s", code, output)
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	var results []map[string]any
	for scanner.Scan() {
		results = append(results, decodeDocsResult(t, scanner.Text()))
	}
	if len(results) != 2 || results[0]["ok"] != true || !strings.Contains(results[0]["text"].(string), "Dpolyline3d") {
		t.Fatalf("bare tool arguments did not search: %#v", results)
	}
	if results[1]["ok"] != true || !strings.Contains(results[1]["text"].(string), "luadraw_base.lua") {
		t.Fatalf("tools envelope did not read without an image/model: %#v", results)
	}
	cacheInfo, ok := results[0]["cache"].(map[string]any)
	if !ok || cacheInfo["namespace"] == "" || cacheInfo["misses"].(float64) == 0 {
		t.Fatalf("index/cache counters missing: %#v", results[0])
	}
	if entries, err := os.ReadDir(cache); err != nil || len(entries) == 0 {
		t.Fatalf("cache was not written under --cache-dir: entries=%v err=%v", entries, err)
	}

	freqRequest := `{"mode":"frequent","limit":8,"max_chars":2048}`
	code, output = runDocsCLI(t, []string{"--stdin", "--output-version=1", "--resource-root", resources, "--cache-dir", cache}, freqRequest)
	if code != 0 {
		t.Fatalf("frequent failed: %d %s", code, output)
	}
	result := decodeDocsResult(t, output)
	cacheInfo, ok = result["cache"].(map[string]any)
	if result["ok"] != true || result["text"] == "" || !ok || cacheInfo["misses"].(float64) != 0 {
		t.Fatalf("frequent did not return a warm indexed result: %#v", result)
	}
}

func TestLuaDrawDocsCLIJSONLContinuesAndRejectsOtherOperations(t *testing.T) {
	resources := docsTestResources(t)
	cache := filepath.Join(t.TempDir(), "cache")
	requests := "{not json}\n" + `{"id":"bad-op","operation":"run","arguments":{}}` + "\n" + `{"mode":"search","query":"Dpolyline3d","limit":1}` + "\n"
	code, output := runDocsCLI(t, []string{"--jsonl", "--output-version=1", "--resource-root", resources, "--cache-dir", cache}, requests)
	if code != 1 {
		t.Fatalf("expected aggregate error exit 1, got %d: %s", code, output)
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	var results []map[string]any
	for scanner.Scan() {
		results = append(results, decodeDocsResult(t, scanner.Text()))
	}
	if len(results) != 3 {
		t.Fatalf("expected one output per nonblank line; got %d: %s", len(results), output)
	}
	if results[0]["ok"] != false || results[1]["ok"] != false || results[1]["id"] != "bad-op" {
		t.Fatalf("invalid JSON and run operation must be isolated errors: %#v", results[:2])
	}
	if results[2]["ok"] != true || !strings.Contains(results[2]["text"].(string), "Dpolyline3d") {
		t.Fatalf("valid request after errors did not succeed: %#v", results[2])
	}
}

func TestLuaDrawDocsCLIJSONOptionsOverrideFlagDefaults(t *testing.T) {
	resources := docsTestResources(t)
	defaultCache := filepath.Join(t.TempDir(), "default-cache")
	jsonCache := filepath.Join(t.TempDir(), "json-cache")
	request := `{"operation":"docs","arguments":{"mode":"search","query":"Dpolyline3d","limit":1},"cache_dir":"` + filepath.ToSlash(jsonCache) + `"}`
	code, output := runDocsCLI(t, []string{"--request", "-", "--resource-root", resources, "--cache-dir", defaultCache, "--query", "does-not-exist"}, request)
	if code != 0 {
		t.Fatalf("request via stdin failed: %d %s", code, output)
	}
	if entries, err := os.ReadDir(jsonCache); err != nil || len(entries) == 0 {
		t.Fatalf("JSON cache_dir did not override flag default: entries=%v err=%v", entries, err)
	}
	if _, err := os.Stat(defaultCache); !os.IsNotExist(err) {
		t.Fatalf("default cache unexpectedly used: err=%v", err)
	}
}

func TestLuaDrawDocsCLIRejectsInputModeMixing(t *testing.T) {
	code, output := runDocsCLI(t, []string{"--stdin", "--jsonl"}, "")
	if code != 2 || !strings.Contains(output, "mutually exclusive") {
		t.Fatalf("expected mutually exclusive input error, got %d %s", code, output)
	}
}
