// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// StorageExporterConfig is the main configuration for storage-exporter
type StorageExporterConfig struct {
	HttpPort      int           `yaml:"httpPort" json:"httpPort"`
	LoadK8SClient bool          `yaml:"loadK8SClient" json:"loadK8SClient"`
	Storage       StorageConfig `yaml:"storage" json:"storage"`
	Metrics       MetricsConfig `yaml:"metrics" json:"metrics"`
}

// StorageConfig contains storage monitoring configuration
type StorageConfig struct {
	ScrapeInterval string        `yaml:"scrapeInterval" json:"scrapeInterval"`
	Mounts         []MountConfig `yaml:"mounts" json:"mounts"`
}

// MountConfig defines a storage mount to monitor
type MountConfig struct {
	Name           string `yaml:"name" json:"name"`
	MountPath      string `yaml:"mountPath" json:"mountPath"`
	StorageType    string `yaml:"storageType" json:"storageType"`
	FilesystemName string `yaml:"filesystemName" json:"filesystemName"`
	PVCName        string `yaml:"pvcName" json:"pvcName"`
}

// GetScrapeInterval returns the scrape interval as duration
func (s StorageConfig) GetScrapeInterval() time.Duration {
	d, err := time.ParseDuration(s.ScrapeInterval)
	if err != nil {
		return 60 * time.Second
	}
	return d
}

// MetricsConfig contains prometheus metrics configuration
type MetricsConfig struct {
	StaticLabels map[string]string `yaml:"staticLabels" json:"staticLabels"`
}

// LoadStorageExporterConfig loads configuration from file
func LoadStorageExporterConfig() (*StorageExporterConfig, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	var config StorageExporterConfig
	decoder := yaml.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.HttpPort == 0 {
		config.HttpPort = 8992
	}

	return &config, nil
}
