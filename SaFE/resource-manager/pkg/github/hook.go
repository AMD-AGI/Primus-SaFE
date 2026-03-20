/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

// WorkflowTracker is the main entry point for GitHub workflow tracking in job-manager.
// It is called by the Syncer when EphemeralRunner events are received.
type WorkflowTracker struct {
	store *Store
}

func NewWorkflowTracker(store *Store) *WorkflowTracker {
	return &WorkflowTracker{store: store}
}

// OnEphemeralRunnerEvent is called when an EphemeralRunner is created, updated, or deleted.
// It extracts GitHub metadata from annotations and creates/updates workflow run records.
func (t *WorkflowTracker) OnEphemeralRunnerEvent(ctx context.Context,
	obj *unstructured.Unstructured, workloadID, cluster string, isCompleted bool) {

	meta := ExtractEphemeralRunnerMeta(obj)
	if !meta.HasGithubMeta() {
		return
	}

	now := time.Now()
	status := "running"
	if isCompleted {
		status = "completed"
	}

	run := &WorkflowRunRecord{
		WorkloadID:   workloadID,
		Cluster:      cluster,
		GithubRunID:  meta.GithubRunID,
		GithubJobID:  meta.GithubJobID,
		WorkflowName: meta.WorkflowName,
		GithubOwner:  meta.Owner,
		GithubRepo:   meta.Repo,
		HeadBranch:   meta.Branch,
		HeadSHA:      meta.SHA,
		Status:       status,
		StartedAt:    &now,
	}

	if err := t.store.UpsertWorkflowRun(ctx, run); err != nil {
		klog.V(1).Infof("[github-tracker] upsert run for workload %s: %v", workloadID, err)
		return
	}

	klog.V(2).Infof("[github-tracker] tracked run: workload=%s github_run=%d workflow=%s status=%s",
		workloadID, meta.GithubRunID, meta.WorkflowName, status)
}
