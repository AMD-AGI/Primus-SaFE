// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package snapshot

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Store implements Store on top of any S3-compatible object storage (MinIO, AWS S3, etc.).
type S3Store struct {
	client     *minio.Client
	bucket     string
	pathPrefix string
}

// NewS3Store creates a new S3Store.
func NewS3Store(cfg S3Config) (*S3Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket %q: %w", cfg.Bucket, err)
		}
	}

	prefix := strings.TrimRight(cfg.PathPrefix, "/")
	if prefix != "" {
		prefix += "/"
	}

	return &S3Store{
		client:     client,
		bucket:     cfg.Bucket,
		pathPrefix: prefix,
	}, nil
}

func (s *S3Store) Type() StoreType { return StoreTypeS3 }

func (s *S3Store) objectKey(storageKey, relPath string) string {
	return s.pathPrefix + storageKey + "/" + relPath
}

// storagePrefix returns the common prefix for all objects under a storage key.
func (s *S3Store) storagePrefix(storageKey string) string {
	return s.pathPrefix + storageKey + "/"
}

func (s *S3Store) Save(ctx context.Context, storageKey string, files []FileEntry) error {
	for _, f := range files {
		key := s.objectKey(storageKey, f.RelPath)
		reader := bytes.NewReader(f.Content)
		_, err := s.client.PutObject(ctx, s.bucket, key, reader, int64(len(f.Content)), minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", key, err)
		}
	}
	return nil
}

func (s *S3Store) Load(ctx context.Context, storageKey string) ([]FileEntry, error) {
	prefix := s.storagePrefix(storageKey)
	var files []FileEntry

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects error: %w", obj.Err)
		}

		reader, err := s.client.GetObject(ctx, s.bucket, obj.Key, minio.GetObjectOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get %s: %w", obj.Key, err)
		}
		content, err := drainAndClose(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", obj.Key, err)
		}

		relPath := strings.TrimPrefix(obj.Key, prefix)
		files = append(files, FileEntry{
			RelPath: relPath,
			Content: content,
			Size:    int64(len(content)),
		})
	}
	return files, nil
}

func (s *S3Store) LoadFile(ctx context.Context, storageKey string, relPath string) ([]byte, error) {
	key := s.objectKey(storageKey, relPath)
	reader, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get %s: %w", key, err)
	}
	return drainAndClose(reader)
}

func (s *S3Store) Delete(ctx context.Context, storageKey string) error {
	prefix := s.storagePrefix(storageKey)
	objectsCh := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsCh)
		for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		}) {
			if obj.Err != nil {
				return
			}
			objectsCh <- obj
		}
	}()

	for errDel := range s.client.RemoveObjects(ctx, s.bucket, objectsCh, minio.RemoveObjectsOptions{}) {
		if errDel.Err != nil {
			return fmt.Errorf("failed to delete %s: %w", errDel.ObjectName, errDel.Err)
		}
	}
	return nil
}

func (s *S3Store) Exists(ctx context.Context, storageKey string) (bool, error) {
	prefix := s.storagePrefix(storageKey)
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		MaxKeys:   1,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return false, obj.Err
		}
		// Found at least one object
		_ = path.Base(obj.Key) // suppress linter
		return true, nil
	}
	return false, nil
}
