/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"time"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// --- Dataset Request/Response Types ---

// CreateDatasetRequest represents the request body for creating a dataset
type CreateDatasetRequest struct {
	DisplayName string `json:"displayName" form:"displayName" binding:"required"`
	Description string `json:"description" form:"description"`
	DatasetType string `json:"datasetType" form:"datasetType" binding:"required"`
	Workspace   string `json:"workspace" form:"workspace"` // Workspace ID for access control, empty means public (downloads to all workspaces)
}

// DatasetDownloadJobInfo represents information about a download job
type DatasetDownloadJobInfo struct {
	JobId     string `json:"jobId"`
	Workspace string `json:"workspace"`
	DestPath  string `json:"destPath"`
}

// DatasetLocalPathInfo represents the download status of a dataset in a specific workspace
type DatasetLocalPathInfo struct {
	Workspace string                 `json:"workspace"`
	Path      string                 `json:"path,omitempty"`
	Status    dbclient.DatasetStatus `json:"status"`
	Message   string                 `json:"message,omitempty"`
}

// DatasetResponse represents the response body for a dataset
type DatasetResponse struct {
	DatasetId     string                   `json:"datasetId"`
	DisplayName   string                   `json:"displayName"`
	Description   string                   `json:"description"`
	DatasetType   string                   `json:"datasetType"`
	Source        string                   `json:"source"`                  // "upload" or "huggingface"
	SourceURL     string                   `json:"sourceUrl,omitempty"`     // HuggingFace URL (if source=huggingface)
	Status        dbclient.DatasetStatus   `json:"status"`                  // Pending/Downloading/Ready/Failed
	StatusMessage string                   `json:"statusMessage,omitempty"` // e.g., "2/3 workspaces completed"
	S3Path        string                   `json:"s3Path"`
	TotalSize     int64                    `json:"totalSize"`
	TotalSizeStr  string                   `json:"totalSizeStr"`
	FileCount     int                      `json:"fileCount"`
	Files         []DatasetFileInfo        `json:"files,omitempty"` // List of files in dataset
	Message       string                   `json:"message,omitempty"`
	LocalPaths    []DatasetLocalPathInfo   `json:"localPaths,omitempty"` // Per-workspace download status
	Workspace     string                   `json:"workspace,omitempty"`  // Workspace ID, empty means public
	UserId        string                   `json:"userId"`
	UserName      string                   `json:"userName"`
	CreationTime  *time.Time               `json:"creationTime,omitempty"`
	UpdateTime    *time.Time               `json:"updateTime,omitempty"`
	DownloadJobs  []DatasetDownloadJobInfo `json:"downloadJobs,omitempty"`
}

// ListDatasetsRequest represents the request parameters for listing datasets
type ListDatasetsRequest struct {
	DatasetType string `form:"datasetType"`
	Workspace   string `form:"workspace"` // Filter by workspace ID, empty returns all accessible datasets
	Search      string `form:"search"`
	PageNum     int    `form:"pageNum,default=1"`
	PageSize    int    `form:"pageSize,default=20"`
	OrderBy     string `form:"orderBy,default=creation_time"`
	Order       string `form:"order,default=desc"`
}

// ListDatasetsResponse represents the response body for listing datasets
type ListDatasetsResponse struct {
	Total    int               `json:"total"`
	PageNum  int               `json:"pageNum"`
	PageSize int               `json:"pageSize"`
	Items    []DatasetResponse `json:"items"`
}

// DatasetFileInfo represents information about a file in a dataset
type DatasetFileInfo struct {
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
	FileSize int64  `json:"fileSize"`
	SizeStr  string `json:"sizeStr"`
}

// DatasetTypeInfo represents information about a dataset type with schema
type DatasetTypeInfo struct {
	Name        string            `json:"name"`        // Type identifier (e.g., "sft", "dpo")
	Description string            `json:"description"` // Human readable description
	Schema      map[string]string `json:"schema"`      // Field schema for this type
}

// ListDatasetTypesResponse represents the response body for listing dataset types
type ListDatasetTypesResponse struct {
	Types []DatasetTypeInfo `json:"types"`
}

// GetDatasetFileRequest represents the request for getting a file from dataset
type GetDatasetFileRequest struct {
	Preview bool `form:"preview"` // If true, return file content; if false, return download URL
}

// GetDatasetFileResponse represents the response for downloading a file
type GetDatasetFileResponse struct {
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
}

// PreviewDatasetFileResponse represents the response for previewing file content
type PreviewDatasetFileResponse struct {
	FileName       string `json:"fileName"`
	Content        string `json:"content"`
	ContentType    string `json:"contentType"`
	Size           int64  `json:"size"`
	Truncated      bool   `json:"truncated"`
	MaxPreviewSize int64  `json:"maxPreviewSize,omitempty"`
}

// ============================================================================
// HuggingFace Import Types
// ============================================================================

// CreateDatasetFromHFRequest represents the request for importing a dataset from HuggingFace
type CreateDatasetFromHFRequest struct {
	URL         string `json:"url" binding:"required"`         // Required: HF dataset URL or repo ID
	DatasetType string `json:"datasetType" binding:"required"` // Required: sft/dpo/pretrain/rlhf/inference/evaluation/other
	Workspace   string `json:"workspace"`                      // Optional: empty = public
	Token       string `json:"token,omitempty"`                // Optional: for private datasets
}

// DatasetSource represents the source type of a dataset
type DatasetSource string

const (
	DatasetSourceUpload      DatasetSource = "upload"      // File upload
	DatasetSourceHuggingFace DatasetSource = "huggingface" // HuggingFace import
)

// HF Dataset Job labels for identifying HF download Jobs
const (
	HFDatasetJobLabel = "hf-dataset-job"
	HFDatasetIdLabel  = "hf-dataset-id"
)
