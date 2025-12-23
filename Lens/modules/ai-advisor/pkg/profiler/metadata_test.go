package profiler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *MetadataManager) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	manager, err := NewMetadataManager(db)
	require.NoError(t, err)

	return db, mock, manager
}

func TestNewMetadataManager(t *testing.T) {
	db, _, _ := setupMockDB(t)
	defer db.Close()

	manager, err := NewMetadataManager(db)

	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.db)
}

func TestNewMetadataManager_NilDB(t *testing.T) {
	manager, err := NewMetadataManager(nil)

	assert.Error(t, err)
	assert.Nil(t, manager)
	assert.Contains(t, err.Error(), "database connection is nil")
}

func TestMetadataManager_SaveMetadata_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	req := &SaveMetadataRequest{
		WorkloadUID:   "workload-001",
		PodUID:        "pod-123",
		PodName:       "training-pod-0",
		PodNamespace:  "default",
		FileName:      "profiler.json",
		FilePath:      "/workspace/logs/profiler.json",
		FileType:      "chrome_trace",
		FileSize:      1024000,
		StorageType:   "object_storage",
		StoragePath:   "profiler/workload-001/2024-12-15/chrome_trace/profiler.json",
		StorageBucket: "profiler-data",
		DownloadURL:   "https://minio.example.com/...",
		Confidence:    "high",
		SourcePID:     12345,
		DetectedAt:    time.Now(),
		CollectedAt:   time.Now(),
		ExpiresAt:     nil,
	}

	now := time.Now()
	
	// Mock INSERT
	mock.ExpectQuery(`INSERT INTO profiler_files`).
		WithArgs(
			req.WorkloadUID, req.PodUID, req.PodName, req.PodNamespace,
			req.FileName, req.FilePath, req.FileType, req.FileSize,
			req.StorageType, req.StoragePath, req.StorageBucket,
			req.DownloadURL, req.Confidence, req.SourcePID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), req.ExpiresAt,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(1, now, now))

	// Mock SELECT (GetMetadataByID)
	mock.ExpectQuery(`SELECT (.+) FROM profiler_files WHERE id`).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workload_uid", "pod_uid", "pod_name", "pod_namespace",
			"file_name", "file_path", "file_type", "file_size",
			"storage_type", "storage_path", "storage_bucket",
			"download_url", "confidence", "source_pid",
			"detected_at", "collected_at", "expires_at",
			"created_at", "updated_at",
		}).AddRow(
			1, req.WorkloadUID, req.PodUID, req.PodName, req.PodNamespace,
			req.FileName, req.FilePath, req.FileType, req.FileSize,
			req.StorageType, req.StoragePath, req.StorageBucket,
			req.DownloadURL, req.Confidence, req.SourcePID,
			req.DetectedAt, req.CollectedAt, nil,
			now, now,
		))

	metadata, err := manager.SaveMetadata(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, metadata)
	assert.Equal(t, int64(1), metadata.ID)
	assert.Equal(t, req.WorkloadUID, metadata.WorkloadUID)
	assert.Equal(t, req.FileName, metadata.FileName)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_SaveMetadata_Error(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	req := &SaveMetadataRequest{
		WorkloadUID: "workload-001",
		FileName:    "profiler.json",
	}

	mock.ExpectQuery(`INSERT INTO profiler_files`).
		WillReturnError(sql.ErrConnDone)

	metadata, err := manager.SaveMetadata(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.Contains(t, err.Error(), "failed to insert metadata")
}

func TestMetadataManager_GetMetadataByID_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	mock.ExpectQuery(`SELECT (.+) FROM profiler_files WHERE id`).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workload_uid", "pod_uid", "pod_name", "pod_namespace",
			"file_name", "file_path", "file_type", "file_size",
			"storage_type", "storage_path", "storage_bucket",
			"download_url", "confidence", "source_pid",
			"detected_at", "collected_at", "expires_at",
			"created_at", "updated_at",
		}).AddRow(
			1, "workload-001", "pod-123", "training-pod-0", "default",
			"profiler.json", "/workspace/logs/profiler.json", "chrome_trace", 1024000,
			"object_storage", "profiler/...", "profiler-data",
			"https://minio.example.com/...", "high", 12345,
			now, now, expiresAt,
			now, now,
		))

	metadata, err := manager.GetMetadataByID(context.Background(), 1)

	require.NoError(t, err)
	assert.NotNil(t, metadata)
	assert.Equal(t, int64(1), metadata.ID)
	assert.Equal(t, "workload-001", metadata.WorkloadUID)
	assert.Equal(t, "profiler.json", metadata.FileName)
	assert.NotNil(t, metadata.ExpiresAt)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_GetMetadataByID_NotFound(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT (.+) FROM profiler_files WHERE id`).
		WithArgs(999).
		WillReturnError(sql.ErrNoRows)

	metadata, err := manager.GetMetadataByID(context.Background(), 999)

	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.Contains(t, err.Error(), "metadata not found")
}

func TestMetadataManager_QueryMetadata_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	req := &QueryMetadataRequest{
		WorkloadUID: "workload-001",
		Limit:       10,
		Offset:      0,
	}

	now := time.Now()

	// Mock COUNT query
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM profiler_files`).
		WithArgs("workload-001").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// Mock SELECT query
	mock.ExpectQuery(`SELECT (.+) FROM profiler_files`).
		WithArgs("workload-001", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workload_uid", "pod_uid", "pod_name", "pod_namespace",
			"file_name", "file_path", "file_type", "file_size",
			"storage_type", "storage_path", "storage_bucket",
			"download_url", "confidence", "source_pid",
			"detected_at", "collected_at", "expires_at",
			"created_at", "updated_at",
		}).
			AddRow(1, "workload-001", "pod-1", "pod-1", "default",
				"file1.json", "/path1", "chrome_trace", 1000,
				"object_storage", "path1", "bucket1",
				"url1", "high", 100,
				now, now, nil, now, now).
			AddRow(2, "workload-001", "pod-2", "pod-2", "default",
				"file2.json", "/path2", "pytorch_trace", 2000,
				"database", "path2", nil,
				"url2", "medium", 200,
				now, now, nil, now, now))

	results, total, err := manager.QueryMetadata(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	assert.Equal(t, "file1.json", results[0].FileName)
	assert.Equal(t, "file2.json", results[1].FileName)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_QueryMetadata_WithFilters(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	startDate := time.Now().Add(-7 * 24 * time.Hour)
	endDate := time.Now()

	req := &QueryMetadataRequest{
		WorkloadUID: "workload-001",
		PodUID:      "pod-123",
		FileType:    "chrome_trace",
		StorageType: "object_storage",
		StartDate:   startDate,
		EndDate:     endDate,
		Limit:       50,
		Offset:      0,
	}

	// Mock COUNT with all filters
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM profiler_files`).
		WithArgs("workload-001", "pod-123", "chrome_trace", "object_storage", startDate, endDate).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock SELECT with all filters
	now := time.Now()
	mock.ExpectQuery(`SELECT (.+) FROM profiler_files`).
		WithArgs("workload-001", "pod-123", "chrome_trace", "object_storage", startDate, endDate, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workload_uid", "pod_uid", "pod_name", "pod_namespace",
			"file_name", "file_path", "file_type", "file_size",
			"storage_type", "storage_path", "storage_bucket",
			"download_url", "confidence", "source_pid",
			"detected_at", "collected_at", "expires_at",
			"created_at", "updated_at",
		}).AddRow(1, "workload-001", "pod-123", "pod", "default",
			"profiler.json", "/path", "chrome_trace", 1000,
			"object_storage", "path", "bucket",
			"url", "high", 100,
			now, now, nil, now, now))

	results, total, err := manager.QueryMetadata(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_UpdateDownloadURL_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	mock.ExpectExec(`UPDATE profiler_files`).
		WithArgs("https://new-url.com/file", expiresAt, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := manager.UpdateDownloadURL(context.Background(), 1, "https://new-url.com/file", &expiresAt)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_UpdateDownloadURL_NotFound(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectExec(`UPDATE profiler_files`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 999).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := manager.UpdateDownloadURL(context.Background(), 999, "url", nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata not found")
}

func TestMetadataManager_DeleteMetadata_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM profiler_files WHERE id`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := manager.DeleteMetadata(context.Background(), 1)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_DeleteMetadata_NotFound(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM profiler_files WHERE id`).
		WithArgs(999).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := manager.DeleteMetadata(context.Background(), 999)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata not found")
}

func TestMetadataManager_DeleteMetadataByWorkload_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM profiler_files WHERE workload_uid`).
		WithArgs("workload-001").
		WillReturnResult(sqlmock.NewResult(0, 5))

	count, err := manager.DeleteMetadataByWorkload(context.Background(), "workload-001")

	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_CleanupExpiredFiles_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM profiler_files WHERE expires_at IS NOT NULL AND expires_at`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	count, err := manager.CleanupExpiredFiles(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_CleanupExpiredFiles_NoExpired(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM profiler_files WHERE expires_at IS NOT NULL AND expires_at`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	count, err := manager.CleanupExpiredFiles(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestMetadataManager_GetStatistics_Success(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	// Mock main statistics query
	mock.ExpectQuery(`SELECT COUNT\(\*\) as total_files`).
		WithArgs("workload-001").
		WillReturnRows(sqlmock.NewRows([]string{
			"total_files", "total_size", "file_types", "storage_types",
		}).AddRow(10, 50000000, 3, 2))

	// Mock type breakdown query
	mock.ExpectQuery(`SELECT file_type, COUNT\(\*\), COALESCE`).
		WithArgs("workload-001").
		WillReturnRows(sqlmock.NewRows([]string{
			"file_type", "count", "size",
		}).
			AddRow("chrome_trace", 5, 30000000).
			AddRow("pytorch_trace", 3, 15000000).
			AddRow("stack_trace", 2, 5000000))

	stats, err := manager.GetStatistics(context.Background(), "workload-001")

	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, "workload-001", stats.WorkloadUID)
	assert.Equal(t, 10, stats.TotalFiles)
	assert.Equal(t, int64(50000000), stats.TotalSize)
	assert.Equal(t, 3, stats.FileTypes)
	assert.Equal(t, 2, stats.StorageTypes)
	assert.Len(t, stats.TypeBreakdown, 3)
	assert.Equal(t, 5, stats.TypeBreakdown["chrome_trace"].Count)
	assert.Equal(t, int64(30000000), stats.TypeBreakdown["chrome_trace"].Size)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataManager_GetStatistics_Error(t *testing.T) {
	db, mock, manager := setupMockDB(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) as total_files`).
		WithArgs("workload-999").
		WillReturnError(sql.ErrConnDone)

	stats, err := manager.GetStatistics(context.Background(), "workload-999")

	assert.Error(t, err)
	assert.Nil(t, stats)
}

func TestProfilerFileMetadata_Fields(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	metadata := &ProfilerFileMetadata{
		ID:            1,
		WorkloadUID:   "workload-001",
		PodUID:        "pod-123",
		PodName:       "training-pod-0",
		PodNamespace:  "default",
		FileName:      "profiler.json",
		FilePath:      "/workspace/logs/profiler.json",
		FileType:      "chrome_trace",
		FileSize:      1024000,
		StorageType:   "object_storage",
		StoragePath:   "profiler/...",
		StorageBucket: "profiler-data",
		DownloadURL:   "https://minio.example.com/...",
		Confidence:    "high",
		SourcePID:     12345,
		DetectedAt:    now,
		CollectedAt:   now,
		ExpiresAt:     &expiresAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	assert.Equal(t, int64(1), metadata.ID)
	assert.Equal(t, "workload-001", metadata.WorkloadUID)
	assert.NotNil(t, metadata.ExpiresAt)
}

func TestSaveMetadataRequest_Validation(t *testing.T) {
	req := &SaveMetadataRequest{
		WorkloadUID: "workload-001",
		FileName:    "profiler.json",
		FileType:    "chrome_trace",
	}

	assert.NotEmpty(t, req.WorkloadUID)
	assert.NotEmpty(t, req.FileName)
	assert.NotEmpty(t, req.FileType)
}

func TestQueryMetadataRequest_EmptyFilters(t *testing.T) {
	req := &QueryMetadataRequest{
		Limit:  50,
		Offset: 0,
	}

	assert.Equal(t, 50, req.Limit)
	assert.Equal(t, 0, req.Offset)
	assert.Empty(t, req.WorkloadUID)
	assert.Empty(t, req.FileType)
}

func TestStatistics_TypeBreakdown(t *testing.T) {
	stats := &Statistics{
		WorkloadUID:  "workload-001",
		TotalFiles:   10,
		TotalSize:    50000000,
		FileTypes:    3,
		StorageTypes: 2,
		TypeBreakdown: map[string]*TypeStats{
			"chrome_trace": {
				Count: 5,
				Size:  30000000,
			},
			"pytorch_trace": {
				Count: 3,
				Size:  15000000,
			},
			"stack_trace": {
				Count: 2,
				Size:  5000000,
			},
		},
	}

	assert.Equal(t, 3, len(stats.TypeBreakdown))
	assert.Equal(t, 5, stats.TypeBreakdown["chrome_trace"].Count)
	
	// Calculate percentage
	chromeTracePercentage := float64(stats.TypeBreakdown["chrome_trace"].Size) / float64(stats.TotalSize) * 100
	assert.InDelta(t, 60.0, chromeTracePercentage, 0.1)
}

func TestTypeStats_Fields(t *testing.T) {
	stats := &TypeStats{
		Count: 5,
		Size:  30000000,
	}

	assert.Equal(t, 5, stats.Count)
	assert.Equal(t, int64(30000000), stats.Size)
}

