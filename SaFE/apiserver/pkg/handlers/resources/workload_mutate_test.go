/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newWorkloadMutateHandler builds a handler seeded with a running workload owned
// by the admin user, with status subresource enabled for updates.
func newWorkloadMutateHandler(workloadId string) (*Handler, *v1.User, *v1.Workload) {
	user := genMockUser()
	role := genMockRole()
	workload := genMockWorkload("test-cluster", "test-workspace")
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)

	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)
	ctrlClient := ctrlruntimefake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(user, role, workload).
		WithStatusSubresource(workload).
		Build()
	h := &Handler{
		Client:           ctrlClient,
		clientSet:        k8sfake.NewSimpleClientset(),
		accessController: authority.NewAccessController(ctrlClient),
	}
	return h, user, workload
}

func TestStopWorkloadWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user, _ := newWorkloadMutateHandler("wl-stop")

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, "wl-stop")
	h.StopWorkload(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Workload should be removed from etcd after stop.
	got := &v1.Workload{}
	err := h.Get(c.Request.Context(), client.ObjectKey{Name: "wl-stop"}, got)
	assert.Error(t, err)
}

func TestPatchWorkloadWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user, _ := newWorkloadMutateHandler("wl-patch")

	newPriority := 1
	body, _ := json.Marshal(view.PatchWorkloadRequest{Priority: &newPriority})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, "wl-patch")
	h.PatchWorkload(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
