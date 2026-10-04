// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package api serves the management console's HTTP API. It only reads the
// running gateways' state and reads/writes the config file; it never changes
// how requests are forwarded.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// Deps is everything the API reads from the running process.
type Deps struct {
	Version    string
	ConfigPath string
	// StartupRevision is the config file's revision when the process loaded
	// it; the running gateways reflect exactly that content.
	StartupRevision string
	RunningConfig   *config.Config
	Simulations     []*simulation.Simulation
	Telemetry       *telemetry.Recorder
	// Upstreams reports the listeners' states; nil reports none.
	Upstreams func() []gateway.UpstreamStatus
	// Static is the console front end, served at "/".
	Static   fs.FS
	identity *runtimeIdentity
}

// runtimeIdentity tracks revisions whose object order still matches startup.
type runtimeIdentity struct {
	sync.RWMutex
	revision string
}

// NewHandler returns the handler for every /api/v1/ endpoint.
func NewHandler(d Deps) http.Handler {
	d.identity = &runtimeIdentity{revision: d.StartupRevision}
	mux := http.NewServeMux()
	running := runningTree(d.RunningConfig)
	mux.HandleFunc("/api/v1/running-config", get(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, running)
	}))
	mux.HandleFunc("/api/v1/status", get(func(w http.ResponseWriter, r *http.Request) {
		type simStatus struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version uint64 `json:"version"`
		}
		sims := make([]simStatus, 0, len(d.Simulations))
		for _, s := range d.Simulations {
			sims = append(sims, simStatus{Name: s.Name, Status: string(s.Status()), Version: s.Version()})
		}
		upstreams := []gateway.UpstreamStatus{}
		if d.Upstreams != nil {
			upstreams = append(upstreams, d.Upstreams()...)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version":          d.Version,
			"startup_revision": d.StartupRevision,
			"config_path":      d.ConfigPath,
			"simulations":      sims,
			"upstreams":        upstreams,
		})
	}))
	mux.HandleFunc("/api/v1/metrics", get(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"gateways": d.Telemetry.Metrics()})
	}))
	mux.HandleFunc("/api/v1/events", get(func(w http.ResponseWriter, r *http.Request) {
		streamEvents(w, r, d.Telemetry)
	}))
	mux.HandleFunc("/api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			serveConfig(w, d)
		case http.MethodPut:
			saveConfig(w, r, d)
		default:
			w.Header().Set("Allow", "GET, PUT")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/api/v1/config/validate", only(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		validateConfig(w, r, d)
	}))
	mux.HandleFunc("/api/v1/simulations/", get(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/simulations/"), "/registers")
		if !ok {
			http.NotFound(w, r)
			return
		}
		for _, s := range d.Simulations {
			if s.Name == name {
				serveRegisters(w, r, s)
				return
			}
		}
		writeError(w, http.StatusNotFound, "simulation %q not found", name)
	}))
	if d.Static != nil {
		mux.Handle("/", http.FileServer(http.FS(d.Static)))
	}
	return secureHeaders(mux)
}

// consolePolicy lets the console load only its own files and talk only to
// its own origin. Inline style attributes are used by the page's templates;
// scripts are never inline.
const consolePolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// secureHeaders stops other sites from framing the console (clickjacking
// on a loopback API without login) and browsers from sniffing content types.
func secureHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := w.Header()
		hdr.Set("Content-Security-Policy", consolePolicy)
		hdr.Set("X-Frame-Options", "DENY")
		hdr.Set("X-Content-Type-Options", "nosniff")
		hdr.Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// Guard wraps the API with the checks its listener needs.
//
// With a token (sidecar mode, where a parent process owns the gateway), every
// request must carry "Authorization: Bearer <token>". On a loopback listener,
// the Host header must name a loopback host: otherwise a web page the user
// opens could rebind its own domain to 127.0.0.1 and drive the API (DNS
// rebinding).
func Guard(h http.Handler, token string, loopbackOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if loopbackOnly && !isLoopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, "host %q not allowed", r.Host)
			return
		}
		if token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// IsLoopbackAddr reports whether a listen address only accepts local
// connections.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	return err == nil && isLoopbackHost(host)
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func get(h http.HandlerFunc) http.HandlerFunc { return only(http.MethodGet, h) }

// only rejects requests whose method is not m.
func only(m string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != m {
			w.Header().Set("Allow", m)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h(w, r)
	}
}

func writeError(w http.ResponseWriter, code int, format string, args ...any) {
	writeJSON(w, code, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("api: failed to write response", "err", err)
	}
}
