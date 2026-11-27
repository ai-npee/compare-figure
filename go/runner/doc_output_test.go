package runner

import (
	"compare-figure/documents"
	"testing"
)

func TestDocumentationOutputForwardingRetainsExplicitFalse(t *testing.T) {
	for _, value := range []bool{false, true} {
		options := Object{}
		config := Object{"docs": Object{"output": Object{"version": 2, "include_bbox": value}}}
		if err := configureDocumentationOutput(config, options); err != nil {
			t.Fatal(err)
		}
		output, err := documents.OutputFromOptions(options)
		if err != nil || output.Version != 2 || output.IncludeBBox == nil || *output.IncludeBBox != value {
			t.Fatalf("forward %t: %#v %v", value, output, err)
		}
	}
	options := Object{}
	if err := configureDocumentationOutput(Object{}, options); err != nil {
		t.Fatal(err)
	}
	if _, exists := options["doc_output"]; exists {
		t.Fatal("missing startup output created an override")
	}
	if err := configureDocumentationOutput(Object{"docs": Object{"output": Object{"version": 7}}}, options); err == nil {
		t.Fatal("invalid protocol was ignored")
	}
}
