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
	jobUID := "edcb8cbd-b452-405e-b368-65e15734b726"
	reqKey := RequestAnnotationKey(jobUID)
	resKey := ResultAnnotationKey(jobUID)
	assert.Equal(t, "primus-safe.ops.job.mp.r."+jobUID, reqKey)
	assert.Equal(t, "primus-safe.ops.job.mp.s."+jobUID, resKey)
	assert.LessOrEqual(t, len(reqKey), MaxK8sAnnotationKeyLength)
	assert.LessOrEqual(t, len(resKey), MaxK8sAnnotationKeyLength)
	assert.NoError(t, ValidateAnnotationKeySuffix(jobUID))
}

func TestAnnotationKeyFitsProductionJobName(t *testing.T) {
	// Regression: prewarm-glm53-ws-ntvmh exceeded the old 64-char key limit.
	jobUID := "edcb8cbd-b452-405e-b368-65e15734b726"
	key := RequestAnnotationKey(jobUID)
	assert.LessOrEqual(t, len(key), MaxK8sAnnotationKeyLength)
}
