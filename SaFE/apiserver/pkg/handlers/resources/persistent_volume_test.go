/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestParseListPersistentVolumeQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?workspaceId=ws-1", nil)
	q, err := parseListPersistentVolumeQuery(c)
	assert.NoError(t, err)
	assert.Equal(t, "ws-1", q.WorkspaceID)
}

func TestBuildListPersistentVolumeSelector(t *testing.T) {
	sel, err := buildListPersistentVolumeSelector(&view.ListPersistentVolumeRequest{WorkspaceID: "ws-1"})
	assert.NoError(t, err)
	assert.False(t, sel.Empty())
}

func TestCvtToPersistentVolumeItem(t *testing.T) {
	pv := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pv-1",
			Labels: map[string]string{common.PfsSelectorKey: "pfs-a"},
		},
		Spec:   corev1.PersistentVolumeSpec{StorageClassName: "sc-1"},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeAvailable},
	}
	item := cvtToPersistentVolumeItem(pv)
	assert.Equal(t, "sc-1", item.StorageClassName)
	assert.Equal(t, "pfs-a", item.Labels[common.PfsSelectorKey])
}

// TestListPersistentVolume exercises the full path using a stubbed dataplane
// client factory injected into the Handler's clientManager.
func TestListPersistentVolume(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	ctrlClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser, mockRole, workspace).Build()

	// Stub the dataplane factory for cluster "c1" backed by a k8s fake with a PV.
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{
		Name:   "pv-1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
	}}
	om := commonutils.NewObjectManager()
	factory := k8sclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", k8sfake.NewSimpleClientset(pv))
	_ = om.Add("c1", factory)

	h := &Handler{
		Client:           ctrlClient,
		accessController: authority.NewAccessController(ctrlClient),
		clientManager:    om,
	}

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?workspaceId=ws-1", nil)
	c.Set(common.UserId, mockUser.Name)
	h.ListPersistentVolume(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListPersistentVolumeResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.TotalCount)
}
