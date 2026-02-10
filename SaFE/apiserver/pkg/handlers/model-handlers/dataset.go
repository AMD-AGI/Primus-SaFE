/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// CreateDataset handles the creation of a new dataset with file upload.
// POST /api/v1/datasets
func (h *Handler) CreateDataset(c *gin.Context) {
	handleDataset(c, h.createDataset)
}

// ListDatasets handles listing datasets with filtering and pagination.
// GET /api/v1/datasets
func (h *Handler) ListDatasets(c *gin.Context) {
	handleDataset(c, h.listDatasets)
}

// GetDataset handles getting a single dataset by ID.
// GET /api/v1/datasets/:id
func (h *Handler) GetDataset(c *gin.Context) {
	handleDataset(c, h.getDataset)
}

// DeleteDataset handles deleting a dataset by ID.
// DELETE /api/v1/datasets/:id
func (h *Handler) DeleteDataset(c *gin.Context) {
	handleDataset(c, h.deleteDataset)
}

// ListDatasetTypes handles listing all available dataset types.
// GET /api/v1/datasets/types
func (h *Handler) ListDatasetTypes(c *gin.Context) {
	handleDataset(c, h.listDatasetTypes)
}

// GetDatasetFile handles getting or previewing a specific file from a dataset.
// GET /api/v1/datasets/:id/files/*path
func (h *Handler) GetDatasetFile(c *gin.Context) {
	handleDataset(c, h.getDatasetFile)
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

	// Check if dataset name already exists
	exists, err := h.dbClient.CheckDatasetNameExists(ctx, req.DisplayName)
	if err != nil {
		klog.ErrorS(err, "failed to check dataset name", "displayName", req.DisplayName)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to check dataset name: %v", err))
	}
	if exists {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("dataset name '%s' already exists", req.DisplayName))
	}

	// Get user info from context
	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)

	// Generate dataset ID
	datasetId := fmt.Sprintf("dataset-%s", uuid.New().String()[:8])
	s3Path := fmt.Sprintf("%s/%s/", DatasetS3Prefix, datasetId)

	// Handle file upload - parse form first
	form, err := c.MultipartForm()
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("failed to parse multipart form: %v", err))
	}

	files := form.File["files"]
	if len(files) == 0 {
		return nil, commonerrors.NewBadRequest("no files uploaded")
	}

	fileCount := len(files)
	var totalSize int64
	for _, fileHeader := range files {
		totalSize += fileHeader.Size
	}

	// Step 1: Create DB record first with Uploading status
	now := pq.NullTime{Time: time.Now().UTC(), Valid: true}
	dataset := &dbclient.Dataset{
		DatasetId:    datasetId,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		DatasetType:  req.DatasetType,
		Status:       dbclient.DatasetStatusUploading, // Initial status is Uploading
		S3Path:       s3Path,
		TotalSize:    totalSize,
		FileCount:    fileCount,
		LocalPaths:   "[]", // Empty JSON array initially
		Workspace:    req.Workspace,
		UserId:       userId,
		UserName:     userName,
		CreationTime: now,
		UpdateTime:   now,
		IsDeleted:    false,
	}

	if err := h.dbClient.UpsertDataset(ctx, dataset); err != nil {
		klog.ErrorS(err, "failed to create dataset record", "datasetId", datasetId)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create dataset: %v", err))
	}

	klog.InfoS("created dataset record with Uploading status", "datasetId", datasetId)

	// Step 2: Upload files to S3
	uploadErr := h.uploadFilesToS3(ctx, files, s3Path)
	if uploadErr != nil {
		// Update status to Failed if upload fails
		if updateErr := h.dbClient.UpdateDatasetStatus(ctx, datasetId, dbclient.DatasetStatusFailed, uploadErr.Error()); updateErr != nil {
			klog.ErrorS(updateErr, "failed to update dataset status to Failed", "datasetId", datasetId)
		}
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to upload files: %v", uploadErr))
	}

	// Step 3: Get download targets and initialize LocalPaths
	var downloadTargets []commonworkspace.DownloadTarget
	if commonconfig.GetDownloadJoImage() != "" && commonconfig.IsS3Enable() {
		downloadTargets, err = h.getDatasetDownloadTargets(ctx, req.Workspace)
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

	// Step 4: Update status to Pending (upload completed, waiting for download)
	dataset.Status = dbclient.DatasetStatusPending
	dataset.LocalPaths = localPathsJSON
	dataset.UpdateTime = pq.NullTime{Time: time.Now().UTC(), Valid: true}

	if err := h.dbClient.UpsertDataset(ctx, dataset); err != nil {
		klog.ErrorS(err, "failed to update dataset status to Pending", "datasetId", datasetId)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to update dataset: %v", err))
	}

	klog.InfoS("updated dataset status to Pending", "datasetId", datasetId)

	// Step 5: Create download jobs if download image is configured
	var downloadJobs []DatasetDownloadJobInfo
	if len(downloadTargets) > 0 {
		downloadJobs, err = h.createDatasetDownloadOpsJobs(ctx, dataset, downloadTargets, userId, userName)
		if err != nil {
			klog.ErrorS(err, "failed to create download jobs", "datasetId", datasetId)
			// Don't fail the request, just log the error
		}
	}

	resp := convertToDatasetResponse(dataset)
	resp.DownloadJobs = downloadJobs
	return resp, nil
}

