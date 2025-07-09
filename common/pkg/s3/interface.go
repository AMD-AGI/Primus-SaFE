/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"

	"github.com/aws/aws-sdk-go/service/s3"
)

type MultiUploadParam struct {
	S3Client       *s3.S3
	Key            string
	Value          string
	UploadId       string
	PartNumber     int64
	CompletedParts []*s3.CompletedPart
}

type Interface interface {
	CreateBucket(ctx context.Context, timeout int64) error
	ListBucket(ctx context.Context, timeout int64) (*s3.ListBucketsOutput, error)
	DeleteBucket(ctx context.Context, timeout int64) error
	IsBucketExisted(ctx context.Context, timeout int64) (bool, error)

	CreateMultiPartUpload(ctx context.Context, key string, timeout int64) (*s3.S3, string, error)
	MultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) (*s3.CompletedPart, error)
	CompleteMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error
	AbortMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error

	PutObject(ctx context.Context, key, value string, timeout int64) error
	ListObject(ctx context.Context, timeout int64) (*s3.ListObjectsOutput, error)
	GetObject(ctx context.Context, key string, timeout int64) error
	DeleteObject(ctx context.Context, key string, timeout int64) error
}
