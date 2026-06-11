/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newFullHandler builds a Handler with both a fake controller-runtime client
// (admin plane) and a k8s fake clientSet (local pods/secrets), seeded with the
// admin user/role and any extra CR objects.
func newFullHandler(crObjs []client.Object, k8sObjs ...runtime.Object) (*Handler, *v1.User) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	all := append([]client.Object{mockUser, mockRole}, crObjs...)
	ctrlClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).Build()
	h := &Handler{
		Client:           ctrlClient,
		clientSet:        k8sfake.NewSimpleClientset(k8sObjs...),
		accessController: authority.NewAccessController(ctrlClient),
	}
	return h, mockUser
}

func TestGetAdminCluster(t *testing.T) {
	h, _ := newFullHandler([]client.Object{&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}})

	_, err := h.getAdminCluster(context.Background(), "")
	assert.Error(t, err)

	c, err := h.getAdminCluster(context.Background(), "c1")
	assert.NoError(t, err)
	assert.Equal(t, "c1", c.Name)

	_, err = h.getAdminCluster(context.Background(), "missing")
	assert.Error(t, err)
}

func TestCvtToGetClusterResponse(t *testing.T) {
	h, _ := newFullHandler(nil)
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Labels: map[string]string{"team": "infra"}},
	}
	resp := cvtToGetClusterResponse(context.Background(), h.Client, cluster)
	assert.Equal(t, "c1", resp.ClusterId)
	assert.Equal(t, "infra", resp.Labels["team"])
}

func TestGetLatestPodName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	older := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "pod-old", Namespace: common.PrimusSafeNamespace,
		Labels:            map[string]string{v1.ClusterManageClusterLabel: "c1"},
		CreationTimestamp: metav1.NewTime(now.Add(-time.Hour)),
	}}
	newer := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "pod-new", Namespace: common.PrimusSafeNamespace,
		Labels:            map[string]string{v1.ClusterManageClusterLabel: "c1"},
		CreationTimestamp: metav1.NewTime(now),
	}}
	h, _ := newFullHandler(nil, older, newer)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	sel := labels.SelectorFromSet(map[string]string{v1.ClusterManageClusterLabel: "c1"})
	name, err := h.getLatestPodName(c, sel)
	assert.NoError(t, err)
	assert.Equal(t, "pod-new", name)

	// No matching pods -> not found.
	sel2 := labels.SelectorFromSet(map[string]string{v1.ClusterManageClusterLabel: "other"})
	_, err = h.getLatestPodName(c, sel2)
	assert.Error(t, err)
}
