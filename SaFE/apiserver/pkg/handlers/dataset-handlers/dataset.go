/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
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

// ListDatasetTypes handles listing all available dataset types.
// GET /api/v1/datasets/types
func (h *Handler) ListDatasetTypes(c *gin.Context) {
	handle(c, h.listDatasetTypes)
}

// GetDatasetTemplate handles getting a template for a specific dataset type.
// GET /api/v1/datasets/templates/:type
func (h *Handler) GetDatasetTemplate(c *gin.Context) {
	handle(c, h.getDatasetTemplate)
}

// createDataset creates a new dataset with file upload.
func (h *Handler) createDataset(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()

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

	// Create download jobs if download image is configured
	var downloadJobs []DownloadJobInfo
	if commonconfig.GetDownloadJoImage() != "" && commonconfig.IsS3Enable() {
		downloadTargets, err := h.getDownloadTargets(ctx, req.Workspace)
		if err != nil {
			klog.ErrorS(err, "failed to get download targets", "datasetId", datasetId)
			// Don't fail the request, just log the error
		} else if len(downloadTargets) > 0 {
			downloadJobs, err = h.createDownloadOpsJobs(ctx, dataset, downloadTargets, userId, userName)
			if err != nil {
				klog.ErrorS(err, "failed to create download jobs", "datasetId", datasetId)
				// Don't fail the request, just log the error
			}
		}
	}

	resp := convertToDatasetResponse(dataset)
	resp.DownloadJobs = downloadJobs
	return resp, nil
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

// listDatasetTypes lists all available dataset types.
func (h *Handler) listDatasetTypes(c *gin.Context) (interface{}, error) {
	return &ListDatasetTypesResponse{
		Types: DatasetTypeDescriptions,
	}, nil
}

// getDatasetTemplate gets a template for a specific dataset type.
func (h *Handler) getDatasetTemplate(c *gin.Context) (interface{}, error) {
	datasetType := c.Param("type")
	if datasetType == "" {
		return nil, commonerrors.NewBadRequest("dataset type is required")
	}

	template, exists := DatasetTemplates[datasetType]
	if !exists {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("template for dataset type '%s' not found", datasetType))
	}

	return template, nil
}

// getDownloadTargets gets the download targets based on workspace parameter.
// If workspace is specified, only that workspace's path is returned.
// If workspace is empty, all workspaces' paths are returned (deduplicated).
func (h *Handler) getDownloadTargets(ctx context.Context, workspace string) ([]DownloadTarget, error) {
	if workspace != "" {
		// Get specific workspace
		ws := &v1.Workspace{}
		if err := h.Get(ctx, client.ObjectKey{Name: workspace}, ws); err != nil {
			return nil, fmt.Errorf("failed to get workspace %s: %w", workspace, err)
		}
		path := getNfsPathFromWorkspace(ws)
		if path == "" {
			return nil, fmt.Errorf("workspace %s has no volume configured", workspace)
		}
		return []DownloadTarget{{
			Workspace: ws.Name,
			Path:      path,
		}}, nil
	}

	// Get all workspaces and deduplicate by path
	workspaceList := &v1.WorkspaceList{}
	if err := h.List(ctx, workspaceList); err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	return getUniqueDownloadPaths(workspaceList.Items), nil
}

// getUniqueDownloadPaths extracts unique download paths from workspaces.
// It prioritizes PFS type volumes, otherwise falls back to the first available volume.
func getUniqueDownloadPaths(workspaces []v1.Workspace) []DownloadTarget {
	pathMap := make(map[string]DownloadTarget) // key: actual storage path

	for _, ws := range workspaces {
		path := getNfsPathFromWorkspace(&ws)
		if path == "" {
			continue
		}

		// Deduplicate: same path only creates one OpsJob
		if _, exists := pathMap[path]; !exists {
			pathMap[path] = DownloadTarget{
				Workspace: ws.Name,
				Path:      path,
			}
		}
	}

	targets := make([]DownloadTarget, 0, len(pathMap))
	for _, target := range pathMap {
		targets = append(targets, target)
	}
	return targets
}

// getNfsPathFromWorkspace retrieves the NFS path from the workspace's volumes.
// It prioritizes PFS type volumes, otherwise falls back to the first available volume's mount path.
func getNfsPathFromWorkspace(workspace *v1.Workspace) string {
	result := ""
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.PFS {
			result = vol.MountPath
			break
		}
	}
	if result == "" && len(workspace.Spec.Volumes) > 0 {
		result = workspace.Spec.Volumes[0].MountPath
	}
	return result
}

// createDownloadOpsJobs creates OpsJobs to download the dataset to the specified targets.
func (h *Handler) createDownloadOpsJobs(ctx context.Context, dataset *dbclient.Dataset, targets []DownloadTarget, userId, userName string) ([]DownloadJobInfo, error) {
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("S3 storage is not enabled")
	}

	s3Endpoint := commonconfig.GetS3Endpoint()
	s3Bucket := commonconfig.GetS3Bucket()
	if s3Endpoint == "" || s3Bucket == "" {
		return nil, fmt.Errorf("S3 configuration is incomplete")
	}

	// Construct S3 URL: {endpoint}/{bucket}/{s3Path}
	s3URL := fmt.Sprintf("%s/%s/%s", s3Endpoint, s3Bucket, dataset.S3Path)

	downloadJobs := make([]DownloadJobInfo, 0, len(targets))

	for _, target := range targets {
		// Get workspace to get cluster ID
		ws := &v1.Workspace{}
		if err := h.Get(ctx, client.ObjectKey{Name: target.Workspace}, ws); err != nil {
			klog.ErrorS(err, "failed to get workspace for download job", "workspace", target.Workspace)
			continue
		}

		// Generate unique job name
		jobName := commonutils.GenerateName(fmt.Sprintf("dataset-dl-%s", dataset.DatasetId))

		// Destination path: datasets/{displayName}
		destPath := fmt.Sprintf("datasets/%s", dataset.DisplayName)

		// Create OpsJob
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: jobName,
				Labels: map[string]string{
					v1.UserIdLabel:      userId,
					v1.DisplayNameLabel: jobName,
					v1.WorkspaceIdLabel: target.Workspace,
					v1.ClusterIdLabel:   ws.Spec.Cluster,
				},
				Annotations: map[string]string{
					v1.UserNameAnnotation: userName,
				},
			},
			Spec: v1.OpsJobSpec{
				Type:  v1.OpsJobDownloadType,
				Image: pointer.String(commonconfig.GetDownloadJoImage()),
				Inputs: []v1.Parameter{
					{Name: v1.ParameterEndpoint, Value: s3URL},
					{Name: v1.ParameterDestPath, Value: destPath},
					{Name: v1.ParameterSecret, Value: DatasetS3Secret},
					{Name: v1.ParameterWorkspace, Value: target.Workspace},
				},
				TTLSecondsAfterFinished: 300, // Auto cleanup after 5 minutes
				TimeoutSecond:           3600, // 1 hour timeout
			},
		}

		if err := h.Create(ctx, job); err != nil {
			klog.ErrorS(err, "failed to create download OpsJob", "jobName", jobName, "workspace", target.Workspace)
			continue
		}

		klog.InfoS("created download OpsJob for dataset", "jobName", jobName, "datasetId", dataset.DatasetId, "workspace", target.Workspace)

		downloadJobs = append(downloadJobs, DownloadJobInfo{
			JobId:     jobName,
			Workspace: target.Workspace,
			DestPath:  target.Path + "/" + destPath,
		})
	}

	return downloadJobs, nil
}
