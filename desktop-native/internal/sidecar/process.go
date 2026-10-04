// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package sidecar runs the gateway in a child process, so the window's
// rendering and garbage collection never share a runtime with forwarding,
// and a crash on either side leaves the other able to report it.
//
// The child is this same executable started with Flag, which runs the
// modbus-gateway command (internal/cli) with the management API on a free
// loopback port: it prints {"event":"ui_ready","addr":...} once listening,
// requires the per-launch token passed in MODMUX_UI_TOKEN, and shuts down
// gracefully when its stdin closes. Closing the window therefore stops
// forwarding, and if the app dies the OS closes the pipe for it.
package sidecar

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Flag, as the first argument, makes the desktop executable run as the
// gateway child process; the remaining arguments are the gateway's.
const Flag = "--sidecar"

// Args returns the child's command-line arguments for a config file.
func Args(config string) []string {
	return []string{Flag, "-config", config, "-ui-listen", "127.0.0.1:0", "-exit-on-stdin-eof"}
}

const (
	readyTimeout = 15 * time.Second
	stopTimeout  = 10 * time.Second
	outputLines  = 500
)

// Output keeps the most recent lines a gateway printed and echoes each to
// Echo, if set.
type Output struct {
	Echo io.Writer

	mu    sync.Mutex
	lines []string
}

func (o *Output) add(line string) {
	o.mu.Lock()
	o.lines = append(o.lines, line)
	if len(o.lines) > outputLines {
		o.lines = append(o.lines[:0:0], o.lines[len(o.lines)-outputLines:]...)
	}
	o.mu.Unlock()
	if o.Echo != nil {
		fmt.Fprintln(o.Echo, line)
	}
}

// Tail returns up to n of the most recent lines, oldest first.
func (o *Output) Tail(n int) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if n > len(o.lines) {
		n = len(o.lines)
	}
	return append([]string(nil), o.lines[len(o.lines)-n:]...)
}

// Process is a running gateway child.
type Process struct {
	URL   string // management API base, e.g. http://127.0.0.1:53121
	Token string

	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan struct{} // closed once the child exited and its output drained
	err   error         // exit status; read after done
}

// Launch starts exe with args and returns once the child's management API
// is listening. If the child exits or does not become ready in time, Launch
// returns an error and the child is gone.
func Launch(exe string, args []string, out *Output) (*Process, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	p := &Process{Token: hex.EncodeToString(b[:]), done: make(chan struct{})}
	p.cmd = exec.Command(exe, args...)
	p.cmd.Env = append(os.Environ(), "MODMUX_UI_TOKEN="+p.Token)
	hideWindow(p.cmd)
	var err error
	if p.stdin, err = p.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := p.cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动网关进程：%w", err)
	}

	ready := make(chan string, 1)
	var drained sync.WaitGroup
	drained.Add(2)
	go func() {
		defer drained.Done()
		scan(stderr, out, nil)
	}()
	go func() {
		defer drained.Done()
		scan(stdout, out, ready)
	}()
	go func() {
		// Wait only after the pipes are drained, so the output that
		// explains an exit is recorded by the time done closes.
		drained.Wait()
		p.err = p.cmd.Wait()
		close(p.done)
	}()

	timer := time.NewTimer(readyTimeout)
	defer timer.Stop()
	select {
	case addr := <-ready:
		p.URL = "http://" + addr
		return p, nil
	case <-p.done:
		return nil, fmt.Errorf("网关启动失败（%v）", p.err)
	case <-timer.C:
		p.cmd.Process.Kill()
		<-p.done
		return nil, fmt.Errorf("网关在 %v 内未就绪", readyTimeout)
	}
}

// scan records each line of r and sends the ui_ready address, once, to ready.
func scan(r io.Reader, out *Output, ready chan<- string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		out.add(line)
		if ready == nil || !strings.Contains(line, `"ui_ready"`) {
			continue
		}
		var msg struct{ Event, Addr string }
		if json.Unmarshal([]byte(line), &msg) == nil && msg.Event == "ui_ready" && msg.Addr != "" {
			ready <- msg.Addr
			ready = nil
		}
	}
	// Keep draining after a scanner error so the child never blocks on a
	// full pipe.
	io.Copy(io.Discard, r)
}

// Done is closed once the child has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

// Err is the child's exit status; valid once Done is closed.
func (p *Process) Err() error { return p.err }

// Stop closes the child's stdin, the graceful path on every OS, and kills it
// if it has not exited within the stop timeout.
func (p *Process) Stop() error {
	p.stdin.Close()
	timer := time.NewTimer(stopTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-timer.C:
		p.cmd.Process.Kill()
		<-p.done
		return errors.New("网关未在超时内退出，已强制结束")
	}
}
