/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

// TestWriteFile writes and reads back file content.
func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	err := WriteFile(path, "hello", 0644)
	assert.NilError(t, err)
	data, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(data), "hello")
}

// TestIsFileExist checks file presence for existing and missing paths.
func TestIsFileExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	assert.Equal(t, IsFileExist(path), false)
	assert.NilError(t, os.WriteFile(path, []byte("x"), 0644))
	assert.Equal(t, IsFileExist(path), true)
	assert.Equal(t, IsFileExist(dir), false)
}

// TestGetDirWatcher creates a watcher for an existing directory.
func TestGetDirWatcher(t *testing.T) {
	dir := t.TempDir()
	watcher, err := GetDirWatcher(dir)
	assert.NilError(t, err)
	assert.Assert(t, watcher != nil)
	assert.NilError(t, watcher.Close())
}

// TestGetDirWatcherInvalidPath returns error for a missing directory.
func TestGetDirWatcherInvalidPath(t *testing.T) {
	_, err := GetDirWatcher(filepath.Join(t.TempDir(), "missing-subdir"))
	assert.Assert(t, err != nil)
}

// TestWriteFileInvalidPath returns error when the target path cannot be created.
func TestWriteFileInvalidPath(t *testing.T) {
	err := WriteFile(filepath.Join(t.TempDir(), "missing-dir", "out.txt"), "data", 0644)
	assert.Assert(t, err != nil)
}
