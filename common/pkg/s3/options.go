/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package s3

import (
	"flag"
	"fmt"
)

type Options struct {
	Config     string
	KubeConfig string
	BucketName string
	Action     string
	ObjectKey  string
}

func (opt *Options) InitFlags() error {
	if opt == nil {
		return fmt.Errorf("the options is not initialized")
	}
	flag.StringVar(&opt.Config, "config", "", "Path to the safe config.toml")
	flag.StringVar(&opt.KubeConfig, "kubeconf", "", "Path to the kube config")
	flag.StringVar(&opt.BucketName, "bucket", "", "bucket name")
	flag.StringVar(&opt.Action, "action", "",
		"Defines action. Supports: createBucket/deleteBucket/listBucket/getObject/listObject/deleteObject/multiPutObject")
	flag.StringVar(&opt.ObjectKey, "objectKey", "", "the object key to handle, "+
		"When the action is multiPutObject, it points to the file path")
	flag.Parse()

	if opt.BucketName == "" {
		return fmt.Errorf("-bucket is not found")
	}
	if opt.Action == "" {
		return fmt.Errorf("-action is not found")
	}
	if opt.Config == "" {
		return fmt.Errorf("-config is not found")
	}
	if opt.KubeConfig == "" {
		return fmt.Errorf("-kubeconf is not found")
	}
	return nil
}
