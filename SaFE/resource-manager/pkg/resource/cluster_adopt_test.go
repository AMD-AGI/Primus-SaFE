/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, cs, []corev1.Node{*k8sNode}, nil))

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
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), nodes, nil))

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
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), nodes, nil))

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
		Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.11"}},
		Capacity:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("64"), corev1.ResourceMemory: resource.MustParse("256Gi")},
	}}
	r := newPlaneReconciler(t, cluster, existing)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{node}, nil))
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
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{*k8sNode}, nil))
	got := &v1.Workspace{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300-gpu"}, got))
	assert.Equal(t, 5, got.Spec.Replica)
	assert.Empty(t, got.Spec.Volumes)
}

func TestSyncAdoptedInventoryCopiesWorkspaceScopes(t *testing.T) {
	cluster := testCluster("ccs300")
	k8sNode := workerNode("gpu9458", "10.0.0.11")
	k8sNode.Labels = map[string]string{v1.WorkspaceIdLabel: "ccs300-gpu", v1.NodeFlavorIdLabel: "mi300x-ccs300"}
	r := newPlaneReconciler(t, cluster)
	scopes := map[string][]v1.WorkspaceScope{"ccs300-gpu": {"Train", "Infer"}}
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{k8sNode}, scopes))
	got := &v1.Workspace{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300-gpu"}, got))
	assert.Equal(t, []v1.WorkspaceScope{"Train", "Infer"}, got.Spec.Scopes)
}

func TestDataPlaneWorkspaceScopes(t *testing.T) {
	gvr := v1.SchemeGroupVersion.WithResource("workspaces")
	workspace := func(name string, scopes ...interface{}) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{}}}
		obj.SetAPIVersion(v1.SchemeGroupVersion.String())
		obj.SetKind(v1.WorkspaceKind)
		obj.SetName(name)
		if len(scopes) > 0 {
			obj.Object["spec"] = map[string]interface{}{"scopes": scopes}
		}
		return obj
	}
	newClient := func(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
			map[schema.GroupVersionResource]string{gvr: "WorkspaceList"}, objs...)
	}

	got, err := dataPlaneWorkspaceScopes(context.Background(), newClient(workspace("gpu", "Train", "Ray"), workspace("empty")))
	assert.NoError(t, err)
	assert.Equal(t, map[string][]v1.WorkspaceScope{"gpu": {"Train", "Ray"}}, got)

	missing := newClient()
	missing.PrependReactor("list", "workspaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(gvr.GroupResource(), "")
	})
	got, err = dataPlaneWorkspaceScopes(context.Background(), missing)
	assert.NoError(t, err)
	assert.Empty(t, got)

	failing := newClient()
	failing.PrependReactor("list", "workspaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})
	_, err = dataPlaneWorkspaceScopes(context.Background(), failing)
	assert.Error(t, err)
}

func TestEnsureAdoptedNodeFillsEmptyWorkspace(t *testing.T) {
	cluster := testCluster("ccs300")
	existing := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cp"},
		Spec:       v1.NodeSpec{Cluster: pointer.String("ccs300"), Hostname: pointer.String("cp")},
	}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ccs300-cpu"}, Spec: v1.WorkspaceSpec{Cluster: "ccs300"}}
	r := newPlaneReconciler(t, existing, ws)
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp", Labels: map[string]string{v1.WorkspaceIdLabel: "ccs300-cpu"}}}
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, k8sNode))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "cp"}, got))
	assert.Equal(t, "ccs300-cpu", got.GetSpecWorkspace())
}

func TestEnsureAdoptedNodeSetsNodeTemplate(t *testing.T) {
	cluster := testCluster("ccs300")
	v1.SetAnnotation(cluster, v1.ClusterAdoptNodeTemplateAnnotation, "node-template")
	existing := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cp"},
		Spec:       v1.NodeSpec{Cluster: pointer.String("ccs300"), Hostname: pointer.String("cp")},
	}
	r := newPlaneReconciler(t, existing)
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu1"}}))
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}}))
	for _, name := range []string{"gpu1", "cp"} {
		got := &v1.Node{}
		assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: name}, got))
		if assert.NotNil(t, got.Spec.NodeTemplate, name) {
			assert.Equal(t, "node-template", got.Spec.NodeTemplate.Name)
			assert.Equal(t, v1.NodeTemplateKind, got.Spec.NodeTemplate.Kind)
		}
	}
}

