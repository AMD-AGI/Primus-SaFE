/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// mutateModelPrewarmDir sets WorkloadModelPrewarmDirAnnotation on the pod of a model prewarm job only
// when the job is verified: it is the OpsJob the workload belongs to, its type is model prewarm, it has
// an external node, and its model path is the only hostpath of the workload. The annotation is removed
// from the request first, so a request cannot set it.
func (m *WorkloadMutator) mutateModelPrewarmDir(ctx context.Context, workload *v1.Workload) {
	v1.RemoveAnnotation(workload, v1.WorkloadModelPrewarmDirAnnotation)
	if m.isVerifiedModelPrewarmDir(ctx, workload) {
		v1.SetAnnotation(workload, v1.WorkloadModelPrewarmDirAnnotation, v1.TrueStr)
	}
}

// keepModelPrewarmDir keeps the mark of a workload on update as it was when the workload was created,
// so that an update cannot add or remove it.
func keepModelPrewarmDir(oldWorkload, newWorkload *v1.Workload) {
	v1.RemoveAnnotation(newWorkload, v1.WorkloadModelPrewarmDirAnnotation)
	if v1.GetAnnotation(oldWorkload, v1.WorkloadModelPrewarmDirAnnotation) == v1.TrueStr {
		v1.SetAnnotation(newWorkload, v1.WorkloadModelPrewarmDirAnnotation, v1.TrueStr)
	}
}

// isVerifiedModelPrewarmDir reports whether the workload is the pod of a verified model prewarm job.
func (m *WorkloadMutator) isVerifiedModelPrewarmDir(ctx context.Context, workload *v1.Workload) bool {
	jobName := v1.GetLabel(workload, v1.OpsJobIdLabel)
	if jobName == "" || len(workload.Spec.Hostpath) != 1 {
		return false
	}
	job := &v1.OpsJob{}
	if err := m.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		return false
	}
	owner := metav1.GetControllerOf(workload)
	if owner == nil || owner.UID != job.UID || job.Spec.Type != v1.OpsJobModelPrewarmType {
		return false
	}
	modelPaths := job.GetParameters(v1.ParameterModelPath)
	if len(modelPaths) != 1 || filepath.Clean(modelPaths[0].Value) != filepath.Clean(workload.Spec.Hostpath[0]) {
		return false
	}
	for _, param := range job.GetParameters(v1.ParameterNode) {
		if param.Value == "" {
			continue
		}
		node, err := getNode(ctx, m.Client, param.Value)
		if err == nil && node.IsExternal() {
			return true
		}
	}
	return false
}
