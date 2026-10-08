/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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

// ParseTimestamp parses the "%C@" form printed by GNU find: seconds, optionally followed
// by a fraction of up to ten digits ("1759946294.1234567890").
func ParseTimestamp(s string) (Timestamp, error) {
	secPart, fracPart, _ := strings.Cut(strings.TrimSpace(s), ".")
	sec, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil {
		return Timestamp{}, fmt.Errorf("invalid timestamp %q", s)
	}
	if len(fracPart) > 9 {
		fracPart = fracPart[:9]
	}
	var nsec int64
	if fracPart != "" {
		for _, c := range fracPart {
			if c < '0' || c > '9' {
				return Timestamp{}, fmt.Errorf("invalid timestamp %q", s)
			}
		}
		fracPart += strings.Repeat("0", 9-len(fracPart))
		if nsec, err = strconv.ParseInt(fracPart, 10, 64); err != nil {
			return Timestamp{}, fmt.Errorf("invalid timestamp %q", s)
		}
	}
	return Timestamp{Sec: sec, Nsec: nsec}, nil
}

// Entry is one path of the container's root filesystem as listed inside the container.
type Entry struct {
	// Path is absolute and clean, e.g. "/usr/bin/python3".
	Path string
	// Type is the GNU find %y letter: f, d, l, b, c, p or s.
	Type byte
	// Ctime is the inode's last status change.
	Ctime Timestamp
}

// listCommand lists the root filesystem without crossing into other filesystems. Each
// record is "<type> <ctime> <path>" terminated by NUL, so any byte but NUL may appear in a
// path. -ignore_readdir_race keeps a file that disappears mid-walk from failing the
// listing; every other error (an unreadable directory above all) fails it, because a
// directory that could not be read would otherwise look like one whose files were deleted.
var listCommand = []string{"find", "/", "-xdev", "-ignore_readdir_race", "-printf", `%y %C@ %p\0`}

// ParseListing parses the output of listCommand.
func ParseListing(r io.Reader) ([]Entry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(splitNUL)
	var out []Entry
	for sc.Scan() {
		rec := sc.Text()
		typ, rest, ok1 := strings.Cut(rec, " ")
		ts, p, ok2 := strings.Cut(rest, " ")
		if !ok1 || !ok2 || len(typ) != 1 || !strings.HasPrefix(p, "/") {
			return nil, fmt.Errorf("malformed listing record %q", rec)
		}
		ctime, err := ParseTimestamp(ts)
		if err != nil {
			return nil, err
		}
		out = append(out, Entry{Path: path.Clean(p), Type: typ[0], Ctime: ctime})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading listing: %w", err)
	}
	return out, nil
}

func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return 0, nil, fmt.Errorf("listing ends without a NUL terminator")
	}
	return 0, nil, nil
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

// ComputeChanges compares the container's current listing with the base image's file set.
// A path is changed when its status-change time is later than since; it is deleted when
// the base image has it and the container does not.
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
