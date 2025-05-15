/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func newWorkspaceReconciler(adminClient client.Client) WorkspaceReconciler {
	return WorkspaceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: adminClient,
		},
		opt:          &defaultWorkspaceOption,
		expectations: make(map[string]sets.Set),
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
	adminClient := fake.NewClientBuilder().WithObjects(workspace, adminNode1, adminNode2).WithScheme(scheme.Scheme).Build()
	time.Sleep(time.Millisecond * 100)

	var err error
	r := newWorkspaceReconciler(adminClient)
	err = r.delete(context.Background(), workspace)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, workspace)
	assert.NilError(t, err)
	assert.Equal(t, workspace.Status.Phase, v1.WorkspaceDeleted)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), "")
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), "")
}

func TestScaleUpWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode1 := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode1.Status.ClusterStatus.Phase = v1.NodeManaged
	adminNode2 := genMockAdminNode("node2", clusterName, nodeFlavor)
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).WithScheme(scheme.Scheme).Build()

	k8sNode1 := genMockK8sNode(adminNode1.Name, clusterName, nodeFlavor.Name, workspace.Name)
	k8sNode2 := genMockK8sNode(adminNode2.Name, clusterName, nodeFlavor.Name, workspace.Name)
	k8sClient := k8sfake.NewClientset(k8sNode1, k8sNode2)
	time.Sleep(time.Millisecond * 100)

	informer := &ClusterInformer{
		clientSet: k8sClient,
	}
	r := newWorkspaceReconciler(adminClient)

	_, err := r.scaleUp(context.Background(), workspace, informer, 1)
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
	adminClient := fake.NewClientBuilder().WithObjects(adminNode1, adminNode2, workspace).WithScheme(scheme.Scheme).Build()
	time.Sleep(time.Millisecond * 100)

	r := newWorkspaceReconciler(adminClient)
	_, err := r.scaleDown(context.Background(), workspace, 1)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode1.Name}, adminNode1)
	assert.NilError(t, err)
	assert.Equal(t, adminNode1.GetSpecWorkspace(), workspace.Name)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode2.Name}, adminNode2)
	assert.NilError(t, err)
	assert.Equal(t, adminNode2.GetSpecWorkspace(), "")
}
