/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
)

// prewarmNode returns an admin node, external when ext is set.
func prewarmNode(name string, ext bool) *v1.Node {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if ext {
		node.Spec.LifecycleMode = v1.NodeLifecycleExternal
		node.Spec.ExternalRef = &v1.NodeExternalRef{Provider: "autopilot", AllocationId: "alloc-" + name}
	}
	return node
}

// prewarmJobOf returns a model-prewarm ops job with one node parameter per name; "" is an excluded node.
func prewarmJobOf(nodes ...string) *v1.OpsJob {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "mp"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobModelPrewarmType}}
	for _, n := range nodes {
		job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{Name: v1.ParameterNode, Value: n})
	}
	return job
}

// A model prewarm of an external node needs a system administrator: a member is refused, and a
// prewarm of nodes with node-agent keeps the authorization it had.
func TestModelPrewarmExternalNodeRequiresSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plain := genPlainUser()
	admin := genMockUser()
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		plain, admin, genMockRole(), prewarmNode("ext-1", true), prewarmNode("reg-1", false)).Build()
	h := &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}}

	asUser := func(user *v1.User, job *v1.OpsJob) error {
		c, _ := newOpsJobCtx(user.Name, "{}")
		requestUser, err := h.getAndSetUsername(c)
		require.NoError(t, err)
		return h.authModelPrewarmExternalNodes(c, job, requestUser)
	}

	err := asUser(plain, prewarmJobOf("reg-1", "ext-1"))
	require.Error(t, err)
	require.True(t, apierrors.IsForbidden(err), "%v", err)

	require.NoError(t, asUser(plain, prewarmJobOf("reg-1")))
	// An excluded node has an empty value and is not prewarmed.
	require.NoError(t, asUser(plain, prewarmJobOf("", "reg-1")))
	require.NoError(t, asUser(admin, prewarmJobOf("reg-1", "ext-1")))
}
