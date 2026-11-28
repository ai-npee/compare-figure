package documents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"doc-library/pkg/doclibrary"
)

// NoteDefinition keeps shared experience maintenance separate from read-only search.
func NoteDefinition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "document_note", "strict": false,
		"description": "Maintain shared Markdown experience notes in a configured source. Actions: create/list/read/update/delete/restore. Anyone using the same source can read and correct any note. Read first and pass its exact revision for update/delete; restore uses the deleted revision. Official/reference documents are read-only. Supply only needed arguments. Save reusable, tested methods with conditions and limitations; omit credentials and task-private material. Writing does not boost frequency.",
		"parameters":  doclibrary.NoteSchema(),
	}}
}

// Note delegates all storage, conflict detection and index consistency to doc-library.
func (d *Docs) Note(ctx context.Context, args map[string]any) (string, error) {
	if d.library == nil {
		return "", fmt.Errorf("no writable experience source is configured")
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	var request doclibrary.NoteRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return "", fmt.Errorf("experience request: %w", err)
	}
	response, err := d.library.Note(ctx, request)
	if err != nil {
		return "", err
	}
	data, err = json.Marshal(response)
	return string(data), err
}
