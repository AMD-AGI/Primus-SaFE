/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func alwaysReachable(_ string, _ time.Duration) bool { return true }

func newTestClusterReconciler(clusters []*v1.Cluster, nodes []*v1.Node, endpoints []*corev1.Endpoints) *ClusterReconciler {
	scheme, _ := genMockScheme()
	clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Cluster{})

	for _, c := range clusters {
		clientBuilder = clientBuilder.WithObjects(c)
	}
	for _, n := range nodes {
		clientBuilder = clientBuilder.WithObjects(n)
	}
	for _, ep := range endpoints {
		clientBuilder = clientBuilder.WithObjects(ep)
	}

	adminClient := clientBuilder.Build()

	fakeClientSet := k8sfake.NewSimpleClientset()
	ctx := context.Background()
	for _, ep := range endpoints {
		if ep.Namespace == "" {
			ep.Namespace = common.PrimusSafeNamespace
		}
		_, _ = fakeClientSet.CoreV1().Endpoints(ep.Namespace).Create(ctx, ep, metav1.CreateOptions{})
	}

	return &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client:    adminClient,
			clientSet: fakeClientSet,
		},
		endpointProber: alwaysReachable,
	}
}

func makeNode(name, ip string, machinePhase v1.NodePhase, clusterPhase v1.NodePhase) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.NodeSpec{
			PrivateIP: ip,
		},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{
				Phase: machinePhase,
			},
			ClusterStatus: v1.NodeClusterStatus{
				Phase: clusterPhase,
			},
		},
	}
}

func makeCluster(name string, nodeNames []string) *v1.Cluster {
	return &v1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.ClusterKind,
			APIVersion: "amd.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ClusterSpec{
			ControlPlane: v1.ControlPlane{
				Nodes: nodeNames,
			},
		},
		Status: v1.ClusterStatus{
			ControlPlaneStatus: v1.ControlPlaneStatus{
				Phase: v1.ReadyPhase,
			},
		},
	}
}

func makeClusterWithStatusEndpoints(name string, nodeNames []string, statusEndpoints []string) *v1.Cluster {
	c := makeCluster(name, nodeNames)
	c.Status.ControlPlaneStatus.Endpoints = statusEndpoints
	return c
}

func makeEndpoints(name string, ips []string) *corev1.Endpoints {
	addresses := make([]corev1.EndpointAddress, len(ips))
	for i, ip := range ips {
		addresses[i] = corev1.EndpointAddress{IP: ip}
	}
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: addresses,
			Ports: []corev1.EndpointPort{{
				Name:     "https",
				Port:     6443,
				Protocol: "TCP",
			}},
		}},
	}
}

// TestGuaranteeEndpoints_UpdatesOnNodeHealthChange verifies that guaranteeEndpoints
// removes unhealthy node IPs from existing Endpoints.
func TestGuaranteeEndpoints_UpdatesOnNodeHealthChange(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeSSHFailed, v1.NodeManaged)
	node3 := makeNode("node3", "10.0.0.3", v1.NodeReady, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2", "node3"})

	existingEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})

	nodes := []*v1.Node{node1, node2, node3}
	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster}, nodes, []*corev1.Endpoints{existingEp},
	)

	err := r.guaranteeEndpoints(ctx, cluster, nodes)
	assert.NilError(t, err)

	updated := new(corev1.Endpoints)
	err = r.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: common.PrimusSafeNamespace}, updated)
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Subsets), 1)
	assert.Equal(t, len(updated.Subsets[0].Addresses), 2)

	ips := map[string]bool{}
	for _, a := range updated.Subsets[0].Addresses {
		ips[a.IP] = true
	}
	assert.Assert(t, ips["10.0.0.1"], "healthy node1 should be present")
	assert.Assert(t, !ips["10.0.0.2"], "unhealthy node2 should be removed")
	assert.Assert(t, ips["10.0.0.3"], "healthy node3 should be present")
}

