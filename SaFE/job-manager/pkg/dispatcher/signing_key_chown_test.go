//go:build linux

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"gotest.tools/assert"
)

// signingKeyCopy is the injector's signing-key loop with its paths moved under
// dir, so it can run outside a Pod.
func signingKeyCopy(t *testing.T, dir, uidMap string) string {
	t.Helper()
	start := strings.Index(envdInjectorScript, "for f in /etc/secrets/")
	end := strings.Index(envdInjectorScript, "\ndone\n")
	assert.Assert(t, start > 0 && end > start)
	assert.NilError(t, os.MkdirAll(filepath.Join(dir, "secrets", "sbx"), 0o700))
	assert.NilError(t, os.MkdirAll(filepath.Join(dir, "run"), 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "secrets", "sbx", "signing-key.pem"), []byte("KEY\n"), 0o600))
	loop := envdInjectorScript[start : end+len("\ndone")]
	loop = strings.ReplaceAll(loop, "/etc/secrets/", filepath.Join(dir, "secrets")+"/")
	loop = strings.ReplaceAll(loop, "/run/signing-key/", filepath.Join(dir, "run")+"/")
	loop = strings.ReplaceAll(loop, "/proc/self/uid_map", uidMap)
	assert.Assert(t, !strings.Contains(loop, "/etc/secrets") && !strings.Contains(loop, "/run/signing-key/"), "%s", loop)
	return "set -e\n" + loop
}

// failingChown is a PATH whose chown fails the way it does for an id the user
// namespace does not map.
func failingChown(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	assert.NilError(t, os.MkdirAll(bin, 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(bin, "chown"),
		[]byte("#!/bin/sh\necho \"chown: $2: Invalid argument\" >&2\nexit 1\n"), 0o700))
	return bin + ":" + os.Getenv("PATH")
}

func TestEnvdInjectorToleratesTheChownOnlyWhereOneUIDIsMapped(t *testing.T) {
	cases := map[string]struct {
		uidMap string
		ok     bool
	}{
		"root mapped to the account alone": {uidMap: "         0   50114255          1\n", ok: true},
		"the whole uid range":              {uidMap: "         0          0 4294967295\n", ok: false},
		"root and a subuid range":          {uidMap: "0 50114255 1\n1 100000 65536\n", ok: false},
		"one uid, but not root":            {uidMap: "1000 50114255 1\n", ok: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			uidMap := filepath.Join(dir, "uid_map")
			assert.NilError(t, os.WriteFile(uidMap, []byte(tc.uidMap), 0o600))
			cmd := exec.Command("sh", "-c", signingKeyCopy(t, dir, uidMap))
			cmd.Env = []string{"PATH=" + failingChown(t, dir)}
			out, err := cmd.CombinedOutput()
			if !tc.ok {
				assert.Assert(t, err != nil, "the injector went on after a failed chown: %s", out)
				return
			}
			assert.NilError(t, err, "%s", out)
			info, err := os.Stat(filepath.Join(dir, "run", "signing-key.pem"))
			assert.NilError(t, err)
			assert.Equal(t, info.Mode().Perm(), os.FileMode(0o400))
		})
	}
}

// In a real root-mapped user namespace, uid 1000 does not exist and the key
// still reaches the container.
func TestEnvdInjectorCopiesTheKeyInARootMappedUserNamespace(t *testing.T) {
	ns := &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}
	probe := exec.Command("true")
	probe.SysProcAttr = ns
	if err := probe.Run(); err != nil {
		t.Skipf("this environment cannot make a user namespace: %v", err)
	}
	dir := t.TempDir()
	cmd := exec.Command("sh", "-c", signingKeyCopy(t, dir, "/proc/self/uid_map"))
	cmd.SysProcAttr = ns
	out, err := cmd.CombinedOutput()
	assert.NilError(t, err, "%s", out)
	info, err := os.Stat(filepath.Join(dir, "run", "signing-key.pem"))
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o400))
}
