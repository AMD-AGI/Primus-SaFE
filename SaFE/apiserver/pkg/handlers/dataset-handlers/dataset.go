/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

import (
	"context"
	"encoding/json"
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

// ListDatasetTypes handles listing all available dataset types.
// GET /api/v1/datasets/types
func (h *Handler) ListDatasetTypes(c *gin.Context) {
	handle(c, h.listDatasetTypes)
}

// GetDatasetFile handles getting or previewing a specific file from a dataset.
// GET /api/v1/datasets/:id/files/*path
func (h *Handler) GetDatasetFile(c *gin.Context) {
	handle(c, h.getDatasetFile)
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

	// Get download targets first to initialize LocalPaths
	var downloadTargets []DownloadTarget
	if commonconfig.GetDownloadJoImage() != "" && commonconfig.IsS3Enable() {
		var err error
		downloadTargets, err = h.getDownloadTargets(ctx, req.Workspace)
		if err != nil {
			klog.ErrorS(err, "failed to get download targets", "datasetId", datasetId)
			// Continue without download targets
		}
	}

	// Initialize LocalPaths with Pending status for each target
	var localPathsJSON string
	if len(downloadTargets) > 0 {
		localPaths := make([]dbclient.DatasetLocalPathDB, 0, len(downloadTargets))
		for _, target := range downloadTargets {
			localPaths = append(localPaths, dbclient.DatasetLocalPathDB{
				Workspace: target.Workspace,
				Path:      target.Path + "/datasets/" + req.DisplayName,
				Status:    dbclient.DatasetStatusPending,
			})
		}
		if jsonBytes, err := json.Marshal(localPaths); err == nil {
			localPathsJSON = string(jsonBytes)
		}
	}

	// Create dataset record in database
	now := pq.NullTime{Time: time.Now().UTC(), Valid: true}
	dataset := &dbclient.Dataset{
		DatasetId:    datasetId,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		DatasetType:  req.DatasetType,
		Status:       dbclient.DatasetStatusPending, // Initial status is Pending, will be updated by Controller
		S3Path:       s3Path,
		TotalSize:    totalSize,
		FileCount:    fileCount,
		LocalPaths:   localPathsJSON,
		Workspace:    req.Workspace, // Workspace ID for access control, empty means public
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
	if len(downloadTargets) > 0 {
		var err error
		downloadJobs, err = h.createDownloadOpsJobs(ctx, dataset, downloadTargets, userId, userName)
		if err != nil {
			klog.ErrorS(err, "failed to create download jobs", "datasetId", datasetId)
			// Don't fail the request, just log the error
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

	// Filter by workspace
	// If workspace is specified, return datasets belonging to that workspace OR public datasets (empty workspace)
	if req.Workspace != "" {
		conditions = append(conditions, sqrl.Or{
			sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): req.Workspace},
			sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): ""},
		})
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

	resp := convertToDatasetResponse(dataset)

	// Get file list from S3
	if h.s3Client != nil {
		files, err := h.getDatasetFileList(context.Background(), dataset.S3Path)
		if err != nil {
			klog.ErrorS(err, "failed to list files from S3", "datasetId", datasetId)
			// Don't fail the request, just return without files
		} else {
			resp.Files = files
		}
	}

	return resp, nil
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

// getDatasetFileList returns the list of files in a dataset from S3.
func (h *Handler) getDatasetFileList(ctx context.Context, s3Path string) ([]DatasetFileInfo, error) {
	// List files from S3 using presign model files API
	filesMap, err := h.s3Client.PresignModelFiles(ctx, s3Path, 1)
	if err != nil {
		return nil, err
	}

	// Convert to file info list
	files := make([]DatasetFileInfo, 0, len(filesMap))
	for filePath := range filesMap {
		// Extract file name from path
		fileName := filepath.Base(filePath)
		// Remove S3 path prefix to get relative path
		relativePath := strings.TrimPrefix(filePath, s3Path)
		files = append(files, DatasetFileInfo{
			FileName: fileName,
			FilePath: relativePath,
			FileSize: 0, // Size not available from presign API
			SizeStr:  "N/A",
		})
	}

	return files, nil
}

// MaxPreviewSize is the maximum file size for preview (100KB)
const MaxPreviewSize = 100 * 1024

// getDatasetFile gets or previews a specific file from a dataset.
func (h *Handler) getDatasetFile(c *gin.Context) (interface{}, error) {
	datasetId := c.Param("id")
	if datasetId == "" {
		return nil, commonerrors.NewBadRequest("dataset id is required")
	}

	// Get file path from URL - the *path wildcard captures everything after /files/
	filePath := c.Param("path")
	if filePath == "" {
		return nil, commonerrors.NewBadRequest("file path is required")
	}
	// Remove leading slash if present
	filePath = strings.TrimPrefix(filePath, "/")

	// Parse query parameters
	var req GetDatasetFileRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request: %v", err))
	}

	// Get dataset to get S3 path
	dataset, err := h.dbClient.GetDataset(context.Background(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to get dataset", "datasetId", datasetId)
		return nil, err
	}

	// Construct full S3 key
	s3Key := dataset.S3Path + filePath

	if req.Preview {
		// Preview mode: read file content and return
		return h.previewFile(c.Request.Context(), s3Key, filePath)
	}

	// Download mode: return presigned URL
	return h.getFileDownloadURL(c.Request.Context(), s3Key, filePath)
}

// getFileDownloadURL returns a presigned URL for downloading a file
func (h *Handler) getFileDownloadURL(ctx context.Context, s3Key, filePath string) (*GetDatasetFileResponse, error) {
	// Get presigned URL for the file
	filesMap, err := h.s3Client.PresignModelFiles(ctx, s3Key, 1) // 1 hour expiry
	if err != nil {
		klog.ErrorS(err, "failed to get presigned URL", "s3Key", s3Key)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get download URL: %v", err))
	}

	// Find the URL for our file
	var downloadURL string
	for _, url := range filesMap {
		downloadURL = url
		break
	}

	if downloadURL == "" {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("file not found: %s", filePath))
	}

	return &GetDatasetFileResponse{
		FileName:    filepath.Base(filePath),
		DownloadURL: downloadURL,
	}, nil
}

// previewFile reads file content from S3 and returns it
func (h *Handler) previewFile(ctx context.Context, s3Key, filePath string) (*PreviewFileResponse, error) {
	// Read file content from S3
	content, err := h.s3Client.GetObject(ctx, s3Key, 60) // 60 second timeout
	if err != nil {
		klog.ErrorS(err, "failed to read file from S3", "s3Key", s3Key)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read file: %v", err))
	}

	// Determine content type based on file extension
	contentType := getContentType(filePath)

	// Check if we need to truncate
	truncated := false
	if int64(len(content)) > MaxPreviewSize {
		content = content[:MaxPreviewSize]
		truncated = true
	}

	return &PreviewFileResponse{
		FileName:       filepath.Base(filePath),
		Content:        content,
		ContentType:    contentType,
		Size:           int64(len(content)),
		Truncated:      truncated,
		MaxPreviewSize: MaxPreviewSize,
	}, nil
}

// getContentType returns the content type based on file extension
func getContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		return "application/json"
	case ".jsonl":
		return "application/jsonl"
	case ".txt":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".md":
		return "text/markdown"
	case ".yaml", ".yml":
		return "application/yaml"
	default:
		return "text/plain"
	}
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
		Workspace:    ds.Workspace,
		UserId:       ds.UserId,
		UserName:     ds.UserName,
	}

	if ds.CreationTime.Valid {
		resp.CreationTime = &ds.CreationTime.Time
	}
	if ds.UpdateTime.Valid {
		resp.UpdateTime = &ds.UpdateTime.Time
	}

	// Parse LocalPaths and generate StatusMessage
	if ds.LocalPaths != "" {
		var dbLocalPaths []dbclient.DatasetLocalPathDB
		if err := json.Unmarshal([]byte(ds.LocalPaths), &dbLocalPaths); err == nil {
			localPaths := make([]LocalPathInfo, 0, len(dbLocalPaths))
			readyCount := 0
			for _, lp := range dbLocalPaths {
				localPaths = append(localPaths, LocalPathInfo{
					Workspace: lp.Workspace,
					Path:      lp.Path,
					Status:    lp.Status,
					Message:   lp.Message,
				})
				if lp.Status == dbclient.DatasetStatusReady {
					readyCount++
				}
			}
			resp.LocalPaths = localPaths
			if len(localPaths) > 0 {
				resp.StatusMessage = fmt.Sprintf("%d/%d workspaces completed", readyCount, len(localPaths))
			}
		}
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
					v1.UserIdLabel:          userId,
					v1.DisplayNameLabel:     jobName,
					v1.WorkspaceIdLabel:     target.Workspace,
					v1.ClusterIdLabel:       ws.Spec.Cluster,
					dbclient.DatasetIdLabel: dataset.DatasetId,
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
				TTLSecondsAfterFinished: 300,  // Auto cleanup after 5 minutes
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
