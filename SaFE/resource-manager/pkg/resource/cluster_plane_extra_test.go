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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func newPlaneReconciler(t *testing.T, objs ...client.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Cluster{}).
		WithObjects(objs...).
		Build()
	return &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
}

func TestGetControllerPlaneNodes(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := newPlaneReconciler(t, node)
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	nodes, err := r.getControllerPlaneNodes(context.Background(), cluster)
	assert.NoError(t, err)
	assert.Len(t, nodes, 1)

	// Missing node -> error.
	cluster.Spec.ControlPlane.Nodes = []string{"missing"}
	_, err = r.getControllerPlaneNodes(context.Background(), cluster)
	assert.Error(t, err)
}

func TestGuaranteeNamespace(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newPlaneReconciler(t)
	// Create namespace.
	assert.NoError(t, r.guaranteeNamespace(context.Background(), cs, "ns1"))
	_, err := cs.CoreV1().Namespaces().Get(context.Background(), "ns1", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent.
	assert.NoError(t, r.guaranteeNamespace(context.Background(), cs, "ns1"))
}

func TestGuaranteeEndpoints(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	nodes := []*v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}}
	assert.NoError(t, r.guaranteeEndpoints(context.Background(), cluster, nodes))
	ep := &corev1.Endpoints{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1", Namespace: common.PrimusSafeNamespace}, ep))
	// Already exists -> no-op.
	assert.NoError(t, r.guaranteeEndpoints(context.Background(), cluster, nodes))
}

func TestGuaranteeServiceResource(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	assert.NoError(t, r.guaranteeServiceResource(context.Background(), cluster))
	svc := &corev1.Service{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1", Namespace: common.PrimusSafeNamespace}, svc))
	// Already exists -> no-op.
	assert.NoError(t, r.guaranteeServiceResource(context.Background(), cluster))
}

func TestGuaranteeServiceNotReady(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	r := newPlaneReconciler(t, cluster)
	// Not ready -> no-op nil.
	assert.NoError(t, r.guaranteeService(context.Background(), cluster))
}

func TestGuaranteeServiceReady(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}
	r := newPlaneReconciler(t, cluster, node)
	assert.NoError(t, r.guaranteeService(context.Background(), cluster))
}

func TestUpdatePodStatus(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)

	succeeded := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
	assert.NoError(t, r.updatePodStatus(context.Background(), cluster, succeeded))
	assert.Equal(t, v1.CreatedPhase, cluster.Status.ControlPlaneStatus.Phase)

	failed := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	assert.NoError(t, r.updatePodStatus(context.Background(), cluster, failed))
	assert.Equal(t, v1.CreationFailed, cluster.Status.ControlPlaneStatus.Phase)
}

func TestUpdateResetPhase(t *testing.T) {
	r := newPlaneReconciler(t)
	cluster := testCluster("c1")

	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}})
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)

	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}})
	assert.Equal(t, v1.DeleteFailedPhase, cluster.Status.ControlPlaneStatus.Phase)

	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	assert.Equal(t, v1.DeletingPhase, cluster.Status.ControlPlaneStatus.Phase)
}

func TestPlaneGetUsernameNoNodes(t *testing.T) {
	r := newPlaneReconciler(t)
	_, err := r.getUsername(context.Background(), testCluster("c1"))
	assert.Error(t, err)
}

func TestGuaranteeClusterControlPlaneNoNodes(t *testing.T) {
	r := newPlaneReconciler(t)
	// No control plane nodes -> nil.
	assert.NoError(t, r.guaranteeClusterControlPlane(context.Background(), testCluster("c1")))
}

func TestResetNilHostsContent(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	// Nil hostsContent -> phase set to Deleted.
	assert.NoError(t, r.reset(context.Background(), cluster, nil))
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
}

func TestPatchKubeControlPlanNodes(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1", "missing"}
	r := newPlaneReconciler(t, node, cluster)
	assert.NoError(t, r.patchKubeControlPlanNodes(context.Background(), cluster))
	updated := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, updated))
	assert.Equal(t, "c1", updated.GetSpecCluster())
}

