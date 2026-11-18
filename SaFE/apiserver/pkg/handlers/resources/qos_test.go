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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func qosRequest(t *testing.T, h *Handler, user, method, name string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	raw, err := json.Marshal(body)
	assert.NoError(t, err)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(method, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user)
	c.Set(common.Name, name)
	switch method {
	case http.MethodPost:
		h.CreateQos(c)
	case http.MethodPut:
		h.UpdateQos(c)
	default:
		h.GetQos(c)
	}
	return rsp
}

func storedQos(t *testing.T, h *Handler, name string) *v1.Qos {
	t.Helper()
	qos := &v1.Qos{}
	err := h.Get(t.Context(), client.ObjectKey{Name: name}, qos)
	assert.NoError(t, err)
	return qos
}

func TestQosCreateUpdateAndGet(t *testing.T) {
	h, user := newAdminHandlerWithObjects()
	body := view.PutQosRequest{
		MaxResources: &v1.ResourceCap{Memory: "512", CPU: "16"},
		MaxRuntime:   map[string]int{string(v1.TrainScope): 7200, string(v1.InferScope): 3600},
	}
	rsp := qosRequest(t, h, user.Name, http.MethodPost, "c1", body)
	assert.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	created := storedQos(t, h, "c1")
	assert.Equal(t, "512Gi", created.Spec.MaxResources.Memory)
	assert.Equal(t, 7200, created.Spec.MaxRuntime[string(v1.TrainScope)])

	rsp = qosRequest(t, h, user.Name, http.MethodPost, "c1", body)
	assert.Equal(t, http.StatusConflict, rsp.Code, rsp.Body.String())

	rsp = qosRequest(t, h, user.Name, http.MethodPost, v1.QosDefaultName, body)
	assert.NotEqual(t, http.StatusOK, rsp.Code, rsp.Body.String())

	rsp = qosRequest(t, h, user.Name, http.MethodPut, "missing", body)
	assert.NotEqual(t, http.StatusOK, rsp.Code, rsp.Body.String())

	update := view.PutQosRequest{
		MaxResources: &v1.ResourceCap{CPU: "16"},
		MaxRuntime:   map[string]int{string(v1.TrainScope): 7200},
	}
	rsp = qosRequest(t, h, user.Name, http.MethodPut, "c1", update)
	assert.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	updated := storedQos(t, h, "c1")
	assert.Empty(t, updated.Spec.MaxResources.Memory)
	_, inferLeft := updated.Spec.MaxRuntime[string(v1.InferScope)]
	assert.False(t, inferLeft)
	assert.Equal(t, 7200, updated.Spec.MaxRuntime[string(v1.TrainScope)])

	rsp = qosRequest(t, h, user.Name, http.MethodGet, "c1", nil)
	assert.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var got view.QosResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	assert.Equal(t, "c1", got.Name)
	assert.Equal(t, "16", got.MaxResources.CPU)
}

func TestQosUpdateClearsEveryTimeout(t *testing.T) {
	existing := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxRuntime: map[string]int{string(v1.RayScope): 1800}},
	}
	h, user := newAdminHandlerWithObjects(existing)
	rsp := qosRequest(t, h, user.Name, http.MethodPut, "c1", view.PutQosRequest{
		MaxResources: &v1.ResourceCap{GPU: "8"},
	})
	assert.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	updated := storedQos(t, h, "c1")
	assert.Empty(t, updated.Spec.MaxRuntime)
	assert.Equal(t, "8", updated.Spec.MaxResources.GPU)
}

// TestWorkloadTimeoutAboveQosNeedsHighPriority verifies that an update to a timeout above the
// qos limit, or to no timeout, is allowed with the high-priority grant and denied without it.
func TestWorkloadTimeoutAboveQosNeedsHighPriority(t *testing.T) {
	f := newAccessFixture(t)
	assert.NoError(t, f.cli.Create(t.Context(), &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxRuntime: map[string]int{string(v1.TrainScope): 7200}},
	}))
	update := func(userId string, timeout int) error {
		requestUser := &v1.User{}
		assert.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: userId}, requestUser))
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{Name: "w-" + userId, Labels: map[string]string{
				v1.ClusterIdLabel: "c1", v1.UserIdLabel: userId,
			}},
			Spec: v1.WorkloadSpec{
				GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
				Timeout:          &[]int{3600}[0],
			},
		}
		roles := f.h.accessController.GetRoles(t.Context(), requestUser)
		return f.h.authWorkloadUpdate(f.ctx(userId, workload.Name, ""), workload, requestUser, roles,
			&view.PatchWorkloadRequest{Timeout: &timeout})
	}
	assert.NoError(t, update(userAdmin, 0))
	assert.NoError(t, update(userAdmin, 9000))
	assert.NoError(t, update(userDefault, 1800))
	assert.Error(t, update(userDefault, 9000))
}
