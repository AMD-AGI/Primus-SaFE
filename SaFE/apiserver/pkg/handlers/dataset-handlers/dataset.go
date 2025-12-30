/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// CreateDataset handles the creation of a new dataset with file upload.
// POST /api/v1/datasets
func (h *Handler) CreateDataset(c *gin.Context) {
	handle(c, h.createDataset)
}

// ListDatasets handles listing datasets with filtering and pagination.
// GET /api/v1/datasets
func (h *Handler) ListDatasets(c *gin.Context) {
	handle(c, h.listDatasets)
}

// GetDataset handles getting a single dataset by ID.
// GET /api/v1/datasets/:id
func (h *Handler) GetDataset(c *gin.Context) {
	handle(c, h.getDataset)
}

// DeleteDataset handles deleting a dataset by ID.
// DELETE /api/v1/datasets/:id
func (h *Handler) DeleteDataset(c *gin.Context) {
	handle(c, h.deleteDataset)
}

// ListDatasetFiles handles listing files in a dataset.
// GET /api/v1/datasets/:id/files
func (h *Handler) ListDatasetFiles(c *gin.Context) {
	handle(c, h.listDatasetFiles)
}

// createDataset creates a new dataset with file upload.
func (h *Handler) createDataset(c *gin.Context) (interface{}, error) {
	// Parse form data
	var req CreateDatasetRequest
	if err := c.ShouldBind(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request: %v", err))
	}

	// Validate dataset type
	if !IsValidDatasetType(req.DatasetType) {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid dataset type: %s", req.DatasetType))
	}

	// Get user info from context
	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)

	// Generate dataset ID
	datasetId := fmt.Sprintf("dataset-%s", uuid.New().String()[:8])
	s3Path := fmt.Sprintf("%s/%s/", DatasetS3Prefix, datasetId)

	// Handle file upload
	form, err := c.MultipartForm()
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("failed to parse multipart form: %v", err))
	}

	files := form.File["files"]
	if len(files) == 0 {
		return nil, commonerrors.NewBadRequest("no files uploaded")
	}

	var totalSize int64
	fileCount := len(files)

	// Upload files to S3
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to open file: %v", err))
		}
		defer file.Close()

		// Read file content
		content, err := io.ReadAll(file)
		if err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read file: %v", err))
		}

		// Upload to S3
		key := s3Path + fileHeader.Filename
		_, err = h.s3Client.PutObject(context.Background(), key, string(content), 300)
		if err != nil {
			klog.ErrorS(err, "failed to upload file to S3", "key", key)
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to upload file: %v", err))
		}

		totalSize += fileHeader.Size
	}

	// Create dataset record in database
	now := pq.NullTime{Time: time.Now().UTC(), Valid: true}
	dataset := &dbclient.Dataset{
		DatasetId:    datasetId,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		DatasetType:  req.DatasetType,
		Status:       DatasetStatusReady,
		S3Path:       s3Path,
		TotalSize:    totalSize,
		FileCount:    fileCount,
		UserId:       userId,
		UserName:     userName,
		CreationTime: now,
		UpdateTime:   now,
		IsDeleted:    false,
	}

	if err := h.dbClient.UpsertDataset(context.Background(), dataset); err != nil {
		klog.ErrorS(err, "failed to create dataset", "datasetId", datasetId)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create dataset: %v", err))
	}

	return convertToDatasetResponse(dataset), nil
}

// listDatasets lists datasets with filtering and pagination.
func (h *Handler) listDatasets(c *gin.Context) (interface{}, error) {
	var req ListDatasetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request: %v", err))
	}

	// Build query conditions
	dbTags := dbclient.GetDatasetFieldTags()
	conditions := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
	}

	// Filter by dataset type
	if req.DatasetType != "" {
		conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(dbTags, "DatasetType"): req.DatasetType})
	}

	// Filter by search keyword (display name)
	if req.Search != "" {
		conditions = append(conditions, sqrl.Like{dbclient.GetFieldTag(dbTags, "DisplayName"): "%" + req.Search + "%"})
	}

	// Count total
	total, err := h.dbClient.CountDatasets(context.Background(), conditions)
	if err != nil {
		klog.ErrorS(err, "failed to count datasets")
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to count datasets: %v", err))
	}

	// Build order by
	orderBy := []string{fmt.Sprintf("%s %s", req.OrderBy, req.Order)}

	// Calculate offset
	offset := (req.PageNum - 1) * req.PageSize

	// Query datasets
	datasets, err := h.dbClient.SelectDatasets(context.Background(), conditions, orderBy, req.PageSize, offset)
	if err != nil {
		klog.ErrorS(err, "failed to list datasets")
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to list datasets: %v", err))
	}

	// Convert to response
	items := make([]DatasetResponse, 0, len(datasets))
	for _, ds := range datasets {
		items = append(items, convertToDatasetResponse(ds))
	}

	return &ListDatasetsResponse{
		Total:    total,
		PageNum:  req.PageNum,
		PageSize: req.PageSize,
		Items:    items,
	}, nil
}