func planeClusterWithNode(t *testing.T) (*v1.Cluster, *ClusterReconciler) {
	t.Helper()
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.PrivateIP = "10.0.0.1"
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.MachineStatus.HostName = "host1"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"username": []byte("root")},
	}
	r := newPlaneReconciler(t, cluster, node, secret)
	return cluster, r
}

func TestCreateNewWorkerPod(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	pod, err := r.createNewWorkerPod(context.Background(), cluster, hosts)
	assert.NoError(t, err)
	assert.NotNil(t, pod)
}

func TestCreateResetPod(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	pod, err := r.createResetPod(context.Background(), cluster, hosts)
	assert.NoError(t, err)
	assert.NotNil(t, pod)
}

func TestGuaranteeCreateWorkerPodCreated(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	pod, err := r.guaranteeCreateWorkerPodCreated(context.Background(), cluster, hosts)
	assert.NoError(t, err)
	assert.NotNil(t, pod)
}

func TestGuaranteeResetWorkPodCreated(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	pod, err := r.guaranteeResetWorkPodCreated(context.Background(), cluster, hosts)
	assert.NoError(t, err)
	assert.NotNil(t, pod)
}

func TestClearPods(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	// No pods -> nil.
	assert.NoError(t, r.clearPods(context.Background(), cluster))
}

func TestGuaranteeDefaultAddonCreatesAddon(t *testing.T) {
	cluster := testCluster("c1")
	template := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "optimus.0.1.4",
			Labels: map[string]string{v1.AddonDefaultLabel: ""},
		},
	}
	r := newPlaneReconciler(t, cluster, template)
	res, err := r.guaranteeDefaultAddon(context.Background(), cluster)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
	// Addon should be created with name "c1-optimus".
	addon := &v1.Addon{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1-optimus"}, addon))
}

func TestUpdateClusterKubeConfigNilConfig(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	// nil restConfig -> no-op nil.
	assert.NoError(t, r.updateClusterKubeConfig(context.Background(), cluster, nil, nil))
}

func TestUpdateClusterKubeConfig(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cs := k8sfakeClientset()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Cluster{}).
		WithObjects(cluster).
		Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs}}
	nodes := []*v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}}
	cfg := &rest.Config{}
	cfg.CertData = []byte("cert")
	cfg.CAData = []byte("ca")
	cfg.KeyData = []byte("key")
	err := r.updateClusterKubeConfig(context.Background(), cluster, nodes, cfg)
	assert.NoError(t, err)
	assert.Equal(t, v1.ReadyPhase, cluster.Status.ControlPlaneStatus.Phase)
	assert.Len(t, cluster.Status.ControlPlaneStatus.Endpoints, 1)
}

func TestResetWithHostsContent(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	// reset with hostsContent + non-deleted/failed phase -> creates reset pod.
	err = r.reset(context.Background(), cluster, hosts)
	assert.NoError(t, err)
}

func TestResetCreationFailedPhase(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.Status.ControlPlaneStatus.Phase = v1.CreationFailed
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	// CreationFailed -> sets DeletedPhase.
	err = r.reset(context.Background(), cluster, hosts)
	assert.NoError(t, err)
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
}

func TestHandleControlPlaneCreation(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	// No SSHSecret on cluster -> generateSSHSecret creates one, then worker pod.
	err := r.handleControlPlaneCreation(context.Background(), cluster)
	assert.NoError(t, err)
	// Worker pod should now exist.
	pod := &corev1.Pod{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{
		Name: "c1-" + string(v1.ClusterCreateAction), Namespace: common.PrimusSafeNamespace,
	}, pod))
}

func TestCreateControlPlanePod(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	err = r.createControlPlanePod(context.Background(), cluster, hosts)
	assert.NoError(t, err)
}

func TestHandleExistingPodOwned(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.UID = "uid-1"
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: common.PrimusSafeNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: cluster.Kind, UID: cluster.UID},
			},
		},
	}
	got, err := r.handleExistingPod(context.Background(), cluster, pod, hosts)
	assert.NoError(t, err)
	assert.NotNil(t, got)
}

func TestGuaranteeDefaultAddonNoTemplates(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	res, err := r.guaranteeDefaultAddon(context.Background(), cluster)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}
