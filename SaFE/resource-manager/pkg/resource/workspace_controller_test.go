/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	testifyassert "github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonquantity "github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func newMockWorkspaceReconciler(adminClient client.Client) WorkspaceReconciler {
	return WorkspaceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: adminClient,
		},
		// The real one is mgr.GetAPIReader(); here the same fake store answers both, which is
		// what the production pair does too once the cache has caught up.
		apiReader:     adminClient,
		option:        &defaultWorkspaceOption,
		expectations:  make(map[string]*nodeExpectations),
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
}

func genMockWorkspace(clusterName, nodeFlavor string, replica int) *v1.Workspace {
	result := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("workspace"),
			Labels: map[string]string{
				v1.ClusterIdLabel: clusterName,
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:    clusterName,
			NodeFlavor: nodeFlavor,
			Replica:    replica,
		},
		Status: v1.WorkspaceStatus{
			Phase: v1.WorkspaceRunning,
		},
	}
	controllerutil.AddFinalizer(result, v1.WorkspaceFinalizer)
	return result
}

func TestDeleteWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	adminNode1.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode1.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	// The claim as well as the label: that is the pair a settled binding leaves behind,
	// and syncWorkspace counts on the claim.
	adminNode1.Spec.Workspace = pointer.String(workspace.Name)
	adminNode2.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode2.Spec.Workspace = pointer.String(workspace.Name)
	adminClient := fake.NewClientBuilder().WithObjects(workspace, adminNode1, adminNode2).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()

	var err error
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceRunning)
	assert.Equal(t, controllerutil.ContainsFinalizer(workspace, v1.WorkspaceFinalizer), true)

	r := newMockWorkspaceReconciler(adminClient)
	// One pass. The claims are released here, and the finalizer goes with them: the labels
	// have not made the round trip yet -- nothing in this test delivers the Node events that
	// would settle them -- and a workspace on its way out has no further decision to make
	// that waiting for them would protect.
	err = r.delete(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, controllerutil.ContainsFinalizer(workspace, v1.WorkspaceFinalizer), false)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceDeleting)
	// And nothing is left behind in the expectations map for a workspace that no longer exists.
	assert.Equal(t, r.meetExpectations(workspace.Name), true)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), "")
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), "")
}

// nodeBlindCache is a manager cache that has not caught up: it answers every read from the
// real store except a Node List, which comes back empty. Writes go straight through, as they
// do in production -- only reads are cached.
type nodeBlindCache struct {
	client.Client
}

func (c nodeBlindCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.Client.List(ctx, list, opts...); err != nil {
		return err
	}
	if nodeList, ok := list.(*v1.NodeList); ok {
		nodeList.Items = nil
	}
	return nil
}

func TestDeleteWorkspaceReleasesANodeTheCacheHasNotSeen(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	claimed := genMockAdminNode("node-fresh", clusterName, nodeFlavor)
	claimed.Spec.Workspace = pointer.String(workspace.Name)
	metav1.SetMetaDataLabel(&claimed.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	store := fake.NewClientBuilder().WithObjects(workspace, claimed).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()

	// The node this controller bound a moment ago, in the state the two readers disagree
	// about: the apiserver has it, the manager's cache does not. That is not a contrived
	// split -- it is what a cache looks like right after its own client's write.
	r := newMockWorkspaceReconciler(nodeBlindCache{store})
	r.apiReader = store

	assert.NilError(t, store.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace))
	assert.NilError(t, r.delete(context.Background(), workspace))
	assert.Equal(t, controllerutil.ContainsFinalizer(workspace, v1.WorkspaceFinalizer), false)

	// The claim is gone. List from the cache instead and it never was: the finalizer comes
	// off a Workspace whose name a live node still carries, and nothing runs again to notice.
	assert.NilError(t, store.Get(context.Background(), client.ObjectKey{Name: claimed.Name}, claimed))
	assert.Equal(t, claimed.GetSpecWorkspace(), "")
}

func TestReconcile(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}

	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 2)
	workspace.Status.Phase = v1.WorkspaceAbnormal
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	metav1.SetMetaDataLabel(&adminNode1.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode1.Spec.Workspace = pointer.String(workspace.Name)
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	adminNode2.Status.Unschedulable = true
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode2.Spec.Workspace = pointer.String(workspace.Name)

	testScheme := scheme.Scheme
	_ = corev1.AddToScheme(testScheme)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace, cluster, nodeFlavor).
		WithStatusSubresource(workspace).WithScheme(testScheme).Build()
	r := newMockWorkspaceReconciler(adminClient)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: workspace.Name,
		},
	}
	k8sClient := k8sfake.NewClientset(ns)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r.clientManager.AddOrReplace(clusterName, k8sClients)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: workspace.Name},
	}
	res, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)
	// Steady state -- 2 held against a spec of 2 -- so nothing below asked to come back, and
	// this is the safety net doing it anyway. Without it a Workspace that reaches its target
	// is never reconciled again until somebody else's event arrives.
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: resyncPeriod}, res)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.AvailableReplica, 1)
	assert.Equal(t, workspace.Status.AbnormalReplica, 1)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceRunning)
}

func TestScaleUpWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode1.Status.ClusterStatus.Phase = v1.NodeManaged
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()

	k8sNode1 := genMockK8sNode(adminNode1.Name, clusterName, nodeFlavor.Name, workspace.Name)
	k8sNode2 := genMockK8sNode(adminNode2.Name, clusterName, nodeFlavor.Name, workspace.Name)
	k8sClient := k8sfake.NewClientset(k8sNode1, k8sNode2)
	k8sClientFactory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r := newMockWorkspaceReconciler(adminClient)

	_, err := r.scaleUp(context.Background(), workspace, k8sClientFactory, 1)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), workspace.Name)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), "")
}

func TestScaleDownWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode1.Status.ClusterStatus.Phase = v1.NodeManaged
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	adminNode1.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode1.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode1.Spec.Workspace = pointer.String(workspace.Name)
	adminNode2.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode2.Spec.Workspace = pointer.String(workspace.Name)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()

	r := newMockWorkspaceReconciler(adminClient)
	_, err := r.scaleDown(context.Background(), workspace, 1)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), workspace.Name)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), "")
}

func TestWorkspaceNodesAction(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode1.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode1.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode1.Spec.Workspace = pointer.String(workspace.Name)
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	actions := map[string]string{
		adminNode1.Name: v1.NodeActionRemove,
		adminNode2.Name: v1.NodeActionAdd,
	}
	metav1.SetMetaDataAnnotation(&workspace.ObjectMeta,
		v1.WorkspaceNodesAction, string(jsonutils.MarshalSilently(actions)))

	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).
		WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), "")
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), workspace.Name)

	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, v1.GetWorkspaceNodesAction(workspace) != "", true)
	err = r.removeNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, v1.GetWorkspaceNodesAction(workspace) != "", false)
}

func TestSyncWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	metav1.SetMetaDataLabel(&adminNode1.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode1.Spec.Workspace = pointer.String(workspace.Name)
	adminNode1.Status.Resources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	}
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	adminNode2.Status.Resources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}
	adminNode2.Status.Unschedulable = true
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode2.Spec.Workspace = pointer.String(workspace.Name)

	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace, nodeFlavor).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	err := r.syncWorkspace(context.Background(), workspace)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.AvailableReplica, 1)
	assert.Equal(t, workspace.Status.AbnormalReplica, 1)
	assert.Equal(t, commonquantity.Equal(workspace.Status.AvailableResources, corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	}), true)

	// TotalResources = AvailableResources + AbnormalResources
	// AbnormalResources uses NodeFlavor's resources (CPU: 256, Memory: 1024Gi), not node's Status.Resources
	assert.Equal(t, commonquantity.Equal(workspace.Status.TotalResources, corev1.ResourceList{
		common.AmdGpu:         resource.MustParse("8"),
		corev1.ResourceCPU:    resource.MustParse("264"),
		corev1.ResourceMemory: resource.MustParse("1040Gi"),
	}), true)
}

func TestWorkspaceExpectations(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	nodeNames := sets.NewSetByKeys("node1", "node2")
	workspaceName := "workspace"
	r.setExpectations(workspaceName, nodeNames)
	assert.Equal(t, r.meetExpectations(workspaceName), false)
	r.observeNode(workspaceName, "node1")
	assert.Equal(t, r.meetExpectations(workspaceName), false)
	r.observeNode(workspaceName, "node2")
	assert.Equal(t, r.meetExpectations(workspaceName), true)

	workspaceName = "workspace2"
	nodeNames = sets.NewSetByKeys("node1", "node2")
	r.setExpectations(workspaceName, nodeNames)
	assert.Equal(t, r.meetExpectations(workspaceName), false)
	r.removeExpectations(workspaceName)
	assert.Equal(t, r.meetExpectations(workspaceName), true)
}

func TestResetWorkspaceStatus(t *testing.T) {
	workspace := genMockWorkspace("cluster", "nodeflavor", 1)
	workspace.Status.AvailableReplica = 1
	workspace.Status.AbnormalReplica = 1
	workspace.Status.TotalResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	}
	workspace.Status.AvailableResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}

	isChanged := resetWorkspaceStatus(workspace)
	assert.Equal(t, isChanged, true)
	assert.Equal(t, workspace.Status.AvailableReplica, 0)
	assert.Equal(t, workspace.Status.AbnormalReplica, 0)
	assert.Equal(t, len(workspace.Status.TotalResources), 0)
	assert.Equal(t, len(workspace.Status.AvailableResources), 0)
}

func TestSortNodesForScalingUp(t *testing.T) {
	tests := []struct {
		name   string
		n1     *corev1.Node
		n2     *corev1.Node
		result string
	}{
		{
			name: "sort by DeletionTimestamp",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "n2",
					DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()},
				},
			},
			result: "n1",
		},
		{
			name: "sort by taint",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{{
						Key: "test-taint",
					}},
				},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n2"},
			},
			result: "n2",
		},
		{
			name: "sort by unschedulable",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n2"},
			},
			result: "n2",
		},
		{
			name: "sort by taint and unschedulable",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
					Taints: []corev1.Taint{{
						Key: "test-taint",
					}},
				},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n2"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{{
						Key: "test-taint",
					}},
				},
			},
			result: "n2",
		},
		{
			name: "sort by name",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "n101",
				},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "n100",
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
			},
			result: "n100",
		},
		{
			name: "sort by name and taint",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "n101",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{{
						Key: "test-taint",
					}},
				},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "n100",
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{{
						Key: "test-taint",
					}},
				},
			},
			result: "n100",
		},
		{
			name: "sort by control-plane",
			n1: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "n1",
					Labels: map[string]string{
						v1.KubernetesControlPlane: "true",
					},
				},
			},
			n2: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n2"},
			},
			result: "n2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes := []*corev1.Node{test.n1, test.n2}
			sortNodesForScalingUp(nodes)
			assert.Equal(t, nodes[0].Name, test.result)
		})
	}
}

func TestBuildTargetList(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []*v1.Node
		target   string
		expected map[string]string
	}{
		{
			name:     "empty nodes",
			nodes:    []*v1.Node{},
			target:   "workspace1",
			expected: map[string]string{},
		},
		{
			name: "single node with target",
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
			},
			target:   "workspace1",
			expected: map[string]string{"node1": "workspace1"},
		},
		{
			name: "multiple nodes with empty target (unbind)",
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
			},
			target:   "",
			expected: map[string]string{"node1": "", "node2": ""},
		},
		{
			name: "multiple nodes with target (bind)",
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
			},
			target:   "workspace1",
			expected: map[string]string{"node1": "workspace1", "node2": "workspace1", "node3": "workspace1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTargetList(tt.nodes, tt.target)
			assert.Equal(t, len(result), len(tt.expected))
			for k, v := range tt.expected {
				assert.Equal(t, result[k].workspace, v)
				// Plain binding carries no migration; only a release for a migration does.
				assert.Assert(t, result[k].migration == nil)
			}
		})
	}
}

