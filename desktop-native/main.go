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
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/filepicker"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/launch"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/runlog"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/sidecar"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/workspace"
	"github.com/ffutop/modbus-gateway/internal/cli"
	"github.com/ffutop/modbus-gateway/internal/config"
)

// version is overridden at build time via -ldflags "-X main.version=...";
// the package scripts also set packaged to "true", so the app logs to a file.
var version, packaged = "dev", ""

func main() {
	if len(os.Args) > 1 && os.Args[1] == sidecar.Flag {
		cli.Main(version, os.Args[2:])
		return
	}
	configFile := flag.String("config", "", "Path to config file (default: config.yaml beside the app)")
	flag.Parse()

	var launchErr error
	exe, err := os.Executable()
	if err == nil {
		home, _ := os.UserHomeDir()
		paths, err := launch.Prepare(exe, home, *configFile, packaged == "true")
		launchErr = err
		if paths.Config != "" {
			*configFile = paths.Config
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
	logs := runlog.New(w.Invalidate)
	// Both desktop records and child output are captured independently of the
	// management API. Disk output remains available after the application exits.
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stderr, runlog.Writer{Store: logs, Source: "desktop"}), nil)))
	slog.Info("桌面应用启动", "version", version)

	// The child loads the file the editor resolved, so both see the same one.
	first := load(*configFile)
	sup := sidecar.NewSupervisor(exe, first.Config.Path, w.Invalidate)
	sup.Output().Echo = os.Stderr // the gateway's log joins the app's
	sup.Output().Logs = logs
	doc := newDocument(sup, w.Invalidate)
	if err := doc.switchTo(first.Config.Path, first.Content, ""); err != nil && launchErr == nil {
		launchErr = err
	}

	build := func(info workspace.Info) *workspace.UI {
		info.Logs, info.Notify = logs, w.Invalidate
		recovery := doc.recovery()
		info.RunningContent = sup.RunningConfig()
		if sup.State().Phase == live.Running && info.RunningContent != "" {
			info.StartErr = nil
		}
		recovery.Rebase(info.Content)
		info.Draft, info.DraftConflict, _ = recovery.Load()
		info.DraftBase = recovery.Baseline()
		info.SaveDraft = recovery.Queue
		info.DraftError = recovery.Err
		savedContent := info.Content
		saved := info.Save
		info.Save = func(text string) error {
			if saved == nil {
				return fmt.Errorf("配置文件不可写")
			}
			if err := saved(text); err != nil {
				slog.Error("配置保存失败", "path", info.Config.Path, "err", err)
				return err
			}
			slog.Info("配置已保存", "path", info.Config.Path)
			savedContent = text
			_ = recovery.Clear(text)
			return nil
		}
		rebase := info.Rebase
		if rebase != nil {
			info.Rebase = func(text string) error {
				if err := rebase(text); err != nil {
					return err
				}
				savedContent = text
				recovery.Rebase(text)
				return nil
			}
		}
		info.ClearDraft = func() error { return recovery.Clear(savedContent) }
		info.Open, info.SaveAs, info.Notice = doc.Open, doc.SaveAs, doc.takeNotice()
		info.PickFile = func(save bool, dir, name string) (string, error) {
			return filepicker.Pick(filepicker.Request{Save: save, Dir: dir, Name: name})
		}
		if launchErr != nil {
			info.StartErr = launchErr
		}
		if st := sup.State(); info.StartErr == nil && st.Phase == live.Stopped && st.Err != nil {
			info.StartErr = st.Err
		}
		if info.StartErr != nil {
			slog.Error("Gateway not running", "err", info.StartErr)
		}
		info.Running = info.StartErr == nil && sup.State().Phase == live.Running
		info.Source, info.Runtime = sup, sup
		return workspace.New(info)
	}

	startable := launchErr == nil && first.StartErr == nil && !first.NewFile
	u := build(first)
	if startable {
		sup.Start()
	}

	go func() {
		err := run(w, sup, doc, u, func() *workspace.UI { return build(load(doc.path())) })
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
// baseline is the exact bytes it shows. A missing file opens as a blank v1
// draft that the first save creates.
func load(configFile string) workspace.Info {
	var info workspace.Info
	file, err := configfile.Open(configFile)
	if err != nil {
		info.Config = &config.Config{}
		info.Config.Path, _ = filepath.Abs(configFile)
		info.StartErr = err
		return info
	}
	info.Content, info.Save, info.Rebase = file.Content, file.Save, file.Rebase
	info.NewFile = !file.Exists()
	if info.NewFile {
		info.Config, _ = config.ParseDraft([]byte("version: 1\n"))
	} else {
		info.Config, err = config.ParseDraft([]byte(file.Content))
		if err == nil {
			err = info.Config.Validate()
		}
		info.StartErr = err
	}
	if info.Config == nil {
		info.Config = &config.Config{}
	}
	info.Config.Path = file.Path
	return info
}

// run drives the window, rebuilding the workspace from the config file
// whenever the gateway restarted or failed to start, or another file was
// opened.
func run(w *app.Window, sup *sidecar.Supervisor, doc *document, u *workspace.UI, build func() *workspace.UI) error {
	epoch, generation := sup.State().Epoch, doc.generation()
	var ops op.Ops
	for {
		ev := w.Event()
		filepicker.Observe(ev) // the window handle that owns the chooser
		switch e := ev.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			if st, gen := sup.State(), doc.generation(); st.Epoch != epoch || gen != generation {
				switched := gen != generation
				epoch, generation = st.Epoch, gen
				next := build()
				if switched {
					u.CarryViewTo(next) // the old draft was saved or belongs to the old file
				} else {
					u.CarryDraftTo(next)
				}
				u = next
			}
			gtx := app.NewContext(&ops, e)
			u.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}
