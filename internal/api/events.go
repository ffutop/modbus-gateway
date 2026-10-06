// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// eventBatchInterval is how often /api/v1/events pushes new requests; the
// stream carries every buffered request, so batching loses nothing.
const eventBatchInterval = 250 * time.Millisecond

type eventView struct {
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
	Error        string  `json:"error,omitempty"`
	// Request and Response are the PDUs in hex, function code first;
	// Response is empty when the request failed.
	Request  string `json:"request"`
	Response string `json:"response,omitempty"`
}

// streamEvents sends the buffered requests, then every new batch, as SSE
// `data:` lines each holding a JSON array, until the client disconnects.
func streamEvents(w http.ResponseWriter, r *http.Request, rec *telemetry.Recorder) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(eventBatchInterval)
	defer ticker.Stop()
	var cursor uint64
	for {
		if events := rec.Since(cursor); len(events) > 0 {
			cursor = events[len(events)-1].Seq
			batch := make([]eventView, len(events))
			for i, e := range events {
				batch[i] = eventView{
					Seq: e.Seq, Time: e.Time.Format(time.RFC3339Nano), Gateway: e.Gateway, Downstream: e.Downstream,
					Source: e.Source, SlaveID: e.SlaveID, FunctionCode: e.FunctionCode, Address: e.Address,
					Quantity: e.Quantity, DurationMs: float64(e.Duration) / float64(time.Millisecond),
					Request: hex.EncodeToString(e.Request), Response: hex.EncodeToString(e.Response),
				}
				if e.Err != nil {
					batch[i].Error = e.Err.Error()
				}
			}
			data, _ := json.Marshal(batch)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
