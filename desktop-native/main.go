// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Command modmux-desktop runs the gateways in-process and shows their
// traffic in a native Gio window.
package main

import (
	"context"
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
	"github.com/ffutop/modbus-gateway/desktop-native/internal/workspace"
	gwapp "github.com/ffutop/modbus-gateway/internal/app"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

const (
	// recorderCapacity is the recorder's per-gateway buffer; it only has to
	// cover one poll interval; the workspace keeps bounded history.
	recorderCapacity = 4096
)

func main() {
	configFile := flag.String("config", "", "Path to config file")
	flag.Parse()

	var launchErr error
	if executable, err := os.Executable(); err == nil {
		home, _ := os.UserHomeDir()
		paths, err := launch.Prepare(executable, home, *configFile)
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
	}

	info := workspace.Info{}
	rec := telemetry.NewRecorder(recorderCapacity)
	info.Recorder = rec
	ctx, cancel := context.WithCancel(context.Background())

	var rt *gwapp.App
	cfg, err := config.LoadConfig(*configFile)
	if err == nil {
		info.Config = cfg
	}
	path := *configFile
	if cfg != nil {
		path = cfg.Path
	}
	if path != "" {
		file, readErr := configfile.Open(path)
		if readErr == nil {
			info.Content, info.Save = file.Content, file.Save
			// Use the exact bytes shown by the editor as the startup baseline,
			// even if another writer changed the file after LoadConfig read it.
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
	if launchErr != nil {
		err = launchErr
	}
	if err == nil {
		rt, err = gwapp.New(info.Config, rec)
	}
	if err != nil {
		slog.Error("Failed to start gateways", "err", err)
		info.StartErr = err
	} else {
		info.Simulations, info.Running = rt.Simulations, true
		rt.Start(ctx)
	}

	w := new(app.Window)
	w.Option(app.Title("ModMux"), app.Size(unit.Dp(1440), unit.Dp(900)), app.MinSize(unit.Dp(1100), unit.Dp(680)))

	go func() {
		err := run(w, workspace.New(info))
		cancel()
		if rt != nil {
			rt.Wait()
			rt.Close()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, u *workspace.UI) error {
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			u.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}
