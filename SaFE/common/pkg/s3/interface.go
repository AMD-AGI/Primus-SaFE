/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type MultiUploadParam struct {
	Key            string
	Value          string
	UploadId       string
	PartNumber     int32
	CompletedParts []types.CompletedPart
}

// S3FileInfo represents file information from S3
type S3FileInfo struct {
	Key  string // Relative path (without prefix)
	Size int64  // File size in bytes
}

type Interface interface {
	CreateMultiPartUpload(ctx context.Context, key string, timeout int64) (string, error)
	MultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error
	CompleteMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error

	PutObject(ctx context.Context, key, value string, timeout int64) (*s3.PutObjectOutput, error)
	PutObjectMultipart(ctx context.Context, key string, reader io.Reader, size int64) error // Stream multipart upload
	GetObject(ctx context.Context, key string, timeout int64) (string, error)
	DeleteObject(ctx context.Context, key string, timeout int64) error

	GeneratePresignedURL(ctx context.Context, key string, expireHour int32) (string, error)
	GeneratePresignedPutURL(ctx context.Context, key string, expireHour int32) (string, error)
	PresignModelFiles(ctx context.Context, prefix string, expireHour int32) (map[string]string, error)
	ListObjectsWithSize(ctx context.Context, prefix string) ([]S3FileInfo, error)

	DownloadFile(ctx context.Context, key, localPath string) error
	DownloadDirectory(ctx context.Context, prefix, localDir string) error
}
