/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GitHub annotation keys on EphemeralRunner (set by ARC controller)
var githubAnnotationKeys = []string{
	"actions.github.com/run-id",
	"actions.github.com/run-number",
	"actions.github.com/job-id",
	"actions.github.com/workflow",
	"actions.github.com/repository",
	"actions.github.com/branch",
	"actions.github.com/sha",
}

// syncGithubAnnotations copies GitHub-related annotations from the remote
// EphemeralRunner K8s object to the admin-plane Workload CRD.
// This allows resource-manager to read GitHub metadata without accessing
// remote clusters.
//
// Called from handleResource for EphemeralRunner events. Only patches
// if there are new annotations to add.
func (r *ClusterClientSets) syncGithubAnnotations(newObj *unstructured.Unstructured) {
	if newObj.GroupVersionKind().Kind != common.CICDEphemeralRunnerKind {
		return
	}

	remoteAnnotations := newObj.GetAnnotations()
	if remoteAnnotations == nil {
		return
	}

	workloadID := v1.GetWorkloadId(newObj)
	if workloadID == "" {
		return
	}

	toSync := make(map[string]string)
	for _, key := range githubAnnotationKeys {
		if val, ok := remoteAnnotations[key]; ok && val != "" {
			toSync[key] = val
		}
	}
	if len(toSync) == 0 {
		return
	}

	ctx := context.Background()
	wl := &v1.Workload{}
	if err := r.adminClient.Get(ctx, client.ObjectKey{
		Namespace: common.PrimusSafeNamespace,
		Name:      workloadID,
	}, wl); err != nil {
		klog.V(2).Infof("[github-sync] cannot get workload %s: %v", workloadID, err)
		return
	}

	existing := wl.GetAnnotations()
	if existing == nil {
		existing = make(map[string]string)
	}

	changed := false
	for k, v := range toSync {
		if existing[k] != v {
			existing[k] = v
			changed = true
		}
	}

	if !changed {
		return
	}

	wl.SetAnnotations(existing)
	if err := r.adminClient.Update(ctx, wl); err != nil {
		klog.V(1).Infof("[github-sync] patch workload %s annotations: %v", workloadID, err)
		return
	}

	klog.Infof("[github-sync] synced %d github annotations to workload %s", len(toSync), workloadID)
}