func TestEnsureAdoptedNodeOmitsUnsetNodeTemplate(t *testing.T) {
	cluster := testCluster("ccs300")
	r := newPlaneReconciler(t)
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu1"}}))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu1"}, got))
	assert.Nil(t, got.Spec.NodeTemplate)
	data, err := json.Marshal(got.Spec)
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "nodeTemplate")
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
	cp.Status.MachineStatus.Phase = v1.NodeReady
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ccs-root", Namespace: "primus-safe"},
		Data: map[string][]byte{utils.AuthorizePub: []byte(testAuthorizedKey)}}
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
	assert.Contains(t, gotCmd, testAuthorizedKeyBody)
}

func TestAuthorizeAdoptedWorkersSkipsExistingNodes(t *testing.T) {
	cluster := testCluster("ccs300")
	cluster.Spec.ControlPlane.Nodes = []string{"cp"}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cp := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}}
	cp.Status.MachineStatus.Phase = v1.NodeReady
	worker := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu9458"}}
	worker.Status.MachineStatus.Phase = v1.NodeReady
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ccs-root", Namespace: "primus-safe"},
		Data: map[string][]byte{utils.AuthorizePub: []byte(testAuthorizedKey)}}
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

// adoptAuthorizeFixture returns an adopted cluster with one control plane and the given secret data.
func adoptAuthorizeFixture(t *testing.T, data map[string][]byte, objs ...client.Object) (*ClusterReconciler, *v1.Cluster) {
	t.Helper()
	cluster := testCluster("ccs300")
	cluster.Spec.ControlPlane.Nodes = []string{"cp"}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cp := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1", Port: pointer.Int32(22)}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ccs-root", Namespace: "primus-safe"}, Data: data}
	return newPlaneReconciler(t, append([]client.Object{cluster, cp, secret}, objs...)...), cluster
}

func workerNode(name, ip string) corev1.Node {
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: ip}}}}
}

func captureNodeSSH(t *testing.T, err error) *string {
	t.Helper()
	cmd := new(string)
	previous := runNodeSSH
	runNodeSSH = func(_ context.Context, _ client.Client, _ *v1.Node, c string) error {
		*cmd = c
		return err
	}
	t.Cleanup(func() { runNodeSSH = previous })
	return cmd
}

func TestAdoptedWorkerAuthorizeScriptFailsOnAnyWorker(t *testing.T) {
	r, cluster := adoptAuthorizeFixture(t, map[string][]byte{utils.AuthorizePub: []byte(testAuthorizedKey)})
	cmd := captureNodeSSH(t, nil)
	assert.NoError(t, r.authorizeAdoptedWorkers(context.Background(), cluster,
		[]corev1.Node{workerNode("w1", "10.0.0.11"), workerNode("w2", "10.0.0.12")}))

	bin := t.TempDir()
	stub := "#!/bin/sh\ncase \"$*\" in *10.0.0.11*) exit 255;; esac\nexit 0\n"
	assert.NoError(t, os.WriteFile(filepath.Join(bin, "ssh"), []byte(stub), 0o755))
	run := exec.Command("bash", "-c", *cmd)
	run.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := run.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "10.0.0.11")
	assert.NotContains(t, string(out), "10.0.0.12")
}

func TestAuthorizeAdoptedWorkersUsesSecretUsername(t *testing.T) {
	r, cluster := adoptAuthorizeFixture(t, map[string][]byte{
		utils.Username: []byte("ubuntu"), utils.AuthorizePub: []byte(testAuthorizedKey)})
	cmd := captureNodeSSH(t, nil)
	assert.NoError(t, r.authorizeAdoptedWorkers(context.Background(), cluster, []corev1.Node{workerNode("w1", "10.0.0.11")}))
	assert.Contains(t, *cmd, "ubuntu@10.0.0.11")
	assert.NotContains(t, *cmd, "root@")
	assert.NotContains(t, *cmd, "/root/.ssh")
}

func TestAuthorizeAdoptedWorkersRejectsUnsafeUsername(t *testing.T) {
	r, cluster := adoptAuthorizeFixture(t, map[string][]byte{
		utils.Username: []byte("a;reboot"), utils.AuthorizePub: []byte(testAuthorizedKey)})
	captureNodeSSH(t, nil)
	assert.Error(t, r.authorizeAdoptedWorkers(context.Background(), cluster, []corev1.Node{workerNode("w1", "10.0.0.11")}))
}

