/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad covers defaults, successful parsing and parse errors.
func TestLoad(t *testing.T) {
	// Missing file returns defaults without error.
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if cfg.ServerPort != 8089 || cfg.MetricsPort != 9090 {
		t.Errorf("expected default ports, got %d/%d", cfg.ServerPort, cfg.MetricsPort)
	}

	// Valid YAML overrides defaults.
	valid := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(valid, []byte("server_port: 1234\nmetrics_port: 5678\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err = Load(valid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ServerPort != 1234 || cfg.MetricsPort != 5678 {
		t.Errorf("expected overridden ports, got %d/%d", cfg.ServerPort, cfg.MetricsPort)
	}

	// Invalid YAML returns an error.
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("server_port: : :\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := Load(bad); err == nil {
		t.Error("expected error for invalid yaml, got nil")
	}
}
