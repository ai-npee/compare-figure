package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigExtendsAndPrototypeLimitInheritance(t *testing.T) {
	config := Object{"models": Object{"text": Object{"api_key_env": "KEY", "max_images": 9, "context_limit": 128000, "max_tokens": 4000}, "base": Object{"model": "vision", "request_body": Object{"a": 1}}, "draw": Object{"extends": "base", "request_body": Object{"b": 2}}}}
	model, e := ModelConfig(config, "draw")
	if e != nil {
		t.Fatal(e)
	}
	if Str(model, "model", "") != "vision" {
		t.Fatal(model)
	}
	if _, ok := model["max_images"]; ok {
		t.Fatal("text image limit inherited")
	}
	if _, ok := model["context_limit"]; ok {
		t.Fatal("text context limit inherited")
	}
	if Num(model, "max_tokens", 0) != 4000 {
		t.Fatal(model)
	}
	if len(Obj(model, "request_body")) != 2 {
		t.Fatal(model)
	}
	if _, e = ModelConfig(Object{"models": Object{"a": Object{"extends": "b"}, "b": Object{"extends": "a"}}}, "a"); e == nil {
		t.Fatal("extends cycle accepted")
	}
}
func TestYAMLConfigAndStageSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(path, []byte("model: native\nlatex:\n  sessions:\n    drawing:\n      max_tool_rounds: 32\n  figure_check:\n    min_overlap: 0.23\ncompile:\n  engine: auto\n"), 0600); e != nil {
		t.Fatal(e)
	}
	config, e := LoadConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	tuning, _, checks, _, compile := StageSettings(config)
	if Num(tuning, "max_tool_rounds", 0) != 32 || Num(checks, "min_overlap", 0) != .23 || Str(compile, "engine", "") != "auto" {
		t.Fatal(config, tuning, checks, compile)
	}
}
func TestMergeNullDeletesWithoutMutatingPrototype(t *testing.T) {
	before := Object{"nested": Object{"keep": 1, "drop": 2}}
	after := Merge(before, Object{"nested": Object{"drop": nil}})
	if len(Obj(after, "nested")) != 1 || len(Obj(before, "nested")) != 2 {
		t.Fatal(before, after)
	}
}
func TestWorkspaceLockRefusesConcurrentRunner(t *testing.T) {
	root := t.TempDir()
	lock, e := acquire(root)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.release()
	if _, e = acquire(root); e == nil {
		t.Fatal("concurrent writer was admitted")
	}
	lock.release()
	next, e := acquire(root)
	if e != nil {
		t.Fatal(e)
	}
	next.release()
}
