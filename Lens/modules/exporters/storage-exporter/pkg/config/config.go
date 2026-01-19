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
	HttpPort      int              `yaml:"httpPort" json:"httpPort"`
	LoadK8SClient bool             `yaml:"loadK8SClient" json:"loadK8SClient"`
	Storage       StorageConfig    `yaml:"storage" json:"storage"`
	Controller    ControllerConfig `yaml:"controller" json:"controller"`
	Metrics       MetricsConfig    `yaml:"metrics" json:"metrics"`
}

// StorageConfig contains storage monitoring configuration
type StorageConfig struct {
	ScrapeInterval string `yaml:"scrapeInterval" json:"scrapeInterval"`
}

// ControllerConfig contains controller configuration
type ControllerConfig struct {
	// Namespace where PVCs and collector pods will be created
	Namespace string `yaml:"namespace" json:"namespace"`
	// CollectorImage is the image used for collector pods
	CollectorImage string `yaml:"collectorImage" json:"collectorImage"`
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
	if config.Controller.Namespace == "" {
		config.Controller.Namespace = "primus-lens"
	}
	if config.Controller.CollectorImage == "" {
		config.Controller.CollectorImage = "alpine:3.19"
	}

	return &config, nil
}
