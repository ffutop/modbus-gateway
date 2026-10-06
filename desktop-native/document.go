// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/sidecar"
)

// document is the configuration file the window edits. Opening another file
// or saving under a new name switches it: later gateway starts use the new
// file and the workspace is rebuilt from it, while a running gateway keeps
// its configuration until it is restarted.
//
// The process works in the file's directory, so relative persistence paths
// resolve beside the configuration, as the gateway child inherits it.
type document struct {
	sup    *sidecar.Supervisor
	notify func()

	mu       sync.Mutex
	file     string
	drafts   *configfile.Recovery
	gen      uint64
	noticeOf string
}

func newDocument(sup *sidecar.Supervisor, notify func()) *document {
	return &document{sup: sup, notify: notify}
}

func (d *document) path() string                   { d.mu.Lock(); defer d.mu.Unlock(); return d.file }
func (d *document) recovery() *configfile.Recovery { d.mu.Lock(); defer d.mu.Unlock(); return d.drafts }
func (d *document) generation() uint64             { d.mu.Lock(); defer d.mu.Unlock(); return d.gen }
func (d *document) takeNotice() (notice string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	notice, d.noticeOf = d.noticeOf, ""
	return
}
func (d *document) setNotice(notice string) { d.mu.Lock(); d.noticeOf = notice; d.mu.Unlock() }

// switchTo makes path, with its current content, the edited file.
func (d *document) switchTo(path, content, notice string) error {
	err := os.Chdir(filepath.Dir(path))
	d.sup.SetConfig(path)
	d.mu.Lock()
	d.file, d.drafts, d.noticeOf = path, configfile.NewRecovery(path, content), notice
	d.gen++
	d.mu.Unlock()
	if d.notify != nil {
		d.notify()
	}
	return err
}

// Open switches to an existing configuration file. A stopped gateway starts
// on it if it is valid.
func (d *document) Open(path string) error {
	if err := checkName(path); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s 是目录", path)
	}
	file, err := configfile.Open(path)
	if err != nil {
		return err
	}
	if err := d.switchTo(file.Path, file.Content, "已打开 "+file.Path); err != nil {
		return err
	}
	slog.Info("已打开配置文件", "path", file.Path)
	if d.sup.State().Phase == live.Stopped && load(file.Path).StartErr == nil {
		d.sup.Start()
	}
	return nil
}

// SaveAs writes the draft to path and switches to it. The draft is saved, so
// the old file's recovery record is no longer needed.
func (d *document) SaveAs(path, text string) error {
	if err := checkName(path); err != nil {
		return err
	}
	file, err := configfile.SaveAs(path, text)
	if err != nil {
		return err
	}
	if old := d.recovery(); old != nil {
		_ = old.Clear(text)
	}
	if err := d.switchTo(file.Path, text, "已另存为 "+file.Path); err != nil {
		return err
	}
	slog.Info("配置已另存为", "path", file.Path)
	return nil
}

// checkName requires a YAML extension: the gateway picks the parser by it.
func checkName(path string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return nil
	}
	return fmt.Errorf("配置文件须以 .yaml 或 .yml 结尾：%s", filepath.Base(path))
}
