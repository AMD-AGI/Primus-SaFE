/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func resetShared(t *testing.T) {
	t.Helper()
	sharedMu.Lock()
	sharedClient = nil
	sharedFingerprint = [32]byte{}
	sharedMu.Unlock()
	t.Cleanup(func() {
		sharedMu.Lock()
		sharedClient = nil
		sharedFingerprint = [32]byte{}
		sharedMu.Unlock()
		viper.Set("external_execution.enabled", false)
		viper.Set("external_execution.controller_url", "")
		viper.Set("external_execution.controller_secret_path", "")
	})
}

func writeTLSDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	certPEM, keyPEM := selfSignedPEM(t)
	for name, body := range map[string][]byte{"ca.crt": certPEM, "tls.crt": certPEM, "tls.key": keyPEM} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSharedRefusesAnUnconfiguredController(t *testing.T) {
	resetShared(t)
	if _, err := Shared(); err == nil {
		t.Fatal("disabled feature must fail")
	}
	viper.Set("external_execution.enabled", true)
	if _, err := Shared(); err == nil {
		t.Fatal("missing url must fail")
	}
	commonconfig.SetValue("external_execution.controller_url", "https://capacity.internal")
	if _, err := Shared(); err == nil {
		t.Fatal("missing tls must fail")
	}
	commonconfig.SetValue("external_execution.controller_url", "http://capacity.internal")
	commonconfig.SetValue("external_execution.controller_secret_path", writeTLSDir(t))
	if _, err := Shared(); err == nil {
		t.Fatal("http url must fail")
	}
}

func TestSharedReusesTheClientUntilMaterialChanges(t *testing.T) {
	resetShared(t)
	viper.Set("external_execution.enabled", true)
	commonconfig.SetValue("external_execution.controller_url", "https://capacity.internal")
	commonconfig.SetValue("external_execution.controller_secret_path", writeTLSDir(t))
	first, err := Shared()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Shared()
	if err != nil || second != first {
		t.Fatal("same material must reuse the client")
	}
	commonconfig.SetValue("external_execution.controller_secret_path", writeTLSDir(t))
	third, err := Shared()
	if err != nil || third == first {
		t.Fatal("rotated material must rebuild the client")
	}
}
