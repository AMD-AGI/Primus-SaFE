package storage

import (
	"context"
	"time"
)

// StorageBackend defines the unified storage interface for profiler files
type StorageBackend interface {
	// Store stores a file with the given content
	Store(ctx context.Context, req *StoreRequest) (*StoreResponse, error)

	// Retrieve retrieves a file by its storage path
	Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error)

	// Delete deletes a file by its file ID
	Delete(ctx context.Context, fileID string) error

	// GenerateDownloadURL generates a download URL for the file
	GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error)

	// GetStorageType returns the storage type identifier
	GetStorageType() string

	// Exists checks if a file exists
	Exists(ctx context.Context, fileID string) (bool, error)
}

// StoreRequest represents a file storage request
type StoreRequest struct {
	FileID      string            `json:"file_id"`
	WorkloadUID string            `json:"workload_uid"`
	FileName    string            `json:"file_name"`
	FileType    string            `json:"file_type"`
	Content     []byte            `json:"-"` // Binary content
	Compressed  bool              `json:"compressed"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// StoreResponse represents a file storage response
type StoreResponse struct {
	FileID      string                 `json:"file_id"`
	StoragePath string                 `json:"storage_path"` // Object key or database ID
	StorageType string                 `json:"storage_type"` // "object_storage" or "database"
	Size        int64                  `json:"size"`
	MD5         string                 `json:"md5"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// RetrieveRequest represents a file retrieval request
type RetrieveRequest struct {
	FileID      string `json:"file_id"`
	StoragePath string `json:"storage_path"`
	Offset      int64  `json:"offset,omitempty"` // Support partial read
	Length      int64  `json:"length,omitempty"` // Support partial read
}

// RetrieveResponse represents a file retrieval response
type RetrieveResponse struct {
	Content    []byte `json:"-"` // Binary content
	Size       int64  `json:"size"`
	Compressed bool   `json:"compressed"`
	MD5        string `json:"md5"`
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	Strategy string               `yaml:"strategy"` // "object_storage", "database", or "auto"
	Object   *ObjectStorageConfig `yaml:"object_storage,omitempty"`
	Database *DatabaseConfig      `yaml:"database,omitempty"`
	Auto     *AutoSelectConfig    `yaml:"auto_select,omitempty"`
}

// ObjectStorageConfig represents object storage configuration
type ObjectStorageConfig struct {
	Type       string `yaml:"type"`        // "minio" or "s3"
	Endpoint   string `yaml:"endpoint"`    // MinIO/S3 endpoint
	Bucket     string `yaml:"bucket"`      // Bucket name
	AccessKey  string `yaml:"access_key"`  // Access key
	SecretKey  string `yaml:"secret_key"`  // Secret key
	UseSSL     bool   `yaml:"use_ssl"`     // Use SSL/TLS
	Region     string `yaml:"region"`      // AWS region (for S3)
	URLExpires string `yaml:"url_expires"` // Presigned URL expiration (e.g., "168h")
}

// DatabaseConfig represents database storage configuration
type DatabaseConfig struct {
	Compression          bool  `yaml:"compression"`             // Enable gzip compression
	ChunkSize            int64 `yaml:"chunk_size"`              // Chunk size for large files (bytes)
	MaxFileSize          int64 `yaml:"max_file_size"`           // Max file size (bytes)
	MaxConcurrentChunks  int   `yaml:"max_concurrent_chunks"`   // Max concurrent chunk operations
}

// AutoSelectConfig represents auto-selection configuration
type AutoSelectConfig struct {
	Enabled       bool  `yaml:"enabled"`        // Enable auto selection
	SizeThreshold int64 `yaml:"size_threshold"` // Size threshold for choosing storage
}