func TestUpdatePhase(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	workspace.Status.Phase = v1.WorkspaceRunning

	adminClient := fake.NewClientBuilder().WithObjects(workspace).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	// Test phase change
	err := r.updatePhase(context.Background(), workspace, v1.WorkspaceCreating)
	assert.NilError(t, err)

	updatedWorkspace := &v1.Workspace{}
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, updatedWorkspace)
	assert.NilError(t, err)
	assert.Equal(t, updatedWorkspace.Status.Phase, v1.WorkspaceCreating)
	assert.Assert(t, updatedWorkspace.Status.UpdateTime != nil)

	// Test no change when phase is same
	prevUpdateTime := updatedWorkspace.Status.UpdateTime
	err = r.updatePhase(context.Background(), updatedWorkspace, v1.WorkspaceCreating)
	assert.NilError(t, err)
	// UpdateTime should not change since phase didn't change
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, updatedWorkspace)
	assert.NilError(t, err)
	assert.Equal(t, updatedWorkspace.Status.UpdateTime.Time, prevUpdateTime.Time)
}

func newWorkspaceReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *WorkspaceReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Workspace{}).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	return &WorkspaceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
		clientManager:         mgr,
		expectations:          map[string]*nodeExpectations{},
		// The same client twice, as newMockWorkspaceReconciler does. In production apiReader is
		// mgr.GetAPIReader() and reads straight from the API server; the fake client has no
		// cache, so one object serves as both. Leaving it nil is what a nil dereference in
		// updateSingleNodeBinding looks like from here -- a panic in the fixture, not the bug.
		apiReader: cl,
		option:    &WorkspaceReconcilerOption{},
	}
}

func TestGuaranteeAndDeleteDataPlaneResourcesFull(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	ctx := context.Background()

	testifyassert.NoError(t, r.guaranteeDataPlaneResources(ctx, ws, cs))
	_, err := cs.CoreV1().Namespaces().Get(ctx, "ws1", metav1.GetOptions{})
	testifyassert.NoError(t, err)

	testifyassert.NoError(t, r.deleteDataPlaneResources(ctx, ws))
	_, err = cs.CoreV1().Namespaces().Get(ctx, "ws1", metav1.GetOptions{})
	testifyassert.Error(t, err)
}

func TestWorkspaceReconcileActive(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "ws1"}})
	testifyassert.NoError(t, err)
}

func TestWorkspaceProcessWorkspaceNoFlavor(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	_, err := r.processWorkspace(context.Background(), ws)
	testifyassert.NoError(t, err)
}

func TestGetClientSetOfDataplaneWorkspace(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newWorkspaceReconcilerFull(t, cs, testCluster("c1"))
	ctx := context.Background()

	got, err := r.getClientSetOfDataplane(ctx, "")
	testifyassert.NoError(t, err)
	testifyassert.Nil(t, got)

	got, err = r.getClientSetOfDataplane(ctx, "c1")
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, got)
}

func TestWorkspaceRelevantChangePredicate(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	p := r.relevantChangePredicate()

	old := &v1.Workspace{}
	upd := &v1.Workspace{}
	// Deletion timestamp set -> true.
	now := metav1.Now()
	upd.DeletionTimestamp = &now
	testifyassert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: upd}))

	// No change -> false.
	testifyassert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestWorkspaceGetClientSetOfDataplaneEmpty(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	cs, err := r.getClientSetOfDataplane(context.Background(), "")
	testifyassert.NoError(t, err)
	testifyassert.Nil(t, cs)
}

func TestWorkspaceGetClientSetOfDataplaneClusterMissing(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	_, err := r.getClientSetOfDataplane(context.Background(), "missing")
	testifyassert.Error(t, err)
}

func TestWorkspaceReconcileNotFound(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	testifyassert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestWorkspaceReconcileNoCluster(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	// No cluster -> clientSet nil -> the exit that returns before processWorkspace, so neither
	// pruneExpectations nor armExpectations is reached on it. Setting spec.cluster bumps the
	// generation and enqueues on its own, but that is the only other door in.
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "ws1"}})
	testifyassert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: resyncPeriod}, res)
}

// The same door, reached the other way, and the one that can strand an outstanding
// expectation: a Workspace naming a Cluster object that is not there. This controller does not
// watch Cluster, so nothing reports the Cluster coming back -- and the return is above
// processWorkspace, so the deadline on the expectation is never read. That is the reported
// wedge again with a different first step.
func TestWorkspaceReconcileComesBackWhenItsClusterIsGone(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "vanished"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	r.setExpectations("ws1", sets.NewSetByKeys("n1"))

	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "ws1"}})
	testifyassert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: resyncPeriod}, res)
	// Not settled and not expired: this exit is above pruneExpectations, so the entry is still
	// there. That is the point -- what is asserted is coming back at all, which is what lets
	// the pass after the Cluster returns be the one that expires it.
}

// Only a gap-filler. A caller that already asked to come back sooner keeps its own answer --
// armExpectations' 30s for an outstanding bind must not be stretched to a quarter hour.
func TestKeepAliveKeepsASoonerRequeue(t *testing.T) {
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: 30 * time.Second},
		keepAlive(ctrlruntime.Result{RequeueAfter: 30 * time.Second}))
	// An immediate rate-limited retry is an answer too. Read as RequeueAfter alone it looks
	// like no answer at all, and becomes a quarter-hour wait.
	assert.Equal(t, ctrlruntime.Result{Requeue: true}, keepAlive(ctrlruntime.Result{Requeue: true}))
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: resyncPeriod}, keepAlive(ctrlruntime.Result{}))
}

func TestWorkspaceDelete(t *testing.T) {
	now := metav1.Now()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:              "ws1",
		DeletionTimestamp: &now,
		Finalizers:        []string{v1.WorkspaceFinalizer},
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	// No nodes bound, no cluster -> deletes resources + removes finalizer.
	err := r.delete(context.Background(), ws)
	testifyassert.NoError(t, err)
}

func TestWorkspaceUpdatePhase(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	err := r.updatePhase(context.Background(), ws, v1.WorkspaceDeleting)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.WorkspaceDeleting, ws.Status.Phase)
	// No change -> no-op.
	testifyassert.NoError(t, r.updatePhase(context.Background(), ws, v1.WorkspaceDeleting))
}

func TestWorkspaceGuaranteeDataPlaneResources(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	mockScheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(mockScheme).Build())
	err = r.guaranteeDataPlaneResources(context.Background(), ws, cs)
	testifyassert.NoError(t, err)
	// Namespace should be created.
	_, err = cs.CoreV1().Namespaces().Get(context.Background(), "ws1", metav1.GetOptions{})
	testifyassert.NoError(t, err)
}

func TestWorkspaceDeleteDataPlaneResourcesNoCluster(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	// No cluster -> clientSet nil -> nil.
	err := r.deleteDataPlaneResources(context.Background(), ws)
	testifyassert.NoError(t, err)
}

func TestJudgeNodeBinding(t *testing.T) {
	cases := []struct {
		name             string
		node             *v1.Node
		target           string
		requester        string
		expectedVerdict  nodeBindVerdict
		expectedInReason string
	}{
		{name: "bind a free node", node: ownedNode("n", ""), target: "ws1", requester: "ws1",
			expectedVerdict: bindProceed},
		{name: "bind a node this workspace already holds", node: ownedNode("n", "ws1"),
			target: "ws1", requester: "ws1", expectedVerdict: bindSettled},
		{name: "bind a node another workspace holds", node: ownedNode("n", "ws2"),
			target: "ws1", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "already bound to ws2"},
		{name: "unbind a node this workspace holds", node: ownedNode("n", "ws1"), target: "",
			requester: "ws1", expectedVerdict: bindProceed},
		{name: "unbind a node that is already free", node: ownedNode("n", ""), target: "",
			requester: "ws1", expectedVerdict: bindSettled},
		// The one the three open-coded copies of this rule all missed: every one of them
		// keyed on a non-empty target, and an unbind's target is always empty.
		{name: "unbind a node another workspace holds", node: ownedNode("n", "ws2"), target: "",
			requester: "ws1", expectedVerdict: bindRefused, expectedInReason: "bound to ws2"},
		{name: "bind a node on somebody else's behalf", node: ownedNode("n", ""),
			target: "ws2", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "ws1 may not bind it to ws2"},
		{name: "bind a node that is being deleted", node: deletingNode(ownedNode("n", "")),
			target: "ws1", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "being deleted"},
		// The half of the managed check admission cannot do: the node passed admission and
		// then lost its managed state, which is the state the check exists to keep out.
		{name: "bind a node that is not managed", node: unmanagedNode(ownedNode("n", "")),
			target: "ws1", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "not managed"},
		// A release still has to go through for a node whose managed state is gone -- that is
		// exactly when the workspace holding it needs to let go.
		{name: "unbind a node that is not managed", node: unmanagedNode(ownedNode("n", "ws1")),
			target: "", requester: "ws1", expectedVerdict: bindProceed},
		{name: "unbind a node that is being deleted", node: deletingNode(ownedNode("n", "ws1")),
			target: "", requester: "ws1", expectedVerdict: bindProceed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verdict, reason := judgeNodeBinding(c.node, c.target, c.requester)
			assert.Equal(t, verdict, c.expectedVerdict)
			if c.expectedInReason != "" {
				assert.Assert(t, strings.Contains(reason, c.expectedInReason), reason)
			}
		})
	}
}

func deletingNode(node *v1.Node) *v1.Node {
	now := metav1.Now()
	node.DeletionTimestamp = &now
	node.Finalizers = []string{v1.NodeFinalizer}
	return node
}

func unmanagedNode(node *v1.Node) *v1.Node {
	node.Status.ClusterStatus.Phase = v1.NodeManagedFailed
	return node
}

// ownedNode returns a managed admin node bound to the given workspace, or free when it is "".
func ownedNode(name, workspaceId string) *v1.Node {
	node := genMockAdminNode(name, "cluster", genMockNodeFlavor())
	if workspaceId != "" {
		node.Spec.Workspace = pointer.String(workspaceId)
		metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, workspaceId)
	}
	return node
}

func storedNode(t *testing.T, cli client.Client, name string) *v1.Node {
	t.Helper()
	node := &v1.Node{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: name}, node))
	return node
}

// A refusal has to reach the caller as an error. The nodes-action annotation is cleared only
// once the whole batch succeeds, and the mutating webhook has already applied the matching
// Spec.Replica change, so a swallowed refusal strands that replica and quietly turns the
// refused bind into an automatic scale-up onto a different node.
func TestUpdateSingleNodeBindingRefusesToTakeANodeFromItsOwner(t *testing.T) {
	node := ownedNode("node1", "ws2")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

func TestUpdateSingleNodeBindingRefusesAnUnbindFromAnyoneButTheOwner(t *testing.T) {
	node := ownedNode("node1", "ws2")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: ""})
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "not the workspace asking")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

func TestUpdateSingleNodeBindingSettlesWhenTheNodeIsAlreadyWhereItShouldBe(t *testing.T) {
	node := ownedNode("node1", "ws1")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.NilError(t, err)
	assert.Equal(t, updated, false)
}

// A node that is gone is an answer, not a failure. Reported as an error it would fail the
// whole batch and buy a rate-limited requeue every round for a request nothing can satisfy.
func TestUpdateSingleNodeBindingTreatsAVanishedNodeAsDone(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", ownedNode("node1", ""), nodeBinding{workspace: "ws1"})
	assert.NilError(t, err)
	assert.Equal(t, updated, false)
}

