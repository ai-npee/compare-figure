package drawing

import "compare-figure/documents"

// Keep fallback and initialized tools on the same small model-facing contract.
// CLI-only library parameters remain accepted by execution, not advertised here.
func docsToolDefinition() map[string]any {
	return (&documents.Docs{}).Definition()
}
