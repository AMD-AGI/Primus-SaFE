/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"slices"
	"testing"

	"gotest.tools/assert"
)

func load() error {
	path := "./test.yaml"
	if err := LoadConfig(path); err != nil {
		return err
	}
	return nil
}

func TestConfig(t *testing.T) {
	err := load()
	assert.NilError(t, err)

	assert.Equal(t, getInt("server.port", 0), 8080)
	assert.Equal(t, getString("server.timeout", ""), "30s")
	assert.Equal(t, getBool("server.enable", false), true)
	assert.Equal(t, getFloat("server.ratio", 0), 0.01)

	assert.Equal(t, getString("database.host", ""), "localhost")
	assert.Equal(t, getInt("database.port", 8081), 8081)
	assert.Equal(t, getInt("database.request_timeout_second", 0), 20)
	assert.Equal(t, slices.Equal(getStrings("database.users"), []string{"user1", "user2"}), true)
}
