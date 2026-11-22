package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWireModelAliasPreservesFileOrder(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		data := []byte(`{"models":{"z-first":{"model":"same-wire","temperature":0.2},"a-second":{"model":"same-wire","temperature":0.8}}}`)
		if format == "yaml" {
			data = []byte("models:\n  z-first:\n    model: same-wire\n    temperature: 0.2\n  a-second:\n    model: same-wire\n    temperature: 0.8\n")
		}
		file := filepath.Join(t.TempDir(), "config."+format)
		if e := os.WriteFile(file, data, 0600); e != nil {
			t.Fatal(e)
		}
		config, e := LoadConfig(file)
		if e != nil {
			t.Fatal(e)
		}
		for repeat := 0; repeat < 10; repeat++ {
			entry, e := ModelConfig(config, "same-wire")
			if e != nil || Num(entry, "temperature", 0) != .2 {
				t.Fatal(format, e, entry)
			}
		}
	}
}
