/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtractEphemeralRunnerMetaFromAnnotations(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetAnnotations(map[string]string{
		AnnotationRunID:      "100",
		AnnotationJobID:      "200",
		AnnotationWorkflow:   "ci",
		AnnotationRepository: "owner/repo",
		AnnotationBranch:     "main",
		AnnotationSHA:        "abc",
	})
	meta := ExtractEphemeralRunnerMeta(obj)
	assert.Equal(t, int64(100), meta.GithubRunID)
	assert.Equal(t, int64(200), meta.GithubJobID)
	assert.Equal(t, "ci", meta.WorkflowName)
	assert.Equal(t, "owner", meta.Owner)
	assert.Equal(t, "repo", meta.Repo)
	assert.True(t, meta.HasGithubMeta())
}

func TestExtractEphemeralRunnerMetaFromStatus(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"workflowRunId":     int64(999),
			"jobRepositoryName": "o2/r2",
		},
	}}
	meta := ExtractEphemeralRunnerMeta(obj)
	assert.Equal(t, int64(999), meta.GithubRunID)
	assert.Equal(t, "o2", meta.Owner)
	assert.Equal(t, "r2", meta.Repo)
}

func TestHasGithubMetaFalse(t *testing.T) {
	meta := &EphemeralRunnerMeta{}
	assert.False(t, meta.HasGithubMeta())
}

func TestParseAnnotationInt64(t *testing.T) {
	assert.Equal(t, int64(0), parseAnnotationInt64(""))
	assert.Equal(t, int64(42), parseAnnotationInt64(" 42 "))
	assert.Equal(t, int64(0), parseAnnotationInt64("nan"))
}

func TestParseRepository(t *testing.T) {
	owner, name := parseRepository("a/b")
	assert.Equal(t, "a", owner)
	assert.Equal(t, "b", name)

	owner, name = parseRepository("single")
	assert.Equal(t, "", owner)
	assert.Equal(t, "single", name)
}
