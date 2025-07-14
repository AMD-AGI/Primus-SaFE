/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"

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

type Interface interface {
	CreateMultiPartUpload(ctx context.Context, key string, timeout int64) (string, error)
	MultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error
	CompleteMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error

	PutObject(ctx context.Context, key, value string, timeout int64) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, key string, timeout int64) error

	GeneratePresignedURL(ctx context.Context, key string, expireDay int32) (string, error)
}
