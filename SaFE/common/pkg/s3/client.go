/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"k8s.io/utils/pointer"
)

const (
	DefaultTimeout = 180
)

// Option Expiration time can only be configured for subdirectories.
type Option struct {
	Subdir    string
	ExpireDay int32
}

// Client - S3 client structure that encapsulates S3 configuration, options and AWS S3 client
// Used to perform various S3 bucket operations including upload, download, delete, etc.
type Client struct {
	*Config
	opt      Option
	s3Client *s3.Client
}

// NewClient creates and returns a new Client instance.
func NewClient(ctx context.Context, opt Option) (Interface, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, err
	}
	cli, err := newFromConfig(config, opt)
	if err != nil {
		return nil, err
	}
	if err = cli.checkBucketExisted(ctx); err != nil {
		return nil, err
	}
	if err = cli.setLifecycleRule(ctx); err != nil {
		return nil, err
	}
	return cli, nil
}

// newFromConfig create S3 client based on configuration.
func newFromConfig(config *Config, opt Option) (*Client, error) {
	if opt.Subdir == "" {
		return nil, fmt.Errorf("the Subdir of option is empty")
	}
	s3Client := s3.NewFromConfig(config.GetS3Config(), func(o *s3.Options) {
		o.UsePathStyle = true
	})
	cli := &Client{
		Config:   config,
		opt:      opt,
		s3Client: s3Client,
	}
	if !strings.HasSuffix(cli.opt.Subdir, "/") {
		cli.opt.Subdir += "/"
	}
	return cli, nil
}

// checkBucketExisted checks BucketExisted and returns the result.
func (c *Client) checkBucketExisted(ctx context.Context) error {
	input := &s3.HeadBucketInput{
		Bucket: c.Bucket,
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, DefaultTimeout)
	defer cancel()

	if _, err := c.s3Client.HeadBucket(timeoutCtx, input); err != nil {
		return err
	}
	return nil
}

// setLifecycleRule set bucket lifecycle rules.
func (c *Client) setLifecycleRule(ctx context.Context) error {
	if c.opt.ExpireDay <= 0 {
		return nil
	}
	input := &s3.PutBucketLifecycleConfigurationInput{
		Bucket: c.Bucket,
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{
			Rules: []types.LifecycleRule{
				{
					ID:     aws.String(fmt.Sprintf("expire-after-%d-day", c.opt.ExpireDay)),
					Prefix: pointer.String(c.opt.Subdir),
					Status: types.ExpirationStatusEnabled,
					Expiration: &types.LifecycleExpiration{
						Days: pointer.Int32(c.opt.ExpireDay),
					},
				},
			},
		},
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, DefaultTimeout)
	defer cancel()
	_, err := c.s3Client.PutBucketLifecycleConfiguration(timeoutCtx, input)
	return err
}

// CreateMultiPartUpload create multipart upload task.
func (c *Client) CreateMultiPartUpload(ctx context.Context, key string, timeout int64) (string, error) {
	if c == nil {
		return "", fmt.Errorf("please init client first")
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.s3Client.CreateMultipartUpload(timeoutCtx, &s3.CreateMultipartUploadInput{
		Bucket: c.Bucket,
		Key:    aws.String(c.WithPrefixKey(key)),
	})
	if err != nil {
		return "", err
	}
	return *resp.UploadId, nil
}

// MultiPartUpload perform multipart upload.
func (c *Client) MultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	input := &s3.UploadPartInput{
		Bucket:     c.Bucket,
		Key:        aws.String(c.WithPrefixKey(param.Key)),
		UploadId:   aws.String(param.UploadId),
		PartNumber: pointer.Int32(param.PartNumber),
		Body:       bytes.NewReader([]byte(param.Value)),
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	output, err := c.s3Client.UploadPart(timeoutCtx, input)
	if err != nil {
		return err
	}
	param.CompletedParts = append(param.CompletedParts, types.CompletedPart{
		ETag:       output.ETag,
		PartNumber: pointer.Int32(param.PartNumber),
	})
	return nil
}

// CompleteMultiPartUpload complete multipart upload.
func (c *Client) CompleteMultiPartUpload(ctx context.Context,
	param *MultiUploadParam, timeout int64) (*s3.CompleteMultipartUploadOutput, error) {
	if c == nil {
		return nil, fmt.Errorf("please init client first")
	}
	if len(param.CompletedParts) == 0 {
		return nil, nil
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:          c.Bucket,
		Key:             aws.String(c.WithPrefixKey(param.Key)),
		UploadId:        aws.String(param.UploadId),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: param.CompletedParts},
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	return c.s3Client.CompleteMultipartUpload(timeoutCtx, input)
}

// AbortMultiPartUpload cancel multipart upload task.
func (c *Client) AbortMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	if len(param.CompletedParts) == 0 {
		return nil
	}
	input := &s3.AbortMultipartUploadInput{
		Bucket:   c.Bucket,
		Key:      aws.String(c.WithPrefixKey(param.Key)),
		UploadId: aws.String(param.UploadId),
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	_, err := c.s3Client.AbortMultipartUpload(timeoutCtx, input)
	return err
}

// PutObject upload object to S3 bucket.
func (c *Client) PutObject(ctx context.Context, key, value string, timeout int64) (*s3.PutObjectOutput, error) {
	if c == nil {
		return nil, fmt.Errorf("please init client first")
	}
	if key == "" || value == "" {
		return nil, fmt.Errorf("the object key or value is empty")
	}
	input := &s3.PutObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(c.WithPrefixKey(key)),
		Body:   bytes.NewReader([]byte(value)),
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	return c.s3Client.PutObject(timeoutCtx, input)
}

// DeleteObject delete object from S3 bucket.
func (c *Client) DeleteObject(ctx context.Context, key string, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	if key == "" {
		return fmt.Errorf("the object key is empty")
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	_, err := c.s3Client.DeleteObject(timeoutCtx, &s3.DeleteObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(c.WithPrefixKey(key)),
	})
	if err != nil {
		return err
	}
	return nil
}

// GeneratePresignedURL generate presigned URL for temporary object access.
func (c *Client) GeneratePresignedURL(ctx context.Context, key string, expireDay int32) (string, error) {
	presigner := s3.NewPresignClient(c.s3Client)

	resp, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(c.WithPrefixKey(key)),
	}, func(o *s3.PresignOptions) {
		o.Expires = time.Duration(expireDay) * 24 * time.Hour
	})
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

// WithPrefixKey add subdirectory prefix to object key name.
func (c *Client) WithPrefixKey(key string) string {
	return c.opt.Subdir + key
}

// WithOptionalTimeout add optional timeout to context.
func WithOptionalTimeout(parent context.Context, timeout int64) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, time.Duration(timeout)*time.Second)
}
