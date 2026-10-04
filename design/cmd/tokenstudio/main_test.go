// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ffutop/modbus-gateway/design"
)

// fixture is a repository with tokens.json and the web directory only, so a
// save writes tokens.json and web/tokens.css and skips the absent targets.
func fixture(t *testing.T) (root string, tokens []byte) {
	t.Helper()
	root = t.TempDir()
	tokens, err := os.ReadFile(filepath.Join("..", "..", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"design", "web"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "design", "tokens.json"), tokens, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, tokens
}

func post(h http.Handler, body []byte, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7790/api/tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:7790")
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSaveWritesSourceAndGenerated(t *testing.T) {
	root, tokens := fixture(t)
	edited := bytes.Replace(tokens, []byte(`"#b91c1c"`), []byte(`"#991b1b"`), 1)
	rec := post(newHandler(root), edited, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	css, err := os.ReadFile(filepath.Join(root, "web", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "--err:#991b1b;") {
		t.Errorf("web/tokens.css not regenerated:\n%s", css)
	}
	src, _ := os.ReadFile(filepath.Join(root, "design", "tokens.json"))
	if !bytes.Equal(src, edited) {
		t.Errorf("tokens.json not saved in canonical layout")
	}
	if _, err := os.Stat(filepath.Join(root, design.DesktopNative)); !os.IsNotExist(err) {
		t.Errorf("created a target in an absent module: %v", err)
	}
}

func TestSaveRejects(t *testing.T) {
	root, tokens := fixture(t)
	h := newHandler(root)
	for name, tc := range map[string]struct {
		body   []byte
		mutate func(*http.Request)
		want   int
	}{
		"invalid tokens":   {[]byte(`{"palette":[{"name":"Bad","value":"red"}]}`), nil, http.StatusUnprocessableEntity},
		"unknown field":    {bytes.Replace(tokens, []byte(`"palette"`), []byte(`"extra":1,"palette"`), 1), nil, http.StatusUnprocessableEntity},
		"cross-site":       {tokens, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, http.StatusForbidden},
		"rebound host":     {tokens, func(r *http.Request) { r.Host = "evil.example:7790"; r.Header.Del("Origin") }, http.StatusForbidden},
		"simple form post": {tokens, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
	} {
		if rec := post(h, tc.body, tc.mutate); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, tc.want, rec.Body)
		}
	}
	src, _ := os.ReadFile(filepath.Join(root, "design", "tokens.json"))
	if !bytes.Equal(src, tokens) {
		t.Error("a rejected request changed tokens.json")
	}
}

func TestServesOnlyAllowedRepoFiles(t *testing.T) {
	root, _ := fixture(t)
	h := newHandler(root)
	for path, served := range map[string]bool{
		"/":                        true,
		"/preview":                 true,
		"/repo/web/../go.mod":      false, // ServeMux redirects to the cleaned path, which 404s
		"/repo/design/tokens.json": false,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7790"+path, nil))
		if (rec.Code == http.StatusOK) != served {
			t.Errorf("GET %s: status %d, served = %v", path, rec.Code, !served)
		}
	}
}
