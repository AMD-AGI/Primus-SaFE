/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetSSOBridgeAllowlist(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	// Unset means same origin only.
	assert.Empty(t, GetSSOBridgeAllowlist())

	viper.Set(ssoBridgeAllowlist, []string{"https://a.example.com/x", " ", "https://b.example.com"})
	assert.Equal(t, []string{"https://a.example.com/x", "https://b.example.com"}, GetSSOBridgeAllowlist())

	viper.Set(ssoBridgeAllowlist, "https://a.example.com, https://b.example.com")
	assert.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, GetSSOBridgeAllowlist())
}
