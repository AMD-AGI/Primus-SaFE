package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// NewStorageBackend creates a storage backend based on configuration
func NewStorageBackend(db *sql.DB, config *StorageConfig) (StorageBackend, error) {
	if config == nil {
		return nil, fmt.Errorf("storage config is nil")
	}

	switch config.Strategy {
	case "object_storage":
		if config.Object == nil {
			return nil, fmt.Errorf("object storage config is missing")
		}
		return NewObjectStorageBackend(config.Object)

	case "database":
		if db == nil {
			return nil, fmt.Errorf("database connection is required for database storage")
		}
		if config.Database == nil {
			return nil, fmt.Errorf("database storage config is missing")
		}
		return NewDatabaseStorageBackend(db, config.Database)

	case "auto":
		if config.Auto == nil || !config.Auto.Enabled {
			return nil, fmt.Errorf("auto selection config is missing or disabled")
		}
		return NewAutoSelectBackend(db, config)

	default:
		return nil, fmt.Errorf("unknown storage strategy: %s", config.Strategy)
	}
}

// AutoSelectBackend automatically selects storage backend based on file size
type AutoSelectBackend struct {
	objectBackend   StorageBackend
	databaseBackend StorageBackend
	sizeThreshold   int64
}

// NewAutoSelectBackend creates an auto-select storage backend
func NewAutoSelectBackend(db *sql.DB, config *StorageConfig) (*AutoSelectBackend, error) {
	if config.Auto == nil {
		return nil, fmt.Errorf("auto selection config is nil")
	}

	// Create both backends
	var objectBackend StorageBackend
	var databaseBackend StorageBackend
	var err error

	// Create object storage backend
	if config.Object != nil {
		objectBackend, err = NewObjectStorageBackend(config.Object)
		if err != nil {
			log.Warnf("Failed to create object storage backend: %v", err)
		}
	}

	// Create database backend
	if db != nil && config.Database != nil {
		databaseBackend, err = NewDatabaseStorageBackend(db, config.Database)
		if err != nil {
			log.Warnf("Failed to create database storage backend: %v", err)
		}
	}

	// Require at least one backend
	if objectBackend == nil && databaseBackend == nil {
		return nil, fmt.Errorf("failed to create any storage backend")
	}

	backend := &AutoSelectBackend{
		objectBackend:   objectBackend,
		databaseBackend: databaseBackend,
		sizeThreshold:   config.Auto.SizeThreshold,
	}

	log.Infof("Initialized auto-select storage backend: threshold=%d bytes", config.Auto.SizeThreshold)
	return backend, nil
}

// selectBackend selects the appropriate backend based on file size
func (a *AutoSelectBackend) selectBackend(size int64) StorageBackend {
	// If file is smaller than threshold, use database (if available)
	if size < a.sizeThreshold && a.databaseBackend != nil {
		log.Debugf("Selected database storage for file of size %d bytes", size)
		return a.databaseBackend
	}

	// Otherwise, use object storage (if available)
	if a.objectBackend != nil {
		log.Debugf("Selected object storage for file of size %d bytes", size)
		return a.objectBackend
	}

	// Fallback to database if object storage is not available
	log.Debugf("Falling back to database storage for file of size %d bytes", size)
	return a.databaseBackend
}

// Store stores a file using the selected backend
func (a *AutoSelectBackend) Store(ctx context.Context, req *StoreRequest) (*StoreResponse, error) {
	backend := a.selectBackend(int64(len(req.Content)))
	return backend.Store(ctx, req)
}

// Retrieve retrieves a file (need to check both backends)
func (a *AutoSelectBackend) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
	// Try database first (faster for small files)
	if a.databaseBackend != nil {
		exists, err := a.databaseBackend.Exists(ctx, req.FileID)
		if err == nil && exists {
			return a.databaseBackend.Retrieve(ctx, req)
		}
	}

	// Try object storage
	if a.objectBackend != nil {
		exists, err := a.objectBackend.Exists(ctx, req.FileID)
		if err == nil && exists {
			return a.objectBackend.Retrieve(ctx, req)
		}
	}

	return nil, fmt.Errorf("file not found in any storage backend: %s", req.FileID)
}

// Delete deletes a file from all backends
func (a *AutoSelectBackend) Delete(ctx context.Context, fileID string) error {
	var lastErr error

	// Try deleting from database
	if a.databaseBackend != nil {
		if err := a.databaseBackend.Delete(ctx, fileID); err != nil {
			lastErr = err
			log.Warnf("Failed to delete from database: %v", err)
		}
	}

	// Try deleting from object storage
	if a.objectBackend != nil {
		if err := a.objectBackend.Delete(ctx, fileID); err != nil {
			lastErr = err
			log.Warnf("Failed to delete from object storage: %v", err)
		}
	}

	return lastErr
}

// GenerateDownloadURL generates a download URL
func (a *AutoSelectBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	// Check database first
	if a.databaseBackend != nil {
		exists, err := a.databaseBackend.Exists(ctx, fileID)
		if err == nil && exists {
			return a.databaseBackend.GenerateDownloadURL(ctx, fileID, expires)
		}
	}

	// Check object storage
	if a.objectBackend != nil {
		exists, err := a.objectBackend.Exists(ctx, fileID)
		if err == nil && exists {
			return a.objectBackend.GenerateDownloadURL(ctx, fileID, expires)
		}
	}

	return "", fmt.Errorf("file not found: %s", fileID)
}

// GetStorageType returns the storage type identifier
func (a *AutoSelectBackend) GetStorageType() string {
	return "auto"
}

// Exists checks if a file exists in any backend
func (a *AutoSelectBackend) Exists(ctx context.Context, fileID string) (bool, error) {
	// Check database
	if a.databaseBackend != nil {
		exists, err := a.databaseBackend.Exists(ctx, fileID)
		if err == nil && exists {
			return true, nil
		}
	}

	// Check object storage
	if a.objectBackend != nil {
		exists, err := a.objectBackend.Exists(ctx, fileID)
		if err == nil && exists {
			return true, nil
		}
	}

	return false, nil
}

// ExistsByWorkloadAndFilename checks if a file with the same name already exists for the workload
func (a *AutoSelectBackend) ExistsByWorkloadAndFilename(ctx context.Context, workloadUID string, fileName string) (bool, error) {
	// Check database first
	if a.databaseBackend != nil {
		exists, err := a.databaseBackend.ExistsByWorkloadAndFilename(ctx, workloadUID, fileName)
		if err == nil && exists {
			return true, nil
		}
	}

	// Check object storage
	if a.objectBackend != nil {
		exists, err := a.objectBackend.ExistsByWorkloadAndFilename(ctx, workloadUID, fileName)
		if err == nil && exists {
			return true, nil
		}
	}

	return false, nil
}
