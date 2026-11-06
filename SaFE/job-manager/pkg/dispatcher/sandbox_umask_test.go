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
	"testing"

	"gotest.tools/assert"
)

// envdExec is how the startup script hands over to envd.
const envdExec = "exec /shared/bin/envd --port=8080 --workspace=/app"

// startupUmasks runs the startup script from the signing-key steps on, in a shell that
// starts with an image's usual umask 022. SANDBOX_STARTUP records its umask, and envd is
// replaced by a process exec'd the same way that records its umask and creates a file and a
// directory, as a command envd starts would. It returns the two umasks and the directory
// the files are in.
func startupUmasks(t *testing.T, env ...string) (startup, envd, out string) {
	t.Helper()
	start := strings.Index(envdStartupScript, `if [ -n "${HYPERLOOM_SIGNING_KEY_PEM-}" ]`)
	assert.Assert(t, start > 0)
	rest, found := strings.CutSuffix(envdStartupScript[start:], envdExec)
	assert.Assert(t, found, "the startup script does not end by exec'ing envd")
	out = t.TempDir()
	script := "umask 022; " + rest +
		`exec /bin/sh -c 'umask > "$OUT/envd-umask"; : > "$OUT/file"; mkdir "$OUT/dir"'`
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"OUT=" + out,
		`SANDBOX_STARTUP=umask > "$OUT/startup-umask"`,
	}, env...)
	output, err := cmd.CombinedOutput()
	assert.NilError(t, err, "%s", output)
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		assert.NilError(t, err)
		return strings.TrimSpace(string(b))
	}
	return read("startup-umask"), read("envd-umask"), out
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	assert.NilError(t, err)
	return info.Mode().Perm()
}

// SANDBOX_STARTUP, envd and what envd starts run under umask 002, so a file or directory a
// session creates is writable by the group of the directory it lands in. That holds when the
// signing key was written from the environment under umask 077 a step earlier: the key stays
// 0400, and its umask is not the one envd inherits.
func TestSandboxStartupRunsEnvdUnderUmask002(t *testing.T) {
	keyDir := t.TempDir()
	for name, env := range map[string][]string{
		"no key in the environment": nil,
		"key written from the environment": {
			"HYPERLOOM_SIGNING_KEY_FILE=" + filepath.Join(keyDir, "key.pem"),
			"HYPERLOOM_SIGNING_KEY_PEM=PEM",
		},
	} {
		t.Run(name, func(t *testing.T) {
			startup, envd, out := startupUmasks(t, env...)
			assert.Equal(t, startup, "0002")
			assert.Equal(t, envd, "0002")
			assert.Equal(t, modeOf(t, filepath.Join(out, "file")), os.FileMode(0o664))
			assert.Equal(t, modeOf(t, filepath.Join(out, "dir")), os.FileMode(0o775))
		})
	}
	assert.Equal(t, modeOf(t, filepath.Join(keyDir, "key.pem")), os.FileMode(0o400))
}
