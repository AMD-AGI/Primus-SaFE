/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package muxbin carries the pod-side reverse-forward multiplexer inside the
// apiserver binary, so that opening a forward does not depend on anything being
// present in the user's image.
//
// The files here are placeholders in the source tree and real binaries in the
// container image; build-rfwd-mux.sh replaces them, and the image build runs it.
// Shipping placeholders rather than committed binaries keeps the repository free of
// build output while still letting a clean checkout compile and test - the tests
// that need a multiplexer that runs build their own.
package muxbin

import (
	"bytes"
	_ "embed"
	"fmt"
)

//go:embed mux-linux-amd64
var linuxAMD64 []byte

//go:embed mux-linux-arm64
var linuxARM64 []byte

// elfMagic is how a real binary is told from the placeholder standing in for one.
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// For returns the multiplexer built for the container's architecture, named the
// way `uname -m` spells it.
func For(machine string) ([]byte, string, error) {
	var binary []byte
	var arch string
	switch machine {
	case "x86_64", "amd64", "x64":
		binary, arch = linuxAMD64, "linux/amd64"
	case "aarch64", "arm64", "armv8l", "armv8b":
		binary, arch = linuxARM64, "linux/arm64"
	default:
		return nil, "", fmt.Errorf("no reverse forward multiplexer is built for machine %q", machine)
	}
	if !bytes.HasPrefix(binary, elfMagic) {
		return nil, arch, fmt.Errorf("this apiserver was built without the %s reverse forward "+
			"multiplexer; run SaFE/apiserver/installer/build-rfwd-mux.sh before building the image", arch)
	}
	return binary, arch, nil
}

// Available reports whether a usable binary is embedded for machine, which is how
// a test decides whether it can inject one.
func Available(machine string) bool {
	_, _, err := For(machine)
	return err == nil
}
