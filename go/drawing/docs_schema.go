package drawing

func docsToolDefinition() map[string]any {
	return toolSchema("luadraw_doc", "Search/read/view LuaDraw docs. Search returns text and PDF hit locations only by default; it does not attach an image. Use mode=view to render a whole page or crop; left/top/right/bottom are page percentages. Search bbox_pts are PDF points, not percentages; bbox_pct is provided for model-directed cropping.", map[string]any{
		"mode":  map[string]any{"type": "string", "enum": []string{"search", "read", "view", "list", "find", "grep", "frequent"}, "description": "search, safe source read, PDF view, root list/find/grep, or frequent excerpts"},
		"query": prop("string", "search terms or source document path"), "doc": prop("string", "document path relative to the LuaDraw resource root"),
		"page": prop("integer", "1-based manual PDF page for text read or image view"), "left": prop("number", "crop left page percentage (0-100)"), "top": prop("number", "crop top page percentage (0-100)"),
		"right": prop("number", "crop right page percentage (0-100)"), "bottom": prop("number", "crop bottom page percentage (0-100)"), "zoom": prop("integer", "target image width in pixels"),
		"limit": prop("integer", "search/frequent result limit"), "offset": prop("integer", "search result offset"), "start_line": prop("integer", "source read first line"), "end_line": prop("integer", "source read last line"),
		"snippet_chars": prop("integer", "search snippet characters, default 1200, 64-8000"), "max_chars": prop("integer", "search/frequent total characters, default 6000/4096, 64-32000"), "line_chars": prop("integer", "frequent line characters, default 256"), "regex": prop("boolean", "grep regex mode; default literal"),
		"entry_id": prop("string", "Stable short or full frequent paragraph ID"), "recursive": prop("boolean", "List nested directories; default false"),
		"include_image": map[string]any{"type": "boolean", "default": false, "description": "Search only: explicitly attach one relevant PDF crop; default false. Ignored by view, which always renders"},
	})
}
