// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Command tokenstudio serves a local page for tuning design/tokens.json
// visually: edit the palette, semantic colors, sizes and density, preview
// them on the console's real components, then save to write tokens.json and
// regenerate every token file.
//
//	go run ./design/cmd/tokenstudio
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ffutop/modbus-gateway/design"
)

//go:embed studio.html preview.html
var pages embed.FS

func main() {
	listen := flag.String("listen", "127.0.0.1:7790", "address to serve on (loopback only)")
	flag.Parse()

	root, err := findRoot()
	if err != nil {
		log.Fatal(err)
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		log.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		log.Fatalf("-listen %s: tokenstudio writes files in the repository, so it only listens on loopback", *listen)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ModMux token studio: http://%s/  (editing %s)\n", ln.Addr(), filepath.Join(root, design.Source))
	log.Fatal(http.Serve(ln, newHandler(root)))
}

// findRoot walks up from the working directory to the repository root, the
// directory that holds design/tokens.json.
func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(design.Source))); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("design/tokens.json not found; run from inside the repository")
		}
		dir = parent
	}
}

// Files the preview may load from the working tree.
var repoFiles = map[string]string{
	"/repo/web/app.css":    "web/app.css",
	"/repo/web/tokens.css": "web/tokens.css",
}

func newHandler(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			servePage(w, "studio.html")
		case "/preview":
			servePage(w, "preview.html")
		default:
			rel, ok := repoFiles[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(root, filepath.FromSlash(rel)))
		}
	})
	mux.HandleFunc("/api/tokens", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(design.Source)))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"tokens": json.RawMessage(data),
				"rules":  design.ContrastRules,
			})
		case http.MethodPost:
			save(w, r, root)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return localOnly(mux)
}

// localOnly rejects requests that did not come from the studio page itself:
// a foreign Host (DNS rebinding) or a cross-site Origin (CSRF) could
// otherwise make a browser rewrite files in the repository.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			if u, err := url.Parse(o); err != nil || u.Host != r.Host {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func save(w http.ResponseWriter, r *http.Request, root string) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "want application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t, err := design.Parse(body)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	wrote, err := t.WriteFiles(root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error(), "wrote": wrote})
		return
	}
	log.Printf("saved: %s", strings.Join(wrote, ", "))
	writeJSON(w, http.StatusOK, map[string]interface{}{"wrote": wrote})
}

func servePage(w http.ResponseWriter, name string) {
	data, err := pages.ReadFile(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
