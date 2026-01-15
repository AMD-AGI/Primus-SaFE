/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

import "time"

// CreateDatasetRequest represents the request body for creating a dataset
type CreateDatasetRequest struct {
	DisplayName string `json:"displayName" form:"displayName" binding:"required"`
	Description string `json:"description" form:"description"`
	DatasetType string `json:"datasetType" form:"datasetType" binding:"required"`
	Workspace   string `json:"workspace" form:"workspace"` // Workspace ID for access control, empty means public (downloads to all workspaces)
}

// DownloadJobInfo represents information about a download job
type DownloadJobInfo struct {
	JobId     string `json:"jobId"`
	Workspace string `json:"workspace"`
	DestPath  string `json:"destPath"`
}

// LocalPathInfo represents the download status of a dataset in a specific workspace
type LocalPathInfo struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// DatasetResponse represents the response body for a dataset
type DatasetResponse struct {
	DatasetId     string            `json:"datasetId"`
	DisplayName   string            `json:"displayName"`
	Description   string            `json:"description"`
	DatasetType   string            `json:"datasetType"`
	Status        string            `json:"status"`                  // Pending/Downloading/Ready/Failed
	StatusMessage string            `json:"statusMessage,omitempty"` // e.g., "2/3 workspaces completed"
	S3Path        string            `json:"s3Path"`
	TotalSize     int64             `json:"totalSize"`
	TotalSizeStr  string            `json:"totalSizeStr"`
	FileCount     int               `json:"fileCount"`
	Files         []DatasetFileInfo `json:"files,omitempty"` // List of files in dataset
	Message       string            `json:"message,omitempty"`
	LocalPaths    []LocalPathInfo   `json:"localPaths,omitempty"` // Per-workspace download status
	Workspace     string            `json:"workspace,omitempty"`  // Workspace ID, empty means public
	UserId        string            `json:"userId"`
	UserName      string            `json:"userName"`
	CreationTime  *time.Time        `json:"creationTime,omitempty"`
	UpdateTime    *time.Time        `json:"updateTime,omitempty"`
	DownloadJobs  []DownloadJobInfo `json:"downloadJobs,omitempty"`
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

// DownloadTarget represents a target for downloading dataset
type DownloadTarget struct {
	Workspace string
	Path      string
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

// PreviewFileResponse represents the response for previewing file content
type PreviewFileResponse struct {
	FileName       string `json:"fileName"`
	Content        string `json:"content"`
	ContentType    string `json:"contentType"`
	Size           int64  `json:"size"`
	Truncated      bool   `json:"truncated"`
	MaxPreviewSize int64  `json:"maxPreviewSize,omitempty"`
}
