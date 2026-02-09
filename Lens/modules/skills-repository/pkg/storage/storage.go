// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/config"
)

// Storage defines the interface for file storage operations
type Storage interface {
	// Upload uploads a file to the storage from an io.Reader
	Upload(ctx context.Context, key string, reader io.Reader) error

	// UploadBytes uploads bytes to the storage
	UploadBytes(ctx context.Context, key string, data []byte) error

	// Download downloads a file from the storage
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// DownloadBytes downloads a file and returns its content as bytes
	DownloadBytes(ctx context.Context, key string) ([]byte, error)

	// Delete deletes a file from the storage
	Delete(ctx context.Context, key string) error

	// Exists checks if a file exists in the storage
	Exists(ctx context.Context, key string) (bool, error)

	// GetURL returns a presigned URL for the file (for S3-compatible storage)
	GetURL(ctx context.Context, key string) (string, error)

	// ListObjects lists all objects with the given prefix
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// ObjectInfo represents information about a stored object
type ObjectInfo struct {
	Key  string
	Size int64
}

// BaseStorage provides default implementations for convenience methods
type BaseStorage struct{}

// UploadBytes uploads bytes using the Upload method
func (b *BaseStorage) UploadBytesHelper(s Storage, ctx context.Context, key string, data []byte) error {
	return s.Upload(ctx, key, bytes.NewReader(data))
}

// DownloadBytes downloads and reads all bytes
func (b *BaseStorage) DownloadBytesHelper(s Storage, ctx context.Context, key string) ([]byte, error) {
	reader, err := s.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// NewStorage creates a new Storage instance based on configuration
func NewStorage(cfg config.StorageConfig) (Storage, error) {
	switch cfg.Provider {
	case "s3", "minio":
		return NewS3Storage(S3Config{
			Endpoint:        cfg.Endpoint,
			PublicURL:       cfg.PublicURL,
			Region:          cfg.Region,
			Bucket:          cfg.Bucket,
			AccessKeyID:     cfg.AccessKey,
			SecretAccessKey: cfg.SecretKey,
			UsePathStyle:    cfg.Provider == "minio", // MinIO uses path style
			URLExpiry:       time.Hour,
		})
	case "local", "":
		basePath := "./data/storage"
		baseURL := "http://localhost:8080/files"
		return NewLocalStorage(basePath, baseURL)
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", cfg.Provider)
	}
}
