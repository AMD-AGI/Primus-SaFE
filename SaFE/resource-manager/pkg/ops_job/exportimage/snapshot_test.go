/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimestamp(t *testing.T) {
	ts, err := ParseTimestamp("1791483545.5881364110")
	require.NoError(t, err)
	assert.Equal(t, Timestamp{Sec: 1791483545, Nsec: 588136411}, ts)

	ts, err = ParseTimestamp("1791483545.5")
	require.NoError(t, err)
	assert.Equal(t, Timestamp{Sec: 1791483545, Nsec: 500000000}, ts)

	ts, err = ParseTimestamp("17")
	require.NoError(t, err)
	assert.Equal(t, Timestamp{Sec: 17}, ts)

	for _, bad := range []string{"", "x", "1.2x", "1.-2"} {
		_, err := ParseTimestamp(bad)
		assert.Error(t, err, bad)
	}
}

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

func TestParseListing(t *testing.T) {
	out := "d 1.0000000000 /\x00f 2.5000000000 /etc/a b\x00l 3.0 /usr/bin/x\nwith-newline\x00"
	entries, err := ParseListing(strings.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, []Entry{
		{Path: "/", Type: 'd', Ctime: Timestamp{Sec: 1}},
		{Path: "/etc/a b", Type: 'f', Ctime: Timestamp{Sec: 2, Nsec: 500000000}},
		{Path: "/usr/bin/x\nwith-newline", Type: 'l', Ctime: Timestamp{Sec: 3}},
	}, entries)

	_, err = ParseListing(strings.NewReader("f 1.0 /a\x00f 1.0 /b"))
	assert.Error(t, err, "a listing cut short must not parse as complete")
	_, err = ParseListing(strings.NewReader("garbage\x00"))
	assert.Error(t, err)
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

func TestComputeChanges(t *testing.T) {
	base := map[string]bool{
		"/etc": true, "/etc/debian_version": true, "/etc/issue": true,
		"/usr": true, "/usr/share": true, "/usr/share/doc": true,
		"/usr/share/doc/tar": true, "/usr/share/doc/tar/README": true, "/usr/share/doc/tar/NEWS": true,
		"/usr/share/doc/gzip": true, "/usr/share/doc/gzip/README": true,
		"/data": true, "/data/vol": true, "/data/vol/inner": true,
		"/opt": true, "/opt/tool": true, "/opt/tool/bin": true,
		"/etc/hosts": true,
	}
	since := Timestamp{Sec: 200}
	current := []Entry{
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
	}
	ch := ComputeChanges(base, current, since, NewFilter([]string{"/data/vol"}))
	assert.Equal(t, []string{"/etc", "/etc/issue", "/opt", "/opt/tool", "/root", "/root/new.txt", "/usr/share/doc"}, ch.Changed)
	// /usr/share/doc/tar/* are covered by the directory's whiteout; /opt/tool/bin by the
	// file that replaced /opt/tool; /data/vol/inner is hidden by a mount, not deleted.
	assert.Equal(t, []string{"/etc/debian_version", "/usr/share/doc/tar"}, ch.Deleted)
	assert.Equal(t, 2, ch.Skipped)
}

func TestComputeChangesNothingChanged(t *testing.T) {
	base := map[string]bool{"/a": true}
	ch := ComputeChanges(base, []Entry{entry("/", 'd', 1), entry("/a", 'f', 1)}, Timestamp{Sec: 5}, NewFilter(nil))
	assert.Empty(t, ch.Changed)
	assert.Empty(t, ch.Deleted)
}
