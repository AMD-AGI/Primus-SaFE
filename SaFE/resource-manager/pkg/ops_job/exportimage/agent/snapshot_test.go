/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The comparison is made below a microsecond: a file the launcher wrote in the same
// second as its hand-over, but earlier, must not count as the user's.
func TestTimestampAfterComparesNanoseconds(t *testing.T) {
	handover := Timestamp{Sec: 100, Nsec: 588136411}
	assert.False(t, Timestamp{Sec: 100, Nsec: 588136410}.After(handover))
	assert.False(t, handover.After(handover))
	assert.True(t, Timestamp{Sec: 100, Nsec: 588136412}.After(handover))
	assert.True(t, Timestamp{Sec: 101}.After(handover))
	assert.False(t, Timestamp{Sec: 99, Nsec: 999999999}.After(handover))
}

func TestParseMountPoints(t *testing.T) {
	mi := `1 0 0:1 / / rw - overlay overlay rw
2 1 0:2 / /proc rw - proc proc rw
3 1 0:3 /x /etc/hosts rw - ext4 /dev/sda rw
4 1 0:4 / /data/my\040volume rw - nfs srv:/v rw
`
	assert.Equal(t, []string{"/proc", "/etc/hosts", "/data/my volume"}, ParseMountPoints(mi))
}

func TestFilter(t *testing.T) {
	f := NewFilter([]string{"/shared-data", "/data/vol"})
	for _, p := range []string{
		"/", "/proc/1", "/sys", "/dev/shm/x", "/etc/hosts", "/etc/resolv.conf", "/etc/hostname",
		"/shared-data", "/shared-data/launcher.sh", "/data/vol", "/data/vol/a/b",
		"/etc/ssh/ssh_host_rsa_key", "/etc/ssh/ssh_host_ed25519_key.pub",
	} {
		assert.True(t, f.Excluded(p), p)
	}
	for _, p := range []string{
		"/etc", "/etc/hostsfile", "/data/volume", "/data", "/root/x", "/etc/ssh/sshd_config",
		"/home/u/.ssh/ssh_host_rsa_key", "/devices",
	} {
		assert.False(t, f.Excluded(p), p)
	}
}

func entry(p string, typ byte, sec int64) Entry {
	return Entry{Path: p, Type: typ, Ctime: Timestamp{Sec: sec}}
}

// sliceSource is a baseline held in memory, in walk order.
type sliceSource struct{ entries []Entry }

func (s *sliceSource) Next() (Entry, bool, error) {
	if len(s.entries) == 0 {
		return Entry{}, false, nil
	}
	e := s.entries[0]
	s.entries = s.entries[1:]
	return e, true, nil
}

func walkOrder(entries []Entry) []Entry {
	out := append([]Entry{}, entries...)
	sort.Slice(out, func(i, j int) bool { return ComparePaths(out[i].Path, out[j].Path) < 0 })
	return out
}

func baseOf(paths ...string) *sliceSource {
	var es []Entry
	for _, p := range paths {
		es = append(es, Entry{Path: p, Type: 'f'})
	}
	return &sliceSource{walkOrder(es)}
}