// Binds only. delete() collects deleting nodes too, and a refused unbind would let the
// Workspace finalizer come off with spec.workspace still naming it -- a node no later bind
// can rescue, because only the owner may release it and the owner is gone.
func TestUpdateSingleNodeBindingRefusesToBindADeletingNode(t *testing.T) {
	node := ownedNode("node1", "")
	node.Finalizers = []string{v1.NodeFinalizer}
	node.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "being deleted")
}

func TestUpdateSingleNodeBindingReleasesADeletingNode(t *testing.T) {
	node := ownedNode("node1", "ws1")
	node.Finalizers = []string{v1.NodeFinalizer}
	node.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: ""})
	assert.NilError(t, err)
	assert.Equal(t, updated, true)
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "")
}

// Losing the optimistic lock means somebody wrote first: read again, judge again. Here the
// second look still says the node is free, so the bind goes through on the retry.
func TestUpdateSingleNodeBindingRetriesAfterAConflict(t *testing.T) {
	node := ownedNode("node1", "")
	conflicts := 0
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if conflicts == 0 {
					conflicts++
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.NilError(t, err)
	assert.Equal(t, updated, true)
	assert.Equal(t, conflicts, 1)
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws1")
}

// And when the second look says the node changed hands, the retry refuses instead of
// overwriting the workspace that won the race.
func TestUpdateSingleNodeBindingRejudgesAfterAConflict(t *testing.T) {
	node := ownedNode("node1", "")
	conflicts := 0
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if conflicts == 0 {
					conflicts++
					// The writer that beat us lands its bind before returning the conflict.
					winner := &v1.Node{}
					if err := c.Get(ctx, client.ObjectKey{Name: obj.GetName()}, winner); err != nil {
						return err
					}
					winner.Spec.Workspace = pointer.String("ws2")
					if err := c.Update(ctx, winner); err != nil {
						return err
					}
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

// The real typed client decodes into the object it is handed without zeroing it first, and
// Spec.Workspace is an omitempty pointer: for a node that is not bound the field is absent on
// the wire, so re-reading through the object a previous attempt already wrote to would leave
// that attempt's own value in place and the retry would judge its own writing as settled. The
// fake client zeroes, which is why this needs an interceptor to reproduce at all.
func TestUpdateSingleNodeBindingReadsIntoAFreshObject(t *testing.T) {
	node := ownedNode("node1", "")
	conflicts := 0
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				stored := &v1.Node{}
				if err := c.Get(ctx, key, stored, opts...); err != nil {
					return err
				}
				target, ok := obj.(*v1.Node)
				if !ok {
					return nil
				}
				// Copy field by field, leaving an absent Spec.Workspace untouched -- what a
				// real decode does, and what the fake client does not.
				target.ObjectMeta = stored.ObjectMeta
				target.Status = stored.Status
				target.Spec.NodeFlavor = stored.Spec.NodeFlavor
				target.Spec.Cluster = stored.Spec.Cluster
				target.Spec.Port = stored.Spec.Port
				if stored.Spec.Workspace != nil {
					target.Spec.Workspace = stored.Spec.Workspace
				}
				return nil
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if conflicts == 0 {
					conflicts++
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.NilError(t, err)
	// Reading into the object the first attempt mutated would carry "ws1" into the retry's
	// judgement, which would then answer bindSettled and report nothing written.
	assert.Equal(t, updated, true)
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws1")
}

// An expectation waits for the workspace label to make the round trip through the data plane,
// and handleNodeEvent credits it only on a *change* of that label. A node whose label already
// reads the target has nothing left to wait for, and waiting anyway wedges the workspace:
// every later reconcile returns early on meetExpectations and it never scales or syncs again.
func TestUpdateNodesBindingSettlesWhenTheLabelAlreadyReadsTheTarget(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "")
	metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	nodes := []*v1.Node{node}
	err := r.updateNodesBinding(context.Background(), workspace, nodes,
		buildTargetList(nodes, workspace.Name))
	assert.NilError(t, err)
	assert.Equal(t, r.meetExpectations(workspace.Name), true)
}

func TestUpdateNodesBindingWaitsWhenTheLabelStillHasToArrive(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "")
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	nodes := []*v1.Node{node}
	err := r.updateNodesBinding(context.Background(), workspace, nodes,
		buildTargetList(nodes, workspace.Name))
	assert.NilError(t, err)
	assert.Equal(t, r.meetExpectations(workspace.Name), false)
}

func TestObserveNodeForAllSettlesEveryWaitingWorkspace(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1", "node2"))
	r.setExpectations("ws2", sets.NewSetByKeys("node1"))
	r.setExpectations("ws3", sets.NewSetByKeys("node3"))

	settled := sets.NewSetByKeys(r.observeNodeForAll("node1")...)
	assert.Equal(t, settled.Len(), 2)
	assert.Equal(t, settled.Has("ws1"), true)
	assert.Equal(t, settled.Has("ws2"), true)
	assert.Equal(t, r.meetExpectations("ws1"), false)
	assert.Equal(t, r.meetExpectations("ws2"), true)
	// The emptied entry is dropped, not left behind: this map is walked on every admin Node
	// event, and NodeK8sReconciler writes those nodes every few seconds.
	_, stillThere := r.expectations["ws2"]
	assert.Equal(t, stillThere, false)
}

// The wedge crediting by the event's workspace id alone leaves behind: the bind landed, so
// the workspace is waiting, and then the node is deleted. Its last event names nobody.
func TestHandleNodeEventSettlesAWaitingWorkspaceWhenTheNodeIsDeleted(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))

	q := resWorkQueue()
	defer q.ShutDown()
	r.handleNodeEvent().Delete(context.Background(),
		event.DeleteEvent{Object: ownedNode("node1", "")}, q)
	assert.Equal(t, r.meetExpectations("ws1"), true)
	// Settling alone is not enough -- the workspace also has to be re-queued, or nothing
	// looks at it again until the next unrelated event.
	assert.Equal(t, q.Len(), 1)
	item, _ := q.Get()
	assert.Equal(t, item.Name, "ws1")
}

func TestReservedNodesCoversOtherWorkspacesPendingClaims(t *testing.T) {
	mine := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(mine, map[string]string{"node1": v1.NodeActionAdd})
	other := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(other, map[string]string{"node2": v1.NodeActionAdd, "node3": v1.NodeActionRemove})
	// A workspace under deletion never processes its annotation -- Reconcile hands it to
	// delete() -- so its claims are abandoned, and reserving them would take node4 out of
	// everyone's reach for good.
	dying := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(dying, map[string]string{"node4": v1.NodeActionAdd})
	dying.Finalizers = []string{v1.WorkspaceFinalizer}
	dying.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}

	cli := fake.NewClientBuilder().WithObjects(mine, other, dying).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	reserved, err := r.reservedNodes(context.Background(), mine.Name)
	assert.NilError(t, err)
	assert.Equal(t, reserved.Has("node2"), true)
	// A workspace's own claim is not reserved against itself, a remove releases rather than
	// claims, and an abandoned claim is not a claim.
	assert.Equal(t, reserved.Has("node1"), false)
	assert.Equal(t, reserved.Has("node3"), false)
	assert.Equal(t, reserved.Has("node4"), false)
}

// Automatic scaling must not take a node a user explicitly asked for and admission accepted.
func TestGetNodesForScalingUpLeavesAReservedNodeAlone(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	free := genMockAdminNode("node1", "cluster", nodeFlavor)
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	claimant := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	setNodesAction(claimant, map[string]string{"node1": v1.NodeActionAdd})

	cli := fake.NewClientBuilder().WithObjects(free, workspace, claimant).
		WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "cluster",
		k8sfake.NewClientset(genMockK8sNode("node1", "cluster", nodeFlavor.Name, "")))

	nodes, err := r.getNodesForScalingUp(context.Background(), workspace, k8sClients, 1)
	assert.NilError(t, err)
	assert.Equal(t, len(nodes), 0)
}

// admissionRules is what admission does to a Workspace write on its way to the API server, in
// the one respect this controller has to live within: a write that moves Spec.Replica and the
// nodes-action annotation together is turned away unless it is a withdrawal, and a withdrawal
// is only one that lands Spec.Replica on exactly the value commonnodes.WithdrawnReplica gives.
// Every write carrying that annotation goes through it -- the webhooks are registered on
// workspaces UPDATE with no object selector, so this controller's own patches are admitted
// like anybody else's.
//
// It is here because the fake client runs no webhooks, and without it these tests pass on a
// withdrawal that never once reaches a real cluster: the whole path is one Patch call, and
// getting that call rejected is not something any assertion about the stored object can see.
// The rule is asserted from the other side too, against the webhooks themselves and in the
// order the API server runs them, in TestWorkspaceAdmitWithdrawalEndToEnd.
func admissionRules() interceptor.Funcs {
	return interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
			patch client.Patch, opts ...client.PatchOption) error {
			workspace, ok := obj.(*v1.Workspace)
			if !ok {
				return c.Patch(ctx, obj, patch, opts...)
			}
			stored := &v1.Workspace{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(workspace), stored); err != nil {
				return err
			}
			if v1.GetWorkspaceNodesAction(stored) != v1.GetWorkspaceNodesAction(workspace) &&
				stored.Spec.Replica != workspace.Spec.Replica &&
				workspace.Spec.Replica != commonnodes.WithdrawnReplica(stored.Spec.Replica,
					parseNodesAction(stored), parseNodesAction(workspace)) {
				return apierrors.NewBadRequest("the operation of specifying nodes and the " +
					"modification of workspace replica cannot be performed simultaneously")
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}
}

func storedWorkspace(t *testing.T, cli client.Client, name string) *v1.Workspace {
	t.Helper()
	workspace := &v1.Workspace{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: name}, workspace))
	return workspace
}

// End to end: a bind that can never succeed is withdrawn rather than retried forever. The
// entry leaves the annotation and the reason is written where whoever asked can read it.
//
// Spec.Replica comes back down in the same patch. The mutating webhook counted this add in
// when it accepted the request, and a count left standing for a node that will never be bound
// is a scale-up onto some other machine -- not what was asked for.
func TestProcessNodesActionWithdrawsARefusedBind(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "ws-other")
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "ws-other")

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	// The only add in the request, so the whole charge comes back: 1 -> 0.
	assert.Equal(t, stored.Spec.Replica, 0)
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "already bound to ws-other"))
}

// A node that has gone away between the request being admitted and this controller reaching
// it is refused like any other entry it cannot carry out. Skipping it instead would drop the
// annotation while leaving the replica the webhook added for it in place, and the next sync
// would spend that replica on whatever machine happened to be free.
func TestProcessNodesActionWithdrawsAVanishedNode(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(workspace, map[string]string{"node1": v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, isUpdated, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	// Nothing is pending after a withdrawal, and saying otherwise waits for a requeue that
	// an annotation merely going away does not produce.
	assert.Equal(t, isUpdated, false)

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "no longer exists"))
}

// A refusal in a batch takes only its own entry down with it: the surviving entry is still
// applied, and the annotation is rewritten to exactly what is left, which is what the webhook
// reads the withdrawal off and what stops the next reconcile accounting for it twice.
func TestProcessNodesActionWithdrawsOnlyTheRefusedEntry(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 2)
	taken := ownedNode("node1", "ws-other")
	free := ownedNode("node2", "")
	setNodesAction(workspace, map[string]string{
		taken.Name: v1.NodeActionAdd,
		free.Name:  v1.NodeActionAdd,
	})
	cli := fake.NewClientBuilder().WithObjects(taken, free, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, storedNode(t, cli, free.Name).GetSpecWorkspace(), workspace.Name)

	stored := storedWorkspace(t, cli, workspace.Name)
	// One of the two adds withdrawn, so one replica back: 2 -> 1.
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), `{"node2":"add"}`)
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "node1: "))

	// Second pass: node2 is bound now, so the whole request is done. It leaves as a plain
	// clear that touches no reason -- nothing was withdrawn this time, and a reason written
	// again beside a shrinking request is a second withdrawal, and a second refund with it.
	reason := v1.GetAnnotation(stored, v1.WorkspaceNodesActionError)
	_, _, err = r.processNodesAction(context.Background(), stored)
	assert.NilError(t, err)
	stored = storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), reason)
}

