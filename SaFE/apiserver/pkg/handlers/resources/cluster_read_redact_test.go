/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestRedactClusterInfra verifies the redaction helper clears every field that
// must be hidden from non-admin cluster readers.
func TestRedactClusterInfra(t *testing.T) {
	subnet := "10.0.0.0/16"
	resp := view.GetClusterResponse{
		Endpoint:       "10.0.0.1:6443",
		SSHSecretId:    "ssh-secret",
		ImageSecretId:  "img-secret",
		KubePodsSubnet: &subnet,
	}
	redactClusterInfra(&resp)
	assert.Empty(t, resp.Endpoint)
	assert.Empty(t, resp.SSHSecretId)
	assert.Empty(t, resp.ImageSecretId)
	assert.Nil(t, resp.KubePodsSubnet)
}

// TestGetClusterRedactsInfraForNonAdmin verifies #2: getCluster returns
// control-plane infrastructure details only to system administrators; other
// authenticated users see the cluster with those fields redacted.
func TestGetClusterRedactsInfraForNonAdmin(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-c"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	normal := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-c"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType},
	}
	subnet := "10.0.0.0/16"
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ssh-secret"}
	cluster.Spec.ControlPlane.ImageSecret = &corev1.ObjectReference{Name: "img-secret"}
	cluster.Spec.ControlPlane.KubePodsSubnet = &subnet

	ctrlClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(admin, normal, cluster).Build()
	h := &Handler{Client: ctrlClient, accessController: &authority.AccessController{Client: ctrlClient}}

	newReq := func(userID string) *gin.Context {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(common.Name, "c1")
		c.Set(common.UserId, userID)
		return c
	}

	// Non-admin: control-plane infra fields must be redacted.
	res, err := h.getCluster(newReq("user-c"))
	assert.NoError(t, err)
	got, ok := res.(view.GetClusterResponse)
	assert.True(t, ok)
	assert.Equal(t, "c1", got.ClusterId)
	assert.Empty(t, got.SSHSecretId)
	assert.Empty(t, got.ImageSecretId)
	assert.Nil(t, got.KubePodsSubnet)

	// Admin: control-plane infra fields must be present.
	res2, err := h.getCluster(newReq("admin-c"))
	assert.NoError(t, err)
	got2, ok := res2.(view.GetClusterResponse)
	assert.True(t, ok)
	assert.Equal(t, "ssh-secret", got2.SSHSecretId)
	assert.Equal(t, "img-secret", got2.ImageSecretId)
	assert.NotNil(t, got2.KubePodsSubnet)
}
