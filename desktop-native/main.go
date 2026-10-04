// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Command modmux-desktop shows the gateways' traffic in a native Gio window.
// The gateway runs in a child process of this same executable (package
// sidecar); closing the window stops it.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/launch"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/sidecar"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/workspace"
	"github.com/ffutop/modbus-gateway/internal/cli"
	"github.com/ffutop/modbus-gateway/internal/config"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == sidecar.Flag {
		cli.Main(version, os.Args[2:])
		return
	}

	configFile := flag.String("config", "", "Path to config file")
	flag.Parse()

	var launchErr error
	exe, err := os.Executable()
	if err == nil {
		home, _ := os.UserHomeDir()
		paths, err := launch.Prepare(exe, home, *configFile)
		launchErr = err
		*configFile = paths.Config
		if launchErr == nil && paths.WorkDir != "" {
			launchErr = os.Chdir(paths.WorkDir)
		}
		if launchErr == nil && paths.Log != "" {
			logFile, err := os.OpenFile(paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				launchErr = err
			} else {
				slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
				os.Stderr = logFile
			}
		}
	} else {
		launchErr = fmt.Errorf("locate executable: %w", err)
	}

	w := new(app.Window)
	w.Option(app.Title("ModMux"), app.Size(unit.Dp(1440), unit.Dp(900)), app.MinSize(unit.Dp(1100), unit.Dp(680)))

	// The child loads the file the editor resolved, so both see the same one.
	first := load(*configFile)
	path := first.Config.Path
	if path == "" {
		path = *configFile
	}
	sup := sidecar.NewSupervisor(exe, path, w.Invalidate)
	sup.Output().Echo = os.Stderr // the gateway's log joins the app's
	build := func(info workspace.Info) *workspace.UI {
		if launchErr != nil {
			info.StartErr = launchErr
		}
		if st := sup.State(); info.StartErr == nil && st.Phase == live.Stopped && st.Err != nil {
			info.StartErr = st.Err
		}
		if info.StartErr != nil {
			slog.Error("Gateway not running", "err", info.StartErr)
		}
		info.Running = info.StartErr == nil
		info.Source, info.Runtime = sup, sup
		return workspace.New(info)
	}
	startable := launchErr == nil && first.StartErr == nil
	u := build(first)
	if startable {
		sup.Start()
	}

	go func() {
		err := run(w, sup, u, func() *workspace.UI { return build(load(path)) })
		sup.Stop()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

// load reads the config file for the editor and the topology. The editor's
// baseline is the exact bytes it shows, even if another writer changed the
// file after LoadConfig read it.
func load(configFile string) workspace.Info {
	var info workspace.Info
	cfg, err := config.LoadConfig(configFile)
	if err == nil {
		info.Config = cfg
	}
	path := configFile
	if cfg != nil {
		path = cfg.Path
	}
	if path != "" {
		file, readErr := configfile.Open(path)
		if readErr == nil {
			info.Content, info.Save = file.Content, file.Save
			var parseErr error
			info.Config, parseErr = config.ParseDraft([]byte(file.Content))
			if err == nil && parseErr != nil {
				err = parseErr
			}
			if err == nil && info.Config != nil {
				err = info.Config.Validate()
			}
			if info.Config == nil {
				info.Config = &config.Config{}
			}
			info.Config.Path = file.Path
		} else if err == nil {
			err = readErr
		}
	}
	if info.Config == nil {
		info.Config = &config.Config{}
		if path != "" {
			info.Config.Path, _ = filepath.Abs(path)
		}
	}
	info.StartErr = err
	return info
}

// run drives the window, rebuilding the workspace from the config file
// whenever the gateway restarted or failed to start.
func run(w *app.Window, sup *sidecar.Supervisor, u *workspace.UI, build func() *workspace.UI) error {
	epoch := sup.State().Epoch
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			if st := sup.State(); st.Epoch != epoch {
				epoch = st.Epoch
				u = build()
			}
			gtx := app.NewContext(&ops, e)
			u.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}
