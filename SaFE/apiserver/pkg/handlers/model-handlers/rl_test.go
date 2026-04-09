/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestBuildRlRayJobPodTemplatesSingleNode(t *testing.T) {
	headResource := v1.WorkloadResource{Replica: 1, GPU: "8"}
	nodeResource := v1.WorkloadResource{GPU: "8"}

	resources, images, entryPoints := buildRlRayJobPodTemplates(
		headResource,
		nodeResource,
		1,
		"test-image",
		"head-init",
		"worker-init",
	)

	assert.Equal(t, len(resources), 1)
	assert.Equal(t, resources[0].Replica, 1)
	assert.Equal(t, len(images), 1)
	assert.Equal(t, images[0], "test-image")
	assert.Equal(t, len(entryPoints), 1)
	assert.Equal(t, entryPoints[0], "head-init")
}

func TestBuildRlRayJobPodTemplatesMultiNode(t *testing.T) {
	headResource := v1.WorkloadResource{Replica: 1, GPU: "8"}
	nodeResource := v1.WorkloadResource{GPU: "8"}

	resources, images, entryPoints := buildRlRayJobPodTemplates(
		headResource,
		nodeResource,
		2,
		"test-image",
		"head-init",
		"worker-init",
	)

	assert.Equal(t, len(resources), 2)
	assert.Equal(t, resources[0].Replica, 1)
	assert.Equal(t, resources[1].Replica, 1)
	assert.Equal(t, len(images), 2)
	assert.DeepEqual(t, images, []string{"test-image", "test-image"})
	assert.Equal(t, len(entryPoints), 2)
	assert.DeepEqual(t, entryPoints, []string{"head-init", "worker-init"})
}
