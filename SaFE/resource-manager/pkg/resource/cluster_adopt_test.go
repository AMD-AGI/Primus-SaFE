/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func TestSyncAdoptedInventoryCreatesFlavorWorkspaceAndNode(t *testing.T) {
	cluster := testCluster("ccs300")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cluster.Spec.ControlPlane.Nodes = []string{"cp"}
	storage := "cephfs"
	capacity := resource.MustParse("1Pi")
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu9458",
			Labels: map[string]string{
				v1.WorkspaceIdLabel:  "ccs300-gpu",
				v1.NodeFlavorIdLabel: "mi300x-ccs300",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.46"}},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:                 resource.MustParse("128"),
				corev1.ResourceMemory:              resource.MustParse("1Ti"),
				corev1.ResourceName(common.AmdGpu): resource.MustParse("8"),
			},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pfs-1", Namespace: "ccs300-gpu"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storage,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: capacity}},
		},
	}
	cs := fake.NewSimpleClientset(k8sNode, pvc)
	r := newPlaneReconciler(t, cluster)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, cs, []corev1.Node{*k8sNode}))

	flavor := &v1.NodeFlavor{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "mi300x-ccs300"}, flavor))
	assert.Equal(t, int64(8), flavor.Spec.Gpu.Quantity.Value())

	workspace := &v1.Workspace{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300-gpu"}, workspace))
	assert.Equal(t, 1, workspace.Spec.Replica)
	assert.Equal(t, v1.TrueStr, v1.GetAnnotation(workspace, v1.WorkspaceAdoptAnnotation))
	assert.Equal(t, "pfs-1", workspace.Spec.Volumes[0].GenFullVolumeId())
	assert.Equal(t, "cephfs", workspace.Spec.Volumes[0].StorageClass)

	node := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu9458"}, node))
	assert.Equal(t, "ccs300", node.GetSpecCluster())
	assert.Equal(t, "ccs300-gpu", node.GetSpecWorkspace())
	assert.Equal(t, "10.235.225.46", node.Spec.PrivateIP)
	assert.Equal(t, "gpu9458", node.GetK8sNodeName())
}

func TestSyncAdoptedInventoryPlainClusterHasNoWorkspace(t *testing.T) {
	cluster := testCluster("plain")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "plain-root", Namespace: "primus-safe"}
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("64"),
		corev1.ResourceMemory: resource.MustParse("256Gi"),
	}
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "cp-a"}, Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.1.0.1"}},
			Capacity:  capacity,
		}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cp-b"}, Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.1.0.2"}},
			Capacity:  capacity,
		}},
	}
	r := newPlaneReconciler(t, cluster)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), nodes))

	flavorName := adoptedFlavor(cluster, &nodes[0]).Name
	assert.Equal(t, flavorName, adoptedFlavor(cluster, &nodes[1]).Name)
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: flavorName}, &v1.NodeFlavor{}))
	for _, name := range []string{"cp-a", "cp-b"} {
		node := &v1.Node{}
		assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: name}, node))
		assert.Equal(t, "plain", node.GetSpecCluster())
		assert.Empty(t, node.GetSpecWorkspace())
		assert.Equal(t, flavorName, node.Spec.NodeFlavor.Name)
	}
	list := &v1.WorkspaceList{}
	assert.NoError(t, r.List(context.Background(), list))
	assert.Empty(t, list.Items)
}

func TestSyncAdoptedInventoryRecordsControlPlaneAndWorkspace(t *testing.T) {
	cluster := testCluster("ccs300")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cluster.Spec.ControlPlane.Nodes = []string{"bootstrap"}
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("32"),
		corev1.ResourceMemory: resource.MustParse("64Gi"),
	}
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu4455", Labels: map[string]string{v1.KubernetesControlPlane: ""}},
			Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.238"}}, Capacity: capacity}},
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu4456", Labels: map[string]string{"node-role.kubernetes.io/master": ""}},
			Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.239"}}, Capacity: capacity}},
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458", Labels: map[string]string{v1.WorkspaceIdLabel: "ccs300-gpu"}},
			Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.46"}}, Capacity: capacity}},
	}
	r := newPlaneReconciler(t, cluster)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), nodes))

	for _, name := range []string{"gpu4455", "gpu4456", "gpu9458"} {
		node := &v1.Node{}
		assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: name}, node))
		assert.Equal(t, "ccs300", node.GetSpecCluster())
		assert.NotEmpty(t, node.Spec.PrivateIP)
	}
	control := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu4455"}, control))
	assert.True(t, v1.IsControlPlane(control))
	worker := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu9458"}, worker))
	assert.False(t, v1.IsControlPlane(worker))
	assert.Equal(t, "ccs300-gpu", worker.GetSpecWorkspace())

	workspace := &v1.Workspace{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300-gpu"}, workspace))
	assert.True(t, v1.IsAdoptedWorkspace(workspace))
	assert.Equal(t, 1, workspace.Spec.Replica)

	got := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300"}, got))
	assert.ElementsMatch(t, []string{"gpu4455", "gpu4456"}, got.Spec.ControlPlane.Nodes)
}

