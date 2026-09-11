/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/cicdlog"
)

const (
	DocId = "docId"
)

type ListLogInput = cicdlog.ListLogInput

type ListLogRequest = cicdlog.Query

// For internal use, the request for searching log context
type ListContextLogRequest struct {
	Query *ListLogRequest
	// For internal use, location to store the result.
	Id int
	// The maximum number of returned results
	Limit int
	// Input the specified document ID to retrieve the log context of that document.
	DocId string
}

// DownloadWorkloadLogRequest is the request for getting workload log download URL.
type DownloadWorkloadLogRequest struct {
	// Timeout in seconds for waiting job completion, default 900 (15 minutes)
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
}

// DownloadWorkloadLogResponse is the response containing the S3 presigned URL for downloading logs.
type DownloadWorkloadLogResponse struct {
	// The S3 presigned URL to download the log file
	DownloadURL string `json:"downloadUrl"`
}
