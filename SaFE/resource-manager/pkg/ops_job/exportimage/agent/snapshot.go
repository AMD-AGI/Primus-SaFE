/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// Timestamp is a file status-change time with nanosecond precision. It is kept as two
// integers because a float64 cannot hold seconds since the epoch and nanoseconds at once,
// and the comparison that decides what goes into the image is made at that precision.
type Timestamp struct {
	Sec  int64
	Nsec int64
}

// After reports whether t is strictly later than o.
func (t Timestamp) After(o Timestamp) bool {
	if t.Sec != o.Sec {
		return t.Sec > o.Sec
	}
	return t.Nsec > o.Nsec
}

// Entry is one path of the container's root file system.
type Entry struct {
	// Path is absolute and clean, e.g. "/usr/bin/python3".
	Path string
	// Type is one letter, as GNU find's %y prints it: f, d, l, b, c, p or s.
	Type byte
	// Ctime is the inode's last status change.
	Ctime Timestamp
}

// ParseMountPoints returns the mount points (field 5) of a /proc/<pid>/mountinfo, except
// the root itself. Mount points are octal-escaped there ("\040" for a space).
func ParseMountPoints(mountinfo string) []string {
	var out []string
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mp := path.Clean(unescapeOctal(fields[4]))
		if mp == "/" || !strings.HasPrefix(mp, "/") {
			continue
		}
		out = append(out, mp)
	}
	return out
}

func unescapeOctal(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// runtimeManaged are paths the container runtime or the kubelet provides at run time.
// They are never part of what the user built: /proc, /sys and /dev are pseudo file
// systems, and the three /etc files are written per Pod (usually bind mounts, which the
// mount table already covers, but not on every runtime).
var runtimeManaged = []string{
	"/proc", "/sys", "/dev",
	"/etc/hosts", "/etc/hostname", "/etc/resolv.conf",
}

// Filter decides which paths are outside the export: anything at or under a mount point
// (volumes, Secrets, the launcher's shared directory, ...) or a runtime-managed path, and
// the SSH host keys, which identify one machine and must never be copied into an image
// that others will run.
type Filter struct {
	prefixes []string
}

// NewFilter builds a Filter from the container's mount points.
func NewFilter(mountPoints []string) Filter {
	seen := map[string]bool{}
	var prefixes []string
	for _, p := range append(append([]string{}, runtimeManaged...), mountPoints...) {
		p = path.Clean(p)
		if p == "/" || seen[p] {
			continue
		}
		seen[p] = true
		prefixes = append(prefixes, p)
	}
	return Filter{prefixes: prefixes}
}

// Excluded reports whether p is left out of the export.
func (f Filter) Excluded(p string) bool {
	if p == "/" {
		return true
	}
	for _, pre := range f.prefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return isHostKey(p)
}

func isHostKey(p string) bool {
	dir, base := path.Split(p)
	return dir == "/etc/ssh/" && strings.HasPrefix(base, "ssh_host_")
}

// Changes is what the new layer has to carry.
type Changes struct {
	// Changed are the paths, absolute and sorted, whose inode changed after the threshold.
	// Directories are carried as metadata only.
	Changed []string
	// Deleted are base-image paths that no longer exist, sorted, with every path whose
	// ancestor is already deleted left out (a whiteout of the directory covers it).
	Deleted []string
	// Skipped counts changed paths that cannot be carried: sockets, which tar cannot
	// archive, and names that start with the whiteout prefix, which a layer cannot hold
	// as ordinary files.
	Skipped int
}

const whiteoutPrefix = ".wh."

// ComputeChanges compares the container's current listing with the files it started with.
// A path is changed when its status-change time is later than since; it is deleted when
// the container started with it and no longer has it.
func ComputeChanges(base map[string]bool, current []Entry, since Timestamp, f Filter) Changes {
	var ch Changes
	types := make(map[string]byte, len(current))
	for _, e := range current {
		types[e.Path] = e.Type
		if f.Excluded(e.Path) || !e.Ctime.After(since) {
			continue
		}
		if e.Type == 's' || strings.HasPrefix(path.Base(e.Path), whiteoutPrefix) {
			ch.Skipped++
			continue
		}
		ch.Changed = append(ch.Changed, e.Path)
	}

	gone := map[string]bool{}
	for p := range base {
		if f.Excluded(p) {
			continue
		}
		if _, ok := types[p]; !ok {
			gone[p] = true
		}
	}
	for p := range gone {
		if coveredByAncestor(p, gone, types) {
			continue
		}
		ch.Deleted = append(ch.Deleted, p)
	}
	sort.Strings(ch.Changed)
	sort.Strings(ch.Deleted)
	return ch
}

// coveredByAncestor reports whether a deleted path needs no whiteout of its own: an
// ancestor is itself deleted, or an ancestor is now something other than a directory (the
// new entry replaces the whole subtree, and a whiteout under a file cannot be applied).
func coveredByAncestor(p string, gone map[string]bool, types map[string]byte) bool {
	for dir := path.Dir(p); dir != "/" && dir != "."; dir = path.Dir(dir) {
		if gone[dir] {
			return true
		}
		if t, ok := types[dir]; ok && t != 'd' {
			return true
		}
	}
	return false
}

// ImageContains returns whether a path is in the saved image: carried by the new layer, or
// in the base image and not deleted.
func ImageContains(base map[string]bool, ch Changes) func(string) bool {
	changed := make(map[string]bool, len(ch.Changed))
	for _, p := range ch.Changed {
		changed[p] = true
	}
	deleted := make(map[string]bool, len(ch.Deleted))
	for _, p := range ch.Deleted {
		deleted[p] = true
	}
	return func(p string) bool {
		if changed[p] {
			return true
		}
		if !base[p] {
			return false
		}
		for q := p; q != "/" && q != "."; q = path.Dir(q) {
			if deleted[q] {
				return false
			}
		}
		return true
	}
}
