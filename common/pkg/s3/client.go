/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"k8s.io/klog/v2"
)

var (
	once     sync.Once
	instance Interface
)

type Client struct {
	*Config
}

func NewClient(ctx context.Context) Interface {
	once.Do(func() {
		if instance != nil {
			return
		}
		instance = newClient(ctx)
	})
	return instance
}

func newClient(ctx context.Context) Interface {
	config, err := GetConfig()
	if err != nil {
		klog.ErrorS(err, "failed to get config")
		return nil
	}
	cli := &Client{
		Config: config,
	}
	if isExist, err := cli.IsBucketExisted(ctx, config.DefaultTimeout); err != nil {
		klog.ErrorS(err, "failed to check bucket")
		return nil
	} else if !isExist {
		if err = cli.CreateBucket(ctx, config.DefaultTimeout); err != nil {
			klog.ErrorS(err, "failed to create bucket")
		}
		klog.Infof("created bucket %s successfully", *config.Bucket)
		return nil
	}
	klog.Infof("init s3 client successfully, endpoint: %s", *cli.Endpoint)
	return cli
}

func (c *Client) newS3Client() (*s3.S3, error) {
	newSession, err := session.NewSession(c.GetS3Config())
	if err != nil {
		return nil, err
	}
	return s3.New(newSession), nil
}

func (c *Client) CreateBucket(ctx context.Context, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return err
	}
	input := &s3.CreateBucketInput{Bucket: c.Bucket}

	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()
	if _, err = s3Client.CreateBucketWithContext(timeoutCtx, input); err != nil {
		return err
	}
	if err = c.putBucketLifecycle(s3Client); err != nil {
		return err
	}
	if err = c.putBucketPolicy(s3Client); err != nil {
		return err
	}
	return nil
}

func (c *Client) putBucketLifecycle(s3Client *s3.S3) error {
	_, err := s3Client.PutBucketLifecycleConfiguration(&s3.PutBucketLifecycleConfigurationInput{
		Bucket: c.Bucket,
		LifecycleConfiguration: &s3.BucketLifecycleConfiguration{
			Rules: []*s3.LifecycleRule{
				{
					ID:     aws.String("ExpireAfterOneDay"),
					Filter: &s3.LifecycleRuleFilter{Prefix: aws.String("")},
					Expiration: &s3.LifecycleExpiration{
						Days: c.ExpireDay,
					},
					Status: aws.String("Enabled"),
				},
			},
		},
	})
	return err
}

func (c *Client) putBucketPolicy(s3Client *s3.S3) error {
	policy := `{
        "Statement": [
            {
                "Sid": "PublicReadGetObject",
                "Effect": "Allow", 
                "Principal": "*",
                "Action": "s3:GetObject",
                "Resource": "arn:aws:s3:::` + *c.Bucket + `/*"
            }
        ]
    }`
	_, err := s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: c.Bucket,
		Policy: aws.String(policy),
	})
	return err
}

func (c *Client) CreateMultiPartUpload(ctx context.Context, key string, timeout int64) (*s3.S3, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("please init client first")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return nil, "", err
	}
	input := &s3.CreateMultipartUploadInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	output, err := s3Client.CreateMultipartUploadWithContext(timeoutCtx, input)
	if err != nil {
		return nil, "", err
	}
	return s3Client, *output.UploadId, nil
}

func (c *Client) MultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) (*s3.CompletedPart, error) {
	if c == nil {
		return nil, fmt.Errorf("please init client first")
	}
	input := &s3.UploadPartInput{
		Bucket:     c.Bucket,
		Key:        aws.String(param.Key),
		UploadId:   aws.String(param.UploadId),
		PartNumber: aws.Int64(param.PartNumber),
		Body:       bytes.NewReader([]byte(param.Value)),
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	output, err := param.S3Client.UploadPartWithContext(timeoutCtx, input)
	if err != nil {
		return nil, err
	}
	return &s3.CompletedPart{
		ETag:       output.ETag,
		PartNumber: aws.Int64(param.PartNumber),
	}, nil
}

func (c *Client) CompleteMultiPartUpload(ctx context.Context, param *MultiUploadParam, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	if len(param.CompletedParts) == 0 {
		return nil
	}
	input := &s3.CompleteMultipartUploadInput{
		Bucket:          c.Bucket,
		Key:             aws.String(param.Key),
		UploadId:        aws.String(param.UploadId),
		MultipartUpload: &s3.CompletedMultipartUpload{Parts: param.CompletedParts},
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	_, err := param.S3Client.CompleteMultipartUploadWithContext(timeoutCtx, input)
	return err
}

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

	_, err := param.S3Client.AbortMultipartUploadWithContext(timeoutCtx, input)
	return err
}

func (c *Client) IsBucketExisted(ctx context.Context, timeout int64) (bool, error) {
	if c == nil {
		return false, fmt.Errorf("please init client first")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return false, err
	}
	input := &s3.HeadBucketInput{
		Bucket: c.Bucket,
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	if _, err = s3Client.HeadBucketWithContext(timeoutCtx, input); err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Client) ListBucket(ctx context.Context, timeout int64) (*s3.ListBucketsOutput, error) {
	if c == nil {
		return nil, fmt.Errorf("please init client first")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return nil, err
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	output, err := s3Client.ListBucketsWithContext(timeoutCtx, nil)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *Client) DeleteBucket(ctx context.Context, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return err
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	_, err = s3Client.DeleteBucketWithContext(timeoutCtx, &s3.DeleteBucketInput{Bucket: c.Bucket})
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) PutObject(ctx context.Context, key, value string, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	if key == "" || value == "" {
		return fmt.Errorf("the object key or value is empty")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte(value)),
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	if _, err = s3Client.PutObjectWithContext(timeoutCtx, input); err != nil {
		return err
	}
	return nil
}

func (c *Client) ListObject(ctx context.Context, timeout int64) (*s3.ListObjectsOutput, error) {
	if c == nil {
		return nil, fmt.Errorf("please init client first")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return nil, err
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	output, err := s3Client.ListObjectsWithContext(timeoutCtx, &s3.ListObjectsInput{Bucket: c.Bucket})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *Client) GetObject(ctx context.Context, key string, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	if key == "" {
		return fmt.Errorf("the object key is empty")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return err
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	resp, err := s3Client.GetObjectWithContext(timeoutCtx, &s3.GetObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	file, err := os.Create(key)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err = file.ReadFrom(resp.Body); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteObject(ctx context.Context, key string, timeout int64) error {
	if c == nil {
		return fmt.Errorf("please init client first")
	}
	if key == "" {
		return fmt.Errorf("the object key is empty")
	}
	s3Client, err := c.newS3Client()
	if err != nil {
		return err
	}
	timeoutCtx, cancel := WithOptionalTimeout(ctx, timeout)
	defer cancel()

	_, err = s3Client.DeleteObjectWithContext(timeoutCtx, &s3.DeleteObjectInput{
		Bucket: c.Bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	return nil
}

func WithOptionalTimeout(parent context.Context, timeout int64) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, time.Duration(timeout)*time.Second)
}
