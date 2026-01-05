/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Read environment variables
	secretPath := os.Getenv("SECRET_PATH")
	inputURL := os.Getenv("INPUT_URL")
	destPath := os.Getenv("DEST_PATH")

	// Validate required environment variables
	if secretPath == "" {
		return fmt.Errorf("[ERROR] SECRET_PATH environment variable is required")
	}
	if inputURL == "" {
		return fmt.Errorf("[ERROR] INPUT_URL environment variable is required")
	}
	if destPath == "" {
		return fmt.Errorf("[ERROR] DEST_PATH environment variable is required")
	}

	fmt.Printf("SECRET_PATH: %s\n", secretPath)
	fmt.Printf("INPUT_URL: %s\n", inputURL)
	fmt.Printf("DEST_PATH: %s\n", destPath)

	// Read access key and secret key from files
	accessKey, err := readSecretFile(secretPath, "access_key")
	if err != nil {
		return fmt.Errorf("[ERROR] failed to read access_key: %w", err)
	}

	secretKey, err := readSecretFile(secretPath, "secret_key")
	if err != nil {
		return fmt.Errorf("[ERROR] failed to read secret_key: %w", err)
	}

	fmt.Println("Credentials loaded successfully")

	// Create S3 config
	config, loc, err := s3.NewConfigFromCredentials(accessKey, secretKey, inputURL)
	if err != nil {
		return fmt.Errorf("[ERROR] failed to create S3 config: %w", err)
	}

	fmt.Printf("S3 Config - Endpoint: %s, Bucket: %s, Key: %s\n", loc.Endpoint, loc.Bucket, loc.Key)

	// Create S3 client
	ctx := context.Background()
	client, err := s3.NewClientFromConfig(ctx, config, s3.Option{})
	if err != nil {
		return fmt.Errorf("[ERROR] failed to create S3 client: %w", err)
	}

	fmt.Println("S3 client created successfully")

	startTime := time.Now()

	// Check if input URL ends with "/" to determine download mode
	if strings.HasSuffix(inputURL, "/") {
		// Directory download
		fmt.Printf("Starting directory download: %s -> %s\n", loc.Key, destPath)
		if err := client.DownloadDirectory(ctx, loc.Key, destPath); err != nil {
			return fmt.Errorf("[ERROR] failed to download directory: %w", err)
		}

		duration := time.Since(startTime)
		fmt.Printf("Directory download completed successfully in %v\n", duration)

		// Get directory size
		totalSize, fileCount, err := getDirSize(destPath)
		if err != nil {
			return fmt.Errorf("[ERROR] failed to stat downloaded directory: %w", err)
		}
		fmt.Printf("[SUCCESS] Downloaded %d files to %s, total size: %d bytes (%.2f GB)\n",
			fileCount, destPath, totalSize, float64(totalSize)/(1024*1024*1024))
	} else {
		// Single file download
		fmt.Printf("Starting file download: %s -> %s\n", loc.Key, destPath)
		if err := client.DownloadFile(ctx, loc.Key, destPath); err != nil {
			return fmt.Errorf("[ERROR] failed to download file: %w", err)
		}

		duration := time.Since(startTime)
		fmt.Printf("File download completed successfully in %v\n", duration)

		// Get file size - need to find the actual downloaded file
		filename := filepath.Base(loc.Key)
		actualPath := filepath.Join(destPath, filename)
		fileInfo, err := os.Stat(actualPath)
		if err != nil {
			return fmt.Errorf("[ERROR] failed to stat downloaded file: %w", err)
		}
		fmt.Printf("[SUCCESS] Downloaded file to %s, size: %d bytes (%.2f GB)\n",
			actualPath, fileInfo.Size(), float64(fileInfo.Size())/(1024*1024*1024))
	}

	return nil
}

// getDirSize calculates the total size and file count of a directory
func getDirSize(path string) (int64, int, error) {
	var totalSize int64
	var fileCount int

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})

	return totalSize, fileCount, err
}

// readSecretFile reads a secret file from the SECRET_PATH directory
func readSecretFile(secretPath, filename string) (string, error) {
	filePath := filepath.Join(secretPath, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Trim whitespace and newlines
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("file %s is empty", filePath)
	}

	return secret, nil
}
