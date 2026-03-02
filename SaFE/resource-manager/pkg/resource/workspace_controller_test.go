/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonquantity "github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func newMockWorkspaceReconciler(adminClient client.Client) WorkspaceReconciler {
	return WorkspaceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: adminClient,
		},
		option:        &defaultWorkspaceOption,
		expectations:  make(map[string]sets.Set),
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
	adminNode2.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminClient := fake.NewClientBuilder().WithObjects(workspace, adminNode1, adminNode2).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()

	var err error
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceRunning)
	assert.Equal(t, controllerutil.ContainsFinalizer(workspace, v1.WorkspaceFinalizer), true)

	r := newMockWorkspaceReconciler(adminClient)
	err = r.delete(context.Background(), workspace)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceDeleting)
	assert.Equal(t, controllerutil.ContainsFinalizer(workspace, v1.WorkspaceFinalizer), true)

	r.observeNode(workspace.Name, adminNode1.Name)
	r.observeNode(workspace.Name, adminNode2.Name)
	err = r.delete(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, controllerutil.ContainsFinalizer(workspace, v1.WorkspaceFinalizer), false)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), "")
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), "")
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
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	adminNode2.Status.Unschedulable = true
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)

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
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)
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
	adminNode2.Spec.Workspace = ptr.To(workspace.Name)
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
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

	_, err := r.processNodesAction(context.Background(), workspace)
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
				assert.Equal(t, result[k], v)
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

func TestGeneratePVC(t *testing.T) {
	workspace := genMockWorkspace("cluster", "nodeflavor", 1)

	tests := []struct {
		name        string
		volume      *v1.WorkspaceVolume
		expectError bool
		validate    func(t *testing.T, pvc *corev1.PersistentVolumeClaim)
	}{
		{
			name: "PFS volume with selector",
			volume: &v1.WorkspaceVolume{
				Type:       v1.PFS,
				Id:         1,
				Capacity:   "100Gi",
				AccessMode: corev1.ReadWriteMany,
				Selector: map[string]string{
					"pfs-selector": "test-pfs",
				},
			},
			expectError: false,
			validate: func(t *testing.T, pvc *corev1.PersistentVolumeClaim) {
				assert.Equal(t, pvc.Name, "pfs-1")
				assert.Equal(t, pvc.Namespace, workspace.Name)
				assert.Assert(t, pvc.Spec.Selector != nil)
				assert.Equal(t, pvc.Spec.Selector.MatchLabels["pfs-selector"], "test-pfs")
				assert.Equal(t, *pvc.Spec.StorageClassName, "")
				assert.Equal(t, pvc.Spec.AccessModes[0], corev1.ReadWriteMany)
			},
		},
		{
			name: "volume with storage class",
			volume: &v1.WorkspaceVolume{
				Type:         v1.PFS,
				Id:           2,
				Capacity:     "50Gi",
				AccessMode:   corev1.ReadWriteOnce,
				StorageClass: "standard",
			},
			expectError: false,
			validate: func(t *testing.T, pvc *corev1.PersistentVolumeClaim) {
				assert.Equal(t, pvc.Name, "pfs-2")
				assert.Equal(t, *pvc.Spec.StorageClassName, "standard")
				assert.Equal(t, pvc.Spec.AccessModes[0], corev1.ReadWriteOnce)
				storageQty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
				assert.Equal(t, storageQty.String(), "50Gi")
			},
		},
		{
			name: "invalid capacity",
			volume: &v1.WorkspaceVolume{
				Type:       v1.PFS,
				Id:         3,
				Capacity:   "invalid",
				AccessMode: corev1.ReadWriteMany,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc, err := generatePVC(tt.volume, workspace)
			if tt.expectError {
				assert.Assert(t, err != nil)
			} else {
				assert.NilError(t, err)
				tt.validate(t, pvc)
			}
		})
	}
}

func TestIsPVCExist(t *testing.T) {
	namespace := "test-namespace"
	pvcName := "test-pvc"

	// Test PVC exists
	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}
	k8sClient := k8sfake.NewClientset(existingPVC)
	exists := isPVCExist(context.Background(), namespace, pvcName, k8sClient)
	assert.Equal(t, exists, true)

	// Test PVC does not exist
	k8sClientEmpty := k8sfake.NewClientset()
	exists = isPVCExist(context.Background(), namespace, pvcName, k8sClientEmpty)
	assert.Equal(t, exists, false)
}

func TestCreateDataplaneNamespace(t *testing.T) {
	// Test create new namespace
	k8sClient := k8sfake.NewClientset()
	err := createDataplaneNamespace(context.Background(), "new-namespace", k8sClient)
	assert.NilError(t, err)

	ns, err := k8sClient.CoreV1().Namespaces().Get(context.Background(), "new-namespace", metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, ns.Name, "new-namespace")

	// Test namespace already exists (should not error)
	err = createDataplaneNamespace(context.Background(), "new-namespace", k8sClient)
	assert.NilError(t, err)

	// Test empty name
	err = createDataplaneNamespace(context.Background(), "", k8sClient)
	assert.Assert(t, err != nil)
}

func TestDeleteDataplaneNamespace(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}
	k8sClient := k8sfake.NewClientset(ns)

	// Test delete existing namespace
	err := deleteDataplaneNamespace(context.Background(), "test-namespace", k8sClient)
	assert.NilError(t, err)

	// Verify namespace is deleted
	_, err = k8sClient.CoreV1().Namespaces().Get(context.Background(), "test-namespace", metav1.GetOptions{})
	assert.Assert(t, err != nil)

	// Test delete non-existent namespace (should not error)
	err = deleteDataplaneNamespace(context.Background(), "non-existent", k8sClient)
	assert.NilError(t, err)

	// Test empty name
	err = deleteDataplaneNamespace(context.Background(), "", k8sClient)
	assert.Assert(t, err != nil)
}

