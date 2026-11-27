package drawing

func docsToolDefinition() map[string]any {
	return toolSchema("document_search", "Search/read/view configured drawing documentation. Search returns text and PDF hit locations by default; view renders a PDF page/crop. Positions are source-qualified.", map[string]any{
		"mode":    map[string]any{"type": "string", "enum": []string{"search", "read", "view", "list", "find", "grep", "frequent"}, "description": "search, safe source read, PDF view, root list/find/grep, or frequent excerpts"},
		"source":  prop("string", "Exact configured source ID: luadraw (upstream), luadraw-notes (project experience, not upstream, if configured), or asymptote. Omit source to search all selected sources or use sources for several. For read/view use the exact returned source ID."),
		"sources": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Exact configured source IDs, mutually exclusive with source. Omitting both uses all sources configured for the backend; selectors do not expand."},
		"query":   prop("string", "search terms or source document path"), "doc": prop("string", "document path relative to the selected source root"),
		"page": prop("integer", "1-based manual PDF page for text read or image view"),
		"region": map[string]any{"type": "object", "properties": map[string]any{
			"left": prop("number", "crop left page percentage (0-100)"), "top": prop("number", "crop top page percentage (0-100)"),
			"right": prop("number", "crop right page percentage (0-100)"), "bottom": prop("number", "crop bottom page percentage (0-100)"),
		}, "required": []string{"left", "top", "right", "bottom"}, "additionalProperties": false},
		"zoom":  prop("integer", "target image width in pixels"),
		"limit": prop("integer", "search/frequent result limit"), "offset": prop("integer", "search result offset"), "start_line": prop("integer", "source read first line"), "end_line": prop("integer", "source read last line"),
		"snippet_chars": prop("integer", "search snippet characters, default 1200, 64-8000"), "max_chars": prop("integer", "search/frequent total characters, default 6000/4096, 64-32000"), "line_chars": prop("integer", "frequent line characters, default 256"), "regex": prop("boolean", "grep regex mode; default literal"),
		"entry_id": prop("string", "Stable short or full frequent paragraph ID"), "recursive": prop("boolean", "List nested directories; default false"),
		"output_version": map[string]any{"type": "integer", "enum": []int{1, 2}, "description": "Output protocol override; host defaults to compact version 2"},
		"include_bbox":   prop("boolean", "Return available percentage regions; omit to inherit startup default, explicit false disables them"),
		"include_image":  map[string]any{"type": "boolean", "default": false, "description": "Search only: explicitly attach one relevant PDF crop; default false. Ignored by view, which always renders"},
	})
}
