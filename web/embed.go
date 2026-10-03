// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package web holds the management console's static front end, compiled
// into the binary so it works offline with no external resources.
package web

import "embed"

// Assets is the console's static files (index.html at the root).
//
//go:embed index.html app.css app.js
var Assets embed.FS
