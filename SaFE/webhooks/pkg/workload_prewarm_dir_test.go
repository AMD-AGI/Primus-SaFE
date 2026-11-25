/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const prewarmModelDir = "/shared_nfs/models/GLM-5.3"

// prewarmDirFixture returns a mutator with an OpsJob of the given type, whose node is external when
// external is set, and a prewarm workload of the job.
func prewarmDirFixture(t *testing.T, jobType v1.OpsJobType, nodeExternal bool,
	modelPath string) (*WorkloadMutator, *v1.Workload) {
	t.Helper()
	spec := v1.NodeSpec{}
	if nodeExternal {
		spec = v1.NodeSpec{
			LifecycleMode: v1.NodeLifecycleExternal,
			ExternalRef:   &v1.NodeExternalRef{Provider: "autopilot", AllocationId: "alloc-1"},
		}
	}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}, Spec: spec}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", UID: "job-uid"},
		Spec: v1.OpsJobSpec{Type: jobType, Inputs: []v1.Parameter{
			{Name: v1.ParameterModelPath, Value: modelPath},
			{Name: v1.ParameterNode, Value: "node-1"},
		}},
	}
	cli := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(node, job).Build()
	isController := true
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "prewarm",
			Labels:          map[string]string{v1.OpsJobIdLabel: "j1"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "OpsJob", Name: "j1", UID: "job-uid", Controller: &isController}},
		},
		Spec: v1.WorkloadSpec{
			Workspace:        "ws1",
			GroupVersionKind: v1.GroupVersionKind{Kind: common.JobKind, Version: common.DefaultVersion},
			Hostpath:         []string{prewarmModelDir},
		},
	}
	return &WorkloadMutator{Client: cli}, workload
}

// TestMutateModelPrewarmDir verifies that the mark of a model prewarm pod is set only for a verified
// job: a model prewarm job with an external node, owning the workload, whose model path is its hostpath.
func TestMutateModelPrewarmDir(t *testing.T) {
	marked := func(w *v1.Workload) bool {
		return v1.GetAnnotation(w, v1.WorkloadModelPrewarmDirAnnotation) == v1.TrueStr
	}
	ctx := context.Background()

	m, w := prewarmDirFixture(t, v1.OpsJobModelPrewarmType, true, prewarmModelDir)
	m.mutateModelPrewarmDir(ctx, w)
	assert.Assert(t, marked(w), "verified job")

	m, w = prewarmDirFixture(t, v1.OpsJobModelPrewarmType, true, prewarmModelDir)
	v1.SetAnnotation(w, v1.WorkloadModelPrewarmDirAnnotation, v1.TrueStr)
	w.Labels = nil
	m.mutateModelPrewarmDir(ctx, w)
	assert.Assert(t, !marked(w), "a request without a job removes the mark")

	m, w = prewarmDirFixture(t, v1.OpsJobModelPrewarmType, true, "/shared_nfs/models/other")
	m.mutateModelPrewarmDir(ctx, w)
	assert.Assert(t, !marked(w), "model path differs from the hostpath")

	m, w = prewarmDirFixture(t, v1.OpsJobModelPrewarmType, false, prewarmModelDir)
	m.mutateModelPrewarmDir(ctx, w)
	assert.Assert(t, !marked(w), "no external node")

	m, w = prewarmDirFixture(t, v1.OpsJobModelPrewarmType, true, prewarmModelDir)
	w.OwnerReferences[0].UID = "other-uid"
	m.mutateModelPrewarmDir(ctx, w)
	assert.Assert(t, !marked(w), "the job does not own the workload")

	m, w = prewarmDirFixture(t, v1.OpsJobType("download"), true, prewarmModelDir)
	m.mutateModelPrewarmDir(ctx, w)
	assert.Assert(t, !marked(w), "not a model prewarm job")
}

// TestKeepModelPrewarmDirOnUpdate verifies that an update keeps the mark the workload was created with.
func TestKeepModelPrewarmDirOnUpdate(t *testing.T) {
	marked := func(w *v1.Workload) bool {
		return v1.GetAnnotation(w, v1.WorkloadModelPrewarmDirAnnotation) == v1.TrueStr
	}
	created := &v1.Workload{}
	v1.SetAnnotation(created, v1.WorkloadModelPrewarmDirAnnotation, v1.TrueStr)

	updated := &v1.Workload{}
	keepModelPrewarmDir(created, updated)
	assert.Assert(t, marked(updated), "the mark is kept")

	forged := &v1.Workload{}
	v1.SetAnnotation(forged, v1.WorkloadModelPrewarmDirAnnotation, v1.TrueStr)
	keepModelPrewarmDir(&v1.Workload{}, forged)
	assert.Assert(t, !marked(forged), "an update cannot add the mark")
}
