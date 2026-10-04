// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Command gentokens writes the files generated from design/tokens.json and
// rewrites tokens.json in its canonical layout. Run it through
// `go generate ./design` from the desktop-native module root.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ffutop/modbus-gateway/desktop-native/design"
)

func main() {
	t, err := design.Load()
	if err != nil {
		fail(err)
	}
	// go generate runs in the design package directory.
	wd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	wrote, err := t.WriteFiles(filepath.Dir(wd))
	for _, p := range wrote {
		fmt.Println("wrote", p)
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gentokens:", err)
	os.Exit(1)
}
