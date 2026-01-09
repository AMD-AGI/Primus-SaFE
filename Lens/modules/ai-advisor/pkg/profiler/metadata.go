// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package profiler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// MetadataManager manages profiler file metadata
type MetadataManager struct {
	db *sql.DB
}

// NewMetadataManager creates a new metadata manager
func NewMetadataManager(db *sql.DB) (*MetadataManager, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	return &MetadataManager{db: db}, nil
}

// ProfilerFileMetadata represents profiler file metadata in database
type ProfilerFileMetadata struct {
	ID             int64                  `json:"id"`
	WorkloadUID    string                 `json:"workload_uid"`
	PodUID         string                 `json:"pod_uid"`
	PodName        string                 `json:"pod_name"`
	PodNamespace   string                 `json:"pod_namespace"`
	FileName       string                 `json:"file_name"`
	FilePath       string                 `json:"file_path"`
	FileType       string                 `json:"file_type"`
	FileSize       int64                  `json:"file_size"`
	StorageType    string                 `json:"storage_type"`
	StoragePath    string                 `json:"storage_path"`
	StorageBucket  string                 `json:"storage_bucket,omitempty"`
	DownloadURL    string                 `json:"download_url"`
	Confidence     string                 `json:"confidence"`
	SourcePID      int                    `json:"source_pid"`
	DetectedAt     time.Time              `json:"detected_at"`
	CollectedAt    time.Time              `json:"collected_at"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SaveMetadata saves profiler file metadata to database
func (m *MetadataManager) SaveMetadata(ctx context.Context, req *SaveMetadataRequest) (*ProfilerFileMetadata, error) {
	query := `
		INSERT INTO profiler_files (
			workload_uid, pod_uid, pod_name, pod_namespace,
			file_name, file_path, file_type, file_size,
			storage_type, storage_path, storage_bucket,
			download_url, confidence, source_pid,
			detected_at, collected_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17
		) RETURNING id, created_at, updated_at
	`

	var id int64
	var createdAt, updatedAt time.Time

	err := m.db.QueryRowContext(ctx, query,
		req.WorkloadUID, req.PodUID, req.PodName, req.PodNamespace,
		req.FileName, req.FilePath, req.FileType, req.FileSize,
		req.StorageType, req.StoragePath, req.StorageBucket,
		req.DownloadURL, req.Confidence, req.SourcePID,
		req.DetectedAt, req.CollectedAt, req.ExpiresAt,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to insert metadata: %w", err)
	}

	log.Infof("Saved profiler file metadata: id=%d, file=%s", id, req.FileName)

	// Return saved metadata
	return m.GetMetadataByID(ctx, id)
}

// SaveMetadataRequest represents a save metadata request
type SaveMetadataRequest struct {
	WorkloadUID   string
	PodUID        string
	PodName       string
	PodNamespace  string
	FileName      string
	FilePath      string
	FileType      string
	FileSize      int64
	StorageType   string
	StoragePath   string
	StorageBucket string
	DownloadURL   string
	Confidence    string
	SourcePID     int
	DetectedAt    time.Time
	CollectedAt   time.Time
	ExpiresAt     *time.Time
}

// GetMetadataByID gets metadata by ID
func (m *MetadataManager) GetMetadataByID(ctx context.Context, id int64) (*ProfilerFileMetadata, error) {
	query := `
		SELECT 
			id, workload_uid, pod_uid, pod_name, pod_namespace,
			file_name, file_path, file_type, file_size,
			storage_type, storage_path, storage_bucket,
			download_url, confidence, source_pid,
			detected_at, collected_at, expires_at,
			created_at, updated_at
		FROM profiler_files
		WHERE id = $1
	`

	metadata := &ProfilerFileMetadata{}
	var storageBucket sql.NullString
	var expiresAt sql.NullTime

	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&metadata.ID, &metadata.WorkloadUID, &metadata.PodUID, &metadata.PodName, &metadata.PodNamespace,
		&metadata.FileName, &metadata.FilePath, &metadata.FileType, &metadata.FileSize,
		&metadata.StorageType, &metadata.StoragePath, &storageBucket,
		&metadata.DownloadURL, &metadata.Confidence, &metadata.SourcePID,
		&metadata.DetectedAt, &metadata.CollectedAt, &expiresAt,
		&metadata.CreatedAt, &metadata.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("metadata not found: id=%d", id)
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	if storageBucket.Valid {
		metadata.StorageBucket = storageBucket.String
	}
	if expiresAt.Valid {
		metadata.ExpiresAt = &expiresAt.Time
	}

	return metadata, nil
}

// QueryMetadata queries metadata with filters
func (m *MetadataManager) QueryMetadata(ctx context.Context, req *QueryMetadataRequest) ([]*ProfilerFileMetadata, int, error) {
	// Build query
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if req.WorkloadUID != "" {
		whereClause += fmt.Sprintf(" AND workload_uid = $%d", argIdx)
		args = append(args, req.WorkloadUID)
		argIdx++
	}

	if req.PodUID != "" {
		whereClause += fmt.Sprintf(" AND pod_uid = $%d", argIdx)
		args = append(args, req.PodUID)
		argIdx++
	}

	if req.FileType != "" {
		whereClause += fmt.Sprintf(" AND file_type = $%d", argIdx)
		args = append(args, req.FileType)
		argIdx++
	}

	if req.StorageType != "" {
		whereClause += fmt.Sprintf(" AND storage_type = $%d", argIdx)
		args = append(args, req.StorageType)
		argIdx++
	}

	if !req.StartDate.IsZero() {
		whereClause += fmt.Sprintf(" AND collected_at >= $%d", argIdx)
		args = append(args, req.StartDate)
		argIdx++
	}

	if !req.EndDate.IsZero() {
		whereClause += fmt.Sprintf(" AND collected_at <= $%d", argIdx)
		args = append(args, req.EndDate)
		argIdx++
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM profiler_files %s", whereClause)
	var total int
	err := m.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count metadata: %w", err)
	}

	// Query with pagination
	query := fmt.Sprintf(`
		SELECT 
			id, workload_uid, pod_uid, pod_name, pod_namespace,
			file_name, file_path, file_type, file_size,
			storage_type, storage_path, storage_bucket,
			download_url, confidence, source_pid,
			detected_at, collected_at, expires_at,
			created_at, updated_at
		FROM profiler_files
		%s
		ORDER BY collected_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query metadata: %w", err)
	}
	defer rows.Close()

	results := make([]*ProfilerFileMetadata, 0)

	for rows.Next() {
		metadata := &ProfilerFileMetadata{}
		var storageBucket sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(
			&metadata.ID, &metadata.WorkloadUID, &metadata.PodUID, &metadata.PodName, &metadata.PodNamespace,
			&metadata.FileName, &metadata.FilePath, &metadata.FileType, &metadata.FileSize,
			&metadata.StorageType, &metadata.StoragePath, &storageBucket,
			&metadata.DownloadURL, &metadata.Confidence, &metadata.SourcePID,
			&metadata.DetectedAt, &metadata.CollectedAt, &expiresAt,
			&metadata.CreatedAt, &metadata.UpdatedAt,
		)

		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan row: %w", err)
		}

		if storageBucket.Valid {
			metadata.StorageBucket = storageBucket.String
		}
		if expiresAt.Valid {
			metadata.ExpiresAt = &expiresAt.Time
		}

		results = append(results, metadata)
	}

	log.Debugf("Queried profiler metadata: total=%d, returned=%d", total, len(results))

	return results, total, nil
}

