/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestCvtToClusterResponseItem tests conversion from v1.Cluster to ClusterResponseItem
func TestCvtToClusterResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		cluster *v1.Cluster
		want    view.ClusterResponseItem
	}{
		{
			name: "basic cluster",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.UserIdLabel: "user-123",
					},
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			want: view.ClusterResponseItem{
				ClusterId:    "test-cluster",
				UserId:       "user-123",
				Phase:        string(v1.ReadyPhase),
				IsProtected:  false,
				CreationTime: now.Format("2006-01-02T15:04:05"),
			},
		},
		{
			name: "protected cluster",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "prod-cluster",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.UserIdLabel:  "admin-user",
						v1.ProtectLabel: "",
					},
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			want: view.ClusterResponseItem{
				ClusterId:    "prod-cluster",
				UserId:       "admin-user",
				Phase:        string(v1.ReadyPhase),
				IsProtected:  true,
				CreationTime: now.Format("2006-01-02T15:04:05"),
			},
		},
		{
			name: "cluster without user",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "no-user-cluster",
					CreationTimestamp: metav1.NewTime(now),
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.CreatingPhase,
					},
				},
			},
			want: view.ClusterResponseItem{
				ClusterId:    "no-user-cluster",
				UserId:       "",
				Phase:        string(v1.CreatingPhase),
				IsProtected:  false,
				CreationTime: now.Format("2006-01-02T15:04:05"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToClusterResponseItem(tt.cluster)
			assert.Equal(t, tt.want.ClusterId, result.ClusterId)
			assert.Equal(t, tt.want.UserId, result.UserId)
			assert.Equal(t, tt.want.Phase, result.Phase)
			assert.Equal(t, tt.want.IsProtected, result.IsProtected)
			// Time comparison - format should match
			assert.Contains(t, result.CreationTime, now.Format("2006-01-02"))
		})
	}
}

// TestParseProcessNodesRequest tests parsing of process nodes request
func TestParseProcessNodesRequest(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		nodeIds []string
		wantErr bool
	}{
		{
			name:    "add nodes",
			action:  "add",
			nodeIds: []string{"node1", "node2", "node3"},
			wantErr: false,
		},
		{
			name:    "remove nodes",
			action:  "remove",
			nodeIds: []string{"node1"},
			wantErr: false,
		},
		{
			name:    "empty node list",
			action:  "add",
			nodeIds: []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &view.ProcessNodesRequest{
				Action:  tt.action,
				NodeIds: tt.nodeIds,
			}

			assert.Equal(t, tt.action, req.Action)
			assert.Equal(t, len(tt.nodeIds), len(req.NodeIds))
		})
	}
}

// --- merged from cluster_crud_test.go ---

func TestGenerateCluster(t *testing.T) {
	h, user := newAdminHandlerWithObjects()
	req := &view.CreateClusterRequest{
		Name:           "cluster-1",
		Description:    "desc",
		Labels:         map[string]string{"team": "infra", v1.PrimusSafePrefix + "x": "skip"},
		IsProtected:    true,
		IsControlPlane: true,
	}
	cluster, err := h.generateCluster(context.Background(), user, req, []byte(`{}`))
	testifyassert.NoError(t, err)
	assert.Equal(t, "cluster-1", cluster.Name)
	assert.Equal(t, "infra", cluster.Labels["team"])
	testifyassert.True(t, v1.IsProtected(cluster))
	testifyassert.True(t, v1.HasLabel(cluster, v1.ClusterControlPlaneLabel))
}

func TestCreateClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.CreateClusterRequest{Name: "cluster-new"})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	h.CreateCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestListClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "b-cluster"}},
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "a-cluster"}},
	)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListClusterResponse
	testifyassert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, "a-cluster", resp.Items[0].ClusterId)
}

func TestGetClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.GetCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestDeleteClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("protected cluster cannot be deleted", func(t *testing.T) {
		protected := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Name:   "c-prot",
			Labels: map[string]string{v1.ProtectLabel: ""},
		}}
		h, user := newAdminHandlerWithObjects(protected)
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c-prot")
		h.DeleteCluster(c)
		testifyassert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful delete (no running workloads)", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c-del"}})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c-del")
		h.DeleteCluster(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestPatchClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}})

	protected := true
	body, _ := json.Marshal(view.PatchClusterRequest{IsProtected: &protected})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.PatchCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestApplyClusterPatch(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "c1",
		Labels: map[string]string{"old": "v"},
	}}

	// No-op patch.
	changed, err := applyClusterPatch(cluster, &view.PatchClusterRequest{})
	testifyassert.NoError(t, err)
	testifyassert.False(t, changed)

	// Set protected + control plane + label changes.
	protected := true
	cp := true
	newLabels := map[string]string{"team": "infra"}
	changed, err = applyClusterPatch(cluster, &view.PatchClusterRequest{
		IsProtected:    &protected,
		IsControlPlane: &cp,
		Labels:         &newLabels,
	})
	testifyassert.NoError(t, err)
	testifyassert.True(t, changed)
	testifyassert.True(t, v1.IsProtected(cluster))
	assert.Equal(t, "infra", cluster.Labels["team"])
	// Old custom label removed.
	_, ok := cluster.Labels["old"]
	testifyassert.False(t, ok)
}

