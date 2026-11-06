/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// victimUserId is another user's id, which a request names to take over that user's identity
// (and, on an external node, that user's directory).
const victimUserId = "fedcba9876543210fedcba9876543210"

// userIdHandler returns a handler whose cluster holds a user without the system-admin role,
// granted everything through the default role, and the control plane node workload creation
// reads.
func userIdHandler(t *testing.T, objs ...client.Object) (*Handler, *v1.User) {
	t.Helper()
	user := genMockUser()
	user.Spec.Roles = []v1.UserRole{v1.DefaultRole}
	role := genMockRole()
	role.Name = string(v1.DefaultRole)
	controlPlaneNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane-node-1",
			Labels: map[string]string{common.KubernetesControlPlane: ""}},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: "192.168.1.100"}}},
	}
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(append([]client.Object{user, role, controlPlaneNode}, objs...)...).
		WithScheme(mockScheme).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	return &Handler{
		Client:    fakeCtrlClient,
		clientSet: k8sfake.NewSimpleClientset(),
		// Its own access controller: NewAccessController keeps the first client it is given.
		accessController: &authority.AccessController{Client: fakeCtrlClient},
	}, user
}

func userIdRequest(user *v1.User, method, path, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// A create request that names another user, in its labels or as the user it is created for,
// still creates a workload whose creator is the user who sent it; the request's other labels
// are kept, so the label was dropped for its key and not with the rest.
func TestCreateWorkloadTakesTheCreatorFromTheRequestUser(t *testing.T) {
	h, user := userIdHandler(t)
	body := fmt.Sprintf(`{
		"displayName": "forged-user",
		"workspaceId": "test-workspace",
		"images": ["pytorch/pytorch:latest"],
		"entryPoints": ["python train.py"],
		"groupVersionKind": {"version": "v1", "kind": "PyTorchJob"},
		"resources": [{"replica": 1, "cpu": "16", "gpu": "4", "memory": "64Gi"}],
		"labels": {%q: %q, "team": "x"},
		"userEntity": {"id": %q, "name": "victim"}
	}`, v1.UserIdLabel, victimUserId, victimUserId)
	result, err := h.createWorkload(userIdRequest(user, http.MethodPost, "/api/v1/workloads", body))
	assert.NilError(t, err)
	created := &v1.Workload{}
	assert.NilError(t, h.Get(context.Background(),
		client.ObjectKey{Name: result.(*view.CreateWorkloadResponse).WorkloadId}, created))
	assert.Equal(t, v1.GetUserId(created), user.Name)
	assert.Equal(t, v1.GetLabel(created, "team"), "x")
}

// A patch request has no way to name the creator: the labels and user it carries are not part of
// a patch, the creator stays, and the rest of the patch is applied.
func TestPatchWorkloadKeepsTheCreator(t *testing.T) {
	workload := genMockWorkload("test-cluster", "test-workspace")
	workload.Name = "test-workload-forged-patch"
	workload.Status.Phase = v1.WorkloadRunning
	workload.Spec.Priority = 0
	h, user := userIdHandler(t, workload)
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)
	assert.NilError(t, h.Update(context.Background(), workload))

	body := fmt.Sprintf(`{"priority": 1, "labels": {%q: %q}, "userEntity": {"id": %q}}`,
		v1.UserIdLabel, victimUserId, victimUserId)
	c := userIdRequest(user, http.MethodPatch, "/api/v1/workloads/"+workload.Name, body)
	c.Set(common.Name, workload.Name)
	_, err := h.patchWorkload(c)
	assert.NilError(t, err)
	patched := &v1.Workload{}
	assert.NilError(t, h.Get(context.Background(), client.ObjectKey{Name: workload.Name}, patched))
	assert.Equal(t, patched.Spec.Priority, 1)
	assert.Equal(t, v1.GetUserId(patched), user.Name)
}
