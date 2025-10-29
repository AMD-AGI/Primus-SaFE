/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

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

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 2)
	workspace.Status.Phase = v1.WorkspaceAbnormal
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	metav1.SetMetaDataLabel(&adminNode1.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	adminNode2.Status.Unschedulable = true
	metav1.SetMetaDataLabel(&adminNode2.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)

	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, nil)
	r.clientManager.AddOrReplace(clusterName, k8sClients)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: workspace.Name},
	}
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceRunning)
	assert.Equal(t, workspace.Status.AvailableReplica, 1)
	assert.Equal(t, workspace.Status.AbnormalReplica, 1)
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

	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).
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
	assert.Equal(t, commonquantity.Equal(workspace.Status.TotalResources, corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("12"),
		corev1.ResourceMemory: resource.MustParse("24Gi"),
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
