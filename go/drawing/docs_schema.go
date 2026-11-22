package drawing

func docsToolDefinition() map[string]any {
	return toolSchema("luadraw_doc", "Search/read/view LuaDraw docs. Search hits report PDF bbox in points; left/top/right/bottom crop inputs are page percentages.", map[string]any{
		"mode":  map[string]any{"type": "string", "enum": []string{"search", "read", "view"}},
		"query": prop("string", "search terms or source document path"), "doc": prop("string", "document path relative to the LuaDraw resource root"),
		"page": prop("integer", "1-based manual PDF page number"), "left": prop("number", "crop left page percentage"), "top": prop("number", "crop top page percentage"),
		"right": prop("number", "crop right page percentage"), "bottom": prop("number", "crop bottom page percentage"), "zoom": prop("integer", "target image width in pixels"),
		"limit": prop("integer", "search result limit"), "offset": prop("integer", "search result offset"), "start_line": prop("integer", "source read first line"), "end_line": prop("integer", "source read last line"),
		"include_image": prop("boolean", "Attach one relevant PDF crop when available"),
	})
}
