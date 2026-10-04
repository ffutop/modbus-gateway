// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package main

import (
	"os"

	"github.com/ffutop/modbus-gateway/internal/cli"
)

// version is overridden at release build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Main(version, os.Args[1:])
}
