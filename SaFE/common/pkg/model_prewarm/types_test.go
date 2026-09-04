/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_prewarm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateModelPath(t *testing.T) {
	assert.NoError(t, ValidateModelPath("/shared_nfs/models/GLM-5.3"))
	assert.Error(t, ValidateModelPath(""))
	assert.Error(t, ValidateModelPath("relative/path"))
	assert.Error(t, ValidateModelPath("/shared_nfs/../etc/passwd"))
}

func TestValidateGlob(t *testing.T) {
	assert.NoError(t, ValidateGlob("*.safetensors"))
	assert.Error(t, ValidateGlob(""))
	assert.Error(t, ValidateGlob("*.safetensors;rm -rf /"))
}

func TestMarshalRoundTrip(t *testing.T) {
	req := &Request{
		OpsJobId:    "job-1",
		ModelPath:   "/models/foo",
		Glob:        "*.safetensors",
		Parallelism: 4,
	}
	raw, err := MarshalRequest(req)
	assert.NoError(t, err)
	parsed, err := ParseRequest(raw)
	assert.NoError(t, err)
	assert.Equal(t, req.OpsJobId, parsed.OpsJobId)
	assert.Equal(t, req.ModelPath, parsed.ModelPath)
}

func TestAnnotationKeys(t *testing.T) {
	assert.Equal(t, "primus-safe.ops.job.model-prewarm.request.job-1", RequestAnnotationKey("job-1"))
	assert.Equal(t, "primus-safe.ops.job.model-prewarm.result.job-1", ResultAnnotationKey("job-1"))
}
