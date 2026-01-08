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
	Workspace   string `json:"workspace" form:"workspace"` // Optional, if empty downloads to all workspaces
}

// DownloadJobInfo represents information about a download job
type DownloadJobInfo struct {
	JobId     string `json:"jobId"`
	Workspace string `json:"workspace"`
	DestPath  string `json:"destPath"`
}

// DatasetResponse represents the response body for a dataset
type DatasetResponse struct {
	DatasetId    string            `json:"datasetId"`
	DisplayName  string            `json:"displayName"`
	Description  string            `json:"description"`
	DatasetType  string            `json:"datasetType"`
	Status       string            `json:"status"`
	S3Path       string            `json:"s3Path"`
	TotalSize    int64             `json:"totalSize"`
	TotalSizeStr string            `json:"totalSizeStr"`
	FileCount    int               `json:"fileCount"`
	Message      string            `json:"message,omitempty"`
	UserId       string            `json:"userId"`
	UserName     string            `json:"userName"`
	CreationTime *time.Time        `json:"creationTime,omitempty"`
	UpdateTime   *time.Time        `json:"updateTime,omitempty"`
	DownloadJobs []DownloadJobInfo `json:"downloadJobs,omitempty"`
}

// ListDatasetsRequest represents the request parameters for listing datasets
type ListDatasetsRequest struct {
	DatasetType string `form:"datasetType"`
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

// ListFilesResponse represents the response body for listing files in a dataset
type ListFilesResponse struct {
	DatasetId string            `json:"datasetId"`
	Files     []DatasetFileInfo `json:"files"`
	Total     int               `json:"total"`
}

// DatasetTypeInfo represents information about a dataset type
type DatasetTypeInfo struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// ListDatasetTypesResponse represents the response body for listing dataset types
type ListDatasetTypesResponse struct {
	Types []DatasetTypeInfo `json:"types"`
}

// DatasetTemplateResponse represents the response body for a dataset template
type DatasetTemplateResponse struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Format      string            `json:"format"`
	Schema      map[string]string `json:"schema"`
	Example     string            `json:"example"`
}

// DownloadTarget represents a target for downloading dataset
type DownloadTarget struct {
	Workspace string
	Path      string
}
