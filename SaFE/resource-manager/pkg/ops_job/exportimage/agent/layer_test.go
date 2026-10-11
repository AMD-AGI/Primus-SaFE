/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func readLayer(t *testing.T, b []byte) map[string]*tar.Header {
	t.Helper()
	out := map[string]*tar.Header{}
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		out[hdr.Name] = hdr
	}
}

// The first name of a hard-linked inode vanished before its contents could be read: the
// second name carries the contents itself, instead of linking to a name the layer does
// not hold.
func TestWriteLayerHardLinkWhenTheFirstNameVanished(t *testing.T) {
	root := t.TempDir()
	write(t, root, "root/a", "data")
	require.NoError(t, os.Link(filepath.Join(root, "root/a"), filepath.Join(root, "root/b")))
	openFileFailing(t, filepath.Join(root, "root/a"))
	var out bytes.Buffer
	st, err := WriteLayer(&out, root, []string{"/root/a", "/root/b"}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, st.Vanished)
	got := readLayer(t, out.Bytes())
	assert.NotContains(t, got, "root/a")
	require.Contains(t, got, "root/b")
	assert.Equal(t, byte(tar.TypeReg), got["root/b"].Typeflag, "not a link to the vanished name")
	assert.Equal(t, int64(4), got["root/b"].Size)
}

// A file capability (what lets ping open a raw socket without root) is kept, as a PAX
// extended attribute record.
func TestWriteLayerKeepsFileCapabilities(t *testing.T) {
	root := t.TempDir()
	write(t, root, "usr/bin/ping", "ELF")
	full := filepath.Join(root, "usr/bin/ping")
	// cap_net_raw+ep, as setcap writes it (VFS_CAP_REVISION_2).
	capability := []byte{0x01, 0x00, 0x00, 0x02, 0x00, 0x20, 0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if err := unix.Setxattr(full, "security.capability", capability, 0); err != nil {
		t.Skipf("cannot set a file capability here: %v", err)
	}
	require.NoError(t, os.Link(full, filepath.Join(root, "usr/bin/ping4")))
	var out bytes.Buffer
	_, err := WriteLayer(&out, root, []string{"/usr/bin/ping", "/usr/bin/ping4"}, nil, nil)
	require.NoError(t, err)
	got := readLayer(t, out.Bytes())
	require.Contains(t, got, "usr/bin/ping")
	assert.Equal(t, string(capability), got["usr/bin/ping"].PAXRecords["SCHILY.xattr.security.capability"])
	assert.Equal(t, byte(tar.TypeLink), got["usr/bin/ping4"].Typeflag, "the link shares the inode, and its attributes")
}

// Which attributes are kept does not depend on the file system the test runs on.
func TestWriteLayerKeepsTheAttributesThatBelongInAnImage(t *testing.T) {
	root := t.TempDir()
	write(t, root, "usr/bin/ping", "ELF")
	write(t, root, "etc/plain", "x")
	old := readXattrs
	t.Cleanup(func() { readXattrs = old })
	readXattrs = func(full string) (map[string]string, error) {
		if filepath.Base(full) != "ping" {
			return nil, nil
		}
		all := map[string]string{
			"security.capability":    "\x01\x00\x00\x02\x00\x20",
			"user.mime_type":         "application/x-executable",
			"security.selinux":       "system_u:object_r:container_file_t:s0:c1,c2",
			"trusted.overlay.opaque": "y",
		}
		out := map[string]string{}
		for k, v := range all {
			if keepXattr(k) {
				out[k] = v
			}
		}
		return out, nil
	}
	var out bytes.Buffer
	_, err := WriteLayer(&out, root, []string{"/etc/plain", "/usr/bin/ping"}, nil, nil)
	require.NoError(t, err)
	got := readLayer(t, out.Bytes())
	assert.Equal(t, map[string]string{
		"SCHILY.xattr.security.capability": "\x01\x00\x00\x02\x00\x20",
		"SCHILY.xattr.user.mime_type":      "application/x-executable",
	}, xattrRecords(got["usr/bin/ping"]))
	assert.Empty(t, xattrRecords(got["etc/plain"]))
}

func xattrRecords(h *tar.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h.PAXRecords {
		if strings.HasPrefix(k, paxXattr) {
			out[k] = v
		}
	}
	return out
}

// openFileFailing makes one path vanish between being listed and being opened.
func openFileFailing(t *testing.T, gone string) {
	old := openFile
	t.Cleanup(func() { openFile = old })
	openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		if name == gone {
			return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
		}
		return old(name, flag, perm)
	}
}
