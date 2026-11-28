package documents_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"compare-figure/documents"
)

type noteTestRecord struct {
	Source   string `json:"source"`
	Doc      string `json:"doc"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
	Deleted  bool   `json:"deleted"`
}

type noteTestResponse struct {
	OK    bool             `json:"ok"`
	Notes []noteTestRecord `json:"notes"`
}

func callNote(t *testing.T, d *documents.Docs, args map[string]any) (noteTestResponse, error) {
	t.Helper()
	text, err := d.Note(context.Background(), args)
	if err != nil {
		return noteTestResponse{}, err
	}
	var response noteTestResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("invalid document_note response %q: %v", text, err)
	}
	if !response.OK {
		t.Fatalf("document_note returned ok=false: %s", text)
	}
	return response, nil
}

func TestDocumentNoteDefinitionAndSharedRevisionedCRUD(t *testing.T) {
	definition := documents.NoteDefinition()["function"].(map[string]any)
	if definition["name"] != "document_note" || definition["strict"] != false {
		t.Fatalf("unexpected tool definition: %#v", definition)
	}
	parameters := definition["parameters"].(map[string]any)
	if required, ok := parameters["required"].([]string); !ok || len(required) != 1 || required[0] != "action" {
		t.Fatalf("note schema must require only action: %#v", parameters["required"])
	}
	properties := parameters["properties"].(map[string]any)
	if len(properties) != 8 {
		t.Fatalf("document_note schema has %d properties, want 8: %#v", len(properties), properties)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "official.md"), []byte("read-only reference\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := documents.Config{
		CacheDir: t.TempDir(),
		Sources: []documents.Source{
			{ID: "luadraw", Root: root, NotesDir: "knowledge/luadraw", Extensions: []string{".md"}},
			{ID: "asymptote", Root: root, NotesDir: "knowledge/asymptote", Extensions: []string{".md"}},
			{ID: "official", Root: root, Extensions: []string{".md"}},
		},
	}
	first, err := documents.NewConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := documents.NewConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = callNote(t, first, map[string]any{"action": "create", "title": "Shared method", "content": "Retain the scale and verify the projection."}); err == nil || !strings.Contains(err.Error(), "exactly one writable source") {
		t.Fatalf("mixed writable sources must require an explicit source: %v", err)
	}
	if _, err = callNote(t, first, map[string]any{"action": "create", "source": "official", "title": "Wrong", "content": "must not write"}); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("official source unexpectedly writable: %v", err)
	}

	created, err := callNote(t, first, map[string]any{"action": "create", "source": "luadraw", "title": "Shared method", "content": "Retain the scale and verify the projection."})
	if err != nil || len(created.Notes) != 1 {
		t.Fatalf("create failed: %#v %v", created, err)
	}
	original := created.Notes[0]
	if original.Source != "luadraw" || !strings.HasPrefix(original.Doc, "knowledge/luadraw/") || original.Revision == "" {
		t.Fatalf("create returned invalid note identity: %#v", original)
	}

	read, err := callNote(t, second, map[string]any{"action": "read", "source": "luadraw", "doc": original.Doc})
	if err != nil || len(read.Notes) != 1 || read.Notes[0].Content == "" {
		t.Fatalf("second library instance could not read shared note: %#v %v", read, err)
	}
	updated, err := callNote(t, second, map[string]any{"action": "update", "source": "luadraw", "doc": original.Doc, "revision": original.Revision, "content": "Retain the scale; verify the projection and label."})
	if err != nil || len(updated.Notes) != 1 || updated.Notes[0].Revision == original.Revision {
		t.Fatalf("update failed: %#v %v", updated, err)
	}
	if _, err = callNote(t, first, map[string]any{"action": "update", "source": "luadraw", "doc": original.Doc, "revision": original.Revision, "content": "stale overwrite"}); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale revision was not rejected: %v", err)
	}
	listing, err := callNote(t, first, map[string]any{"action": "list", "source": "luadraw", "limit": 10, "offset": 0})
	if err != nil || len(listing.Notes) != 1 || listing.Notes[0].Content != "" || listing.Notes[0].Revision != updated.Notes[0].Revision {
		t.Fatalf("list did not expose the current metadata-only revision: %#v %v", listing, err)
	}
	deleted, err := callNote(t, first, map[string]any{"action": "delete", "source": "luadraw", "doc": original.Doc, "revision": updated.Notes[0].Revision})
	if err != nil || len(deleted.Notes) != 1 || !deleted.Notes[0].Deleted {
		t.Fatalf("delete failed: %#v %v", deleted, err)
	}
	readDeleted, err := callNote(t, second, map[string]any{"action": "read", "source": "luadraw", "doc": original.Doc})
	if err != nil || len(readDeleted.Notes) != 1 || !readDeleted.Notes[0].Deleted {
		t.Fatalf("deleted note was not readable for restore: %#v %v", readDeleted, err)
	}
	restored, err := callNote(t, second, map[string]any{"action": "restore", "source": "luadraw", "doc": original.Doc, "revision": deleted.Notes[0].Revision})
	if err != nil || len(restored.Notes) != 1 || restored.Notes[0].Deleted {
		t.Fatalf("restore failed: %#v %v", restored, err)
	}
	final, err := callNote(t, first, map[string]any{"action": "read", "source": "luadraw", "doc": original.Doc})
	if err != nil || len(final.Notes) != 1 || !strings.Contains(final.Notes[0].Content, "verify the projection and label") {
		t.Fatalf("restored content did not persist across instances: %#v %v", final, err)
	}
}

func TestDocumentNoteWithoutWritableSourceFailsClosed(t *testing.T) {
	d, err := documents.NewConfig(documents.Config{Sources: []documents.Source{{ID: "official", Root: t.TempDir()}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Note(context.Background(), map[string]any{"action": "list", "source": "official"}); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("read-only docs unexpectedly support notes: %v", err)
	}
	unconfigured := &documents.Docs{}
	if _, err = unconfigured.Note(context.Background(), map[string]any{"action": "list"}); err == nil || !strings.Contains(err.Error(), "no writable experience source") {
		t.Fatalf("unconfigured docs unexpectedly support notes: %v", err)
	}
}