// The refund happens exactly once, whatever the contention. Annotation and replica move in a
// single optimistically-locked patch, so a competing write does not take half of it: the patch
// is rejected whole, nothing is stored, and the reconcile that follows recomputes the refusal
// against the request that actually exists rather than replaying the one it had in hand.
//
// The failure this rules out is a workspace losing a replica per requeue. It cannot be seen by
// looking at one pass -- both a correct and a double-counting implementation write 1 the first
// time they get through -- so the test has to lose a patch and then come back.
func TestDropRefusedActionsRefundsExactlyOnceAcrossAConflict(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 2)
	taken := ownedNode("node1", "ws-other")
	free := ownedNode("node2", "")
	setNodesAction(workspace, map[string]string{
		taken.Name: v1.NodeActionAdd,
		free.Name:  v1.NodeActionAdd,
	})
	rules := admissionRules()
	admit := rules.Patch
	conflicts := 1
	rules.Patch = func(ctx context.Context, c client.WithWatch, obj client.Object,
		patch client.Patch, opts ...client.PatchOption) error {
		if _, ok := obj.(*v1.Workspace); ok && conflicts > 0 {
			conflicts--
			return apierrors.NewConflict(schema.GroupResource{Resource: "workspaces"},
				obj.GetName(), errors.New("somebody else got there first"))
		}
		return admit(ctx, c, obj, patch, opts...)
	}
	cli := fake.NewClientBuilder().WithObjects(taken, free, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(rules).Build()
	r := newMockWorkspaceReconciler(cli)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.Assert(t, apierrors.IsConflict(err))
	// Not half applied: the request is whole and the replica is untouched.
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Spec.Replica, 2)
	assert.Equal(t, v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "")
	assert.Assert(t, strings.Contains(v1.GetWorkspaceNodesAction(stored), "node1"))

	// The requeue, off the object as it now stands.
	_, _, err = r.processNodesAction(context.Background(), stored)
	assert.NilError(t, err)
	stored = storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), `{"node2":"add"}`)

	// And once the request is done there is nothing left to give back, however many times it
	// comes round again.
	_, _, err = r.processNodesAction(context.Background(), stored)
	assert.NilError(t, err)
	assert.Equal(t, storedWorkspace(t, cli, workspace.Name).Spec.Replica, 1)
}

// A refused remove is withdrawn the same way as a refused add. What differs is the accounting:
// see commonnodes.WithdrawnReplica for why a remove gets no replica back.
func TestProcessNodesActionWithdrawsARefusedRemove(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "ws-other")
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionRemove})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "ws-other")
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "which is not the workspace asking"))
}

// An entry that is simply already true is not a refusal, and must not be reported as one: the
// mutating webhook skipped it when it counted, so there is nothing charged to give back, and a
// reason annotation appearing alongside the shrinking request is exactly what admission reads
// as a withdrawal -- one that would then be expected to carry a refund it does not owe.
func TestProcessNodesActionLeavesReplicaAloneForASettledEntry(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "")
	node.Spec.Workspace = pointer.String(workspace.Name)
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "")
}

func setNodesAction(workspace *v1.Workspace, actions map[string]string) {
	metav1.SetMetaDataAnnotation(&workspace.ObjectMeta,
		v1.WorkspaceNodesAction, string(jsonutils.MarshalSilently(actions)))
}

// The contention this whole change exists for: two workspaces reaching for the same free node
// at the same time. Both pass admission, because at the moment each is admitted the node is
// genuinely unowned; both read it and both find it free. What separates them is the optimistic
// lock -- the patch carries the resourceVersion the judgement was made against, so the second
// writer's patch is rejected rather than applied, and its retry re-reads and refuses.
//
// The interceptor makes the interleaving deterministic: the competing write lands after this
// caller has read and judged, but before its patch.
func TestUpdateSingleNodeBindingLetsOnlyOneWorkspaceWin(t *testing.T) {
	node := ownedNode("node1", "")
	raced := false
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if !raced {
					raced = true
					winner := &v1.Node{}
					if err := c.Get(ctx, client.ObjectKey{Name: obj.GetName()}, winner); err != nil {
						return err
					}
					winner.Spec.Workspace = pointer.String("ws2")
					if err := c.Update(ctx, winner); err != nil {
						return err
					}
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	// Without the lock on the patch, ws1 would overwrite ws2 here and both workspaces would
	// believe they own node1.
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

// A node moving from one workspace to the next passes through the empty label on the way, and
// that intermediate event must not credit the workspace still waiting for the label to arrive.
// Credit it early and syncWorkspace counts a node the workspace does not yet hold as missing,
// binds a spare machine, and hands it back on the following round -- real churn on hardware.
func TestHandleNodeEventDoesNotSettleAWorkspaceTheLabelHasNotReached(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))
	r.setExpectations("ws2", sets.NewSetByKeys("node1"))

	oldNode := ownedNode("node1", "ws1")
	newNode := ownedNode("node1", "")
	newNode.Spec.Workspace = pointer.String("ws2")

	q := resWorkQueue()
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)

	assert.Equal(t, r.meetExpectations("ws1"), true, "the workspace the node left is settled")
	assert.Equal(t, r.meetExpectations("ws2"), false, "the incoming workspace is still waiting")

	// And it is settled by its own label arriving.
	arrived := ownedNode("node1", "ws2")
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: newNode, ObjectNew: arrived}, q)
	assert.Equal(t, r.meetExpectations("ws2"), true)
}

// The other side of the same rule. A node unmanaged in the window between a bind writing the
// claim and the label making the round trip loses the claim with no label to lose alongside it,
// so the claim going is the only event its owner will ever get. Miss it and the workspace waits
// on that node for good: meetExpectations never comes true, and processWorkspace returns at the
// top of every round without scaling or syncing status again.
func TestHandleNodeEventSettlesAClaimReleasedBeforeTheLabelArrived(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))

	oldNode := ownedNode("node1", "")
	oldNode.Spec.Workspace = pointer.String("ws1")
	newNode := ownedNode("node1", "")

	q := resWorkQueue()
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)

	assert.Equal(t, r.meetExpectations("ws1"), true)
	assert.Equal(t, q.Len(), 1, "and the workspace is woken to notice the node is gone")
}

// The guard that keeps the branch above from undoing the one before it. An ordinary unbind
// drops the claim first and the label after, and it is the label that settles: the workspace
// counts what it holds by label, so crediting the claim would tell it the node was gone while
// it is still counting it, and it would bind a replacement it does not need.
func TestHandleNodeEventDoesNotSettleAClaimThatStillHasItsLabel(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))

	oldNode := ownedNode("node1", "ws1")
	newNode := ownedNode("node1", "ws1")
	newNode.Spec.Workspace = nil

	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, resWorkQueue())

	assert.Equal(t, r.meetExpectations("ws1"), false)

	// The label following is what settles it.
	gone := ownedNode("node1", "")
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: newNode, ObjectNew: gone}, resWorkQueue())
	assert.Equal(t, r.meetExpectations("ws1"), true)
}

// Reading the other workspaces' claims is what keeps two of them off the same free node, so a
// read that fails has to stop the round rather than scale up against a partial answer.
func TestGetNodesForScalingUpStopsWhenTheClaimsCannotBeRead(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	free := genMockAdminNode("node1", "cluster", nodeFlavor)
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	cli := fake.NewClientBuilder().WithObjects(free, workspace, nodeFlavor).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList,
				opts ...client.ListOption) error {
				if _, ok := list.(*v1.WorkspaceList); ok {
					return apierrors.NewInternalError(errors.New("boom"))
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "cluster",
		k8sfake.NewClientset(genMockK8sNode("node1", "cluster", nodeFlavor.Name, "")))

	_, err := r.getNodesForScalingUp(context.Background(), workspace, k8sClients, 1)
	assert.ErrorContains(t, err, "boom")
}

func bindingCount(t *testing.T, action, outcome string) float64 {
	t.Helper()
	return testutil.ToFloat64(rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, outcome))
}

// The outcome labels are the only thing that separates "binding is busy" from "binding is
// being turned down" on a dashboard, and neither of the two new ones is observable any other
// way -- a refusal and an exhausted retry both surface to the caller as a plain error.
func TestUpdateSingleNodeBindingCountsWhatItTurnedDown(t *testing.T) {
	node := ownedNode("node1", "ws2")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	before := bindingCount(t, "bind", "refused")
	_, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, bindingCount(t, "bind", "refused"), before+1)
}

// Counted once per exhausted call, not once per attempt: a counter that climbs with the retry
// budget measures the budget rather than the contention.
func TestUpdateSingleNodeBindingCountsAnExhaustedRetryOnce(t *testing.T) {
	node := ownedNode("node1", "")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				return apierrors.NewConflict(schema.GroupResource{Resource: "nodes"},
					obj.GetName(), errors.New("conflict"))
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	before := bindingCount(t, "bind", "conflict")
	_, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.Assert(t, apierrors.IsConflict(err))
	assert.Equal(t, bindingCount(t, "bind", "conflict"), before+1)
}

// A read that stops working -- throttling, an RBAC change -- has to look like binding failing
// rather than like binding traffic going to zero.
func TestUpdateSingleNodeBindingCountsAReadThatFails(t *testing.T) {
	node := ownedNode("node1", "")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				return apierrors.NewInternalError(errors.New("throttled"))
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	before := bindingCount(t, "bind", "failed")
	_, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, nodeBinding{workspace: "ws1"})
	assert.ErrorContains(t, err, "throttled")
	assert.Equal(t, bindingCount(t, "bind", "failed"), before+1)
}

// The one place outside updateSingleNodeBinding that clears spec.workspace. A bind writes spec
// first and waits for the label to make the round trip, so a node unmanaged inside that window
// carries the claim and not the label -- and tying the release to the label would strand it
// for good, since only the owner may release and the owner has been told the node is gone.
func TestCleanupNodeAfterUnmanageReleasesAClaimThatHasNoLabelYet(t *testing.T) {
	node := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	node.Spec.Workspace = pointer.String("ws1")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockNodeReconciler(cli)

	assert.NilError(t, r.cleanupNodeAfterUnmanage(context.Background(), node))
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "")
}

// A request whose entries are all withdrawn leaves nothing pending, and processNodesAction
// says so by returning false. That is not a stall: false is what lets processWorkspace carry
// on into syncWorkspace and the scaling decision in the same pass, against the workspace this
// very call refunded. Nothing waits for an event, because nothing is waiting at all.
//
// The alternative, returning true, is what would stall it -- true means "stop here, an
// expectation will bring you back", and a withdrawal registers no expectation and changes no
// field either predicate on this controller watches.
func TestProcessWorkspaceScalesInTheSamePassAsAWithdrawal(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 3)
	// One node genuinely held, and a request naming a node that no longer exists.
	held := ownedNode("node1", workspace.Name)
	held.Labels[v1.NodeFlavorIdLabel] = nodeFlavor.Name
	setNodesAction(workspace, map[string]string{"vanished": v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(held, workspace, nodeFlavor).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)
	r.clientManager.AddOrReplace("cluster", commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "cluster", k8sfake.NewClientset()))

	result, err := r.processWorkspace(context.Background(), workspace)
	assert.NilError(t, err)

	stored := storedWorkspace(t, cli, workspace.Name)
	// The withdrawal itself: the add is gone and the replica it was counted into came back.
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, stored.Spec.Replica, 2)
	// syncWorkspace ran after it, in this same call -- the status was recomputed from the one
	// node actually held rather than left at its zero value.
	assert.Equal(t, stored.Status.AvailableReplica, 1)
	// And the scaling decision ran too, against the refunded target: 1 held against 2 wanted
	// is a scale-up, and with no free node in the fleet it asks to be tried again. Reaching
	// this at all is the point -- it is the work the request was standing in front of.
	assert.Equal(t, result.RequeueAfter, r.option.nodeWait)
}