// QueryMetadataRequest represents a query request
type QueryMetadataRequest struct {
	WorkloadUID string
	PodUID      string
	FileType    string
	StorageType string
	StartDate   time.Time
	EndDate     time.Time
	Limit       int
	Offset      int
}

// UpdateDownloadURL updates the download URL for a file
func (m *MetadataManager) UpdateDownloadURL(ctx context.Context, id int64, downloadURL string, expiresAt *time.Time) error {
	query := `
		UPDATE profiler_files
		SET download_url = $1, expires_at = $2, updated_at = NOW()
		WHERE id = $3
	`

	result, err := m.db.ExecContext(ctx, query, downloadURL, expiresAt, id)
	if err != nil {
		return fmt.Errorf("failed to update download URL: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("metadata not found: id=%d", id)
	}

	log.Infof("Updated download URL for file: id=%d", id)

	return nil
}

// DeleteMetadata deletes metadata by ID
func (m *MetadataManager) DeleteMetadata(ctx context.Context, id int64) error {
	query := `DELETE FROM profiler_files WHERE id = $1`

	result, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("metadata not found: id=%d", id)
	}

	log.Infof("Deleted metadata: id=%d", id)

	return nil
}

// DeleteMetadataByWorkload deletes all metadata for a workload
func (m *MetadataManager) DeleteMetadataByWorkload(ctx context.Context, workloadUID string) (int64, error) {
	query := `DELETE FROM profiler_files WHERE workload_uid = $1`

	result, err := m.db.ExecContext(ctx, query, workloadUID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete metadata: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()

	log.Infof("Deleted metadata for workload: workload=%s, count=%d", workloadUID, rowsAffected)

	return rowsAffected, nil
}

// CleanupExpiredFiles cleans up expired file metadata
func (m *MetadataManager) CleanupExpiredFiles(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM profiler_files
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`

	result, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired files: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()

	if rowsAffected > 0 {
		log.Infof("Cleaned up %d expired profiler files", rowsAffected)
	}

	return rowsAffected, nil
}

// GetStatistics gets statistics about profiler files
func (m *MetadataManager) GetStatistics(ctx context.Context, workloadUID string) (*Statistics, error) {
	query := `
		SELECT 
			COUNT(*) as total_files,
			COALESCE(SUM(file_size), 0) as total_size,
			COUNT(DISTINCT file_type) as file_types,
			COUNT(DISTINCT storage_type) as storage_types
		FROM profiler_files
		WHERE workload_uid = $1
	`

	stats := &Statistics{WorkloadUID: workloadUID}
	err := m.db.QueryRowContext(ctx, query, workloadUID).Scan(
		&stats.TotalFiles,
		&stats.TotalSize,
		&stats.FileTypes,
		&stats.StorageTypes,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	// Get file type breakdown
	typeQuery := `
		SELECT file_type, COUNT(*), COALESCE(SUM(file_size), 0)
		FROM profiler_files
		WHERE workload_uid = $1
		GROUP BY file_type
	`

	rows, err := m.db.QueryContext(ctx, typeQuery, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get type breakdown: %w", err)
	}
	defer rows.Close()

	stats.TypeBreakdown = make(map[string]*TypeStats)

	for rows.Next() {
		var fileType string
		var count int
		var size int64

		if err := rows.Scan(&fileType, &count, &size); err != nil {
			return nil, err
		}

		stats.TypeBreakdown[fileType] = &TypeStats{
			Count: count,
			Size:  size,
		}
	}

	return stats, nil
}

// Statistics represents profiler file statistics
type Statistics struct {
	WorkloadUID   string               `json:"workload_uid"`
	TotalFiles    int                  `json:"total_files"`
	TotalSize     int64                `json:"total_size"`
	FileTypes     int                  `json:"file_types"`
	StorageTypes  int                  `json:"storage_types"`
	TypeBreakdown map[string]*TypeStats `json:"type_breakdown"`
}

// TypeStats represents statistics for a specific file type
type TypeStats struct {
	Count int   `json:"count"`
	Size  int64 `json:"size"`
}

