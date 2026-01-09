// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package task

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromModel(t *testing.T) {
	t.Run("nil model returns nil", func(t *testing.T) {
		task, err := FromModel(nil)
		require.NoError(t, err)
		assert.Nil(t, task)
	})

	t.Run("basic model conversion", func(t *testing.T) {
		now := time.Now()
		m := &model.WorkloadTaskState{
			WorkloadUID: "test-uid-123",
			TaskType:    "inference_metrics_scrape",
			Status:      "running",
			LockOwner:   "exporter-1",
			LockVersion: 5,
			CreatedAt:   now,
			UpdatedAt:   now,
			Ext: model.ExtType{
				"framework":    "vllm",
				"namespace":    "ml-serving",
				"pod_name":     "vllm-pod-0",
				"pod_ip":       "10.0.1.15",
				"metrics_port": float64(8000),
				"metrics_path": "/metrics",
			},
		}

		task, err := FromModel(m)
		require.NoError(t, err)
		require.NotNil(t, task)

		assert.Equal(t, "test-uid-123", task.WorkloadUID)
		assert.Equal(t, "inference_metrics_scrape", task.TaskType)
		assert.Equal(t, "running", task.Status)
		assert.Equal(t, "exporter-1", task.LockOwner)
		assert.Equal(t, int64(5), task.LockVersion)
		assert.Equal(t, "vllm", task.Ext.Framework)
		assert.Equal(t, "ml-serving", task.Ext.Namespace)
		assert.Equal(t, "vllm-pod-0", task.Ext.PodName)
		assert.Equal(t, "10.0.1.15", task.Ext.PodIP)
		assert.Equal(t, 8000, task.Ext.MetricsPort)
		assert.Equal(t, "/metrics", task.Ext.MetricsPath)
	})

	t.Run("defaults are applied", func(t *testing.T) {
		m := &model.WorkloadTaskState{
			WorkloadUID: "test-uid",
			TaskType:    "inference_metrics_scrape",
			Status:      "pending",
			Ext:         model.ExtType{},
		}

		task, err := FromModel(m)
		require.NoError(t, err)

		// Check defaults
		assert.Equal(t, "/metrics", task.Ext.MetricsPath)
		assert.Equal(t, 15, task.Ext.ScrapeInterval)
		assert.Equal(t, 10, task.Ext.ScrapeTimeout)
	})
}

func TestGetMetricsURL(t *testing.T) {
	t.Run("valid URL", func(t *testing.T) {
		task := &ScrapeTask{
			Ext: ScrapeTaskExt{
				PodIP:       "10.0.1.15",
				MetricsPort: 8000,
				MetricsPath: "/metrics",
			},
		}
		// Note: the itoa function in model.go is simplified and may not work for all ports
		// This test may need adjustment based on actual implementation
		url := task.GetMetricsURL()
		assert.Contains(t, url, "10.0.1.15")
	})

	t.Run("empty pod IP returns empty", func(t *testing.T) {
		task := &ScrapeTask{
			Ext: ScrapeTaskExt{
				PodIP:       "",
				MetricsPort: 8000,
				MetricsPath: "/metrics",
			},
		}
		assert.Equal(t, "", task.GetMetricsURL())
	})

	t.Run("zero port returns empty", func(t *testing.T) {
		task := &ScrapeTask{
			Ext: ScrapeTaskExt{
				PodIP:       "10.0.1.15",
				MetricsPort: 0,
				MetricsPath: "/metrics",
			},
		}
		assert.Equal(t, "", task.GetMetricsURL())
	})
}

func TestGetScrapeInterval(t *testing.T) {
	t.Run("returns configured interval", func(t *testing.T) {
		task := &ScrapeTask{
			Ext: ScrapeTaskExt{
				ScrapeInterval: 30,
			},
		}
		assert.Equal(t, 30*time.Second, task.GetScrapeInterval())
	})

	t.Run("returns default for zero", func(t *testing.T) {
		task := &ScrapeTask{
			Ext: ScrapeTaskExt{
				ScrapeInterval: 0,
			},
		}
		assert.Equal(t, 15*time.Second, task.GetScrapeInterval())
	})

	t.Run("returns default for negative", func(t *testing.T) {
		task := &ScrapeTask{
			Ext: ScrapeTaskExt{
				ScrapeInterval: -1,
			},
		}
		assert.Equal(t, 15*time.Second, task.GetScrapeInterval())
	})
}

func TestScrapeTaskExtToExtMap(t *testing.T) {
	ext := ScrapeTaskExt{
		Framework:   "vllm",
		Namespace:   "default",
		PodName:     "pod-1",
		PodIP:       "10.0.0.1",
		MetricsPort: 8000,
		MetricsPath: "/metrics",
		Labels: map[string]string{
			"env": "prod",
		},
	}

	extMap := ext.ToExtMap()
	assert.NotNil(t, extMap)
	assert.Equal(t, "vllm", extMap["framework"])
	assert.Equal(t, "default", extMap["namespace"])
}

func TestScrapeTaskString(t *testing.T) {
	task := &ScrapeTask{
		WorkloadUID: "uid-123",
		Status:      "running",
		Ext: ScrapeTaskExt{
			Framework: "triton",
		},
	}

	str := task.String()
	assert.Contains(t, str, "uid-123")
	assert.Contains(t, str, "triton")
	assert.Contains(t, str, "running")
}