// A node's WorkspaceIdLabel is a mirror of spec.workspace and lags it by a whole round trip, so
// for the length of that lag a workspace is labelled on nodes it no longer holds. syncWorkspace
// is what turns "which nodes" into CurrentReplica, and CurrentReplica is what the scaling switch
// subtracts the target from to get a count of machines to release -- while the candidates for
// that count come from GetIdleNodesOfWorkspace, which answers on the claim.
//
// Counting one way and choosing the other is not a rounding error, it is an over-release with a
// specific victim: the arithmetic asks for one node more than the workspace holds, the candidate
// list is short by exactly that one, and the shortfall is made up out of a machine the workspace
// genuinely holds and is using. Deterministically, not as a race -- the stale entry is the one
// the filter drops, so the good one is always what is left to take.
func TestSyncWorkspaceCountsTheClaimNotTheLabel(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	// Held outright.
	held := ownedNode("held", workspace.Name)
	held.Labels[v1.NodeFlavorIdLabel] = nodeFlavor.Name
	// Still labelled for this workspace, but the claim has already moved to another one.
	stale := ownedNode("stale", workspace.Name)
	stale.Labels[v1.NodeFlavorIdLabel] = nodeFlavor.Name
	stale.Spec.Workspace = pointer.String("ws-other")
	cli := fake.NewClientBuilder().WithObjects(held, stale, workspace, nodeFlavor).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	assert.NilError(t, r.syncWorkspace(context.Background(), workspace))

	stored := storedWorkspace(t, cli, workspace.Name)
	// One, not two. Two would make CurrentReplica exceed the target and send the workspace
	// into scale-down with nothing spare to give.
	assert.Equal(t, stored.Status.AvailableReplica, 1)
	assert.Equal(t, stored.CurrentReplica(), 1)

	// Which is the same number GetIdleNodesOfWorkspace answers with. That the two agree is the
	// whole property; asserting the count alone would not notice them drifting apart again.
	idle, err := commonnodes.GetIdleNodesOfWorkspace(context.Background(), cli, workspace.Name)
	assert.NilError(t, err)
	assert.Equal(t, len(idle), stored.CurrentReplica())
	assert.Equal(t, idle[0].Name, "held")
}

// The pass that arms an expectation is the only one that can schedule the visit which reads
// its deadline, and until this it did not: scaleUp returns an empty Result once the bind
// succeeds, the claim it wrote re-enqueues nothing -- handleNodeEvent wants the label, which
// is the thing being waited for -- the Workspace predicates reject a generation-equal resync,
// and the manager sets no SyncPeriod. So the deadline repaired the second and later visits to
// the gate and not the first, which is the only visit a lost label round trip ever gets.
//
// Driven through a real scaleUp rather than a hand-installed entry: the wedge is a property of
// the pass that creates an expectation, and an entry put there by the test proves nothing
// about it. The bind assertions are what keep this honest -- with no node taken, scaleUp
// requeues on its own at :772 and the requeue below would be true for the wrong reason.
func TestProcessWorkspaceArmsTheDeadlineItJustCreated(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	free := genMockAdminNode("node1", "cluster", nodeFlavor)
	free.Status.ClusterStatus.Phase = v1.NodeManaged
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	cli := fake.NewClientBuilder().WithObjects(free, workspace, nodeFlavor).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)
	r.clientManager.AddOrReplace("cluster", commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "cluster", k8sfake.NewClientset(
			genMockK8sNode(free.Name, "cluster", nodeFlavor.Name, ""))))

	result, err := r.processWorkspace(context.Background(), workspace)
	assert.NilError(t, err)

	// A node was actually taken, and its label has not made the round trip yet.
	assert.Equal(t, storedNode(t, cli, free.Name).GetSpecWorkspace(), workspace.Name)
	assert.Equal(t, r.meetExpectations(workspace.Name), false,
		"the bind left nothing outstanding, so this says nothing about arming one")
	// And the pass that created it left something to bring the workspace back.
	assert.Equal(t, result.RequeueAfter, r.option.nodeWait,
		"the reconcile that armed the expectation scheduled no way back to the gate")
}

// armExpectations only fills a gap. A reconcile already coming back keeps the time it asked
// for -- every requeue in this controller is shorter than expectationTimeout -- and a
// workspace waiting on nothing is not given a wake-up it has no use for.
func TestArmExpectationsOnlyFillsTheGap(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	// Whole-value, not just RequeueAfter: Requeue is the field a caller could be using to ask
	// for an immediate rate-limited retry, and it is the one this must not trample.
	assert.Equal(t, ctrlruntime.Result{}, r.armExpectations("ws-a", ctrlruntime.Result{}),
		"nothing outstanding, so nothing to come back for")

	r.setExpectations("ws-a", sets.NewSetByKeys("n1"))
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: time.Second},
		r.armExpectations("ws-a", ctrlruntime.Result{RequeueAfter: time.Second}),
		"a sooner requeue was already asked for")
	assert.Equal(t, ctrlruntime.Result{Requeue: true},
		r.armExpectations("ws-a", ctrlruntime.Result{Requeue: true}),
		"an immediate retry is an answer too, and must not become a wait")
	assert.Equal(t, ctrlruntime.Result{RequeueAfter: r.option.nodeWait},
		r.armExpectations("ws-a", ctrlruntime.Result{}))
}

// A RequeueAfter of zero is not a short wait, it is no requeue at all -- the exact behaviour
// the arming exists to remove. newWorkspaceReconcilerFull builds its option with a zero
// nodeWait, and a guard that disarms itself under a zero option is not a guard.
func TestExpectationRetryNeverCollapsesToZero(t *testing.T) {
	r := newWorkspaceReconcilerFull(t, k8sfake.NewClientset())
	assert.Equal(t, r.option.nodeWait, time.Duration(0), "this fixture is why the fallback exists")

	r.setExpectations("ws-a", sets.NewSetByKeys("n1"))
	assert.Equal(t, r.armExpectations("ws-a", ctrlruntime.Result{}).RequeueAfter, defaultExpectationRetry)
}

// Abandoning a wait is a data plane fault the workspace has just stopped reporting: the
// annotation was cleared on the pass after the bind, so nothing on the object records that the
// binding was given up on. The counter is the only thing left to alert on, and it is counted
// per node because that is the granularity a deadline is kept and dropped at.
func TestAbandonedExpectationIsCounted(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	before := testutil.ToFloat64(rmmetrics.WorkspaceExpectationExpiredTotal)

	r.setExpectations("ws-a", sets.NewSetByKeys("n1", "n2", "n3"))
	expire(&r, "ws-a", "n1")
	expire(&r, "ws-a", "n2")
	r.pruneExpectations("ws-a")

	assert.Equal(t, testutil.ToFloat64(rmmetrics.WorkspaceExpectationExpiredTotal)-before, float64(2),
		"abandonment is counted per node, as it is pruned per node")
	assert.Equal(t, r.meetExpectations("ws-a"), false, "n3 was still worth waiting on")
}

// markMigrating puts a node in the state the source workspace leaves it in: released, and
// carrying the target that is expected to pick it up.
func markMigrating(node *v1.Node, from, target string) {
	node.Spec.Workspace = nil
	delete(node.Labels, v1.WorkspaceIdLabel)
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      from,
		Target:    target,
		StartTime: &metav1.Time{Time: time.Now().UTC()},
	})
}

func TestIsNodeEligibleForScalingUp(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	option := *r.option
	option.migrateTimeout = time.Hour
	r.option = &option

	cases := []struct {
		name  string
		mutil func(node *v1.Node)
		want  bool
	}{
		{name: "free node of a matching flavor", want: true},
		{name: "machine not ready", want: false, mutil: func(node *v1.Node) {
			node.Status.MachineStatus.Phase = v1.NodeSSHFailed
		}},
		{name: "not managed", want: false, mutil: func(node *v1.Node) {
			node.Status.ClusterStatus.Phase = v1.NodeManaging
		}},
		{name: "bound by spec", want: false, mutil: func(node *v1.Node) {
			node.Spec.Workspace = ptr.To("other")
		}},
		{name: "bound by label", want: false, mutil: func(node *v1.Node) {
			metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, "other")
		}},
		{name: "flavor mismatch", want: false, mutil: func(node *v1.Node) {
			metav1.SetMetaDataLabel(&node.ObjectMeta, v1.NodeFlavorIdLabel, "another-flavor")
		}},
		{name: "reserved by a migration to another workspace", want: false, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", "ws-b")
		}},
		// Not even for the target. Taking the node here would finish the crossing without the
		// replica the migration was supposed to add, and the source has already given one up.
		{name: "reserved by a migration to this workspace", want: false, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", workspace.Name)
		}},
		{name: "unreadable migration payload does not park the node", want: true, mutil: func(node *v1.Node) {
			metav1.SetMetaDataAnnotation(&node.ObjectMeta, v1.NodeMigrateAnnotation, "{")
		}},
		// The source workspace can be deleted mid-migration, and a node can leave the
		// cluster and come back still carrying the annotation. Nothing would then clear it,
		// so a reservation is only honoured while it is young enough to still be real.
		{name: "a reservation older than the timeout is ignored", want: true, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", "ws-b")
			v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
				From:      "ws-a",
				Target:    "ws-b",
				StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * time.Hour)},
			})
		}},
		{name: "a reservation with no start time cannot be aged and is ignored", want: true, mutil: func(node *v1.Node) {
			v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{From: "ws-a", Target: "ws-b"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := genMockAdminNode("node1", clusterName, nodeFlavor)
			node.Status.ClusterStatus.Phase = v1.NodeManaged
			if tc.mutil != nil {
				tc.mutil(node)
			}
			assert.Equal(t, isNodeEligibleForScalingUp(node, workspace), tc.want)
		})
	}
}

// A migration leaves the node unbound and of a matching flavor for as long as it takes the
// target to claim it, which is everything a workspace short of a replica looks for. Nobody
// takes it that way -- not a bystander, and not the target either: arriving through the
// scaling loop means arriving against a replica the target already wanted, so the one the
// migration was supposed to add never is, and the source has already given one up.
func TestScaleUpLeavesAMigratingNodeToItsHandover(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	target := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	bystander := genMockWorkspace(clusterName, nodeFlavor.Name, 1)

	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
	markMigrating(adminNode, "ws-source", target.Name)

	adminClient := fake.NewClientBuilder().WithObjects(adminNode, target, bystander).
		WithStatusSubresource(target, bystander).WithScheme(scheme.Scheme).Build()
	k8sClient := k8sfake.NewClientset(genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, ""))
	k8sClientFactory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r := newMockWorkspaceReconciler(adminClient)

	for _, ws := range []*v1.Workspace{bystander, target} {
		_, err := r.scaleUp(context.Background(), ws, k8sClientFactory, 1)
		assert.NilError(t, err)
		assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode))
		assert.Equal(t, adminNode.GetSpecWorkspace(), "",
			"workspace(%s) took a node in the middle of a crossing", ws.Name)
	}

	// Once the reservation has aged out it is nobody's, and the scaling loop may have it.
	v1.SetNodeMigrateInfo(adminNode, &v1.NodeMigrateInfo{
		From:      "ws-source",
		Target:    target.Name,
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	assert.NilError(t, adminClient.Update(context.Background(), adminNode))
	_, err := r.scaleUp(context.Background(), target, k8sClientFactory, 1)
	assert.NilError(t, err)
	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode))
	assert.Equal(t, adminNode.GetSpecWorkspace(), target.Name)
}