func TestSyncAdoptedInventoryUsesManualFlavor(t *testing.T) {
	cluster := testCluster("plain")
	v1.SetAnnotation(cluster, v1.ClusterAdoptNodeFlavorAnnotation, "manual-cpu")
	existing := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "manual-cpu"}, Spec: v1.NodeFlavorSpec{
		Cpu: v1.CpuChip{Quantity: resource.MustParse("4")}, Memory: resource.MustParse("8Gi"),
	}}
	node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{
		Capacity: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("64"), corev1.ResourceMemory: resource.MustParse("256Gi")},
	}}
	r := newPlaneReconciler(t, cluster, existing)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{node}))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, got))
	assert.Equal(t, "manual-cpu", got.Spec.NodeFlavor.Name)
	flavor := &v1.NodeFlavor{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "manual-cpu"}, flavor))
	assert.Equal(t, int64(4), flavor.Spec.Cpu.Quantity.Value())
}

func TestSyncAdoptedInventoryKeepsExistingWorkspace(t *testing.T) {
	cluster := testCluster("ccs300")
	existing := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ccs300-gpu", Labels: map[string]string{v1.ClusterIdLabel: "ccs300"}},
		Spec:       v1.WorkspaceSpec{Cluster: "ccs300", Replica: 5, QueuePolicy: v1.QueueFifoPolicy},
	}
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458", Labels: map[string]string{
		v1.WorkspaceIdLabel: "ccs300-gpu", v1.NodeFlavorIdLabel: "mi300x-ccs300",
	}}}
	r := newPlaneReconciler(t, cluster, existing)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{*k8sNode}))
	got := &v1.Workspace{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300-gpu"}, got))
	assert.Equal(t, 5, got.Spec.Replica)
	assert.Empty(t, got.Spec.Volumes)
}

func TestEnsureAdoptedNodeFillsEmptyWorkspace(t *testing.T) {
	cluster := testCluster("ccs300")
	existing := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cp"},
		Spec:       v1.NodeSpec{Cluster: pointer.String("ccs300"), Hostname: pointer.String("cp")},
	}
	r := newPlaneReconciler(t, existing)
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp", Labels: map[string]string{v1.WorkspaceIdLabel: "ccs300-cpu"}}}
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, k8sNode))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "cp"}, got))
	assert.Equal(t, "ccs300-cpu", got.GetSpecWorkspace())
}

func TestEnsureAdoptedNodeSkipsForeignCluster(t *testing.T) {
	cluster := testCluster("ccs300")
	foreign := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458"}, Spec: v1.NodeSpec{Cluster: pointer.String("other")}}
	r := newPlaneReconciler(t, foreign)
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458"}}
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, k8sNode))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu9458"}, got))
	assert.Equal(t, "other", got.GetSpecCluster())
}

func TestAuthorizeAdoptedWorkersUsesControlPlane(t *testing.T) {
	cluster := testCluster("ccs300")
	cluster.Spec.ControlPlane.Nodes = []string{"cp"}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cp := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1", Port: pointer.Int32(22)}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ccs-root", Namespace: "primus-safe"},
		Data: map[string][]byte{utils.AuthorizePub: []byte("ssh-ed25519 AAAA ccs")}}
	r := newPlaneReconciler(t, cluster, cp, secret)
	var gotNode, gotCmd string
	previous := runNodeSSH
	runNodeSSH = func(_ context.Context, _ client.Client, node *v1.Node, cmd string) error {
		gotNode, gotCmd = node.Name, cmd
		return nil
	}
	defer func() { runNodeSSH = previous }()

	worker := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458"}, Status: corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.46"}}}}
	control := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}, Status: corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.238"}}}}
	assert.NoError(t, r.authorizeAdoptedWorkers(context.Background(), cluster, []corev1.Node{control, worker}))
	assert.Equal(t, "cp", gotNode)
	assert.Contains(t, gotCmd, "root@10.235.225.46")
	assert.NotContains(t, gotCmd, "root@10.235.225.238")
	assert.Contains(t, gotCmd, "ssh-ed25519 AAAA ccs")
}

func TestAuthorizeAdoptedWorkersSkipsExistingNodes(t *testing.T) {
	cluster := testCluster("ccs300")
	cluster.Spec.ControlPlane.Nodes = []string{"cp"}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cp := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}}
	worker := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458"}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ccs-root", Namespace: "primus-safe"},
		Data: map[string][]byte{utils.AuthorizePub: []byte("ssh-ed25519 AAAA ccs")}}
	r := newPlaneReconciler(t, cluster, cp, worker, secret)
	called := false
	previous := runNodeSSH
	runNodeSSH = func(context.Context, client.Client, *v1.Node, string) error {
		called = true
		return nil
	}
	defer func() { runNodeSSH = previous }()

	k8sNodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "cp"}, Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.238"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458"}, Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.235.225.46"}}}},
	}
	assert.NoError(t, r.authorizeAdoptedWorkers(context.Background(), cluster, k8sNodes))
	assert.False(t, called)
}

func TestVolumesFromPVCsIgnoresUnrelatedClaims(t *testing.T) {
	storage := "cephfs"
	pvcs := []corev1.PersistentVolumeClaim{
		{ObjectMeta: metav1.ObjectMeta{Name: "data-db-0"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &storage}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pfs-2"}, Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storage,
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Pi")}},
		}},
	}
	volumes := volumesFromPVCs(pvcs)
	assert.Len(t, volumes, 1)
	assert.Equal(t, 2, volumes[0].Id)
	assert.Equal(t, "/mnt/pfs-2", volumes[0].MountPath)
}