func listing(entries ...Entry) func(func(Entry) error) error {
	entries = walkOrder(entries)
	return func(visit func(Entry) error) error {
		for _, e := range entries {
			if err := visit(e); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestComputeChanges(t *testing.T) {
	base := baseOf(
		"/etc", "/etc/debian_version", "/etc/issue",
		"/usr", "/usr/share", "/usr/share/doc",
		"/usr/share/doc/tar", "/usr/share/doc/tar/README", "/usr/share/doc/tar/NEWS",
		"/usr/share/doc/gzip", "/usr/share/doc/gzip/README",
		"/data", "/data/vol", "/data/vol/inner",
		"/opt", "/opt/tool", "/opt/tool/bin",
		"/etc/hosts",
		"/var/lib/dpkg/info/a.list", "/var/lib/dpkg/info/b.list",
	)
	since := Timestamp{Sec: 200}
	current := listing(
		entry("/", 'd', 300),
		entry("/etc", 'd', 300),
		entry("/etc/issue", 'f', 300),                // modified by the user
		entry("/etc/hosts", 'f', 300),                // runtime-managed
		entry("/etc/ssh", 'd', 150),                  // launcher, before hand-over
		entry("/etc/ssh/ssh_host_rsa_key", 'f', 300), // regenerated later: still never exported
		entry("/etc/ssh/sshd_config", 'f', 150),
		entry("/usr", 'd', 100),
		entry("/usr/share", 'd', 100),
		entry("/usr/share/doc", 'd', 300),
		entry("/usr/share/doc/gzip", 'd', 100),
		entry("/usr/share/doc/gzip/README", 'f', 100),
		entry("/usr/sbin/sshd", 'f', 200), // exactly at hand-over: the launcher's
		entry("/data", 'd', 100),
		entry("/data/vol", 'd', 300), // a mount point: its contents are not listed
		entry("/opt", 'd', 300),
		entry("/opt/tool", 'f', 300), // a directory replaced by a file
		entry("/root", 'd', 300),
		entry("/root/new.txt", 'f', 300),
		entry("/run/app.sock", 's', 300),
		entry("/tmp/.wh.odd", 'f', 300),
		entry("/var/lib/dpkg/info/a.list", 'f', 100),
		entry("/var/lib/dpkg/info/c.list", 'f', 300),
	)
	ch, err := ComputeChanges(base, current, since, NewFilter([]string{"/data/vol"}))
	require.NoError(t, err)
	assert.Equal(t, []string{"/etc", "/etc/issue", "/opt", "/opt/tool", "/root", "/root/new.txt", "/usr/share/doc",
		"/var/lib/dpkg/info/c.list"}, ch.Changed)
	// /usr/share/doc/tar/* are covered by the directory's whiteout; /opt/tool/bin by the
	// file that replaced /opt/tool; /data/vol/inner is hidden by a mount, not deleted.
	assert.Equal(t, []string{"/etc/debian_version", "/usr/share/doc/tar", "/var/lib/dpkg/info/b.list"}, ch.Deleted)
	assert.Equal(t, 2, ch.Skipped)
	assert.Zero(t, ch.Unsettled)

	in := ImageContains(ch)
	assert.True(t, in("/var/lib/dpkg/info/a.list"), "in the base image, unchanged")
	assert.False(t, in("/var/lib/dpkg/info/b.list"), "deleted")
	assert.True(t, in("/var/lib/dpkg/info/c.list"), "added")
	assert.False(t, in("/var/lib/dpkg/info/d.list"), "never there")
}

func sortedCopy(s []string) []string {
	out := append([]string{}, s...)
	sort.Slice(out, func(i, j int) bool { return ComparePaths(out[i], out[j]) < 0 })
	return out
}

// The record is made in the background: a directory the user had already changed when
// it was listed may have lost files the record never saw. They are counted.
func TestComputeChangesCountsUnsettledDirectories(t *testing.T) {
	since := Timestamp{Sec: 200}
	base := &sliceSource{[]Entry{
		{Path: "/", Type: 'd', Ctime: Timestamp{Sec: 100}},
		{Path: "/a", Type: 'd', Ctime: Timestamp{Sec: 250}}, // listed after the user changed it
		{Path: "/b", Type: 'd', Ctime: Timestamp{Sec: 150}},
		{Path: "/c", Type: 'f', Ctime: Timestamp{Sec: 250}}, // a file: no entries to lose
	}}
	ch, err := ComputeChanges(base, listing(
		entry("/", 'd', 100), entry("/a", 'd', 250), entry("/b", 'd', 150), entry("/c", 'f', 250),
	), since, NewFilter(nil))
	require.NoError(t, err)
	assert.Equal(t, 1, ch.Unsettled)
}

func TestComputeChangesRefusesAListingOutOfOrder(t *testing.T) {
	_, err := ComputeChanges(baseOf(), func(visit func(Entry) error) error {
		if err := visit(entry("/b", 'f', 1)); err != nil {
			return err
		}
		return visit(entry("/a", 'f', 1))
	}, Timestamp{}, NewFilter(nil))
	assert.Error(t, err)
}

// ComparePaths is the order Walk visits paths in, which both sides of the merge rely on.
func TestComparePathsIsWalkOrder(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"a/b", "a-c", "a.b/x", "a b", "a/d/c", "ab", "a0", "A", "\xff"} {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, nil, 0o644))
	}
	var walked []string
	require.NoError(t, Walk(root, NewFilter(nil), func(e Entry) error {
		walked = append(walked, e.Path)
		return nil
	}))
	assert.Equal(t, sortedCopy(walked), walked)
	assert.Equal(t, -1, ComparePaths("/a/b", "/a-c"))
	assert.Equal(t, 1, ComparePaths("/a-c", "/a/b"))
	assert.Equal(t, -1, ComparePaths("/a", "/a/b"))
	assert.Equal(t, 0, ComparePaths("/a", "/a"))
}

func TestComputeChangesNothingChanged(t *testing.T) {
	ch, err := ComputeChanges(baseOf("/a"), listing(entry("/", 'd', 1), entry("/a", 'f', 1)), Timestamp{Sec: 5}, NewFilter(nil))
	require.NoError(t, err)
	assert.Empty(t, ch.Changed)
	assert.Empty(t, ch.Deleted)
}