// migration builds a source workspace carrying a migration of one node to a target, with the
// node in whatever state the caller wants it -- the three the reconciler has to tell apart
// are "still bound here", "released and waiting", and "arrived".
type migration struct {
	reconciler *WorkspaceReconciler
	client     client.Client
	source     *v1.Workspace
	target     *v1.Workspace
	node       *v1.Node
}

func newMigration(t *testing.T, timeout time.Duration,
	placeNode func(node *v1.Node, source, target *v1.Workspace)) *migration {
	t.Helper()
	nodeFlavor := genMockNodeFlavor()
	source := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	target := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	node.Status.ClusterStatus.Phase = v1.NodeManaged
	placeNode(node, source, target)
	metav1.SetMetaDataAnnotation(&source.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{node.Name: v1.BuildMigrateAction(target.Name)})))

	// The same admission rule the real webhook applies to a nodes-action write. Without it
	// these tests judge writes the API server would refuse -- which is how a replica
	// adjustment that could never be persisted passed here once already.
	adminClient := fake.NewClientBuilder().WithObjects(node, source, target).
		WithStatusSubresource(source, target).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	reconciler := newMockWorkspaceReconciler(adminClient)
	option := *reconciler.option
	option.migrateTimeout = timeout
	reconciler.option = &option
	return &migration{
		reconciler: &reconciler, client: adminClient, source: source, target: target, node: node,
	}
}

func boundToSource(node *v1.Node, source, _ *v1.Workspace) {
	node.Spec.Workspace = ptr.To(source.Name)
	metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, source.Name)
}

func (m *migration) reload(t *testing.T) {
	t.Helper()
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.source.Name}, m.source))
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.target.Name}, m.target))
}

// The release and the reservation are one patch: a node unbound without the reservation, even
// for the moment between two writes, is an unassigned node of a matching flavor that any
// workspace in the cluster short of a replica may take.
func TestProcessNodesActionMigrateReleasesAndReserves(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, true)

	m.reload(t)
	assert.Equal(t, m.node.GetSpecWorkspace(), "")
	info := v1.GetNodeMigrateInfo(m.node)
	assert.Assert(t, info != nil)
	assert.Equal(t, info.From, m.source.Name)
	assert.Equal(t, info.Target, m.target.Name)
	assert.Assert(t, info.StartTime != nil)
	// The source keeps the action: it is the record that a migration is under way, and the
	// only thing that brings the reconciler back to finish it.
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
	// Nothing has been asked of the target yet -- the node is still on its way out.
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
}

// Handing the node over means asking the target for it the way a user would, because that
// request is what raises the target's replica. Binding the node here would move it without
// the target ever accounting for it.
func TestProcessNodesActionMigrateHandsOverToTheTarget(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})

	result, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	// Not "updated": a crossing can last as long as the timeout allows when the handover
	// cannot land, and the workspace still has to be synced and scaled in the meantime.
	assert.Equal(t, isUpdated, false)
	// A busy target changes nothing about the node, so no event brings us back to ask again.
	assert.Assert(t, result.RequeueAfter > 0)

	m.reload(t)
	actions, err := parseWorkspaceNodesAction(t, m.target)
	assert.NilError(t, err)
	assert.Equal(t, actions[m.node.Name], v1.NodeActionAdd)
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
	assert.Equal(t, m.node.GetSpecWorkspace(), "")
}

// A target already busy with a node action is the ordinary case, not a failure: a workspace
// takes one at a time. The nodes stay reserved and the handover is retried.
func TestProcessNodesActionMigrateWaitsForABusyTarget(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	busy := string(jsonutils.MarshalSilently(map[string]string{"other-node": v1.NodeActionAdd}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, busy)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	result, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)
	assert.Assert(t, result.RequeueAfter > 0)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), busy)
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) != nil)
}

// Once the node has landed there is nothing left to drive, and the action has to go: left
// behind it would block every later node action on the source workspace.
func TestProcessNodesActionMigrateClearsTheActionOnArrival(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, _, target *v1.Workspace) {
		node.Spec.Workspace = ptr.To(target.Name)
		metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, target.Name)
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, m.node.GetSpecWorkspace(), m.target.Name)
}

// A migration that never completes must not park the node forever. Past the timeout the
// reservation comes off and the node becomes an ordinary unassigned node -- it does not
// return to the source, which gave it up and had its replica lowered to match.
func TestProcessNodesActionMigrateGivesUpAfterTheTimeout(t *testing.T) {
	m := newMigration(t, time.Minute, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
		v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
			From:      source.Name,
			Target:    target.Name,
			StartTime: &metav1.Time{Time: time.Now().UTC().Add(-time.Hour)},
		})
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
	assert.Equal(t, m.node.GetSpecWorkspace(), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
}

// The reservation can be taken off by hand. The source then has no node to migrate, and has
// to stop rather than hand over a node it no longer holds any claim on.
func TestProcessNodesActionMigrateGivesUpWithoutAReservation(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, _, _ *v1.Workspace) {
		node.Spec.Workspace = nil
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
}

// Binding a node to the workspace it was migrating to is the end of the crossing, so the
// reservation comes off in the same patch. Left on, it would keep every other workspace off
// a node that has already arrived.
func TestUpdateSingleNodeBindingClearsTheReservationOnArrival(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	markMigrating(node, "ws-source", "ws-target")
	adminClient := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws-target", node, nodeBinding{workspace: "ws-target"})
	assert.NilError(t, err)
	assert.Equal(t, updated, true)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "ws-target")
	assert.Assert(t, v1.GetNodeMigrateInfo(node) == nil)
}

// A node the target already holds still has to have its reservation cleared: the bind landed,
// the reservation write did not, and nothing else would ever take it off.
func TestUpdateSingleNodeBindingClearsAStaleReservation(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	markMigrating(node, "ws-source", "ws-target")
	node.Spec.Workspace = ptr.To("ws-target")
	adminClient := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, err := r.updateSingleNodeBinding(context.Background(), "ws-target", node, nodeBinding{workspace: "ws-target"})
	assert.NilError(t, err)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "ws-target")
	assert.Assert(t, v1.GetNodeMigrateInfo(node) == nil)
}

func parseWorkspaceNodesAction(t *testing.T, workspace *v1.Workspace) (map[string]string, error) {
	t.Helper()
	actions := make(map[string]string)
	err := json.Unmarshal([]byte(v1.GetWorkspaceNodesAction(workspace)), &actions)
	return actions, err
}

// drain returns the workspaces an event handler put on the queue.
func drain(q v1.RequestWorkQueue) []string {
	var names []string
	for q.Len() > 0 {
		item, shutdown := q.Get()
		if shutdown {
			break
		}
		names = append(names, item.Name)
		q.Done(item)
	}
	sort.Strings(names)
	return names
}

// The source workspace drives the migration and holds its one action slot until it sees the
// node land. Landing takes the node's workspace label from empty to the target, so waking
// workspaces by label alone wakes the target and leaves the source to find out at its next
// resync -- and every node operation asked of the source in between is refused as busy.
func TestHandleNodeEventWakesTheSourceWhenTheNodeLands(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	h := r.handleNodeEvent()
	handlerFuncs, ok := h.(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	assert.Assert(t, ok)

	// Old: released and reserved for the target. New: bound, reservation cleared -- the two
	// halves of the patch that ends a migration.
	released := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	markMigrating(released, "ws-source", "ws-target")
	landed := released.DeepCopy()
	landed.Spec.Workspace = ptr.To("ws-target")
	metav1.SetMetaDataLabel(&landed.ObjectMeta, v1.WorkspaceIdLabel, "ws-target")
	delete(landed.Annotations, v1.NodeMigrateAnnotation)

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	handlerFuncs.Update(context.Background(), event.UpdateEvent{ObjectOld: released, ObjectNew: landed}, q)

	queued := drain(q)
	assert.Assert(t, sliceHas(queued, "ws-source"), "the source was not woken by the landing, queued: %v", queued)
	assert.Assert(t, sliceHas(queued, "ws-target"), "the target was not woken by the landing, queued: %v", queued)
}

// The release half: the node leaves the source and is reserved for a target that has not been
// asked for it yet, so the target has nothing to do -- but the source has to come straight
// back to hand it over rather than waiting out a resync.
func TestHandleNodeEventWakesBothEndsOnRelease(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	handlerFuncs := r.handleNodeEvent().(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})

	bound := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	bound.Spec.Workspace = ptr.To("ws-source")
	metav1.SetMetaDataLabel(&bound.ObjectMeta, v1.WorkspaceIdLabel, "ws-source")
	released := bound.DeepCopy()
	markMigrating(released, "ws-source", "ws-target")

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	handlerFuncs.Update(context.Background(), event.UpdateEvent{ObjectOld: bound, ObjectNew: released}, q)

	queued := drain(q)
	assert.Assert(t, sliceHas(queued, "ws-source"), "queued: %v", queued)
	assert.Assert(t, sliceHas(queued, "ws-target"), "queued: %v", queued)
}

func sliceHas(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// A node that ends up in a third workspace is not ours to hand over. Going on would have the
// source asking the target to take someone else's node, once a pass until the timeout, while
// holding its own action slot for the whole of it.
func TestProcessNodesActionMigrateGivesUpOnANodeBoundElsewhere(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
		node.Spec.Workspace = ptr.To("ws-somewhere-else")
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
}

// A release that has to be retried keeps the clock it started with, so a release that keeps
// failing still ages out instead of holding the action slot indefinitely.
func TestClassifyMigrationKeepsTheOriginalStartTime(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)
	// Truncated: the annotation is JSON, and metav1.Time round-trips at second precision.
	started := &metav1.Time{Time: time.Now().UTC().Add(-20 * time.Minute).Truncate(time.Second)}
	v1.SetNodeMigrateInfo(m.node, &v1.NodeMigrateInfo{
		From: m.source.Name, Target: m.target.Name, StartTime: started,
	})

	state, info, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationRelease)
	assert.Assert(t, info.StartTime != nil)
	assert.Equal(t, info.StartTime.Time.Equal(started.Time), true,
		"the retry re-stamped the clock instead of keeping it")
}

// The expectations gate waits on bindings observed through node labels, which come from the
// data plane. A migration held behind it would never be carried on and never time out, so the
// node would stay released and reserved with nothing driving it.
func TestProcessWorkspaceCarriesAMigrationWithExpectationsOutstanding(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// Something the data plane has not reported back on yet.
	m.reconciler.setExpectations(m.source.Name, sets.NewSetByKeys("some-other-node"))
	assert.Equal(t, m.reconciler.meetExpectations(m.source.Name), false)

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	m.reload(t)
	actions, err := parseWorkspaceNodesAction(t, m.target)
	assert.NilError(t, err)
	assert.Equal(t, actions[m.node.Name], v1.NodeActionAdd,
		"the handover did not happen while an unrelated expectation was outstanding")
}

// Once the finalizer is gone there is no workspace left to carry the migration or to give up
// on it, so a node this workspace released has to be let go on the way out. Nothing else
// would ever clear the reservation.
func TestDeleteWorkspaceReleasesTheNodesItWasMigrating(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	controllerutil.AddFinalizer(m.source, v1.WorkspaceFinalizer)
	assert.NilError(t, m.client.Update(context.Background(), m.source))

	assert.NilError(t, m.reconciler.delete(context.Background(), m.source))

	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil,
		"the reservation outlived the workspace that was driving it")
}

// Guessing is the one thing not to do with an action this does not understand: falling
// through to the add branch binds the node with none of the replica accounting a real add is
// admitted with.
func TestProcessNodesActionIgnoresAnUnknownAction(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	node.Status.ClusterStatus.Phase = v1.NodeManaged
	metav1.SetMetaDataAnnotation(&workspace.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{node.Name: "migrate:"})))

	adminClient := fake.NewClientBuilder().WithObjects(node, workspace).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "", "a malformed action claimed the node")
}

