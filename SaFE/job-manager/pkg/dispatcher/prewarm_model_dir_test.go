/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// prewarmModelDirJob returns an external job of a model prewarm, opted out of user directory mounts,
// with the given annotations.
func prewarmModelDirJob(annotations map[string]string) *v1.Workload {
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[v1.WorkloadUserDirMountsOffAnnotation] = v1.TrueStr
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "prewarm", Annotations: annotations},
		Spec:       v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.JobKind}},
		Status:     v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{ClaimId: "claim-1"}},
	}
}

// A model prewarm pod of an external node mounts its model directory below the declared root, read-only,
// only when the workload carries the mark the webhook sets for a verified model prewarm job; without the
// mark the external workload is held to the declared root itself.
func TestPrewarmModelDirMountsBelowDeclaredRoot(t *testing.T) {
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared_nfs", MountPath: "/shared_nfs"},
	}}}
	const modelDir = "/shared_nfs/models/GLM-5.3"

	marked := prewarmModelDirJob(map[string]string{v1.WorkloadModelPrewarmDirAnnotation: v1.TrueStr})
	assert.NilError(t, checkExternalHostPath(marked, ws, modelDir))
	source, subPath, readOnly := hostPathSource(marked, ws, modelDir)
	assert.Equal(t, modelDir, source)
	assert.Equal(t, "", subPath)
	assert.Assert(t, readOnly)

	unmarked := prewarmModelDirJob(nil)
	assert.ErrorContains(t, checkExternalHostPath(unmarked, ws, modelDir),
		`hostpath "/shared_nfs/models/GLM-5.3" of an external workload is below the declared root "/shared_nfs"`)
}
