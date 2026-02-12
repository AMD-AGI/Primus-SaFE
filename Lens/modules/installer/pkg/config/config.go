// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"context"
	"fmt"
	"os"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Config holds the installer configuration
type Config struct {
	// Task identification
	TaskID      int32
	ClusterName string

	// Control Plane DB connection
	CPDBHost     string
	CPDBPort     int
	CPDBName     string
	CPDBUser     string
	CPDBPassword string
	CPDBSSLMode  string

	// Helm settings
	HelmTimeout string
	DryRun      bool
}

// LoadFromEnv loads configuration from environment variables
// If CP_DB_USER/CP_DB_PASSWORD are not set, it will try to load from K8s secret
func LoadFromEnv() (*Config, error) {
	taskIDStr := os.Getenv("TASK_ID")
	if taskIDStr == "" {
		return nil, fmt.Errorf("TASK_ID environment variable is required")
	}
	taskID, err := strconv.ParseInt(taskIDStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid TASK_ID: %w", err)
	}

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		return nil, fmt.Errorf("CLUSTER_NAME environment variable is required")
	}

	cpDBHost := os.Getenv("CP_DB_HOST")
	if cpDBHost == "" {
		cpDBHost = "primus-lens-control-plane-primary.primus-lens.svc.cluster.local"
	}

	cpDBPortStr := os.Getenv("CP_DB_PORT")
	cpDBPort := 5432
	if cpDBPortStr != "" {
		if p, err := strconv.Atoi(cpDBPortStr); err == nil {
			cpDBPort = p
		}
	}

	cpDBName := os.Getenv("CP_DB_NAME")
	if cpDBName == "" {
		cpDBName = "primus-lens-control-plane"
	}

	cpDBUser := os.Getenv("CP_DB_USER")
	cpDBPassword := os.Getenv("CP_DB_PASSWORD")

	// If credentials not in env, try to load from K8s secret
	if cpDBUser == "" || cpDBPassword == "" {
		secretUser, secretPassword, err := loadDBCredentialsFromSecret()
		if err != nil {
			return nil, fmt.Errorf("CP_DB_USER/CP_DB_PASSWORD not set and failed to load from secret: %w", err)
		}
		if cpDBUser == "" {
			cpDBUser = secretUser
		}
		if cpDBPassword == "" {
			cpDBPassword = secretPassword
		}
	}

	cpDBSSLMode := os.Getenv("CP_DB_SSL_MODE")
	if cpDBSSLMode == "" {
		cpDBSSLMode = "require"
	}

	helmTimeout := os.Getenv("HELM_TIMEOUT")
	if helmTimeout == "" {
		helmTimeout = "10m"
	}

	dryRun := os.Getenv("DRY_RUN") == "true"

	return &Config{
		TaskID:       int32(taskID),
		ClusterName:  clusterName,
		CPDBHost:     cpDBHost,
		CPDBPort:     cpDBPort,
		CPDBName:     cpDBName,
		CPDBUser:     cpDBUser,
		CPDBPassword: cpDBPassword,
		CPDBSSLMode:  cpDBSSLMode,
		HelmTimeout:  helmTimeout,
		DryRun:       dryRun,
	}, nil
}

// loadDBCredentialsFromSecret loads database credentials from K8s secret
func loadDBCredentialsFromSecret() (string, string, error) {
	// Use in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", "", fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Secret name and namespace (same as used by control plane components)
	secretName := os.Getenv("CP_DB_SECRET_NAME")
	if secretName == "" {
		secretName = "primus-lens-control-plane-pguser-primus-lens-control-plane"
	}
	secretNamespace := os.Getenv("CP_DB_SECRET_NAMESPACE")
	if secretNamespace == "" {
		secretNamespace = "primus-lens"
	}

	secret, err := clientset.CoreV1().Secrets(secretNamespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, secretName, err)
	}

	user := string(secret.Data["user"])
	password := string(secret.Data["password"])

	if user == "" || password == "" {
		return "", "", fmt.Errorf("user or password not found in secret %s/%s", secretNamespace, secretName)
	}

	return user, password, nil
}

// GetCPDBDSN returns the PostgreSQL connection string for control plane DB
func (c *Config) GetCPDBDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.CPDBHost, c.CPDBPort, c.CPDBUser, c.CPDBPassword, c.CPDBName, c.CPDBSSLMode,
	)
}