// Two things bind nodes for one workspace: the scaling loop, and the node action a user
// asked for. Replacing what the workspace is waiting on drops whatever the other one had
// outstanding -- the workspace then reads as settled with a binding still in flight, and the
// next scaling decision is taken on counts that have not caught up.
func TestSetExpectationsKeepsWhatIsAlreadyOutstanding(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	r.setExpectations("ws-a", sets.NewSetByKeys("scaling-down-node"))
	r.setExpectations("ws-a", sets.NewSetByKeys("node-action-node"))
	assert.Equal(t, r.meetExpectations("ws-a"), false)

	r.observeNode("ws-a", "node-action-node")
	assert.Equal(t, r.meetExpectations("ws-a"), false,
		"the workspace read as settled while the earlier binding was still in flight")

	r.observeNode("ws-a", "scaling-down-node")
	assert.Equal(t, r.meetExpectations("ws-a"), true)
}

// Once a reservation has expired it stops being honoured, so the node can be picked up by
// someone other than its target. The reservation has done its work either way and must come
// off, or it goes on naming workspaces with no part in this node and waking them for it.
func TestUpdateSingleNodeBindingClearsAnExpiredReservationOnWhoeverTakesTheNode(t *testing.T) {
	node := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      "ws-source",
		Target:    "ws-target",
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	adminClient := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, err := r.updateSingleNodeBinding(context.Background(), "ws-unrelated", node, nodeBinding{workspace: "ws-unrelated"})
	assert.NilError(t, err)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "ws-unrelated")
	assert.Assert(t, v1.GetNodeMigrateInfo(node) == nil, "the reservation outlived the crossing")
}

// The node webhook stops honouring a reservation at the shared timeout and cannot read this
// controller's setting, so a longer or unset one here would leave the migration still being
// driven after the node has stopped being protected.
func TestMigrateTimeoutNeverOutlastsTheGuard(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	for _, configured := range []time.Duration{0, -time.Minute, 2 * v1.DefaultNodeMigrateTimeout} {
		option := *r.option
		option.migrateTimeout = configured
		r.option = &option
		assert.Equal(t, r.migrateTimeout(), v1.DefaultNodeMigrateTimeout)
	}
	option := *r.option
	option.migrateTimeout = time.Minute
	r.option = &option
	assert.Equal(t, r.migrateTimeout(), time.Minute)
}

// Nothing settles an expectation but the event saying the binding landed, and a workspace
// still holding one runs nothing at all -- no status, no scaling, and no deletion, since
// removing the finalizer waits behind the same gate. A node deleted before its label was
// written, or an event missed, would leave it that way for good.
func TestExpectationsStopBeingWaitedOnEventually(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws-a", sets.NewSetByKeys("a-node-nobody-will-report-on"))
	assert.Equal(t, r.meetExpectations("ws-a"), false)

	expire(&r, "ws-a", "a-node-nobody-will-report-on")

	assert.Equal(t, r.meetExpectations("ws-a"), true, "the workspace waited for good")
	// Reading does not consume the way out; pruning is what removes it.
	r.RLock()
	_, still := r.expectations["ws-a"]
	r.RUnlock()
	assert.Equal(t, still, true)

	r.pruneExpectations("ws-a")
	r.RLock()
	_, afterPrune := r.expectations["ws-a"]
	r.RUnlock()
	assert.Equal(t, afterPrune, false)
}

// expire backdates one node's deadline so the wait for it has run out.
func expire(r *WorkspaceReconciler, workspaceId, nodeName string) {
	r.Lock()
	defer r.Unlock()
	r.expectations[workspaceId].deadlines[nodeName] = time.Now().Add(-time.Second)
}

// A workspace that keeps binding nodes keeps having entries added, and that is exactly the
// workspace where a stale one hides -- node actions run before the gate and go on adding to
// it. Each node has to age on its own clock.
func TestExpectationsAgeOnTheirOwnClock(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws-a", sets.NewSetByKeys("stale-node"))
	expire(&r, "ws-a", "stale-node")

	// A later binding for another node must not put the stale one back on the clock.
	r.setExpectations("ws-a", sets.NewSetByKeys("fresh-node"))
	assert.Equal(t, r.meetExpectations("ws-a"), false, "the fresh binding is still worth waiting on")

	r.observeNode("ws-a", "fresh-node")
	assert.Equal(t, r.meetExpectations("ws-a"), true, "the stale node was put back on the clock")
}

// Waiting for a workspace that no longer exists reaches a conclusion that is already certain,
// having held the source's one action slot for the whole of the migration timeout to get
// there.
func TestProcessNodesActionMigrateGivesUpWhenTheTargetIsGone(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	assert.NilError(t, m.client.Delete(context.Background(), m.target))

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.source.Name}, m.source))
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
}

// The admission side refuses a workspace taking on a crossing someone else started; this is
// the same line held where the work is done.
func TestClassifyMigrationGivesUpOnSomeoneElsesCrossing(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, _, target *v1.Workspace) {
		markMigrating(node, "another-workspace", target.Name)
	})
	state, _, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationAbandoned)
}

// Giving up is final -- the reservation comes off and the source's replica is not given back
// -- so a cache that has not caught up must not be what decides it.
func TestProcessNodesActionMigrateConfirmsAMissingTargetBeforeGivingUp(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// Gone as far as the cache is concerned, still there as far as the apiserver is. The node
	// goes in too: every read on this path goes through the apiReader now.
	m.reconciler.apiReader = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.target.DeepCopy(), m.node.DeepCopy()).Build()
	m.target.Finalizers = nil
	assert.NilError(t, m.client.Update(context.Background(), m.target))
	assert.NilError(t, m.client.Delete(context.Background(), m.target))
	assert.Assert(t, apierrors.IsNotFound(
		m.client.Get(context.Background(), client.ObjectKey{Name: m.target.Name}, m.target.DeepCopy())))

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) != nil,
		"a stale cache read was enough to give up on the migration")
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.source.Name}, m.source))
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
}

// The deadline is only read when something asks, and nothing asks unless the workspace is
// reconciled again. Blocked on the gate, nothing else brings it back: a node released
// mid-migration carries no workspace on its labels for a node event to route by.
func TestProcessWorkspaceAsksToComeBackWhileItIsWaiting(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	r.option = &WorkspaceReconcilerOption{nodeWait: 30 * time.Second}
	r.setExpectations(ws.Name, sets.NewSetByKeys("a-node-still-in-flight"))

	result, err := r.processWorkspace(context.Background(), ws)
	assert.NilError(t, err)
	assert.Assert(t, result.RequeueAfter > 0, "the workspace was left with nothing to bring it back")
}

// An entry is removed when the node settles or when a prune reaches it, and a binding can
// arrive before either has happened. Inheriting the lapsed deadline opens the gate on that
// node at once -- and on every binding of it after that, since each one inherits it again.
func TestSetExpectationsRestartsTheClockOnALapsedEntry(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws-a", sets.NewSetByKeys("node1"))

	r.Lock()
	r.expectations["ws-a"].deadlines["node1"] = time.Now().Add(-time.Second)
	r.Unlock()
	assert.Equal(t, r.meetExpectations("ws-a"), true)

	// Bound again, so there is something to wait for again.
	r.setExpectations("ws-a", sets.NewSetByKeys("node1"))
	assert.Equal(t, r.meetExpectations("ws-a"), false,
		"the new binding inherited a deadline that had already passed")
}

// A node on its way out is not going anywhere else, held by the source or not. Admission
// turns these away; getting here means it started deleting after the request was accepted.
func TestClassifyMigrationOnANodeBeingDeleted(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)
	now := metav1.NewTime(time.Now())
	m.node.DeletionTimestamp = &now
	m.node.Finalizers = []string{v1.NodeFinalizer}

	state, _, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationAbandoned)

	markMigrating(m.node, m.source.Name, m.target.Name)
	m.node.DeletionTimestamp = &now
	state, _, _ = m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationAbandoned)
}

// Admission turns these away, so getting here means the node started deleting after the
// request was accepted. Given up on rather than carried further -- releasing a node on its
// way out is allowed and binding one is not, so the crossing would only be turned down by the
// far end, once a pass, until it timed out.
func TestProcessNodesActionMigrateGivesUpOnANodeThatStartedDeleting(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		boundToSource(node, source, target)
		node.Finalizers = []string{v1.NodeFinalizer}
	})
	assert.NilError(t, m.client.Delete(context.Background(), m.node))

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Assert(t, v1.GetAnnotation(m.source, v1.WorkspaceNodesActionError) != "",
		"the request was dropped with no reason recorded")
}

// A node released for a workspace that has gone is withdrawn like any other refusal, so the
// request does not disappear from the console without a word.
func TestProcessNodesActionMigrateRecordsWhyAGoneTargetWasGivenUpOn(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	assert.NilError(t, m.client.Delete(context.Background(), m.target))
	m.reconciler.apiReader = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.node.DeepCopy()).Build()

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Assert(t, v1.GetAnnotation(m.source, v1.WorkspaceNodesActionError) != "",
		"the request vanished with no reason recorded")
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
}

// The target's mutating webhook rewrites the annotation as it accepts it, so the string read
// back is not the string written. Compared by text, a workspace mistakes its own handover for
// somebody else's job and waits out the timeout beside it.
func TestHandOverToTargetRecognisesItsOwnRequestAfterTheWebhookRewritesIt(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// What the target carries after its webhook dropped an entry that was already true: a
	// subset of what the handover asked for.
	rewritten := string(jsonutils.MarshalSilently(map[string]string{m.node.Name: v1.NodeActionAdd}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, rewritten)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	assert.NilError(t, m.reconciler.handOverToTarget(context.Background(), m.target.Name,
		[]string{m.node.Name, "already-landed"}))

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), rewritten,
		"it wrote over a request that already asked for this node")
}

// A request somebody else made that happens to name these nodes is not this handover. Read as
// one, the handover reports itself done and writes nothing -- and if that other request is
// later withdrawn the nodes are left with nobody asking for them, while the replica it moved
// goes back to a workspace they did reach.
func TestHandOverToTargetDoesNotAdoptSomebodyElsesRequest(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// Names our node, and two more we never asked about.
	theirs := string(jsonutils.MarshalSilently(map[string]string{
		m.node.Name: v1.NodeActionAdd, "n2": v1.NodeActionAdd, "n3": v1.NodeActionAdd,
	}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, theirs)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	err := m.reconciler.handOverToTarget(context.Background(), m.target.Name, []string{m.node.Name})
	assert.Assert(t, err != nil, "somebody else's request was taken for this handover")
	assert.Assert(t, errors.Is(err, errMigrationTargetBusy))

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), theirs)
}

// A target carrying somebody else's action is waited for, not overwritten.
func TestHandOverToTargetLeavesAnotherRequestAlone(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	other := string(jsonutils.MarshalSilently(map[string]string{"other-node": v1.NodeActionAdd}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, other)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	err := m.reconciler.handOverToTarget(context.Background(), m.target.Name, []string{m.node.Name})
	assert.Assert(t, err != nil)
	assert.Assert(t, errors.Is(err, errMigrationTargetBusy), "a busy target read as something worse")

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), other)
}

