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

// signingKeyFileAfterStartup runs the startup script's signing-key steps (the
// PEM block and what follows it, up to SANDBOX_STARTUP) and prints what
// HYPERLOOM_SIGNING_KEY_FILE is left as.
func signingKeyFileAfterStartup(t *testing.T, env ...string) string {
	t.Helper()
	start := strings.Index(envdStartupScript, `if [ -n "${HYPERLOOM_SIGNING_KEY_PEM-}" ]`)
	end := strings.Index(envdStartupScript, ` && if [ -n "${SANDBOX_STARTUP-}" ]`)
	assert.Assert(t, start > 0 && end > start)
	script := envdStartupScript[start:end] + ` && printf '%s' "${HYPERLOOM_SIGNING_KEY_FILE-UNSET}"`
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	assert.NilError(t, err, "%s", out)
	return string(out)
}

func TestStartupUnsetsTheSigningKeyFileWhenNoKeyIsThere(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "signing-key.pem")

	// The platform names the tmpfs copy for every Sandbox; without a key it is dropped.
	assert.Equal(t, signingKeyFileAfterStartup(t, "HYPERLOOM_SIGNING_KEY_FILE="+key), "UNSET")
	assert.Equal(t, signingKeyFileAfterStartup(t), "UNSET")

	// The injector copied a key: the variable stays.
	assert.NilError(t, os.WriteFile(key, []byte("KEY\n"), 0o400))
	assert.Equal(t, signingKeyFileAfterStartup(t, "HYPERLOOM_SIGNING_KEY_FILE="+key), key)

	// A PEM in the environment is written to the named file, which then stays named.
	pemFile := filepath.Join(dir, "pem", "key.pem")
	assert.Equal(t, signingKeyFileAfterStartup(t,
		"HYPERLOOM_SIGNING_KEY_FILE="+pemFile, "HYPERLOOM_SIGNING_KEY_PEM=PEM"), pemFile)
	got, err := os.ReadFile(pemFile)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "PEM\n")
}