// TestGuaranteeEndpoints_ProbeFiltersUnreachable verifies that nodes passing
// IsMachineReady but failing TCP probe are excluded from Endpoints.
func TestGuaranteeEndpoints_ProbeFiltersUnreachable(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeReady, v1.NodeManaged) // Ready but unreachable
	node3 := makeNode("node3", "10.0.0.3", v1.NodeReady, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2", "node3"})

	existingEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})

	nodes := []*v1.Node{node1, node2, node3}
	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster}, nodes, []*corev1.Endpoints{existingEp},
	)
	// Simulate node2 being unreachable via probe
	r.endpointProber = func(addr string, _ time.Duration) bool {
		return !strings.HasPrefix(addr, "10.0.0.2:")
	}

	err := r.guaranteeEndpoints(ctx, cluster, nodes)
	assert.NilError(t, err)

	updated := new(corev1.Endpoints)
	err = r.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: common.PrimusSafeNamespace}, updated)
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Subsets[0].Addresses), 2)

	ips := map[string]bool{}
	for _, a := range updated.Subsets[0].Addresses {
		ips[a.IP] = true
	}
	assert.Assert(t, ips["10.0.0.1"])
	assert.Assert(t, !ips["10.0.0.2"], "probe-failed node2 should be excluded")
	assert.Assert(t, ips["10.0.0.3"])
}

// TestGuaranteeEndpoints_NoUpdateWhenUnchanged verifies that guaranteeEndpoints
// does not update when all healthy nodes are already in the Endpoints.
func TestGuaranteeEndpoints_NoUpdateWhenUnchanged(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeReady, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2"})

	existingEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2"})

	nodes := []*v1.Node{node1, node2}
	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster}, nodes, []*corev1.Endpoints{existingEp},
	)

	err := r.guaranteeEndpoints(ctx, cluster, nodes)
	assert.NilError(t, err)

	updated := new(corev1.Endpoints)
	err = r.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: common.PrimusSafeNamespace}, updated)
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Subsets[0].Addresses), 2)
}

// TestGuaranteeEndpoints_SkipsUpdateWhenAllUnhealthy verifies that when all nodes
// are unhealthy, the existing Endpoints is not modified.
func TestGuaranteeEndpoints_SkipsUpdateWhenAllUnhealthy(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeSSHFailed, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeSSHFailed, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2"})

	existingEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2"})

	nodes := []*v1.Node{node1, node2}
	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster}, nodes, []*corev1.Endpoints{existingEp},
	)

	err := r.guaranteeEndpoints(ctx, cluster, nodes)
	assert.NilError(t, err)

	updated := new(corev1.Endpoints)
	err = r.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: common.PrimusSafeNamespace}, updated)
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Subsets[0].Addresses), 2)
}

// TestGetClusterEndpoint_FiltersUnhealthyNodes verifies that getClusterEndpoint
// only returns addresses for healthy nodes.
func TestGetClusterEndpoint_FiltersUnhealthyNodes(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeSSHFailed, v1.NodeManaged)
	node3 := makeNode("node3", "10.0.0.3", v1.NodeReady, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2", "node3"})

	srcEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})

	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster},
		[]*v1.Node{node1, node2, node3},
		[]*corev1.Endpoints{srcEp},
	)

	addresses, err := r.getClusterEndpoint(ctx, cluster)
	assert.NilError(t, err)
	assert.Equal(t, len(addresses), 2)

	ips := map[string]bool{}
	for _, a := range addresses {
		ips[a.IP] = true
	}
	assert.Assert(t, ips["10.0.0.1"], "healthy node1 should be returned")
	assert.Assert(t, !ips["10.0.0.2"], "unhealthy node2 should be filtered out")
	assert.Assert(t, ips["10.0.0.3"], "healthy node3 should be returned")
}

// TestGuaranteeForwardEndpoints_RemovesAddress verifies that when an address
// is removed from the source, the forward Endpoints is updated accordingly.
func TestGuaranteeForwardEndpoints_RemovesAddress(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeSSHFailed, v1.NodeManaged)
	node3 := makeNode("node3", "10.0.0.3", v1.NodeReady, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2", "node3"})

	srcEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.3"})

	forwardName := generateForwardName(clusterName)
	forwardEp := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      forwardName,
			Namespace: common.PrimusSafeNamespace,
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.0.0.1"}, {IP: "10.0.0.2"}, {IP: "10.0.0.3"},
			},
			Ports: []corev1.EndpointPort{{
				Name: "http", Port: 80, Protocol: corev1.ProtocolTCP,
			}},
		}},
	}

	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster},
		[]*v1.Node{node1, node2, node3},
		[]*corev1.Endpoints{srcEp, forwardEp},
	)

	err := r.guaranteeForwardEndpoints(ctx, cluster)
	assert.NilError(t, err)

	updated, err := r.clientSet.CoreV1().Endpoints(common.PrimusSafeNamespace).Get(ctx, forwardName, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Subsets), 1)
	assert.Equal(t, len(updated.Subsets[0].Addresses), 2)

	ips := map[string]bool{}
	for _, a := range updated.Subsets[0].Addresses {
		ips[a.IP] = true
	}
	assert.Assert(t, ips["10.0.0.1"], "healthy node1 should remain")
	assert.Assert(t, !ips["10.0.0.2"], "unhealthy node2 should be removed from forward")
	assert.Assert(t, ips["10.0.0.3"], "healthy node3 should remain")
}

