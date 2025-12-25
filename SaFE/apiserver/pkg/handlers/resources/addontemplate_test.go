/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
)

// TestCvtToAddonTemplateResponseItem tests conversion from v1.AddonTemplate to AddonTemplateResponseItem
func TestCvtToAddonTemplateResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		template *v1.AddonTemplate
		validate func(*testing.T, view.AddonTemplateResponseItem)
	}{
		{
			name: "basic addon template",
			template: &v1.AddonTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "prometheus-template",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonTemplateSpec{
					Type:        v1.AddonTemplateHelm,
					Category:    "Monitoring",
					Version:     "1.0.0",
					Description: "Prometheus monitoring stack",
					GpuChip:     "", // Empty means applies to all chips
					Required:    false,
				},
			},
			validate: func(t *testing.T, result view.AddonTemplateResponseItem) {
				assert.Equal(t, "prometheus-template", result.AddonTemplateId)
				assert.Equal(t, string(v1.AddonTemplateHelm), result.Type)
				assert.Equal(t, "Monitoring", result.Category)
				assert.Equal(t, "1.0.0", result.Version)
				assert.Equal(t, "Prometheus monitoring stack", result.Description)
				assert.Empty(t, result.GpuChip)
				assert.False(t, result.Required)
				assert.Contains(t, result.CreationTime, now.Format("2006-01-02"))
			},
		},
		{
			name: "required template with AMD GPU",
			template: &v1.AddonTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rocm-driver",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonTemplateSpec{
					Type:        v1.AddonTemplateHelm,
					Category:    "Driver",
					Version:     "5.7.0",
					Description: "AMD ROCm driver",
					GpuChip:     v1.AmdGpuChip,
					Required:    true,
				},
			},
			validate: func(t *testing.T, result view.AddonTemplateResponseItem) {
				assert.Equal(t, "rocm-driver", result.AddonTemplateId)
				assert.Equal(t, "Driver", result.Category)
				assert.Equal(t, "5.7.0", result.Version)
				assert.Equal(t, string(v1.AmdGpuChip), result.GpuChip)
				assert.True(t, result.Required)
			},
		},
		{
			name: "nvidia GPU template",
			template: &v1.AddonTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cuda-driver",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonTemplateSpec{
					Type:        v1.AddonTemplateHelm,
					Category:    "Driver",
					Version:     "12.2",
					Description: "NVIDIA CUDA driver",
					GpuChip:     v1.NvidiaGpuChip,
					Required:    true,
				},
			},
			validate: func(t *testing.T, result view.AddonTemplateResponseItem) {
				assert.Equal(t, "cuda-driver", result.AddonTemplateId)
				assert.Equal(t, string(v1.NvidiaGpuChip), result.GpuChip)
				assert.Equal(t, "12.2", result.Version)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToAddonTemplateResponseItem(tt.template)
			tt.validate(t, result)
		})
	}
}

// TestCvtToGetAddonTemplateResponse tests conversion to detailed addon template response
func TestCvtToGetAddonTemplateResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		template *v1.AddonTemplate
		validate func(*testing.T, view.GetAddonTemplateResponse)
	}{
		{
			name: "complete addon template",
			template: &v1.AddonTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "grafana",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonTemplateSpec{
					Type:                 v1.AddonTemplateHelm,
					Category:             "Monitoring",
					Version:              "9.5.0",
					Description:          "Grafana dashboard",
					GpuChip:              "", // Empty means applies to all chips
					Required:             false,
					URL:                  "https://grafana.com/grafana",
					Icon:                 "grafana-icon.png",
					HelmDefaultValues:    "replicas: 1\nport: 3000",
					HelmDefaultNamespace: "monitoring",
				},
			},
			validate: func(t *testing.T, result view.GetAddonTemplateResponse) {
				assert.Equal(t, "grafana", result.AddonTemplateId)
				assert.Equal(t, "https://grafana.com/grafana", result.URL)
				assert.Equal(t, "grafana-icon.png", result.Icon)
				assert.Equal(t, "replicas: 1\nport: 3000", result.HelmDefaultValues)
				assert.Equal(t, "monitoring", result.HelmDefaultNamespace)
			},
		},
		{
			name: "minimal template",
			template: &v1.AddonTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "minimal",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonTemplateSpec{
					Type:     v1.AddonTemplateHelm,
					Category: "Other",
					Version:  "1.0.0",
				},
			},
			validate: func(t *testing.T, result view.GetAddonTemplateResponse) {
				assert.Equal(t, "minimal", result.AddonTemplateId)
				assert.Empty(t, result.URL)
				assert.Empty(t, result.Icon)
				assert.Empty(t, result.HelmDefaultValues)
				assert.Empty(t, result.HelmDefaultNamespace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToGetAddonTemplateResponse(tt.template)
			tt.validate(t, result)
		})
	}
}
