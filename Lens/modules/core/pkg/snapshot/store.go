// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package snapshot provides a pluggable storage backend for workload code snapshots.
// It abstracts away whether files are stored on S3-compatible object storage or on a
// local (or network-mounted) filesystem.  Only one backend is active at a time,
// controlled by configuration.
package snapshot

import (
	"context"
	"fmt"
	"io"
)

// StoreType identifies the active storage backend.
type StoreType string

const (
	StoreTypeS3    StoreType = "s3"
	StoreTypeLocal StoreType = "local"
)

// FileEntry describes a single file inside a snapshot.
type FileEntry struct {
	// RelPath is the path relative to the snapshot root, e.g. "src/train.py".
	RelPath string `json:"rel_path"`
	// Content is the raw file bytes.
	Content []byte `json:"-"`
	// Size in bytes (set on read).
	Size int64 `json:"size"`
}

// Store is the interface every snapshot backend must implement.
type Store interface {
	// Type returns the backend type (s3 / local).
	Type() StoreType

	// Save persists all files for a given workload snapshot.
	// storageKey is typically "{workload_uid}/{fingerprint}".
	// Implementations MUST be idempotent: re-saving the same key overwrites.
	Save(ctx context.Context, storageKey string, files []FileEntry) error

	// Load retrieves every file belonging to a snapshot.
	Load(ctx context.Context, storageKey string) ([]FileEntry, error)

	// LoadFile retrieves a single file from a snapshot.
	LoadFile(ctx context.Context, storageKey string, relPath string) ([]byte, error)

	// Delete removes all objects under a storage key.
	Delete(ctx context.Context, storageKey string) error

	// Exists checks whether any object exists under the storage key.
	Exists(ctx context.Context, storageKey string) (bool, error)
}

// Config holds the configuration for initializing a Store.
type Config struct {
	// Type selects the backend: "s3" or "local".
	Type StoreType `yaml:"type" json:"type"`

	// S3 backend settings (used when Type == "s3").
	S3 S3Config `yaml:"s3" json:"s3"`

	// Local backend settings (used when Type == "local").
	Local LocalConfig `yaml:"local" json:"local"`
}

// S3Config configures the S3-compatible backend.
type S3Config struct {
	Endpoint  string `yaml:"endpoint" json:"endpoint"`
	Bucket    string `yaml:"bucket" json:"bucket"`
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	Secure    bool   `yaml:"secure" json:"secure"`
	// PathPrefix is prepended to every object key, e.g. "code-snapshots/".
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`
}

// LocalConfig configures the local filesystem backend.
type LocalConfig struct {
	// RootDir is the base directory for all snapshots.
	RootDir string `yaml:"root_dir" json:"root_dir"`
}

// New creates a Store from the given Config.
func New(cfg Config) (Store, error) {
	switch cfg.Type {
	case StoreTypeS3:
		return NewS3Store(cfg.S3)
	case StoreTypeLocal:
		return NewLocalStore(cfg.Local)
	default:
		return nil, fmt.Errorf("unknown snapshot store type: %q", cfg.Type)
	}
}

// StorageKeyFor builds the canonical storage key for a workload snapshot.
func StorageKeyFor(workloadUID, fingerprint string) string {
	return workloadUID + "/" + fingerprint
}

// drainAndClose is a tiny helper used by backends to fully consume and close a reader.
func drainAndClose(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}
