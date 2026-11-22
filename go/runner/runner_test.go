package runner

import (
	"bytes"
	"compare-figure/distribution"
	"compare-figure/luadraw"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const fixtureLua = `local ld = luadraw
local g = ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}
g:Dline(-1,1,"red,thick")
g:Dcircle(0,1,nil,"blue")
g:Show()
`

func realFixture(t *testing.T) (string, string) {
	t.Helper()
	resources, e := distribution.FindRoot("")
	if e != nil {
		t.Skip(e)
	}
	status, e := luadraw.Probe(context.Background(), filepath.Join(resources, "vendor", "luadraw-3.7", "luadraw"))
	if e != nil || !Bool(status, "ready", false) {
		t.Skip("no real LuaDraw compiler", e, status)
	}
	base := t.TempDir()
	source := filepath.Join(base, "reference.lua")
	if e = os.WriteFile(source, []byte(fixtureLua), 0600); e != nil {
		t.Fatal(e)
	}
	built, e := luadraw.Compile(context.Background(), source, filepath.Join(base, "producer"), Object{"resource_root": resources, "luadraw_root": filepath.Join(resources, "vendor", "luadraw-3.7", "luadraw"), "helper_path": filepath.Join(resources, "scripts", "luadraw_runtime.lua")})
	if e != nil || !Bool(built, "ok", false) {
		t.Fatal(e, built)
	}
	reference, e := rasterize(context.Background(), Str(built, "pdf", ""), filepath.Join(base, "reference"))
	if e != nil {
		t.Fatal(e)
	}
	return resources, reference
}
func modelCalls() []Object {
	args := func(value Object) string { data, _ := json.Marshal(value); return string(data) }
	return []Object{{"id": "write", "type": "function", "function": Object{"name": "write_file", "arguments": args(Object{"path": "figure.lua", "content": fixtureLua})}}, {"id": "compile", "type": "function", "function": Object{"name": "compile", "arguments": "{}"}}, {"id": "compare", "type": "function", "function": Object{"name": "compare_figure", "arguments": args(Object{"detail": "gray"})}}, {"id": "submit", "type": "function", "function": Object{"name": "submit", "arguments": args(Object{"path": "figure.lua"})}}}
}
func setupModel(t *testing.T) (string, *atomic.Int64, *atomic.Bool) {
	t.Helper()
	count := &atomic.Int64{}
	imageSeen := &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		raw, e := io.ReadAll(r.Body)
		if e != nil {
			t.Error(e)
		}
		if bytes.Contains(raw, []byte("data:image/")) {
			imageSeen.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Object{"choices": []any{Object{"message": Object{"role": "assistant", "content": "", "tool_calls": modelCalls()}, "finish_reason": "tool_calls"}}, "usage": Object{"prompt_tokens": 200, "completion_tokens": 50}})
	}))
	t.Cleanup(server.Close)
	return server.URL + "/v1", count, imageSeen
}
func runnerOptions(t *testing.T, resources, reference, endpoint string) Options {
	t.Helper()
	config := filepath.Join(t.TempDir(), "model.json")
	if e := atomicJSON(config, Object{"base_url": endpoint, "model": "fake-vision", "api_key_env": "COMPARE_FIGURE_TEST_KEY", "stream": false}); e != nil {
		t.Fatal(e)
	}
	threshold, scale := 90.0, -1.0
	return Options{Original: reference, Context: "Synthetic fixture", Out: filepath.Join(t.TempDir(), "run"), Config: config, ResourceRoot: resources, PassThreshold: &threshold, MaxScaleDev: &scale}
}

func TestRunnerRealCompileDeliveryIdempotentWithoutModelKey(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, imageSeen := setupModel(t)
	opts := runnerOptions(t, resources, reference, endpoint)
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "fake-key-never-export")
	result, e := Run(context.Background(), opts)
	if e != nil || !Bool(result, "submitted", false) {
		t.Fatal(e, result)
	}
	if count.Load() != 1 || !imageSeen.Load() {
		t.Fatal("expected one tool batch with reference", count.Load(), imageSeen.Load())
	}
	if !strings.HasSuffix(Str(result, "editable_source", ""), ".lua") {
		t.Fatal(result)
	}
	path := filepath.Join(opts.Out, "result.json")
	before, _ := os.ReadFile(path)
	transcriptBefore, _ := os.ReadFile(filepath.Join(opts.Out, "session.jsonl"))
	if bytes.Contains(transcriptBefore, []byte("fake-key-never-export")) || bytes.Contains(transcriptBefore, []byte("data:image")) {
		t.Fatal("key or inline media leaked into transcript")
	}
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "")
	again, e := Run(context.Background(), opts)
	if e != nil || !Bool(again, "submitted", false) || Bool(again, "api_contacted", true) {
		t.Fatal(e, again)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) || count.Load() != 1 {
		t.Fatal("completed repeat changed result or contacted model")
	}
	changed := opts
	changed.Context = "different task"
	if _, e = Run(context.Background(), changed); e == nil {
		t.Fatal("different input accepted into same workspace")
	}
}

func TestRunnerAcceptedStateRecoversMissingDeliveryWithoutModel(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, _ := setupModel(t)
	opts := runnerOptions(t, resources, reference, endpoint)
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "fake-key")
	if _, e := Run(context.Background(), opts); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(opts.Out, "result.json")
	if !inside(opts.Out, path) {
		t.Fatal("unexpected delete target")
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "")
	result, e := Run(context.Background(), Options{Out: opts.Out, ResourceRoot: resources, Resume: true})
	if e != nil || !Bool(result, "submitted", false) || Bool(result, "api_contacted", true) {
		t.Fatal(e, result)
	}
	if count.Load() != 1 {
		t.Fatal("recovery called model", count.Load())
	}
}

func TestRunnerAuthFailureRerunStillSendsOriginalTask(t *testing.T) {
	resources, reference := realFixture(t)
	endpoint, count, imageSeen := setupModel(t)
	opts := runnerOptions(t, resources, reference, endpoint)
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "")
	if _, e := Run(context.Background(), opts); e == nil {
		t.Fatal("missing key accepted")
	}
	t.Setenv("COMPARE_FIGURE_TEST_KEY", "fake-key")
	result, e := Run(context.Background(), opts)
	if e != nil || !Bool(result, "submitted", false) || count.Load() != 1 || !imageSeen.Load() {
		t.Fatal("initialization retry lost task image", e, result, count.Load(), imageSeen.Load())
	}
}

func TestPreflightDoesNotCreateSessionOrNeedModel(t *testing.T) {
	resources, reference := realFixture(t)
	out := filepath.Join(t.TempDir(), "not-created")
	result, e := Run(context.Background(), Options{Original: reference, Out: out, ResourceRoot: resources, Check: true})
	if e != nil || !Bool(result, "check", false) {
		t.Fatal(e, result)
	}
	if _, e = os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("preflight created session", e)
	}
}
