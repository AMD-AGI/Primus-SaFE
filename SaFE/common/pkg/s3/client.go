/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"k8s.io/utils/pointer"
)

const (
	DefaultTimeout = 180

	partSize           = 100 * 1024 * 1024  // 100MB per part
	largeFileThreshold = 1024 * 1024 * 1024 // Files larger than 1GB use concurrent download
)

type Option struct {
	ExpireDay int32
}

// Client - S3 client structure that encapsulates S3 configuration, options and AWS S3 client
// Used to perform various S3 bucket operations including upload, download, delete, etc.
type Client struct {
	*Config
	opt      Option
	s3Client *s3.Client
}

// NewClient creates and returns a new Client instance using system-wide S3 settings.
func NewClient(ctx context.Context, opt Option) (Interface, error) {
	config, err := NewConfig()
	if err != nil {
		return nil, err
	}
	return NewClientFromConfig(ctx, config, opt)
}

// newClient creates and returns a new Client instance using config
func NewClientFromConfig(ctx context.Context, config *Config, opt Option) (Interface, error) {
	s3Client := s3.NewFromConfig(config.Config, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	cli := &Client{
		Config:   config,
		opt:      opt,
		s3Client: s3Client,
	}
	if err := cli.checkBucketExisted(ctx); err != nil {
		return nil, err
	}
	if err := cli.setLifecycleRule(ctx); err != nil {
		return nil, err
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
		Key:    aws.String(key),
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
		Key:        aws.String(param.Key),
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
		Key:             aws.String(param.Key),
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
		Key:      aws.String(param.Key),
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
		Key:    aws.String(key),
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
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	return nil
}

// GeneratePresignedURL generate presigned URL for temporary object access.
func (c *Client) GeneratePresignedURL(ctx context.Context, key string, expireHour int32) (string, error) {
	presigner := s3.NewPresignClient(c.s3Client)

	resp, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = time.Duration(expireHour) * time.Hour
	})
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

// PresignModelFiles generates presigned URLs for all files under the given S3 prefix.
// Returns a map of relative file path to presigned URL.
func (c *Client) PresignModelFiles(ctx context.Context, prefix string, expireHour int32) (map[string]string, error) {
	if c == nil {
		return nil, fmt.Errorf("please init client first")
	}

	// List all objects under prefix
	result, err := c.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: c.Bucket,
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	if len(result.Contents) == 0 {
		return nil, fmt.Errorf("no objects found with prefix: %s", prefix)
	}

	// Generate presigned URL for each file
	presigner := s3.NewPresignClient(c.s3Client)
	urls := make(map[string]string)

	for _, obj := range result.Contents {
		key := *obj.Key
		if strings.HasSuffix(key, "/") {
			continue // skip directories
		}

		resp, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: c.Bucket,
			Key:    aws.String(key),
		}, func(o *s3.PresignOptions) {
			o.Expires = time.Duration(expireHour) * time.Hour
		})
		if err != nil {
			return nil, fmt.Errorf("failed to presign %s: %w", key, err)
		}

		// Use relative path (remove prefix) as key
		relativePath := strings.TrimPrefix(key, prefix)
		relativePath = strings.TrimPrefix(relativePath, "/")
		urls[relativePath] = resp.URL
	}

	return urls, nil
}

// DownloadFile downloads a file from S3 to a local directory.
// The localDir is treated as a directory path, and the original filename from the S3 key is used.
// For example: key="models/config.json", localDir="/tmp" -> "/tmp/config.json"
// Automatically chooses between simple download (for small files) and concurrent download (for large files).
func (c *Client) DownloadFile(ctx context.Context, key, localDir string) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}

	// Extract filename from key and build full local path
	filename := filepath.Base(key)
	localPath := filepath.Join(localDir, filename)

	return c.downloadToPath(ctx, key, localPath)
}

// downloadToPath downloads a file from S3 to the exact local path specified.
// This is an internal function used by both DownloadFile and DownloadDirectory.
func (c *Client) downloadToPath(ctx context.Context, key, localPath string) error {
	// Get file size
	head, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	fileSize := *head.ContentLength

	// For small files, use simple download
	if fileSize < largeFileThreshold {
		return c.downloadSmallFile(ctx, key, localPath)
	}

	// For large files, use concurrent download
	return c.downloadLargeFile(ctx, key, localPath)
}

// downloadSmallFile performs a simple single-request download for small files.
func (c *Client) downloadSmallFile(ctx context.Context, key, localPath string) error {
	resp, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err = io.Copy(file, resp.Body); err != nil {
		os.Remove(localPath) // Clean up on error
		return err
	}

	return nil
}

// downloadLargeFile performs concurrent multipart download for large files.
func (c *Client) downloadLargeFile(ctx context.Context, key, localPath string) error {
	downloader := manager.NewDownloader(c.s3Client, func(d *manager.Downloader) {
		d.PartSize = partSize
		d.Concurrency = 5
	})

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = downloader.Download(ctx, file, &s3.GetObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		os.Remove(localPath)
		return err
	}
	return nil
}

// DownloadDirectory downloads all files matching the given prefix to a local directory.
// The prefix is used to filter objects in S3, and the relative path structure is preserved.
// For example, if prefix is "models/v1/" and localDir is "/tmp/models",
// then "models/v1/config.json" will be downloaded to "/tmp/models/config.json".
func (c *Client) DownloadDirectory(ctx context.Context, prefix, localDir string) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}

	// Ensure prefix ends with "/" for directory-like behavior
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	// List all objects with the given prefix
	paginator := s3.NewListObjectsV2Paginator(c.s3Client, &s3.ListObjectsV2Input{
		Bucket: c.Bucket,
		Prefix: aws.String(prefix),
	})

	var downloadErrors []error
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects with prefix %s: %w", prefix, err)
		}

		for _, obj := range page.Contents {
			key := *obj.Key

			// Skip directory markers (keys ending with "/")
			if strings.HasSuffix(key, "/") {
				continue
			}

			// Calculate relative path by removing the prefix
			relativePath := strings.TrimPrefix(key, prefix)
			localPath := filepath.Join(localDir, relativePath)

			// Download the file to exact path (preserving directory structure)
			if err := c.downloadToPath(ctx, key, localPath); err != nil {
				downloadErrors = append(downloadErrors, fmt.Errorf("failed to download %s: %w", key, err))
			}
		}
	}

	if len(downloadErrors) > 0 {
		return fmt.Errorf("encountered %d errors during download: %v", len(downloadErrors), downloadErrors[0])
	}

	return nil
}

// WithOptionalTimeout add optional timeout to context.
func WithOptionalTimeout(parent context.Context, timeout int64) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, time.Duration(timeout)*time.Second)
}
