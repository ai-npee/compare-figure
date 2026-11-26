package documents

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
)

// Suggestions use the dependency's filtered navigator rather than maintaining
// a second filesystem walk or fuzzy index in the adapter.
func (d *Docs) missingPathError(ctx context.Context, request Request, cause error) error {
	name := request.Doc
	if name == "" {
		name = request.Query
	}
	name = strings.Split(name, "#page=")[0]
	parent := path.Dir(name)
	response, err := d.library.Execute(ctx, Request{Mode: "list", Source: request.Source, Sources: request.Sources, Doc: parent, Limit: 32})
	if err != nil || len(response.Results) == 0 {
		return fmt.Errorf("%w; use mode=list or mode=find to locate an actual allowed path", cause)
	}
	sort.SliceStable(response.Results, func(i, j int) bool {
		return response.Results[i].Kind == "directory" && response.Results[j].Kind != "directory"
	})
	paths := make([]string, 0, len(response.Results))
	for _, r := range response.Results {
		paths = append(paths, r.Source+":"+r.Doc)
	}
	return fmt.Errorf("%w; actual paths in the requested directory (up to 32; directories first): %s; list files/directories with mode=list doc=%q or locate the filename with mode=find", cause, strings.Join(paths, ", "), parent)
}