// A plain merge patch carries no resourceVersion, so it never conflicts and never retries --
// it just writes over whatever landed on the target between the read and the write. Here that
// is somebody else's node action, and its replica has already been counted for it.
func TestHandOverToTargetDoesNotOverwriteARequestThatLandedFirst(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	competing := string(jsonutils.MarshalSilently(map[string]string{"someone-elses-node": v1.NodeActionAdd}))

	// Slipped in after the handover has read the target and before it writes.
	raced := false
	m.reconciler.Client = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.node.DeepCopy(), m.source.DeepCopy(), m.target.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				if err := cl.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				workspace, ok := obj.(*v1.Workspace)
				if !ok || raced || key.Name != m.target.Name {
					return nil
				}
				raced = true
				winner := workspace.DeepCopy()
				metav1.SetMetaDataAnnotation(&winner.ObjectMeta, v1.WorkspaceNodesAction, competing)
				return cl.Update(ctx, winner)
			},
		}).Build()

	err := m.reconciler.handOverToTarget(context.Background(), m.target.Name, []string{m.node.Name})
	assert.Assert(t, err != nil, "the handover reported success over somebody else's request")

	stored := &v1.Workspace{}
	assert.NilError(t, m.reconciler.Get(context.Background(), client.ObjectKey{Name: m.target.Name}, stored))
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), competing,
		"the request that landed first was overwritten")
}

// A pass can have both: one node to bind, and another already released and waiting to be
// taken. Returning for the binding alone leaves the wait to whatever event the binding
// happens to produce, or to the resync hours later if it produces none.
func TestProcessNodesActionKeepsTheWaitWhenItAlsoBindsSomething(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	source := genMockWorkspace("cluster", nodeFlavor.Name, 2)
	target := genMockWorkspace("cluster", nodeFlavor.Name, 1)

	toRelease := genMockAdminNode("node1", "cluster", nodeFlavor)
	toRelease.Status.ClusterStatus.Phase = v1.NodeManaged
	boundToSource(toRelease, source, target)
	waiting := genMockAdminNode("node2", "cluster", nodeFlavor)
	waiting.Status.ClusterStatus.Phase = v1.NodeManaged
	markMigrating(waiting, source.Name, target.Name)

	metav1.SetMetaDataAnnotation(&source.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{
			toRelease.Name: v1.BuildMigrateAction(target.Name),
			waiting.Name:   v1.BuildMigrateAction(target.Name),
		})))
	// Busy, so the second node stays waiting instead of being taken this pass.
	metav1.SetMetaDataAnnotation(&target.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{"someone-else": v1.NodeActionAdd})))

	adminClient := fake.NewClientBuilder().WithObjects(toRelease, waiting, source, target).
		WithStatusSubresource(source, target).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	result, isUpdated, err := r.processNodesAction(context.Background(), source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, true, "a node was bound this pass")
	assert.Assert(t, result.RequeueAfter > 0, "the node still waiting to be taken was forgotten")
}

// Nothing removes a reservation once it has expired -- it is only ignored where it is read --
// so a node carrying a stale one would wake both of its workspaces on every write it ever
// receives, and the data plane writes node status every few seconds.
func TestHandleNodeEventIgnoresAnExpiredReservation(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	handlerFuncs := r.handleNodeEvent().(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})

	stale := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	v1.SetNodeMigrateInfo(stale, &v1.NodeMigrateInfo{
		From:      "ws-source",
		Target:    "ws-target",
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	// The sort of write the data plane makes constantly.
	touched := stale.DeepCopy()
	touched.Status.MachineStatus.HostName = "renamed"

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	handlerFuncs.Update(context.Background(), event.UpdateEvent{ObjectOld: stale, ObjectNew: touched}, q)

	assert.Equal(t, len(drain(q)), 0, "a reservation nobody is driving kept waking its workspaces")
}

// An annotation this cannot read is one whose claims are unknown, and reading unknown as
// empty hands the nodes it names to whoever asks next -- which is the thing reservedNodes
// exists to prevent.
func TestReservedNodesFailsClosedOnAnUnreadableClaim(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	asking := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	other := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	metav1.SetMetaDataAnnotation(&other.ObjectMeta, v1.WorkspaceNodesAction, "{not json")

	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(asking, other).Build())

	_, err := r.reservedNodes(context.Background(), asking.Name)
	assert.Assert(t, err != nil, "an unreadable claim was read as claiming nothing")
}

// The target's own admission turning the request down for good is a judgement, not a passing
// condition: retrying waits for a workspace to change its mind.
func TestProcessNodesActionMigrateGivesUpWhenTheTargetRefuses(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	m.reconciler.Client = refusingTarget(m, apierrors.NewBadRequest("the flavor does not match"))

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	stored := &v1.Node{}
	assert.NilError(t, m.reconciler.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, stored))
	assert.Assert(t, v1.GetNodeMigrateInfo(stored) == nil, "the node was left reserved for a refusal")
}

// A conflict is not that judgement. This codebase answers an admission refusal with 409 and so
// does optimistic locking, and RetryOnConflict hands back the last conflict when it runs out
// of attempts -- so a target being written by its own controller looks exactly like one that
// has refused. The crossing keeps its reservation and waits for the timeout to decide.
func TestProcessNodesActionMigrateKeepsGoingThroughAConflict(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	m.reconciler.Client = refusingTarget(m, apierrors.NewConflict(
		schema.GroupResource{Resource: "workspaces"}, m.target.Name, errors.New("modified")))

	result, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Assert(t, result.RequeueAfter > 0, "the crossing was not asked to come back")

	stored := &v1.Node{}
	assert.NilError(t, m.reconciler.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, stored))
	assert.Assert(t, v1.GetNodeMigrateInfo(stored) != nil,
		"a crossing that would have landed was given up on over a passing conflict")
}

// refusingTarget builds a client whose writes to the migration target always fail with err.
func refusingTarget(m *migration, err error) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.node.DeepCopy(), m.source.DeepCopy(), m.target.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if workspace, ok := obj.(*v1.Workspace); ok && workspace.Name == m.target.Name {
					return err
				}
				return cl.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
}

// assertNoInvoluntaryEviction is the invariant every abandonment has to leave behind: the
// workspace must never end up counting fewer nodes than it holds. Counting fewer is what makes
// the scaling loop release a healthy node nobody asked to give up -- the failure this feature
// has produced in four different places, each time with a test that read every field but this
// one. Counting more is the other direction and is meant: a crossing that did not happen
// leaves the workspace short of a machine it should have, and scaling up replaces it.
func assertNoInvoluntaryEviction(t *testing.T, m *migration) int {
	t.Helper()
	stored := &v1.Workspace{}
	assert.NilError(t, m.reconciler.Get(context.Background(),
		client.ObjectKey{Name: m.source.Name}, stored))
	nodes := &v1.NodeList{}
	assert.NilError(t, m.reconciler.List(context.Background(), nodes))
	held := 0
	for i := range nodes.Items {
		if nodes.Items[i].GetSpecWorkspace() == m.source.Name {
			held++
		}
	}
	assert.Assert(t, stored.Spec.Replica >= held,
		"workspace(%s) counts %d nodes and holds %d, so it will release one it still has",
		m.source.Name, stored.Spec.Replica, held)
	return held
}

// Every way a crossing can be given up on, against the one thing all of them have to leave
// true. The workspace enters each of them as it would in production: the count went out with
// the request, whether or not the node has followed it yet.
func TestMigrationAbandonmentNeverEvictsAHealthyNode(t *testing.T) {
	cases := []struct {
		name      string
		place     func(node *v1.Node, source, target *v1.Workspace)
		setUp     func(t *testing.T, m *migration)
		wantExact bool
	}{
		{
			name:  "the node starts deleting before it is released",
			place: boundToSource,
			// It never left, so the count has to come back to exactly what is held: one more
			// would buy a machine to stand beside a node the workspace still has.
			wantExact: true,
			setUp: func(t *testing.T, m *migration) {
				assert.NilError(t, m.client.Delete(context.Background(), m.node))
			},
		},
		{
			name:  "the node ends up in a third workspace",
			place: boundToSource,
			setUp: func(t *testing.T, m *migration) {
				m.node.Spec.Workspace = ptr.To("somewhere-else")
				assert.NilError(t, m.client.Update(context.Background(), m.node))
			},
		},
		{
			name: "the reservation is taken off after the release",
			place: func(node *v1.Node, source, target *v1.Workspace) {
				markMigrating(node, source.Name, target.Name)
				delete(node.Annotations, v1.NodeMigrateAnnotation)
			},
		},
		{
			name: "the target is gone by the time it is asked",
			place: func(node *v1.Node, source, target *v1.Workspace) {
				markMigrating(node, source.Name, target.Name)
			},
			setUp: func(t *testing.T, m *migration) {
				assert.NilError(t, m.client.Delete(context.Background(), m.target))
				m.reconciler.apiReader = fake.NewClientBuilder().WithScheme(scheme.Scheme).
					WithObjects(m.node.DeepCopy()).Build()
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
				node.Finalizers = []string{v1.NodeFinalizer}
				tc.place(node, source, target)
			})
			// As admission leaves it: the count went out when the request was accepted, so a
			// workspace with this one node counts none of it from here on.
			m.source.Spec.Replica = 0
			assert.NilError(t, m.client.Update(context.Background(), m.source))
			if tc.setUp != nil {
				tc.setUp(t, m)
			}

			_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
			assert.NilError(t, err)

			m.reload(t)
			assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "", "the request was left behind")
			held := assertNoInvoluntaryEviction(t, m)
			if tc.wantExact {
				assert.Equal(t, m.source.Spec.Replica, held,
					"the node never left, so the count should be back to what it holds")
			}
		})
	}
}

// Only a node that was released carries a reservation, so skipping the whole iteration for
// nodes without one skipped it for exactly the two crossings given up on before the release --
// the ones where the user is told nothing at all and the node is still sitting where it was.
func TestAbandonMigrationsTellsBothEndsEvenBeforeTheRelease(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)
	events := record.NewFakeRecorder(8)
	m.reconciler.recorder = events

	// No reservation on it: it never left.
	assert.Assert(t, !v1.HasAnnotation(m.node, v1.NodeMigrateAnnotation))
	assert.NilError(t, m.reconciler.abandonMigrations(context.Background(), m.source,
		[]*v1.Node{m.node}, map[string]string{m.node.Name: m.target.Name}))

	var told []string
	for len(events.Events) > 0 {
		told = append(told, <-events.Events)
	}
	assert.Equal(t, len(told), 2, "not both workspaces were told: %v", told)
	for _, event := range told {
		assert.Assert(t, strings.Contains(event, "NodeMigrationAbandoned"), event)
		assert.Assert(t, strings.Contains(event, m.node.Name), event)
	}
}

// A reservation left behind by some other crossing can name the same target while having been
// written for a different source. Inheriting its clock starts this crossing already old, and a
// migration of a few seconds is then given up on for not completing within the timeout.
func TestClassifyMigrationDoesNotInheritAnotherCrossingsClock(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)
	// Left over from a crossing this workspace had no part in, and long past its timeout.
	v1.SetNodeMigrateInfo(m.node, &v1.NodeMigrateInfo{
		From:      "some-other-workspace",
		Target:    m.target.Name,
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-3 * time.Hour)},
	})

	state, info, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationRelease)
	assert.Assert(t, info.StartTime != nil)
	assert.Assert(t, !v1.IsNodeMigrationExpired(info, v1.DefaultNodeMigrateTimeout),
		"this crossing started out already expired, on somebody else's clock")

	// Its own earlier attempt is still inherited, so a release that has to be retried does
	// not keep restarting the clock.
	started := &metav1.Time{Time: time.Now().UTC().Add(-time.Minute).Truncate(time.Second)}
	v1.SetNodeMigrateInfo(m.node, &v1.NodeMigrateInfo{
		From: m.source.Name, Target: m.target.Name, StartTime: started,
	})
	_, info, _ = m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, info.StartTime.Time.Equal(started.Time), true, "the retry restarted the clock")
}

func TestWorkspaceHandleNodeEvent(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	h := r.handleNodeEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}
	h.Create(context.Background(), event.CreateEvent{Object: node}, q)
	newNode := node.DeepCopy()
	newNode.Labels[v1.WorkspaceIdLabel] = "ws2"
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: node, ObjectNew: newNode}, q)
}
