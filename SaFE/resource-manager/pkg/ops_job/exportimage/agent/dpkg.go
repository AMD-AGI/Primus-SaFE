/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"bytes"
	"strings"
)

// DpkgStatusPath is the Debian package database.
const DpkgStatusPath = "/var/lib/dpkg/status"

const dpkgInfoDir = "/var/lib/dpkg/info/"

// ReconcileDpkgStatus drops from a dpkg status file every installed package whose file
// list is not in the image.
//
// The export leaves out what the platform launcher installed, but when the user later
// runs apt, the status file they rewrite also records the launcher's packages. Kept as it
// is, the saved image would claim openssh-server, socat and their dependencies are
// installed while none of their files are there, and the next launcher would see them as
// present and skip installing them. Without the record, the next container installs them
// afresh. inImage reports whether a path is in the saved image. It returns the new status
// file and the packages it dropped.
func ReconcileDpkgStatus(status []byte, inImage func(string) bool) ([]byte, []string) {
	var out bytes.Buffer
	var dropped []string
	for _, stanza := range splitStanzas(status) {
		pkg, arch, installed := stanzaFields(stanza)
		if pkg != "" && installed && !inImage(dpkgInfoDir+pkg+".list") && !inImage(dpkgInfoDir+pkg+":"+arch+".list") {
			dropped = append(dropped, pkg)
			continue
		}
		out.Write(stanza)
		out.WriteByte('\n')
	}
	return out.Bytes(), dropped
}

// splitStanzas splits on blank lines, keeping each stanza's own trailing newline.
func splitStanzas(b []byte) [][]byte {
	var out [][]byte
	var cur []byte
	for _, line := range bytes.SplitAfter(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, line...)
	}
	if len(cur) > 0 {
		if cur[len(cur)-1] != '\n' {
			cur = append(cur, '\n')
		}
		out = append(out, cur)
	}
	return out
}

func stanzaFields(stanza []byte) (pkg, arch string, installed bool) {
	for _, line := range strings.Split(string(stanza), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "Package":
			pkg = v
		case "Architecture":
			arch = v
		case "Status":
			installed = strings.HasSuffix(v, " installed")
		}
	}
	return pkg, arch, installed
}
