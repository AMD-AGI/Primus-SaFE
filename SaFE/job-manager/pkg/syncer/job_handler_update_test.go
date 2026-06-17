/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestUpdateAdminWorkloadByJobDeleted(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	// ResourceDel action -> getK8sObjectStatus returns a K8sDeleted status without
	// needing the data-plane client factory, so updateAdminWorkloadByJob can run.
	msg := &resourceMessage{
		action:        ResourceDel,
		name:          "obj",
		dispatchCount: 1,
		gvk:           schema.GroupVersionKind{Kind: "Job"},
	}
	out, err := r.updateAdminWorkloadByJob(context.Background(), nil, w, msg)
	assert.NilError(t, err)
	assert.Assert(t, out != nil)
}