func TestDeletePVC(t *testing.T) {
	namespace := "test-namespace"
	pvcName := "test-pvc"

	// Create PVC with finalizer
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       pvcName,
			Namespace:  namespace,
			Finalizers: []string{"kubernetes.io/pvc-protection"},
		},
	}
	k8sClient := k8sfake.NewClientset(pvc)

	// Test delete PVC
	err := deletePVC(context.Background(), pvcName, namespace, k8sClient)
	assert.NilError(t, err)

	// Verify PVC is deleted
	_, err = k8sClient.CoreV1().PersistentVolumeClaims(namespace).Get(context.Background(), pvcName, metav1.GetOptions{})
	assert.Assert(t, err != nil)

	// Test delete non-existent PVC (should not error)
	err = deletePVC(context.Background(), "non-existent", namespace, k8sClient)
	assert.NilError(t, err)
}

func TestDeletePV(t *testing.T) {
	workspace := genMockWorkspace("cluster", "nodeflavor", 1)

	// Create PV with workspace label and finalizer
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
			Labels: map[string]string{
				v1.OwnerLabel: workspace.Name,
			},
			Finalizers: []string{"kubernetes.io/pv-protection"},
		},
	}
	k8sClient := k8sfake.NewClientset(pv)

	// Test delete PV
	err := deletePV(context.Background(), workspace, k8sClient)
	assert.NilError(t, err)

	// Verify PV is deleted
	_, err = k8sClient.CoreV1().PersistentVolumes().Get(context.Background(), "test-pv", metav1.GetOptions{})
	assert.Assert(t, err != nil)

	// Test delete when no PV exists (should not error)
	k8sClientEmpty := k8sfake.NewClientset()
	err = deletePV(context.Background(), workspace, k8sClientEmpty)
	assert.NilError(t, err)
}

func TestDeleteWorkspaceSecrets(t *testing.T) {
	workspace := genMockWorkspace("cluster", "nodeflavor", 1)

	// Create secrets in workspace namespace
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: workspace.Name,
		},
	}
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret2",
			Namespace: workspace.Name,
		},
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: workspace.Name,
		},
	}
	k8sClient := k8sfake.NewClientset(ns, secret1, secret2)

	// Verify secrets exist
	secrets, err := k8sClient.CoreV1().Secrets(workspace.Name).List(context.Background(), metav1.ListOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(secrets.Items), 2)

	// Test delete all secrets
	err = deleteWorkspaceSecrets(context.Background(), workspace, k8sClient)
	assert.NilError(t, err)

	// Verify secrets are deleted
	secrets, err = k8sClient.CoreV1().Secrets(workspace.Name).List(context.Background(), metav1.ListOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(secrets.Items), 0)
}

func TestSyncDataPlanePVC(t *testing.T) {
	workspace := genMockWorkspace("cluster", "nodeflavor", 1)
	workspace.Spec.Volumes = []v1.WorkspaceVolume{
		{
			Type:         v1.PFS,
			Id:           1,
			Capacity:     "100Gi",
			AccessMode:   corev1.ReadWriteMany,
			StorageClass: "standard",
		},
	}

	// Create namespace and an unexpected PVC that should be deleted
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: workspace.Name,
		},
	}
	unexpectedPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pfs-old-vol",
			Namespace: workspace.Name,
		},
	}
	k8sClient := k8sfake.NewClientset(ns, unexpectedPVC)

	// Test sync - should delete unexpected PVC and create desired one
	err := syncDataPlanePVC(context.Background(), workspace, k8sClient)
	assert.NilError(t, err)

	// Verify unexpected PVC is deleted
	_, err = k8sClient.CoreV1().PersistentVolumeClaims(workspace.Name).Get(context.Background(), "pfs-old-vol", metav1.GetOptions{})
	assert.Assert(t, err != nil)

	// Verify desired PVC is created
	pvc, err := k8sClient.CoreV1().PersistentVolumeClaims(workspace.Name).Get(context.Background(), "pfs-1", metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, pvc.Name, "pfs-1")
}

func TestCreatePV(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("100Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}
	k8sClient := k8sfake.NewClientset()

	// Test create PV
	err := createPV(context.Background(), pv, k8sClient)
	assert.NilError(t, err)

	// Verify PV is created
	createdPV, err := k8sClient.CoreV1().PersistentVolumes().Get(context.Background(), "test-pv", metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, createdPV.Name, "test-pv")

	// Test create duplicate PV (should not error)
	err = createPV(context.Background(), pv, k8sClient)
	assert.NilError(t, err)
}

func TestCreatePVC(t *testing.T) {
	namespace := "test-namespace"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}
	k8sClient := k8sfake.NewClientset(ns)

	// Test create PVC
	err := createPVC(context.Background(), pvc, k8sClient)
	assert.NilError(t, err)

	// Verify PVC is created
	createdPVC, err := k8sClient.CoreV1().PersistentVolumeClaims(namespace).Get(context.Background(), "test-pvc", metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, createdPVC.Name, "test-pvc")

	// Test create duplicate PVC (should not error)
	err = createPVC(context.Background(), pvc, k8sClient)
	assert.NilError(t, err)
}
