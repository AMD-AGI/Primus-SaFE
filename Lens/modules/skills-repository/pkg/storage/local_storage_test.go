// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLocalStorage(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, "http://localhost:8080/files")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	if storage.basePath != tmpDir {
		t.Errorf("NewLocalStorage() basePath = %v, want %v", storage.basePath, tmpDir)
	}
}

func TestLocalStorage_UploadAndDownload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, "http://localhost:8080/files")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	ctx := context.Background()
	testKey := "test/file.txt"
	testContent := []byte("Hello, World!")

	// Test UploadBytes
	if err := storage.UploadBytes(ctx, testKey, testContent); err != nil {
		t.Errorf("UploadBytes() error = %v", err)
	}

	// Test Exists
	exists, err := storage.Exists(ctx, testKey)
	if err != nil {
		t.Errorf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}

	// Test DownloadBytes
	downloaded, err := storage.DownloadBytes(ctx, testKey)
	if err != nil {
		t.Errorf("DownloadBytes() error = %v", err)
	}
	if !bytes.Equal(downloaded, testContent) {
		t.Errorf("DownloadBytes() = %v, want %v", downloaded, testContent)
	}

	// Test Download (io.Reader version)
	reader, err := storage.Download(ctx, testKey)
	if err != nil {
		t.Errorf("Download() error = %v", err)
	}
	defer reader.Close()

	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("Failed to read: %v", err)
	}
	if !bytes.Equal(readContent, testContent) {
		t.Errorf("Download() content = %v, want %v", readContent, testContent)
	}
}

func TestLocalStorage_Upload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, "http://localhost:8080/files")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	ctx := context.Background()
	testKey := "nested/dir/file.txt"
	testContent := []byte("Nested content")

	// Test Upload with io.Reader
	err = storage.Upload(ctx, testKey, bytes.NewReader(testContent))
	if err != nil {
		t.Errorf("Upload() error = %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, testKey)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Upload() did not create file")
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, "http://localhost:8080/files")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	ctx := context.Background()
	testKey := "to-delete.txt"
	testContent := []byte("Delete me")

	// Create file
	if err := storage.UploadBytes(ctx, testKey, testContent); err != nil {
		t.Fatalf("UploadBytes() error = %v", err)
	}

	// Delete file
	if err := storage.Delete(ctx, testKey); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify file is gone
	exists, err := storage.Exists(ctx, testKey)
	if err != nil {
		t.Errorf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true after delete, want false")
	}
}

func TestLocalStorage_GetURL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	baseURL := "http://localhost:8080/files"
	storage, err := NewLocalStorage(tmpDir, baseURL)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	ctx := context.Background()
	testKey := "test/file.txt"

	url, err := storage.GetURL(ctx, testKey)
	if err != nil {
		t.Errorf("GetURL() error = %v", err)
	}

	expectedURL := baseURL + "/" + testKey
	if url != expectedURL {
		t.Errorf("GetURL() = %v, want %v", url, expectedURL)
	}
}

func TestLocalStorage_ListObjects(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, "http://localhost:8080/files")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	ctx := context.Background()

	// Create files
	files := map[string][]byte{
		"list/file1.txt":        []byte("content1"),
		"list/file2.txt":        []byte("content22"),
		"list/subdir/file3.txt": []byte("content333"),
	}
	for key, content := range files {
		if err := storage.UploadBytes(ctx, key, content); err != nil {
			t.Fatalf("UploadBytes() error = %v", err)
		}
	}

	// List objects
	objects, err := storage.ListObjects(ctx, "list")
	if err != nil {
		t.Errorf("ListObjects() error = %v", err)
	}

	if len(objects) != 3 {
		t.Errorf("ListObjects() returned %d objects, want 3", len(objects))
	}

	// Check sizes
	for _, obj := range objects {
		expectedSize := int64(len(files[obj.Key]))
		if obj.Size != expectedSize {
			t.Errorf("Object %s size = %d, want %d", obj.Key, obj.Size, expectedSize)
		}
	}
}

func TestLocalStorage_NonExistentFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, "http://localhost:8080/files")
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	ctx := context.Background()

	// Test Download non-existent file
	_, err = storage.Download(ctx, "non-existent.txt")
	if err == nil {
		t.Error("Download() expected error for non-existent file")
	}

	// Test Exists for non-existent file
	exists, err := storage.Exists(ctx, "non-existent.txt")
	if err != nil {
		t.Errorf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true for non-existent file")
	}

	// Test Delete non-existent file (should not error)
	err = storage.Delete(ctx, "non-existent.txt")
	if err != nil {
		t.Errorf("Delete() should not error for non-existent file: %v", err)
	}
}
