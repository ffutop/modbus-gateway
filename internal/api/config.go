// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/ffutop/modbus-gateway/internal/config"
)

func serveConfig(w http.ResponseWriter, d Deps) {
	doc, err := config.ReadDocument(d.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	tree, err := doc.Tree()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	d.identity.RLock()
	indexMatches := doc.Revision == d.identity.revision
	d.identity.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"running_index_matches": indexMatches,
		"revision":              doc.Revision,
		"schema_version":        doc.SchemaVersion,
		"running_matches":       doc.Revision == d.StartupRevision,
		"config":                tree,
	})
}

// validateConfig reports every problem the edited file would have at the
// next startup, without writing anything.
func validateConfig(w http.ResponseWriter, r *http.Request, d Deps) {
	doc, ok := editedDocument(w, r, d)
	if !ok {
		return
	}
	problems, err := draftProblems(doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": len(problems) == 0, "problems": problems})
}

// saveConfig writes the edited file only when the result passes every check
// the next startup would make. It never touches the running gateways.
func saveConfig(w http.ResponseWriter, r *http.Request, d Deps) {
	doc, ok := editedDocument(w, r, d)
	if !ok {
		return
	}
	if doc.SchemaVersion != 1 {
		// A v0 `local` downstream implies a simulation the file cannot
		// express, so it cannot be written back without loss.
		writeError(w, http.StatusUnprocessableEntity, "this config is read-only in the console: migrate it to 'version: 1' first")
		return
	}
	problems, err := draftProblems(doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"problems": problems})
		return
	}
	content := doc.Bytes()
	if err := config.WriteFile(d.ConfigPath, content); err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	d.identity.Lock()
	if d.identity.revision == doc.Revision {
		d.identity.revision = config.Revision(content)
	}
	d.identity.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"revision": config.Revision(content), "restart_required": true})
}

// editedDocument reads the config file and applies the request's edits,
// answering the request itself when that fails.
func editedDocument(w http.ResponseWriter, r *http.Request, d Deps) (*config.Document, bool) {
	var req struct {
		BaseRevision string        `json:"base_revision"`
		Edits        []config.Edit `json:"edits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return nil, false
	}
	doc, err := config.ReadDocument(d.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return nil, false
	}
	if req.BaseRevision != doc.Revision {
		// The edits were made against content that is no longer on disk
		// (e.g. someone saved from a text editor); their paths may no longer
		// point where the user meant.
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":            "the config file changed since it was read; reload it and redo the edits",
			"current_revision": doc.Revision,
		})
		return nil, false
	}
	if err := doc.Apply(req.Edits); err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return nil, false
	}
	return doc, true
}

// draftProblems renders an edited document and checks it the way the next
// startup would. A draft that does not even parse is one problem at the root.
func draftProblems(doc *config.Document) ([]config.Problem, error) {
	cfg, err := config.ParseDraft(doc.Bytes())
	if err != nil {
		return []config.Problem{{Path: []any{}, Message: err.Error()}}, nil
	}
	problems := cfg.Problems()
	if problems == nil {
		problems = []config.Problem{}
	}
	return problems, nil
}