// uploadFilesToS3 uploads files to S3 and returns error if any upload fails.
func (h *Handler) uploadFilesToS3(ctx context.Context, files []*multipart.FileHeader, s3Path string) error {
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", fileHeader.Filename, err)
		}
		defer file.Close()

		// Upload to S3 using multipart upload (handles both small and large files)
		key := s3Path + fileHeader.Filename
		if err := h.s3Client.PutObjectMultipart(ctx, key, file, fileHeader.Size); err != nil {
			klog.ErrorS(err, "failed to upload file to S3", "key", key, "size", fileHeader.Size)
			return fmt.Errorf("failed to upload file %s: %v", fileHeader.Filename, err)
		}

		klog.InfoS("uploaded file to S3", "key", key, "size", fileHeader.Size)
	}
	return nil
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

	// Protect system datasets (benchmark datasets owned by primus-safe-system)
	if dataset.UserId == common.UserSystem {
		return nil, commonerrors.NewBadRequest("system datasets cannot be deleted")
	}

	// Delete files from S3 (best effort, don't fail if S3 delete fails)
	if dataset.S3Path != "" && h.s3Client != nil {
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
	// List files from S3 with size information
	s3Files, err := h.s3Client.ListObjectsWithSize(ctx, s3Path)
	if err != nil {
		return nil, err
	}

	// Convert to file info list
	files := make([]DatasetFileInfo, 0, len(s3Files))
	for _, f := range s3Files {
		files = append(files, DatasetFileInfo{
			FileName: filepath.Base(f.Key),
			FilePath: f.Key,
			FileSize: f.Size,
			SizeStr:  commonutils.FormatFileSize(f.Size),
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
		return h.previewDatasetFile(c.Request.Context(), s3Key, filePath)
	}

	// Download mode: return presigned URL
	return h.getDatasetFileDownloadURL(c.Request.Context(), s3Key, filePath)
}

// getDatasetFileDownloadURL returns a presigned URL for downloading a file
func (h *Handler) getDatasetFileDownloadURL(ctx context.Context, s3Key, filePath string) (*GetDatasetFileResponse, error) {
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

// previewDatasetFile reads file content from S3 and returns it
func (h *Handler) previewDatasetFile(ctx context.Context, s3Key, filePath string) (*PreviewDatasetFileResponse, error) {
	// Read file content from S3
	content, err := h.s3Client.GetObject(ctx, s3Key, 60) // 60 second timeout
	if err != nil {
		klog.ErrorS(err, "failed to read file from S3", "s3Key", s3Key)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read file: %v", err))
	}

	// Determine content type based on file extension
	contentType := getDatasetContentType(filePath)

	// Check if we need to truncate
	truncated := false
	if int64(len(content)) > MaxPreviewSize {
		content = content[:MaxPreviewSize]
		truncated = true
	}

	return &PreviewDatasetFileResponse{
		FileName:       filepath.Base(filePath),
		Content:        content,
		ContentType:    contentType,
		Size:           int64(len(content)),
		Truncated:      truncated,
		MaxPreviewSize: MaxPreviewSize,
	}, nil
}

// getDatasetContentType returns the content type based on file extension using mime standard library.
// Falls back to custom mappings for types not in the standard library.
func getDatasetContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	// Try standard library first
	contentType := mime.TypeByExtension(ext)
	if contentType != "" {
		return contentType
	}

	// Custom mappings for types not in standard library
	switch ext {
	case ".jsonl":
		return "application/jsonl"
	case ".yaml", ".yml":
		return "application/yaml"
	default:
		return "text/plain"
	}
}

// convertToDatasetResponse converts a database dataset to response format.
func convertToDatasetResponse(ds *dbclient.Dataset) DatasetResponse {
	source := ds.Source
	if source == "" {
		source = string(DatasetSourceUpload) // Default for old records
	}
	resp := DatasetResponse{
		DatasetId:    ds.DatasetId,
		DisplayName:  ds.DisplayName,
		Description:  ds.Description,
		DatasetType:  ds.DatasetType,
		Source:       source,
		SourceURL:    ds.SourceURL,
		Status:       ds.Status,
		S3Path:       ds.S3Path,
		TotalSize:    ds.TotalSize,
		TotalSizeStr: commonutils.FormatFileSize(ds.TotalSize),
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
			localPaths := make([]DatasetLocalPathInfo, 0, len(dbLocalPaths))
			readyCount := 0
			for _, lp := range dbLocalPaths {
				localPaths = append(localPaths, DatasetLocalPathInfo{
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

// listDatasetTypes lists all available dataset types.
func (h *Handler) listDatasetTypes(c *gin.Context) (interface{}, error) {
	return &ListDatasetTypesResponse{
		Types: GetDatasetTypeDescriptions(),
	}, nil
}

// getDatasetDownloadTargets gets the download targets based on workspace parameter.
// If workspace is specified, only that workspace's path is returned.
// If workspace is empty, all workspaces' paths are returned (deduplicated).
func (h *Handler) getDatasetDownloadTargets(ctx context.Context, workspace string) ([]commonworkspace.DownloadTarget, error) {
	if workspace != "" {
		// Get specific workspace
		ws := &v1.Workspace{}
		if err := h.k8sClient.Get(ctx, client.ObjectKey{Name: workspace}, ws); err != nil {
			return nil, fmt.Errorf("failed to get workspace %s: %w", workspace, err)
		}
		path := commonworkspace.GetNfsPathFromWorkspace(ws)
		if path == "" {
			return nil, fmt.Errorf("workspace %s has no volume configured", workspace)
		}
		return []commonworkspace.DownloadTarget{{
			Workspace: ws.Name,
			Path:      path,
		}}, nil
	}

	// Get all workspaces and deduplicate by path
	workspaceList := &v1.WorkspaceList{}
	if err := h.k8sClient.List(ctx, workspaceList); err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	return commonworkspace.GetUniqueDownloadPaths(workspaceList.Items), nil
}

// createDatasetDownloadOpsJobs creates OpsJobs to download the dataset to the specified targets.
func (h *Handler) createDatasetDownloadOpsJobs(ctx context.Context, dataset *dbclient.Dataset, targets []commonworkspace.DownloadTarget, userId, userName string) ([]DatasetDownloadJobInfo, error) {
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

	downloadJobs := make([]DatasetDownloadJobInfo, 0, len(targets))

	for _, target := range targets {
		// Get workspace to get cluster ID
		ws := &v1.Workspace{}
		if err := h.k8sClient.Get(ctx, client.ObjectKey{Name: target.Workspace}, ws); err != nil {
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

		if err := h.k8sClient.Create(ctx, job); err != nil {
			klog.ErrorS(err, "failed to create download OpsJob", "jobName", jobName, "workspace", target.Workspace)
			continue
		}

		klog.InfoS("created download OpsJob for dataset", "jobName", jobName, "datasetId", dataset.DatasetId, "workspace", target.Workspace)

		downloadJobs = append(downloadJobs, DatasetDownloadJobInfo{
			JobId:     jobName,
			Workspace: target.Workspace,
			DestPath:  target.Path + "/" + destPath,
		})
	}

	return downloadJobs, nil
}

// ImportDatasetFromHF handles importing a dataset from HuggingFace.
// POST /api/v1/datasets/import-hf
func (h *Handler) ImportDatasetFromHF(c *gin.Context) {
	handleDataset(c, h.importDatasetFromHF)
}

// importDatasetFromHF implements the HuggingFace dataset import logic.
func (h *Handler) importDatasetFromHF(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()

	// 1. Parse request
	var req CreateDatasetFromHFRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request: %v", err))
	}

	// 2. Validate dataset type
	if !IsValidDatasetType(req.DatasetType) {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid dataset type: %s", req.DatasetType))
	}

	// 3. Normalize URL
	normalizedURL := normalizeHFDatasetURL(req.URL)

	// 4. Check if dataset from this URL already exists
	exists, err := h.checkDatasetURLExists(ctx, normalizedURL)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, commonerrors.NewBadRequest("dataset from this URL already exists")
	}

	// 5. Fetch HuggingFace metadata (displayName, description)
	hfInfo, err := GetHFDatasetInfo(req.URL)
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("failed to fetch HF dataset info: %v", err))
	}

	// 6. Get user info
	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)

	// 7. Generate dataset ID and S3 path
	datasetId := fmt.Sprintf("dataset-%s", uuid.New().String()[:8])
	s3Path := fmt.Sprintf("%s/%s/", DatasetS3Prefix, datasetId)

	// 8. Create database record (status=Pending)
	now := pq.NullTime{Time: time.Now().UTC(), Valid: true}
	dataset := &dbclient.Dataset{
		DatasetId:    datasetId,
		DisplayName:  hfInfo.DisplayName,
		Description:  hfInfo.Description,
		DatasetType:  req.DatasetType,
		Status:       dbclient.DatasetStatusPending,
		S3Path:       s3Path,
		LocalPaths:   "[]",
		Source:       string(DatasetSourceHuggingFace),
		SourceURL:    normalizedURL,
		Workspace:    req.Workspace,
		UserId:       userId,
		UserName:     userName,
		CreationTime: now,
		UpdateTime:   now,
		IsDeleted:    false,
	}

	if err := h.dbClient.UpsertDataset(ctx, dataset); err != nil {
		klog.ErrorS(err, "failed to create dataset record", "datasetId", datasetId)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create dataset: %v", err))
	}

	klog.InfoS("Created HF dataset record", "datasetId", datasetId, "displayName", hfInfo.DisplayName, "url", normalizedURL)

	// 9. Create HF → S3 download Job
	job, err := h.createHFDatasetDownloadJob(ctx, dataset, req.Token)
	if err != nil {
		// Update status to Failed
		if updateErr := h.dbClient.UpdateDatasetStatus(ctx, datasetId, dbclient.DatasetStatusFailed, err.Error()); updateErr != nil {
			klog.ErrorS(updateErr, "failed to update dataset status to Failed", "datasetId", datasetId)
		}
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create download job: %v", err))
	}

	// 10. Update database with Job name and status
	dataset.HFJobName = job.Name
	dataset.Status = dbclient.DatasetStatusUploading
	dataset.Message = fmt.Sprintf("HF download job created: %s", job.Name)
	dataset.UpdateTime = pq.NullTime{Time: time.Now().UTC(), Valid: true}

	if err := h.dbClient.UpsertDataset(ctx, dataset); err != nil {
		klog.ErrorS(err, "failed to update dataset with job info", "datasetId", datasetId)
	}

	return convertToDatasetResponse(dataset), nil
}

// createHFDatasetDownloadJob creates a K8s Job to download dataset from HuggingFace to S3.
func (h *Handler) createHFDatasetDownloadJob(ctx context.Context, dataset *dbclient.Dataset, hfToken string) (*batchv1.Job, error) {
	// Get S3 configuration
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("S3 storage is not enabled")
	}
	s3Endpoint := commonconfig.GetS3Endpoint()
	s3AccessKey := commonconfig.GetS3AccessKey()
	s3SecretKey := commonconfig.GetS3SecretKey()
	s3Bucket := commonconfig.GetS3Bucket()
	if s3Endpoint == "" || s3AccessKey == "" || s3SecretKey == "" || s3Bucket == "" {
		return nil, fmt.Errorf("S3 configuration is incomplete")
	}

	repoID := cleanDatasetRepoID(dataset.SourceURL)
	s3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, dataset.S3Path)

	var envs []corev1.EnvVar

	// HF Token (optional, for private datasets)
	if hfToken != "" {
		envs = append(envs, corev1.EnvVar{Name: "HF_TOKEN", Value: hfToken})
	}

	// S3 credentials
	envs = append(envs,
		corev1.EnvVar{Name: "AWS_ACCESS_KEY_ID", Value: s3AccessKey},
		corev1.EnvVar{Name: "AWS_SECRET_ACCESS_KEY", Value: s3SecretKey},
		corev1.EnvVar{Name: "AWS_DEFAULT_REGION", Value: "us-east-1"},
		corev1.EnvVar{Name: "S3_ENDPOINT", Value: s3Endpoint},
	)

	cmd := []string{"/bin/sh", "-c", fmt.Sprintf(`
		set -e
		echo "Downloading dataset from HuggingFace: %s"
		mkdir -p /tmp/dataset
		huggingface-cli download %s --repo-type dataset --local-dir /tmp/dataset || exit 1
		echo "Uploading dataset to S3: %s"
		aws s3 cp /tmp/dataset %s --recursive --endpoint-url %s --exclude ".cache/*" || exit 1
		echo "Dataset download completed successfully"
	`, repoID, repoID, s3Path, s3Path, s3Endpoint)}

	image := commonconfig.GetModelDownloaderImage()
	jobName := commonutils.GenerateName(fmt.Sprintf("hf-dataset-%s", dataset.DatasetId))
	backoffLimit := int32(3)
	ttlSeconds := int32(300)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				HFDatasetJobLabel: "true",
				HFDatasetIdLabel:  dataset.DatasetId,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						HFDatasetJobLabel: "true",
						HFDatasetIdLabel:  dataset.DatasetId,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{{
						Name:            "downloader",
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         cmd,
						Env:             envs,
					}},
				},
			},
		},
	}

	if err := h.k8sClient.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create K8s Job: %w", err)
	}

	klog.InfoS("Created HF dataset download job",
		"datasetId", dataset.DatasetId,
		"jobName", jobName,
		"repoID", repoID,
		"s3Path", s3Path)

	return job, nil
}

// normalizeHFDatasetURL normalizes a HuggingFace dataset URL.
func normalizeHFDatasetURL(urlOrID string) string {
	urlOrID = strings.TrimSpace(urlOrID)
	urlOrID = strings.TrimSuffix(urlOrID, "/")

	// If it's already a full URL, return as-is
	if strings.HasPrefix(urlOrID, "https://huggingface.co/") {
		return urlOrID
	}

	// If it's a repo ID, construct the full URL
	repoID := cleanDatasetRepoID(urlOrID)
	return fmt.Sprintf("https://huggingface.co/datasets/%s", repoID)
}

// checkDatasetURLExists checks if a dataset with the given source URL already exists.
func (h *Handler) checkDatasetURLExists(ctx context.Context, sourceURL string) (bool, error) {
	dbTags := dbclient.GetDatasetFieldTags()
	conditions := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "SourceURL"): sourceURL},
	}
	count, err := h.dbClient.CountDatasets(ctx, conditions)
	if err != nil {
		return false, commonerrors.NewInternalError(fmt.Sprintf("failed to check dataset URL: %v", err))
	}
	return count > 0, nil
}
