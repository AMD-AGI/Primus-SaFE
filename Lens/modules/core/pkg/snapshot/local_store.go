// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalStore implements Store on a local (or network-mounted) filesystem.
type LocalStore struct {
	rootDir string
}

// NewLocalStore creates a new LocalStore.
func NewLocalStore(cfg LocalConfig) (*LocalStore, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("local snapshot store root_dir must not be empty")
	}
	if err := os.MkdirAll(cfg.RootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root dir %s: %w", cfg.RootDir, err)
	}
	return &LocalStore{rootDir: cfg.RootDir}, nil
}

func (l *LocalStore) Type() StoreType { return StoreTypeLocal }

// dir returns the absolute directory for a given storage key.
func (l *LocalStore) dir(storageKey string) string {
	return filepath.Join(l.rootDir, storageKey)
}

func (l *LocalStore) Save(_ context.Context, storageKey string, files []FileEntry) error {
	baseDir := l.dir(storageKey)
	for _, f := range files {
		fullPath := filepath.Join(baseDir, f.RelPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed to create dir for %s: %w", f.RelPath, err)
		}
		if err := os.WriteFile(fullPath, f.Content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.RelPath, err)
		}
	}
	return nil
}

func (l *LocalStore) Load(_ context.Context, storageKey string) ([]FileEntry, error) {
	baseDir := l.dir(storageKey)
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, nil
	}

	var files []FileEntry
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(baseDir, path)
		relPath = filepath.ToSlash(relPath)

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files = append(files, FileEntry{
			RelPath: relPath,
			Content: content,
			Size:    info.Size(),
		})
		return nil
	})
	return files, err
}

func (l *LocalStore) LoadFile(_ context.Context, storageKey string, relPath string) ([]byte, error) {
	fullPath := filepath.Join(l.dir(storageKey), relPath)
	return os.ReadFile(fullPath)
}

func (l *LocalStore) Delete(_ context.Context, storageKey string) error {
	return os.RemoveAll(l.dir(storageKey))
}

func (l *LocalStore) Exists(_ context.Context, storageKey string) (bool, error) {
	baseDir := l.dir(storageKey)
	info, err := os.Stat(baseDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Directory must contain at least one file
	if !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