func TestAdoptedSSHPublicKeyDerivedFromPrivateKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	assert.NoError(t, err)
	secret := &corev1.Secret{Data: map[string][]byte{utils.Authorize: pem.EncodeToMemory(block)}}
	pub, err := sshPublicKeyFromSecret(secret)
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(pub, "ssh-ed25519 "))
}

const (
	testAuthorizedKeyBody = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILC+5uiHVWt7jtqKQdzmOW9IWbzlnCElVQKrlSVDO+pO"
	testAuthorizedKey     = testAuthorizedKeyBody + " ccs"
)

func TestAdoptedSSHPublicKeyDropsShellText(t *testing.T) {
	for name, raw := range map[string]string{
		"comment substitution": testAuthorizedKeyBody + " $(reboot)",
		"heredoc terminator":   testAuthorizedKeyBody + " ccs\nEOF\nreboot",
	} {
		t.Run(name, func(t *testing.T) {
			pub, err := sshPublicKeyFromSecret(&corev1.Secret{Data: map[string][]byte{utils.AuthorizePub: []byte(raw)}})
			assert.NoError(t, err)
			assert.Equal(t, testAuthorizedKeyBody, pub)
		})
	}
	_, err := sshPublicKeyFromSecret(&corev1.Secret{Data: map[string][]byte{utils.AuthorizePub: []byte("$(reboot)")}})
	assert.Error(t, err)
}

func TestAuthorizeAdoptedWorkersSkipsPasswordSecret(t *testing.T) {
	r, cluster := adoptAuthorizeFixture(t, map[string][]byte{
		utils.Username: []byte("root"), utils.Password: []byte("secret")})
	cmd := captureNodeSSH(t, nil)
	assert.NoError(t, r.authorizeAdoptedWorkers(context.Background(), cluster, []corev1.Node{workerNode("w1", "10.0.0.11")}))
	assert.Empty(t, *cmd)
}

func TestPendingAdoptedWorkerIPsRetriesUnreadyNodes(t *testing.T) {
	unready := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "w1"}}
	ready := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "w2"}}
	ready.Status.MachineStatus.Phase = v1.NodeReady
	r, cluster := adoptAuthorizeFixture(t, nil, unready, ready)
	ips, err := r.pendingAdoptedWorkerIPs(context.Background(), cluster,
		[]corev1.Node{workerNode("w1", "10.0.0.11"), workerNode("w2", "10.0.0.12"), workerNode("w3", "10.0.0.13")})
	assert.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.11", "10.0.0.13"}, ips)
}

func TestPendingAdoptedWorkerIPsIncludesUnreadyControlPlanes(t *testing.T) {
	ready := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp2"}}
	ready.Status.MachineStatus.Phase = v1.NodeReady
	unready := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp3"}}
	unready.Status.MachineStatus.Phase = v1.NodeSSHFailed
	r, cluster := adoptAuthorizeFixture(t, nil, ready, unready)
	cluster.Spec.ControlPlane.Nodes = []string{"cp", "cp2", "cp3"}
	cp3 := workerNode("cp3", "10.0.0.3")
	cp3.Labels = map[string]string{"node-role.kubernetes.io/control-plane": ""}
	ips, err := r.pendingAdoptedWorkerIPs(context.Background(), cluster,
		[]corev1.Node{workerNode("cp2", "10.0.0.2"), cp3})
	assert.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.3"}, ips)
}

func TestAdoptClusterContentsRegistersNodesWhenAuthorizeFails(t *testing.T) {
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Spec.ControlPlane.Nodes = []string{"cp"}
	cluster.Spec.ControlPlane.SSHSecret = &corev1.ObjectReference{Name: "ccs-root", Namespace: "primus-safe"}
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cp := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1", Port: pointer.Int32(22)}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ccs-root", Namespace: "primus-safe"},
		Data: map[string][]byte{utils.AuthorizePub: []byte(testAuthorizedKey)}}
	worker := workerNode("w1", "10.0.0.11")
	r := newClusterReconcilerFull(t, fake.NewSimpleClientset(&worker), cluster, cp, secret)
	captureNodeSSH(t, fmt.Errorf("ssh failed"))

	assert.Error(t, r.adoptClusterContents(context.Background(), cluster))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "w1"}, got))
	assert.Equal(t, "c1", got.GetSpecCluster())
}

