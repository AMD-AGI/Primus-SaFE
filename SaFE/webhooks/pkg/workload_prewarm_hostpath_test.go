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

// TestValidateHostPathOfPrewarmPod verifies the admission of a model prewarm pod's host path: the model
// directory is admitted without the grant when the workspace declares it, through a volume with user
// directories or a subPath, and it needs the grant mark for a volume that requires the privileged grant.
func TestValidateHostPathOfPrewarmPod(t *testing.T) {
	scheme := newScheme(t)
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, MountPath: "/data", HostPath: "/data", EnableUserDir: true},
		{Id: 2, Type: v1.HOSTPATH, MountPath: "/models", HostPath: "/nfs", SubPath: "models"},
		{Id: 3, Type: v1.HOSTPATH, MountPath: "/secret", HostPath: "/secret", RequiresPrivilegedGrant: true},
	}}}
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()}
	ctx := context.Background()

	prewarmPod := func(modelPath string, granted bool) *v1.Workload {
		w := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{Name: "prewarm"},
			Spec: v1.WorkloadSpec{
				Workspace:        "ws1",
				GroupVersionKind: v1.GroupVersionKind{Kind: common.JobKind, Version: common.DefaultVersion},
				Hostpath:         []string{modelPath},
			},
		}
		if granted {
			v1.SetAnnotation(w, v1.WorkloadHostPathAuthzAnnotation, v1.TrueStr)
		}
		return w
	}

	assert.NilError(t, v.validateHostPath(ctx, prewarmPod("/data/models/glm", false), nil))
	assert.NilError(t, v.validateHostPath(ctx, prewarmPod("/nfs/models/glm", false), nil))
	assert.NilError(t, v.validateHostPath(ctx, prewarmPod("/secret/glm", true), nil))
	assert.ErrorContains(t, v.validateHostPath(ctx, prewarmPod("/secret/glm", false), nil), "workload/privileged")
}
