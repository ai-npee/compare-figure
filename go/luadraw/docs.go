package luadraw

import (
	"context"

	"compare-figure/documents"
)

// Docs is the compatibility adapter for existing LuaDraw documentation clients.
// The shared doc-library dependency owns all indexing and cache implementation.
type Docs struct{ docs *documents.Docs }

// NewDocs keeps accepting the LuaDraw vendor root and caller-selected cache.
func NewDocs(root, cache string) (*Docs, error) {
	docs, err := documents.New(root, cache, "luadraw")
	if err != nil {
		return nil, err
	}
	return &Docs{docs: docs}, nil
}

// Definition preserves the public luadraw_doc tool alias.
func (d *Docs) Definition() map[string]any {
	definition := d.docs.Definition()
	definition["function"].(map[string]any)["name"] = "luadraw_doc"
	return definition
}
func (d *Docs) Execute(ctx context.Context, args map[string]any, outdir string) (string, string, error) {
	return d.docs.Execute(ctx, args, outdir)
}
func (d *Docs) ExecuteResponse(ctx context.Context, args map[string]any, outdir string) (map[string]any, error) {
	return d.docs.ExecuteResponse(ctx, args, outdir)
}
func (d *Docs) FrequentSnippet(ctx context.Context, limit, lineChars, maxChars int) (string, error) {
	return d.docs.FrequentSnippet(ctx, limit, lineChars, maxChars)
}
