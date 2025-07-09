/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

type Config struct {
	*aws.Config
	Bucket *string
	// The unit is seconds
	DefaultTimeout int64
	ExpireDay      *int64
}

func GetConfig() (*Config, error) {
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("s3 is disabled")
	}
	if commonconfig.GetS3AccessKey() == "" {
		return nil, fmt.Errorf("failed to find AccessKey of s3")
	}
	if commonconfig.GetS3SecretKey() == "" {
		return nil, fmt.Errorf("failed to find SecretKey of s3")
	}
	if commonconfig.GetS3Endpoint() == "" {
		return nil, fmt.Errorf("failed to find endpoint of s3")
	}
	if commonconfig.GetS3Bucket() == "" {
		return nil, fmt.Errorf("failed to find bucket of s3")
	}

	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(
			commonconfig.GetS3AccessKey(), commonconfig.GetS3SecretKey(), ""),
		Endpoint:         pointer.String(commonconfig.GetS3Endpoint()),
		Region:           aws.String("us-east-1"),
		S3ForcePathStyle: aws.Bool(true),
	}
	klog.Infof("S3 Config, endpoint: %s, bucket: %s", commonconfig.GetS3Endpoint(), commonconfig.GetS3Bucket())

	config := &Config{
		Config:         s3Config,
		DefaultTimeout: 180,
		ExpireDay:      pointer.Int64(int64(commonconfig.GetS3ExpireDay())),
	}
	config.setBucket()
	return config, nil
}

func (c *Config) GetS3Config() *aws.Config {
	return c.Config
}

func (c *Config) setBucket() {
	bucket := ""
	schemeIdx := strings.Index(commonconfig.GetS3Bucket(), "://")
	if schemeIdx == -1 {
		bucket = commonconfig.GetS3Bucket()
	} else {
		bucket = commonconfig.GetS3Bucket()[schemeIdx+3:]
	}
	c.Bucket = pointer.String(bucket)
}