func TestSyncAdoptedInventorySkipsForeignWorkspace(t *testing.T) {
	cluster := testCluster("ccs300")
	foreign := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "dev"}, Spec: v1.WorkspaceSpec{Cluster: "other"}}
	k8sNode := workerNode("gpu1", "10.0.0.11")
	k8sNode.Labels = map[string]string{v1.WorkspaceIdLabel: "dev"}
	r := newPlaneReconciler(t, cluster, foreign)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{k8sNode}, nil))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu1"}, got))
	assert.Empty(t, got.GetSpecWorkspace())
	assert.Empty(t, v1.GetWorkspaceId(got))
}

func TestSyncAdoptedInventoryRejectsMixedFlavorWorkspace(t *testing.T) {
	cluster := testCluster("ccs300")
	gpu := workerNode("gpu1", "10.0.0.11")
	gpu.Labels = map[string]string{v1.WorkspaceIdLabel: "dev", v1.NodeFlavorIdLabel: "mi300x"}
	cpu := workerNode("cpu1", "10.0.0.12")
	cpu.Labels = map[string]string{v1.WorkspaceIdLabel: "dev", v1.NodeFlavorIdLabel: "epyc"}
	r := newPlaneReconciler(t, cluster)

	err := r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{gpu, cpu}, nil)
	assert.ErrorContains(t, err, "dev(epyc,mi300x)")
	for _, list := range []client.ObjectList{&v1.NodeFlavorList{}, &v1.WorkspaceList{}, &v1.NodeList{}} {
		assert.NoError(t, r.List(context.Background(), list))
		assert.Zero(t, meta.LenList(list))
	}
}

func TestEnsureAdoptedNodeClaimsUnboundNode(t *testing.T) {
	cluster := testCluster("ccs300")
	unbound := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu1"}}
	r := newPlaneReconciler(t, unbound)
	assert.NoError(t, r.ensureAdoptedNode(context.Background(), cluster, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu1"}}))
	got := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "gpu1"}, got))
	assert.Equal(t, "ccs300", got.GetSpecCluster())
	assert.Equal(t, "ccs300", v1.GetClusterId(got))
}

func TestSyncAdoptedInventorySkipsInvalidNodes(t *testing.T) {
	cluster := testCluster("ccs300")
	noIP := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a-no-ip"}}
	badName := workerNode("10-0-0-7", "10.0.0.7")
	cp := workerNode("cp1", "10.0.0.1")
	cp.Labels = map[string]string{v1.KubernetesControlPlane: ""}
	r := newPlaneReconciler(t, cluster)
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(),
		[]corev1.Node{noIP, badName, cp}, nil))
	nodes := &v1.NodeList{}
	assert.NoError(t, r.List(context.Background(), nodes))
	assert.Len(t, nodes.Items, 1)
	assert.Equal(t, "cp1", nodes.Items[0].Name)
	got := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300"}, got))
	assert.Equal(t, []string{"cp1"}, got.Spec.ControlPlane.Nodes)
}

func TestSyncAdoptedInventoryRecordsOnlyRegisteredControlPlanes(t *testing.T) {
	cluster := testCluster("ccs300")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Spec.ControlPlane.Nodes = []string{}
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if obj.GetName() == "cp2" {
					return apierrors.NewBadRequest("spec.nodeTemplate: Invalid value")
				}
				return c.Create(ctx, obj, opts...)
			},
		}).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	cp1, cp2 := workerNode("cp1", "10.0.0.1"), workerNode("cp2", "10.0.0.2")
	cp1.Labels = map[string]string{v1.KubernetesControlPlane: ""}
	cp2.Labels = map[string]string{v1.KubernetesControlPlane: ""}
	assert.Error(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), []corev1.Node{cp1, cp2}, nil))
	got := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ccs300"}, got))
	assert.Equal(t, []string{"cp1"}, got.Spec.ControlPlane.Nodes)
}

func TestSyncAdoptedInventoryToleratesStaleCache(t *testing.T) {
	cluster := testCluster("ccs300")
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj.(type) {
				case *v1.NodeFlavor, *v1.Node:
					return apierrors.NewNotFound(v1.Resource("object"), key.Name)
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	k8sNodes := []corev1.Node{workerNode("gpu1", "10.0.0.11"), workerNode("gpu2", "10.0.0.12")}
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), k8sNodes, nil))
	assert.NoError(t, r.syncAdoptedInventory(context.Background(), cluster, fake.NewSimpleClientset(), k8sNodes, nil))
}

