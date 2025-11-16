/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// TestCvtImageToFlatResponse tests conversion from model.Image to flat Image response
func TestCvtImageToFlatResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		images   []*model.Image
		validate func(*testing.T, []Image)
	}{
		{
			name: "single image conversion",
			images: []*model.Image{
				{
					ID:          1,
					Tag:         "docker.io/library/nginx:latest",
					Description: "Nginx web server",
					CreatedBy:   "user1",
					CreatedAt:   now,
				},
			},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 1)
				assert.Equal(t, int32(1), result[0].Id)
				assert.Equal(t, "docker.io/library/nginx:latest", result[0].Tag)
				assert.Equal(t, "Nginx web server", result[0].Description)
				assert.Equal(t, "user1", result[0].CreatedBy)
				assert.Equal(t, now.Unix(), result[0].CreatedAt)
			},
		},
		{
			name: "multiple images conversion",
			images: []*model.Image{
				{
					ID:          1,
					Tag:         "harbor.example.com/project/app:v1.0",
					Description: "App v1.0",
					CreatedBy:   "admin",
					CreatedAt:   now,
				},
				{
					ID:          2,
					Tag:         "harbor.example.com/project/app:v2.0",
					Description: "App v2.0",
					CreatedBy:   "admin",
					CreatedAt:   now.Add(time.Hour),
				},
			},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 2)
				assert.Equal(t, int32(1), result[0].Id)
				assert.Equal(t, int32(2), result[1].Id)
				assert.Equal(t, "App v1.0", result[0].Description)
				assert.Equal(t, "App v2.0", result[1].Description)
			},
		},
		{
			name:   "empty images list",
			images: []*model.Image{},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 0)
			},
		},
		{
			name: "image without description",
			images: []*model.Image{
				{
					ID:        10,
					Tag:       "gcr.io/project/image:tag",
					CreatedBy: "user",
					CreatedAt: now,
				},
			},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 1)
				assert.Empty(t, result[0].Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtImageToFlatResponse(tt.images)
			tt.validate(t, result)
		})
	}
}

// TestGenerateImportImageJobName tests generation of import image job names
func TestGenerateImportImageJobName(t *testing.T) {
	tests := []struct {
		name    string
		imageId int32
	}{
		{
			name:    "basic image ID",
			imageId: 123,
		},
		{
			name:    "large image ID",
			imageId: 999999,
		},
		{
			name:    "image ID 1",
			imageId: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateImportImageJobName(tt.imageId)

			// Job name should start with prefix "imptimg-"
			assert.Contains(t, result, "imptimg-")

			// Should be non-empty and have reasonable length
			assert.NotEmpty(t, result)
			assert.Greater(t, len(result), 20) // "imptimg-" + ID + "-" + 16-char hash

			// Verify format: imptimg-{id}-{16-hex-digits}
			assert.Regexp(t, `^imptimg-\d+-[0-9a-f]{16}$`, result)

			// Generate again should produce different result (due to timestamp)
			time.Sleep(1 * time.Millisecond)
			result2 := generateImportImageJobName(tt.imageId)
			assert.NotEqual(t, result, result2, "Different calls should produce different hashes")
		})
	}
}

