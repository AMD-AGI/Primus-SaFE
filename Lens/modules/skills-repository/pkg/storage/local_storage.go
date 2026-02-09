// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage implements Storage interface for local filesystem
type LocalStorage struct {
	basePath string
	baseURL  string
}

// NewLocalStorage creates a new LocalStorage
func NewLocalStorage(basePath, baseURL string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &LocalStorage{
		basePath: basePath,
		baseURL:  baseURL,
	}, nil
}

// Upload uploads a file to local storage
func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	filePath := filepath.Join(s.basePath, key)

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// UploadBytes uploads bytes to local storage
func (s *LocalStorage) UploadBytes(ctx context.Context, key string, data []byte) error {
	return s.Upload(ctx, key, bytes.NewReader(data))
}

// Download downloads a file from local storage
func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	filePath := filepath.Join(s.basePath, key)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return file, nil
}

// DownloadBytes downloads a file and returns its content as bytes
func (s *LocalStorage) DownloadBytes(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Delete deletes a file from local storage
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	filePath := filepath.Join(s.basePath, key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// Exists checks if a file exists in local storage
func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	filePath := filepath.Join(s.basePath, key)
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetURL returns a URL for the file
func (s *LocalStorage) GetURL(ctx context.Context, key string) (string, error) {
	return fmt.Sprintf("%s/%s", s.baseURL, key), nil
}

// ListObjects lists all objects with the given prefix
func (s *LocalStorage) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	basePath := filepath.Join(s.basePath, prefix)

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(s.basePath, path)
			objects = append(objects, ObjectInfo{
				Key:  relPath,
				Size: info.Size(),
			})
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return objects, nil
}

// Copy copies a file from srcKey to dstKey
func (s *LocalStorage) Copy(ctx context.Context, srcKey, dstKey string) error {
	srcPath := filepath.Join(s.basePath, srcKey)
	dstPath := filepath.Join(s.basePath, dstKey)

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Read source file
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Write to destination
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	return nil
}

// CopyPrefix copies all files under srcPrefix to dstPrefix
func (s *LocalStorage) CopyPrefix(ctx context.Context, srcPrefix, dstPrefix string) error {
	objects, err := s.ListObjects(ctx, srcPrefix)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		// Calculate new key by replacing prefix
		newKey := dstPrefix + obj.Key[len(srcPrefix):]
		if err := s.Copy(ctx, obj.Key, newKey); err != nil {
			return err
		}
	}

	return nil
}
