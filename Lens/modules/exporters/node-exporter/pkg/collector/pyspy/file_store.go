// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// FileStore manages py-spy output file storage
type FileStore struct {
	config      *config.PySpyConfig
	files       map[string]*FileInfo // taskID -> FileInfo
	filesMu     sync.RWMutex
	profilesDir string
}

// NewFileStore creates a new file store
func NewFileStore(cfg *config.PySpyConfig) (*FileStore, error) {
	profilesDir := filepath.Join(cfg.StoragePath, "profiles")

	// Create profiles directory if it doesn't exist
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create profiles directory: %v", err)
	}

	fs := &FileStore{
		config:      cfg,
		files:       make(map[string]*FileInfo),
		profilesDir: profilesDir,
	}

	// Load existing files from disk
	if err := fs.loadExistingFiles(); err != nil {
		log.Warnf("Failed to load existing files: %v", err)
	}

	return fs, nil
}

// loadExistingFiles scans the profiles directory for existing files
func (fs *FileStore) loadExistingFiles() error {
	entries, err := os.ReadDir(fs.profilesDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskID := entry.Name()
		taskDir := filepath.Join(fs.profilesDir, taskID)

		files, err := os.ReadDir(taskDir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			filePath := filepath.Join(taskDir, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}

			format := fs.detectFormat(file.Name())

			fs.files[taskID] = &FileInfo{
				TaskID:    taskID,
				FileName:  file.Name(),
				FilePath:  filePath,
				FileSize:  info.Size(),
				Format:    format,
				CreatedAt: info.ModTime(),
			}
		}
	}

	log.Infof("Loaded %d existing py-spy files", len(fs.files))
	return nil
}

// detectFormat detects the output format from filename
func (fs *FileStore) detectFormat(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".svg":
		return string(FormatFlamegraph)
	case ".json":
		return string(FormatSpeedscope)
	case ".txt":
		return string(FormatRaw)
	default:
		return string(FormatFlamegraph)
	}
}

// PrepareOutputFile creates the output directory and returns the file path
func (fs *FileStore) PrepareOutputFile(taskID string, format OutputFormat) (string, error) {
	taskDir := filepath.Join(fs.profilesDir, taskID)

	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create task directory: %v", err)
	}

	ext := format.GetFileExtension()
	fileName := fmt.Sprintf("profile.%s", ext)
	filePath := filepath.Join(taskDir, fileName)

	return filePath, nil
}

// RegisterFile registers a completed file in the store
func (fs *FileStore) RegisterFile(taskID, filePath, format string) {
	info, err := os.Stat(filePath)
	if err != nil {
		log.Warnf("Failed to stat file %s: %v", filePath, err)
		return
	}

	fs.filesMu.Lock()
	defer fs.filesMu.Unlock()

	fs.files[taskID] = &FileInfo{
		TaskID:    taskID,
		FileName:  filepath.Base(filePath),
		FilePath:  filePath,
		FileSize:  info.Size(),
		Format:    format,
		CreatedAt: info.ModTime(),
	}
}

// GetFile returns file info for a task
func (fs *FileStore) GetFile(taskID string) (*FileInfo, bool) {
	fs.filesMu.RLock()
	defer fs.filesMu.RUnlock()

	file, ok := fs.files[taskID]
	return file, ok
}

// GetFileByPath returns file info by file path
func (fs *FileStore) GetFileByPath(filePath string) (*FileInfo, bool) {
	fs.filesMu.RLock()
	defer fs.filesMu.RUnlock()

	for _, file := range fs.files {
		if file.FilePath == filePath {
			return file, true
		}
	}
	return nil, false
}

// ListFiles returns a list of files, optionally filtered
func (fs *FileStore) ListFiles(req *FileListRequest) *FileListResponse {
	fs.filesMu.RLock()
	defer fs.filesMu.RUnlock()

	var files []*FileInfo

	for _, file := range fs.files {
		// Apply filters
		if req.TaskID != "" && file.TaskID != req.TaskID {
			continue
		}
		// Note: PodUID filter would require additional metadata storage
		files = append(files, file)
	}

	// Sort by creation time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].CreatedAt.After(files[j].CreatedAt)
	})

	totalCount := len(files)

	// Apply limit
	if req.Limit > 0 && len(files) > req.Limit {
		files = files[:req.Limit]
	}

	return &FileListResponse{
		Files:      files,
		TotalCount: totalCount,
	}
}

// ReadFile reads the content of a file
func (fs *FileStore) ReadFile(taskID, fileName string) (io.ReadCloser, *FileInfo, error) {
	fs.filesMu.RLock()
	file, ok := fs.files[taskID]
	fs.filesMu.RUnlock()

	if !ok {
		return nil, nil, fmt.Errorf("file not found for task: %s", taskID)
	}

	if fileName != "" && file.FileName != fileName {
		return nil, nil, fmt.Errorf("file name mismatch: expected %s, got %s", fileName, file.FileName)
	}

	f, err := os.Open(file.FilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open file: %v", err)
	}

	return f, file, nil
}

// DeleteFile deletes a file for a task
func (fs *FileStore) DeleteFile(taskID string) error {
	fs.filesMu.Lock()
	file, ok := fs.files[taskID]
	if ok {
		delete(fs.files, taskID)
	}
	fs.filesMu.Unlock()

	if !ok {
		return nil // File doesn't exist, nothing to delete
	}

	// Delete file from disk
	taskDir := filepath.Dir(file.FilePath)
	if err := os.RemoveAll(taskDir); err != nil {
		log.Warnf("Failed to delete task directory %s: %v", taskDir, err)
		return err
	}

	return nil
}

// CleanupOldFiles removes files older than retention period
func (fs *FileStore) CleanupOldFiles() (int, error) {
	retentionDuration := time.Duration(fs.config.FileRetentionDays) * 24 * time.Hour
	cutoffTime := time.Now().Add(-retentionDuration)

	fs.filesMu.Lock()
	defer fs.filesMu.Unlock()

	var toDelete []string
	for taskID, file := range fs.files {
		if file.CreatedAt.Before(cutoffTime) {
			toDelete = append(toDelete, taskID)
		}
	}

	deletedCount := 0
	for _, taskID := range toDelete {
		file := fs.files[taskID]
		taskDir := filepath.Dir(file.FilePath)

		if err := os.RemoveAll(taskDir); err != nil {
			log.Warnf("Failed to delete old task directory %s: %v", taskDir, err)
			continue
		}

		delete(fs.files, taskID)
		deletedCount++
	}

	if deletedCount > 0 {
		log.Infof("Cleaned up %d old py-spy files (retention: %d days)", deletedCount, fs.config.FileRetentionDays)
	}

	return deletedCount, nil
}

// GetStorageStats returns storage statistics
func (fs *FileStore) GetStorageStats() (fileCount int, totalSize int64) {
	fs.filesMu.RLock()
	defer fs.filesMu.RUnlock()

	for _, file := range fs.files {
		fileCount++
		totalSize += file.FileSize
	}

	return fileCount, totalSize
}

// StartCleanupRoutine starts a background routine to cleanup old files
func (fs *FileStore) StartCleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fs.CleanupOldFiles()
		}
	}
}

