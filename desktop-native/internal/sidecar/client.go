// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package sidecar

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

const (
	maxEvents      = 8192 // requests kept until the window reads them
	pollInterval   = 250 * time.Millisecond
	statusInterval = time.Second
	windowIdle     = 2 * time.Second // a register window not asked for this long is not refreshed
	retryDelay     = 500 * time.Millisecond
	requestTimeout = 2 * time.Second
)

// Client is a live.Source over a gateway's management API. Background
// goroutines keep its copy fresh; the methods only read that copy.
type Client struct {
	base, token string
	http        *http.Client
	cancel      context.CancelFunc

	mu        sync.Mutex
	events    []telemetry.Event // ascending Seq
	last      uint64
	upstreams []gateway.UpstreamStatus
	windows   map[window]*cached
}

type window struct {
	sim          string
	table        live.Table
	start, count uint16
}

type cached struct {
	values []uint16
	ok     bool
	wanted time.Time
}

// NewClient starts following the API at base until Close.
func NewClient(base, token string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{base: base, token: token, http: &http.Client{}, cancel: cancel, windows: map[window]*cached{}}
	go c.follow(ctx)
	go c.poll(ctx)
	return c
}

// Close stops the background requests; the last known data stays readable.
func (c *Client) Close() { c.cancel() }

func (c *Client) Since(seq uint64) []telemetry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := sort.Search(len(c.events), func(i int) bool { return c.events[i].Seq > seq })
	if i == len(c.events) {
		return nil
	}
	return append([]telemetry.Event(nil), c.events[i:]...)
}

func (c *Client) Registers(sim string, t live.Table, start, count uint16) ([]uint16, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := window{sim, t, start, count}
	cw := c.windows[w]
	if cw == nil {
		cw = &cached{}
		c.windows[w] = cw
	}
	cw.wanted = time.Now()
	return append([]uint16(nil), cw.values...), cw.ok
}

func (c *Client) Upstreams() []gateway.UpstreamStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.upstreams == nil {
		return nil
	}
	return append([]gateway.UpstreamStatus{}, c.upstreams...)
}

func (c *Client) request(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return resp, nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := c.request(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// follow reads the event stream, reconnecting until ctx ends. A new stream
// starts with the gateway's buffered backlog; events already kept are
// skipped by Seq.
func (c *Client) follow(ctx context.Context) {
	for ctx.Err() == nil {
		c.stream(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(retryDelay):
		}
	}
}

// eventJSON is one request as /api/v1/events sends it.
type eventJSON struct {
	Seq          uint64  `json:"seq"`
	Time         string  `json:"time"`
	Gateway      string  `json:"gateway"`
	Downstream   string  `json:"downstream"`
	Source       string  `json:"source"`
	SlaveID      byte    `json:"slave_id"`
	FunctionCode byte    `json:"function_code"`
	Address      uint16  `json:"address"`
	Quantity     uint16  `json:"quantity"`
	DurationMs   float64 `json:"duration_ms"`
	Error        string  `json:"error"`
	Request      string  `json:"request"`
	Response     string  `json:"response"`
}

func (e *eventJSON) event() telemetry.Event {
	t, _ := time.Parse(time.RFC3339Nano, e.Time)
	ev := telemetry.Event{
		Seq: e.Seq, Time: t, Gateway: e.Gateway, Downstream: e.Downstream, Source: e.Source,
		SlaveID: e.SlaveID, FunctionCode: e.FunctionCode, Address: e.Address, Quantity: e.Quantity,
		Duration: time.Duration(e.DurationMs * float64(time.Millisecond)),
	}
	if e.Error != "" {
		ev.Err = errors.New(e.Error)
	}
	ev.Request, _ = hex.DecodeString(e.Request)
	if e.Response != "" {
		ev.Response, _ = hex.DecodeString(e.Response)
	}
	return ev
}

func (c *Client) stream(ctx context.Context) {
	resp, err := c.request(ctx, "/api/v1/events")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 256*1024), 64*1024*1024)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var batch []eventJSON
		if json.Unmarshal([]byte(data), &batch) != nil {
			continue
		}
		c.mu.Lock()
		for i := range batch {
			if batch[i].Seq > c.last {
				c.last = batch[i].Seq
				c.events = append(c.events, batch[i].event())
			}
		}
		if len(c.events) > maxEvents {
			c.events = append(c.events[:0:0], c.events[len(c.events)-maxEvents:]...)
		}
		c.mu.Unlock()
	}
}

// poll refreshes the register windows the window asked for recently, and
// the listener states.
func (c *Client) poll(ctx context.Context) {
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	var lastStatus time.Time
	for {
		if time.Since(lastStatus) >= statusInterval {
			lastStatus = time.Now()
			var status struct {
				Upstreams []gateway.UpstreamStatus `json:"upstreams"`
			}
			if c.getJSON(ctx, "/api/v1/status", &status) == nil {
				if status.Upstreams == nil {
					status.Upstreams = []gateway.UpstreamStatus{}
				}
				c.mu.Lock()
				c.upstreams = status.Upstreams
				c.mu.Unlock()
			}
		}
		c.refreshWindows(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (c *Client) refreshWindows(ctx context.Context) {
	c.mu.Lock()
	var wanted []window
	for w, cw := range c.windows {
		switch idle := time.Since(cw.wanted); {
		case idle > 5*windowIdle:
			delete(c.windows, w)
		case idle <= windowIdle:
			wanted = append(wanted, w)
		}
	}
	c.mu.Unlock()
	for _, w := range wanted {
		var got struct {
			Values []uint16 `json:"values"`
		}
		path := fmt.Sprintf("/api/v1/simulations/%s/registers?table=%s&start=%d&count=%d", url.PathEscape(w.sim), w.table, w.start, w.count)
		err := c.getJSON(ctx, path, &got)
		c.mu.Lock()
		if cw := c.windows[w]; cw != nil {
			cw.values, cw.ok = got.Values, err == nil && len(got.Values) == int(w.count)
		}
		c.mu.Unlock()
	}
}
