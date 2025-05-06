/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package s3

import (
	"context"

	"k8s.io/client-go/kubernetes"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

var (
	conf *Config
)

type Config struct {
	Enable   string                `json:"enable" yaml:"enable"`
	Clusters []SingleClusterConfig `json:"clusters" yaml:"clusters"`
}

func (c Config) Enabled() bool {
	return c.Enable == "true"
}

type SingleClusterConfig struct {
	Module    []string `json:"module" yaml:"module"`
	Namespace string   `json:"namespace" yaml:"namespace"`
	Secret    string   `json:"secret" yaml:"secret"`
	Endpoint  string   `json:"endpoint" yaml:"endpoint"`
	Region    string   `json:"region" yaml:"region"`
}

type ClusterConfig struct {
	AccessKey    string `json:"access_key" yaml:"access_key"`
	SecretKey    string `json:"secret_key" yaml:"secret_key"`
	Endpoint     string `json:"endpoint" yaml:"endpoint"`
	Token        string `json:"token" yaml:"token"`
	BucketRegion string `json:"bucket_region" yaml:"bucket_region"`
}

func initConfigs(ctx context.Context) error {
	clusterConfig := commonconfig.GetS3Configs()
	err := jsonutils.DecodeFromMapWithJson(clusterConfig, &conf)
	if err != nil {
		return err
	}
	return nil
}

func loadCLusterConfig(ctx context.Context, k8sClient kubernetes.Interface, c *SingleClusterConfig) (ClusterConfig, error) {
	ak, sk, err := getS3Secret(ctx, k8sClient, c)
	if err != nil {
		return ClusterConfig{}, err
	}
	return ClusterConfig{
		AccessKey:    ak,
		SecretKey:    sk,
		Endpoint:     c.Endpoint,
		Token:        "",
		BucketRegion: c.Region,
	}, nil
}
