// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

//go:build !windows

package sidecar

import "os/exec"

func hideWindow(*exec.Cmd) {}
