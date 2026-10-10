/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The record of the image's files starts only once the launcher's own bootstrap is done.
// The bootstrap (apt-get in build_authoring.sh, for one) deletes and rewrites base-image
// files; a record that listed them first would make the export white them out as the
// user's deletions, and the saved image would lose them.
func TestLauncherRecordsAfterItsBootstrap(t *testing.T) {
	src, err := os.ReadFile("../../../../../docker/preprocess/launcher.sh")
	require.NoError(t, err)
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755))
	}
	// The fake bootstrap waits a while for a record that has already started, so that a
	// record started alongside it is always seen to come first.
	write("build_bnxt.sh", "")
	write("build_authoring.sh", `i=0
while [ ! -e "`+dir+`/record" ] && [ $i -lt 20 ]; do sleep 0.1; i=$((i+1)); done
: > "`+dir+`/bootstrapped"
`)
	write("save-image", `[ "$1" = record ] || exit 0
if [ -e "`+dir+`/bootstrapped" ]; then echo after > "`+dir+`/record"; else echo before > "`+dir+`/record"; fi
`)
	launcher := filepath.Join(dir, "launcher.sh")
	require.NoError(t, os.WriteFile(launcher, []byte(strings.ReplaceAll(string(src), "/shared-data", dir)), 0o755))

	out, err := exec.Command("/bin/sh", launcher).CombinedOutput()
	require.NoError(t, err, string(out))
	var got []byte
	require.Eventually(t, func() bool {
		got, err = os.ReadFile(filepath.Join(dir, "record"))
		return err == nil
	}, 10*time.Second, 50*time.Millisecond, "the record never ran")
	require.Equal(t, "after\n", string(got))
}
