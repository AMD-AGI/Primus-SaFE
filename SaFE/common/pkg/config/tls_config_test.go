/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestOutboundTLSVerifyFromConfig locks the config-key wiring: the value the
// Helm chart writes (tls.verify_outbound) must drive IsOutboundTLSVerifyEnabled.
// This guards against the getter and the chart drifting apart.
func TestOutboundTLSVerifyFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "enabled", content: "tls:\n  verify_outbound: true\n", want: true},
		{name: "disabled", content: "tls:\n  verify_outbound: false\n", want: false},
		{name: "absent defaults to false", content: "server:\n  port: 8088\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			viper.Reset()
			if err := LoadConfig(path); err != nil {
				t.Fatalf("load config: %v", err)
			}
			assert.Equal(t, tt.want, IsOutboundTLSVerifyEnabled())
		})
	}
}