// getDataset gets a single dataset by ID.
func (h *Handler) getDataset(c *gin.Context) (interface{}, error) {
	datasetId := c.Param("id")
	if datasetId == "" {
		return nil, commonerrors.NewBadRequest("dataset id is required")
	}

	dataset, err := h.dbClient.GetDataset(context.Background(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to get dataset", "datasetId", datasetId)
		return nil, err
	}

	return convertToDatasetResponse(dataset), nil
}

// deleteDataset deletes a dataset by ID.
func (h *Handler) deleteDataset(c *gin.Context) (interface{}, error) {
	datasetId := c.Param("id")
	if datasetId == "" {
		return nil, commonerrors.NewBadRequest("dataset id is required")
	}

	// Get dataset first to get S3 path
	dataset, err := h.dbClient.GetDataset(context.Background(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to get dataset", "datasetId", datasetId)
		return nil, err
	}

	// Delete files from S3 (best effort, don't fail if S3 delete fails)
	if dataset.S3Path != "" {
		if err := h.s3Client.DeleteObject(context.Background(), dataset.S3Path, 60); err != nil {
			klog.ErrorS(err, "failed to delete dataset files from S3", "datasetId", datasetId, "s3Path", dataset.S3Path)
			// Continue with database deletion even if S3 deletion fails
		}
	}

	// Mark dataset as deleted in database
	if err := h.dbClient.SetDatasetDeleted(context.Background(), datasetId); err != nil {
		klog.ErrorS(err, "failed to delete dataset", "datasetId", datasetId)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to delete dataset: %v", err))
	}

	return gin.H{"message": "dataset deleted successfully", "datasetId": datasetId}, nil
}

// listDatasetFiles lists files in a dataset.
func (h *Handler) listDatasetFiles(c *gin.Context) (interface{}, error) {
	datasetId := c.Param("id")
	if datasetId == "" {
		return nil, commonerrors.NewBadRequest("dataset id is required")
	}

	// Get dataset to get S3 path
	dataset, err := h.dbClient.GetDataset(context.Background(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to get dataset", "datasetId", datasetId)
		return nil, err
	}

	// List files from S3 using presign model files API
	filesMap, err := h.s3Client.PresignModelFiles(context.Background(), dataset.S3Path, 1)
	if err != nil {
		klog.ErrorS(err, "failed to list files from S3", "datasetId", datasetId, "s3Path", dataset.S3Path)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to list files: %v", err))
	}

	// Convert to file info list
	files := make([]DatasetFileInfo, 0, len(filesMap))
	for filePath := range filesMap {
		// Extract file name from path
		fileName := filepath.Base(filePath)
		// Remove S3 path prefix to get relative path
		relativePath := strings.TrimPrefix(filePath, dataset.S3Path)
		files = append(files, DatasetFileInfo{
			FileName: fileName,
			FilePath: relativePath,
			FileSize: 0, // Size not available from presign API
			SizeStr:  "N/A",
		})
	}

	return &ListFilesResponse{
		DatasetId: datasetId,
		Files:     files,
		Total:     len(files),
	}, nil
}

// convertToDatasetResponse converts a database dataset to response format.
func convertToDatasetResponse(ds *dbclient.Dataset) DatasetResponse {
	resp := DatasetResponse{
		DatasetId:    ds.DatasetId,
		DisplayName:  ds.DisplayName,
		Description:  ds.Description,
		DatasetType:  ds.DatasetType,
		Status:       ds.Status,
		S3Path:       ds.S3Path,
		TotalSize:    ds.TotalSize,
		TotalSizeStr: formatFileSize(ds.TotalSize),
		FileCount:    ds.FileCount,
		Message:      ds.Message,
		UserId:       ds.UserId,
		UserName:     ds.UserName,
	}

	if ds.CreationTime.Valid {
		resp.CreationTime = &ds.CreationTime.Time
	}
	if ds.UpdateTime.Valid {
		resp.UpdateTime = &ds.UpdateTime.Time
	}

	return resp
}

// formatFileSize formats file size in human readable format.
func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/TB)
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
