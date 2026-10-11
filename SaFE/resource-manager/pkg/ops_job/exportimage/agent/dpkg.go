/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// PackagesPath is where the launcher lists, before its own bootstrap, the dpkg file lists
// the image holds (RecordPackages). The record of the image's files is made after the
// bootstrap, so it also lists the packages the launcher installs, whose files are never
// saved; this list is what tells the image's packages apart from those.
const PackagesPath = "/shared-data/save-image.packages"

// RecordPackages writes to file the dpkg file lists (/var/lib/dpkg/info/*.list) under
// root, one path per line; none for an image without dpkg. It replaces file whole.
func RecordPackages(file, root string) error {
	ents, err := os.ReadDir(filepath.Join(root, dpkgInfoDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var b strings.Builder
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".list") {
			b.WriteString(dpkgInfoDir + e.Name() + "\n")
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// readPackages reads what RecordPackages wrote; nil, without an error, when there is no
// such file (a container started by a launcher that did not list them).
func readPackages(file string) (map[string]bool, error) {
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the image's packages: %w", err)
	}
	out := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			out[l] = true
		}
	}
	return out, nil
}