func TestAdoptedFlavorRoundsMemoryToGi(t *testing.T) {
	node := func(memory string) *corev1.Node {
		return &corev1.Node{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("192"),
			corev1.ResourceMemory: resource.MustParse(memory),
		}}}
	}
	a := adoptedFlavor(nil, node("2113629180Ki"))
	b := adoptedFlavor(nil, node("2113629184Ki"))
	assert.Equal(t, a.Name, b.Name)
	assert.Equal(t, "adopted-c192-m2015gi", a.Name)
	assert.Equal(t, "2015Gi", a.Spec.Memory.String())
}

func TestAdoptedExtendedResources(t *testing.T) {
	node := &corev1.Node{Status: corev1.NodeStatus{
		Capacity:    corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("943819063296")},
		Allocatable: corev1.ResourceList{"rdma/hca": resource.MustParse("1k")},
	}}
	res := adoptedExtendedResources(node, "rdma/hca")
	storage := res[corev1.ResourceEphemeralStorage]
	rdma := res["rdma/hca"]
	assert.Equal(t, "879Gi", storage.String())
	assert.Equal(t, "1k", rdma.String())
	_, ok := adoptedExtendedResources(node, "")["rdma/hca"]
	assert.False(t, ok)
}

func TestEnsureAdoptedFlavorAddsMissingResources(t *testing.T) {
	existing := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "f1"}, Spec: v1.NodeFlavorSpec{
		Memory:          resource.MustParse("10Gi"),
		ExtendResources: corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("100Gi")},
	}}
	r := newPlaneReconciler(t, existing)
	desired := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "f1"}, Spec: v1.NodeFlavorSpec{
		Memory: resource.MustParse("20Gi"),
		ExtendResources: corev1.ResourceList{
			corev1.ResourceEphemeralStorage: resource.MustParse("879Gi"),
			"rdma/hca":                      resource.MustParse("1k"),
		},
	}}
	assert.NoError(t, r.ensureAdoptedFlavor(context.Background(), desired))
	got := &v1.NodeFlavor{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "f1"}, got))
	storage := got.Spec.ExtendResources[corev1.ResourceEphemeralStorage]
	rdma := got.Spec.ExtendResources["rdma/hca"]
	assert.Equal(t, "100Gi", storage.String())
	assert.Equal(t, "1k", rdma.String())
	assert.Equal(t, "10Gi", got.Spec.Memory.String())
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

func TestAdoptedClusterDeletionReleasesAdminRecordsOnly(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Spec.ControlPlane.Nodes = []string{"cp1"}
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cluster.DeletionTimestamp = &now
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase

	adminNode := func(name string) *v1.Node {
		n := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
		n.Spec.Cluster = pointer.String("c1")
		n.Status.MachineStatus.HostName = name
		return n
	}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	addon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "c1-a1"},
		Spec: v1.AddonSpec{Cluster: &corev1.ObjectReference{Name: "c1"}}}
	otherAddon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "c2-a1"},
		Spec: v1.AddonSpec{Cluster: &corev1.ObjectReference{Name: "c2"}}}

	cs := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "w1"}})
	r := newClusterReconcilerFull(t, cs, cluster, adminNode("cp1"), adminNode("w1"), ws, addon, otherAddon)

	for i := 0; i < 10; i++ {
		_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
		assert.NoError(t, err)
	}

	assert.True(t, apierrors.IsNotFound(r.Get(ctx, client.ObjectKey{Name: "c1"}, &v1.Cluster{})))
	nodes := &v1.NodeList{}
	assert.NoError(t, r.List(ctx, nodes))
	assert.Empty(t, nodes.Items)
	assert.True(t, apierrors.IsNotFound(r.Get(ctx, client.ObjectKey{Name: "ws1"}, &v1.Workspace{})))
	assert.True(t, apierrors.IsNotFound(r.Get(ctx, client.ObjectKey{Name: "c1-a1"}, &v1.Addon{})))
	assert.NoError(t, r.Get(ctx, client.ObjectKey{Name: "c2-a1"}, &v1.Addon{}))

	pods := &corev1.PodList{}
	assert.NoError(t, r.List(ctx, pods))
	assert.Empty(t, pods.Items)
	k8sNodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, k8sNodes.Items, 2)
}
