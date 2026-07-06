/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// syncGithubAnnotations copies GitHub workflow metadata from the remote
// EphemeralRunner's status fields to the admin-plane Workload CRD annotations.
//
// EphemeralRunner status fields (set by ARC when job is assigned):
//   - status.workflowRunId
//   - status.jobId
//   - status.jobDisplayName
//   - status.jobRepositoryName
//   - status.jobWorkflowRef
//
// These are written as annotations on the Workload CRD so that
// resource-manager can read them without accessing remote clusters.
func (r *ClusterClientSets) syncGithubAnnotations(newObj *unstructured.Unstructured) {
	if newObj.GroupVersionKind().Kind != common.CICDEphemeralRunnerKind {
		return
	}

	workloadID := v1.GetWorkloadId(newObj)
	if workloadID == "" {
		return
	}

	toSync := make(map[string]string)

	if v, ok, _ := unstructured.NestedInt64(newObj.Object, "status", "workflowRunId"); ok && v > 0 {
		toSync["actions.github.com/run-id"] = fmt.Sprint(v)
	}
	if v, ok, _ := unstructured.NestedInt64(newObj.Object, "status", "jobId"); ok && v > 0 {
		toSync["actions.github.com/job-id"] = fmt.Sprint(v)
	}
	if v, ok, _ := unstructured.NestedString(newObj.Object, "status", "jobDisplayName"); ok && v != "" {
		toSync["actions.github.com/workflow"] = v
	}
	if v, ok, _ := unstructured.NestedString(newObj.Object, "status", "jobRepositoryName"); ok && v != "" {
		toSync["actions.github.com/repository"] = v
	}
	if v, ok, _ := unstructured.NestedString(newObj.Object, "status", "jobWorkflowRef"); ok && v != "" {
		toSync["actions.github.com/workflow-ref"] = v
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
	if len(existing) == 0 {
		existing = make(map[string]string)
	}

	// Collect only the annotations that actually change.
	delta := map[string]any{}
	for k, v := range toSync {
		if existing[k] != v {
			delta[k] = v
		}
	}
	if len(delta) == 0 {
		return
	}

	// Patch only the changed annotation keys with a resourceVersion-guarded JSON
	// merge patch (same pattern as scheduler.updateStatus): a stale copy conflicts
	// instead of clobbering a concurrent spec/status write via a full-object Update.
	patchObj := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": wl.ResourceVersion,
			"annotations":     delta,
		},
	}
	p := jsonutils.MarshalSilently(patchObj)
	if err := r.adminClient.Patch(ctx, wl, client.RawPatch(apitypes.MergePatchType, p)); err != nil {
		klog.V(1).Infof("[github-sync] patch workload %s annotations: %v", workloadID, err)
		return
	}

	klog.Infof("[github-sync] synced github metadata to workload %s: run-id=%s repo=%s",
		workloadID, toSync["actions.github.com/run-id"], toSync["actions.github.com/repository"])
}
