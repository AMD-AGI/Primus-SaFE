/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	stanzaBase    = "Package: base-files\nStatus: install ok installed\nArchitecture: amd64\n\n"
	stanzaSSH     = "Package: openssh-server\nStatus: install ok installed\nArchitecture: amd64\n\n"
	stanzaUserPkg = "Package: jq\nStatus: install ok installed\nArchitecture: amd64\n\n"
)

// The launcher's order: the packages the image holds are listed, its bootstrap installs
// openssh-server, the record of the image's files starts, the entry point is handed over,
// and the user installs a package of their own, which rewrites the dpkg status file. The
// saved image carries that status file, but not openssh-server's files, which the
// launcher put there; its status must not claim openssh-server is installed, or the next
// launcher would not install it and sshd would not start.
func TestExportDropsTheLaunchersPackagesRecordedAfterItsBootstrap(t *testing.T) {
	root := t.TempDir()
	write(t, root, "var/lib/dpkg/status", stanzaBase)
	write(t, root, "var/lib/dpkg/info/base-files.list", "/etc/issue\n")
	write(t, root, "etc/issue", "Debian\n")
	shared := filepath.Join(root, "shared-data")
	require.NoError(t, os.MkdirAll(shared, 0o755))
	baseline := filepath.Join(shared, "save-image.base")
	require.NoError(t, RecordPackages(filepath.Join(shared, filepath.Base(PackagesPath)), root))

	tick()
	write(t, root, "usr/sbin/sshd", "installed by the launcher")
	write(t, root, "var/lib/dpkg/info/openssh-server.list", "/usr/sbin/sshd\n")
	write(t, root, "var/lib/dpkg/status", stanzaBase+stanzaSSH)
	tick()
	_, err := Record(baseline, root, testMountinfo)
	require.NoError(t, err)
	write(t, root, ".run.sh", "sleep infinity")
	tick()
	write(t, root, "usr/bin/jq", "installed by the user")
	write(t, root, "var/lib/dpkg/info/jq.list", "/usr/bin/jq\n")
	write(t, root, "var/lib/dpkg/status", stanzaBase+stanzaSSH+stanzaUserPkg)

	r := newTLSRegistry(t)
	env := Env{Root: root, Baseline: baseline, RunFile: filepath.Join(root, ".run.sh"),
		Mountinfo: testMountinfo, Dial: r.dial}
	resp, err := Export(context.Background(), r.request(), env, noRenewal{})
	require.NoError(t, err)
	got := layerMembers(t, r, resp)
	assert.NotContains(t, got, "usr/sbin/sshd", "the launcher's files stay out")
	assert.Equal(t, stanzaBase+stanzaUserPkg, got["var/lib/dpkg/status"])
	assert.Equal(t, []string{"openssh-server"}, resp.DroppedPackages)
}

// An image without dpkg has no packages to list.
func TestRecordPackagesWithoutDpkg(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(t.TempDir(), "packages")
	require.NoError(t, RecordPackages(file, root))
	got, err := readPackages(file)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got)
}
