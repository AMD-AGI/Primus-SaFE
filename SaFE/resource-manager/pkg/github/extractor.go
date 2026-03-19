/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ExtractEphemeralRunnerMeta extracts GitHub metadata from an EphemeralRunner
// K8s object's annotations and status fields.
func ExtractEphemeralRunnerMeta(obj *unstructured.Unstructured) *EphemeralRunnerMeta {
	meta := &EphemeralRunnerMeta{}
	annotations := obj.GetAnnotations()

	if annotations != nil {
		meta.GithubRunID = parseAnnotationInt64(annotations[AnnotationRunID])
		meta.GithubJobID = parseAnnotationInt64(annotations[AnnotationJobID])
		meta.WorkflowName = annotations[AnnotationWorkflow]
		meta.Repository = annotations[AnnotationRepository]
		meta.Branch = annotations[AnnotationBranch]
		meta.SHA = annotations[AnnotationSHA]
	}

	if meta.GithubRunID == 0 {
		if v, ok, _ := unstructured.NestedInt64(obj.Object, "status", "workflowRunId"); ok {
			meta.GithubRunID = v
		}
	}
	if meta.Repository == "" {
		if v, ok, _ := unstructured.NestedString(obj.Object, "status", "jobRepositoryName"); ok {
			meta.Repository = v
		}
	}

	if meta.Repository != "" {
		meta.Owner, meta.Repo = parseRepository(meta.Repository)
	}

	return meta
}

func parseAnnotationInt64(value string) int64 {
	if value == "" {
		return 0
	}
	v, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return v
}

func parseRepository(repo string) (owner, name string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", repo
}

// HasGithubMeta returns true if the meta has meaningful GitHub data.
func (m *EphemeralRunnerMeta) HasGithubMeta() bool {
	return m.GithubRunID > 0 || m.WorkflowName != ""
}
