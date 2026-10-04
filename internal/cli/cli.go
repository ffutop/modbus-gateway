// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package cli is the modbus-gateway command: the root binary runs it, and the
// desktop app runs it in a child process (sidecar mode), so both forward
// requests through exactly the same startup path.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ffutop/modbus-gateway/internal/api"
	"github.com/ffutop/modbus-gateway/internal/app"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/web"
)

// Main runs the gateway with the command-line arguments args (without the
// program name) and returns when it has shut down; fatal errors exit the
// process.
func Main(version string, args []string) {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configFile := flags.String("config", "", "Path to config file")
	uiListen := flags.String("ui-listen", "", "Enable the management API on this address, overriding the config's ui section (port 0 = any free port)")
	exitOnStdinEOF := flags.Bool("exit-on-stdin-eof", false, "Shut down gracefully when stdin is closed (sidecar mode: the parent process owns the lifetime)")
	flags.Parse(args)

	// Load Configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	setupLogger(cfg.Log)

	// A parent process running the gateway as a sidecar starts it with
	// -ui-listen and reads the actual address back from the ui_ready line on
	// stdout.
	announceUI := *uiListen != ""
	if announceUI {
		cfg.UI = config.UIConfig{Enabled: true, Listen: *uiListen}
	}

	if cfg.Pprof.Enabled {
		addr := cfg.Pprof.Address
		if addr == "" {
			addr = "localhost:6060"
		}
		go func() {
			slog.Info("Starting pprof server", "addr", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				slog.Error("Failed to start pprof server", "err", err)
			}
		}()
	}

	slog.Info("Starting Modbus Gateway...")

	// Telemetry only exists for the management console; without `ui` the
	// forwarding path is exactly what it was before.
	var recorder *telemetry.Recorder
	if cfg.UI.Enabled {
		recorder = telemetry.NewRecorder(1000)
	}

	rt, err := app.New(cfg, recorder)
	if err != nil {
		slog.Error("Failed to start gateways. Exiting.", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	uiServer := startUI(cfg.UI, os.Getenv("MODMUX_UI_TOKEN"), announceUI, api.Deps{
		Version:         version,
		ConfigPath:      cfg.Path,
		StartupRevision: cfg.Revision,
		RunningConfig:   cfg,
		Simulations:     rt.SortedSimulations(),
		Telemetry:       recorder,
		Static:          web.Assets,
	})

	// Wait for a signal, or (sidecar mode) for stdin to close. Closing stdin
	// is how the parent stops us on every OS: on Windows killing a child is a
	// hard kill that would skip flushing persistence, and if the parent
	// crashes the OS closes the pipe for it.
	stdinClosed := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	if *exitOnStdinEOF {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			close(stdinClosed)
		}()
	}
	select {
	case <-sigChan:
		slog.Info("Shutting down...")
	case <-stdinClosed:
		slog.Info("Shutting down...", "reason", "stdin closed")
	}

	if uiServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		uiServer.Shutdown(shutdownCtx)
		shutdownCancel()
	}
	cancel()
	rt.Wait()
	rt.Close()

	slog.Info("Goodbye.")
}

func setupLogger(cfg config.LogConfig) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	switch cfg.Level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	}

	var handler slog.Handler
	if cfg.File != "" && cfg.File != "-" {
		f, err := os.OpenFile(cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Failed to open log file, falling back to stdout: %v\n", err)
			handler = slog.NewTextHandler(os.Stdout, opts)
		} else {
			handler = slog.NewTextHandler(f, opts)
		}
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

// startUI serves the management API when enabled; it returns nil otherwise,
// so a config without `ui` opens no extra port. A non-empty token makes every
// request authenticate; announce prints {"event":"ui_ready","addr":...} on
// stdout once listening, for a parent process running the gateway as a
// sidecar.
func startUI(cfg config.UIConfig, token string, announce bool, deps api.Deps) *http.Server {
	if !cfg.Enabled {
		return nil
	}
	addr := cfg.Address()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to start management API", "addr", addr, "err", err)
		os.Exit(1)
	}
	actual := ln.Addr().String()
	// Requests derive from serverCtx, canceled on Shutdown, so long-lived
	// event streams end at once instead of holding shutdown to its timeout.
	serverCtx, cancelRequests := context.WithCancel(context.Background())
	srv := &http.Server{
		Handler:     api.Guard(api.NewHandler(deps), token, api.IsLoopbackAddr(actual)),
		BaseContext: func(net.Listener) context.Context { return serverCtx },
	}
	srv.RegisterOnShutdown(cancelRequests)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("Management API stopped", "addr", actual, "err", err)
		}
	}()
	slog.Info("Started management API", "addr", actual, "token_required", token != "")
	if announce {
		ready, _ := json.Marshal(map[string]string{"event": "ui_ready", "addr": actual})
		fmt.Println(string(ready))
	}
	return srv
}