// TestGuaranteeForwardEndpoints_ProbeFiltersUnreachablePort80 verifies that
// addresses failing the port 80 probe are excluded from forward Endpoints.
func TestGuaranteeForwardEndpoints_ProbeFiltersUnreachablePort80(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeReady, v1.NodeManaged)
	cluster := makeCluster(clusterName, []string{"node1", "node2"})

	srcEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2"})

	forwardName := generateForwardName(clusterName)
	forwardEp := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      forwardName,
			Namespace: common.PrimusSafeNamespace,
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{
				{IP: "10.0.0.1"}, {IP: "10.0.0.2"},
			},
			Ports: []corev1.EndpointPort{{
				Name: "http", Port: 80, Protocol: corev1.ProtocolTCP,
			}},
		}},
	}

	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster},
		[]*v1.Node{node1, node2},
		[]*corev1.Endpoints{srcEp, forwardEp},
	)
	// node2 port 80 unreachable
	r.endpointProber = func(addr string, _ time.Duration) bool {
		return !strings.HasPrefix(addr, "10.0.0.2:")
	}

	err := r.guaranteeForwardEndpoints(ctx, cluster)
	assert.NilError(t, err)

	updated, err := r.clientSet.CoreV1().Endpoints(common.PrimusSafeNamespace).Get(ctx, forwardName, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Subsets[0].Addresses), 1)
	assert.Equal(t, updated.Subsets[0].Addresses[0].IP, "10.0.0.1")
}

// TestSyncClusterStatusEndpoints verifies that cluster status endpoints
// are synced with the source Endpoints resource.
func TestSyncClusterStatusEndpoints(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)
	node2 := makeNode("node2", "10.0.0.2", v1.NodeReady, v1.NodeManaged)

	// Cluster status has stale 3 endpoints
	cluster := makeClusterWithStatusEndpoints(clusterName,
		[]string{"node1", "node2"},
		[]string{"https://10.0.0.1:6443", "https://10.0.0.2:6443", "https://10.0.0.3:6443"},
	)

	// Source Endpoints already updated to 2 IPs (by guaranteeEndpoints)
	srcEp := makeEndpoints(clusterName, []string{"10.0.0.1", "10.0.0.2"})

	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster},
		[]*v1.Node{node1, node2},
		[]*corev1.Endpoints{srcEp},
	)

	err := r.syncClusterStatusEndpoints(ctx, cluster)
	assert.NilError(t, err)

	// Re-read the cluster to verify status was patched
	updated := new(v1.Cluster)
	err = r.Get(ctx, types.NamespacedName{Name: clusterName}, updated)
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Status.ControlPlaneStatus.Endpoints), 2)

	epSet := map[string]bool{}
	for _, ep := range updated.Status.ControlPlaneStatus.Endpoints {
		epSet[ep] = true
	}
	assert.Assert(t, epSet["https://10.0.0.1:6443"])
	assert.Assert(t, epSet["https://10.0.0.2:6443"])
	assert.Assert(t, !epSet["https://10.0.0.3:6443"], "removed node should not be in status")
}

// TestSyncClusterStatusEndpoints_NoUpdateWhenInSync verifies no patch
// is issued when status endpoints are already in sync.
func TestSyncClusterStatusEndpoints_NoUpdateWhenInSync(t *testing.T) {
	ctx := context.Background()
	clusterName := "test-cluster"

	node1 := makeNode("node1", "10.0.0.1", v1.NodeReady, v1.NodeManaged)

	cluster := makeClusterWithStatusEndpoints(clusterName,
		[]string{"node1"},
		[]string{"https://10.0.0.1:6443"},
	)

	srcEp := makeEndpoints(clusterName, []string{"10.0.0.1"})

	r := newTestClusterReconciler(
		[]*v1.Cluster{cluster},
		[]*v1.Node{node1},
		[]*corev1.Endpoints{srcEp},
	)

	err := r.syncClusterStatusEndpoints(ctx, cluster)
	assert.NilError(t, err)

	updated := new(v1.Cluster)
	err = r.Get(ctx, types.NamespacedName{Name: clusterName}, updated)
	assert.NilError(t, err)
	assert.Equal(t, len(updated.Status.ControlPlaneStatus.Endpoints), 1)
	assert.Equal(t, updated.Status.ControlPlaneStatus.Endpoints[0], "https://10.0.0.1:6443")
}