// --- merged from cluster_more_test.go ---

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
	testifyassert.Error(t, err)

	c, err := h.getAdminCluster(context.Background(), "c1")
	testifyassert.NoError(t, err)
	assert.Equal(t, "c1", c.Name)

	_, err = h.getAdminCluster(context.Background(), "missing")
	testifyassert.Error(t, err)
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
	testifyassert.NoError(t, err)
	assert.Equal(t, "pod-new", name)

	// No matching pods -> not found.
	sel2 := labels.SelectorFromSet(map[string]string{v1.ClusterManageClusterLabel: "other"})
	_, err = h.getLatestPodName(c, sel2)
	testifyassert.Error(t, err)
}

// --- merged from cluster_nodes_test.go ---

func readyCluster(name string) *v1.Cluster {
	c := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
	c.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	return c
}

func TestProcessClusterNode(t *testing.T) {
	cluster := readyCluster("c1")
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	h, _ := newAdminHandlerWithObjects(cluster, node)

	// Add node to cluster.
	err := h.processClusterNode(context.Background(), cluster, "node-1", v1.NodeActionAdd)
	testifyassert.NoError(t, err)

	// Node not found -> error.
	err = h.processClusterNode(context.Background(), cluster, "missing", v1.NodeActionAdd)
	testifyassert.Error(t, err)
}

func TestRemoveNodesFromWorkspaceNoWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	h, user := newAdminHandlerWithObjects(node)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)

	// Node has no workspace -> no-op, no error.
	err := h.removeNodesFromWorkspace(c, []string{"node-1"}, false)
	testifyassert.NoError(t, err)
}

func TestProcessClusterNodesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("cluster not ready", func(t *testing.T) {
		cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c-notready"}}
		h, user := newAdminHandlerWithObjects(cluster)
		body, _ := json.Marshal(view.ProcessNodesRequest{NodeIds: []string{"node-1"}, Action: v1.NodeActionAdd})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c-notready")
		h.ProcessClusterNodes(c)
		testifyassert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful add", func(t *testing.T) {
		cluster := readyCluster("c1")
		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
		h, user := newAdminHandlerWithObjects(cluster, node)
		body, _ := json.Marshal(view.ProcessNodesRequest{NodeIds: []string{"node-1"}, Action: v1.NodeActionAdd})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c1")
		h.ProcessClusterNodes(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

// --- merged from cluster_read_redact_test.go ---

// TestRedactClusterInfra verifies the redaction helper clears every field that
// must be hidden from non-admin cluster readers.
func TestRedactClusterInfra(t *testing.T) {
	subnet := "10.0.0.0/16"
	svcAddr := "10.254.0.0/16"
	kubeSpray := "docker.io/kubespray:v1"
	resp := view.GetClusterResponse{
		Endpoint:           "10.0.0.1:6443",
		SSHSecretId:        "ssh-secret",
		ImageSecretId:      "img-secret",
		KubePodsSubnet:     &subnet,
		KubeServiceAddress: &svcAddr,
		KubeSprayImage:     &kubeSpray,
		Nodes:              []string{"node-a", "node-b"},
		KubeApiServerArgs:  map[string]string{"foo": "bar"},
	}
	redactClusterInfra(&resp)
	testifyassert.Empty(t, resp.Endpoint)
	testifyassert.Empty(t, resp.SSHSecretId)
	testifyassert.Empty(t, resp.ImageSecretId)
	testifyassert.Nil(t, resp.KubePodsSubnet)
	testifyassert.Nil(t, resp.Nodes)
	testifyassert.Nil(t, resp.KubeServiceAddress)
	testifyassert.Nil(t, resp.KubeApiServerArgs)
	testifyassert.Nil(t, resp.KubeSprayImage)
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
	svcAddr := "10.254.0.0/16"
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ssh-secret"}
	cluster.Spec.ControlPlane.ImageSecret = &corev1.ObjectReference{Name: "img-secret"}
	cluster.Spec.ControlPlane.KubePodsSubnet = &subnet
	cluster.Spec.ControlPlane.KubeServiceAddress = &svcAddr
	cluster.Spec.ControlPlane.Nodes = []string{"node-a", "node-b"}
	cluster.Spec.ControlPlane.KubeApiServerArgs = map[string]string{"foo": "bar"}

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
	testifyassert.NoError(t, err)
	got, ok := res.(view.GetClusterResponse)
	testifyassert.True(t, ok)
	assert.Equal(t, "c1", got.ClusterId)
	testifyassert.Empty(t, got.SSHSecretId)
	testifyassert.Empty(t, got.ImageSecretId)
	testifyassert.Nil(t, got.KubePodsSubnet)
	testifyassert.Nil(t, got.Nodes)
	testifyassert.Nil(t, got.KubeServiceAddress)
	testifyassert.Nil(t, got.KubeApiServerArgs)

	// Admin: control-plane infra fields must be present.
	res2, err := h.getCluster(newReq("admin-c"))
	testifyassert.NoError(t, err)
	got2, ok := res2.(view.GetClusterResponse)
	testifyassert.True(t, ok)
	assert.Equal(t, "ssh-secret", got2.SSHSecretId)
	assert.Equal(t, "img-secret", got2.ImageSecretId)
	testifyassert.NotNil(t, got2.KubePodsSubnet)
	testifyassert.NotNil(t, got2.Nodes)
	testifyassert.NotNil(t, got2.KubeServiceAddress)
}