// TestGenerateTargetImageName tests generation of target image names
func TestGenerateTargetImageName(t *testing.T) {
	tests := []struct {
		name               string
		targetRegistryHost string
		sourceImage        string
		expectedContains   []string
		wantErr            bool
	}{
		{
			name:               "valid docker.io image",
			targetRegistryHost: "harbor.example.com",
			sourceImage:        "docker.io/library/nginx:latest",
			expectedContains:   []string{"harbor.example.com", "sync", "library/nginx:latest"},
			wantErr:            false,
		},
		{
			name:               "valid gcr.io image",
			targetRegistryHost: "my-registry.io",
			sourceImage:        "gcr.io/project/app:v1.0",
			expectedContains:   []string{"my-registry.io", "sync", "project/app:v1.0"},
			wantErr:            false,
		},
		{
			name:               "invalid source image - no registry",
			targetRegistryHost: "harbor.example.com",
			sourceImage:        "nginx:latest",
			expectedContains:   nil,
			wantErr:            true,
		},
		{
			name:               "invalid source image - empty",
			targetRegistryHost: "harbor.example.com",
			sourceImage:        "",
			expectedContains:   nil,
			wantErr:            true,
		},
		{
			name:               "complex image path",
			targetRegistryHost: "internal-harbor.com",
			sourceImage:        "quay.io/organization/team/app:v2.3.1",
			expectedContains:   []string{"internal-harbor.com", "sync", "organization/team/app:v2.3.1"},
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateTargetImageName(tt.targetRegistryHost, tt.sourceImage)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

// TestGenerateAuthValue tests generation of authentication values
func TestGenerateAuthValue(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		validate func(*testing.T, string)
	}{
		{
			name:     "basic auth",
			username: "admin",
			password: "password123",
			validate: func(t *testing.T, result string) {
				// Decode and verify
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, "admin:password123", string(decoded))
			},
		},
		{
			name:     "username with special characters",
			username: "user@example.com",
			password: "pass",
			validate: func(t *testing.T, result string) {
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, "user@example.com:pass", string(decoded))
			},
		},
		{
			name:     "empty credentials",
			username: "",
			password: "",
			validate: func(t *testing.T, result string) {
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, ":", string(decoded))
			},
		},
		{
			name:     "password with special characters",
			username: "user",
			password: "P@ssw0rd!#$%",
			validate: func(t *testing.T, result string) {
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, "user:P@ssw0rd!#$%", string(decoded))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAuthValue(tt.username, tt.password)
			assert.NotEmpty(t, result)
			tt.validate(t, result)
		})
	}
}

// TestCreateImageRequestValid tests validation of CreateImageRequest
func TestCreateImageRequestValid(t *testing.T) {
	tests := []struct {
		name      string
		request   CreateImageRequest
		wantValid bool
		wantMsg   string
	}{
		{
			name: "valid request",
			request: CreateImageRequest{
				Registry:    "harbor.example.com",
				ImageTag:    "myapp:v1.0",
				Description: "My application",
				IsShare:     true,
			},
			wantValid: true,
			wantMsg:   "",
		},
		{
			name: "valid request without optional fields",
			request: CreateImageRequest{
				ImageTag: "nginx:latest",
			},
			wantValid: true,
			wantMsg:   "",
		},
		{
			name: "invalid - empty image tag",
			request: CreateImageRequest{
				Registry:    "harbor.example.com",
				Description: "Test",
			},
			wantValid: false,
			wantMsg:   "imageTag is required",
		},
		{
			name:      "invalid - completely empty",
			request:   CreateImageRequest{},
			wantValid: false,
			wantMsg:   "imageTag is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, msg := tt.request.Valid()
			assert.Equal(t, tt.wantValid, valid)
			if !tt.wantValid {
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}

// TestCreateRegistryRequestValidate tests validation of CreateRegistryRequest
func TestCreateRegistryRequestValidate(t *testing.T) {
	tests := []struct {
		name       string
		request    CreateRegistryRequest
		isCreate   bool
		wantErr    bool
		errMessage string
	}{
		{
			name: "valid create request",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				UserName: "admin",
				Password: "password123",
				Default:  true,
			},
			isCreate: true,
			wantErr:  false,
		},
		{
			name: "valid update request without password",
			request: CreateRegistryRequest{
				Id:       1,
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				UserName: "admin",
				Default:  false,
			},
			isCreate: false,
			wantErr:  false,
		},
		{
			name: "invalid - missing name",
			request: CreateRegistryRequest{
				Url:      "https://harbor.example.com",
				UserName: "admin",
				Password: "password",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "name is required",
		},
		{
			name: "invalid - missing url",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				UserName: "admin",
				Password: "password",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "url is required",
		},
		{
			name: "invalid - missing username",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				Password: "password",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "token is required",
		},
		{
			name: "invalid - missing password on create",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				UserName: "admin",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "password is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate(tt.isCreate)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
