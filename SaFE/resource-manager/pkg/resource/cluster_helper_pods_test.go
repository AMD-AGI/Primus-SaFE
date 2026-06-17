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
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func testHostsContent() *HostTemplateContent {
	return &HostTemplateContent{
		ClusterName:   "c1",
		PodHostsAlias: map[string]string{"host1": "1.2.3.4"},
	}
}

func testCluster(name string) *v1.Cluster {
	c := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
	c.APIVersion = "amd.com/v1"
	c.Kind = "Cluster"
	return c
}

func TestKubesprayPatchConntrackModprobeWhen(t *testing.T) {
	out := kubesprayPatchConntrackModprobeWhen()
	assert.Contains(t, out, "cd /kubespray")
	assert.Contains(t, out, "modprobe_conntrack_module")
}

func TestGenerateWorkerPod(t *testing.T) {
	cluster := testCluster("c1")
	pod := generateWorkerPod(v1.ClusterCreateAction, cluster, "root", "echo hi", "img:1", "cm1", testHostsContent())
	assert.Equal(t, "c1-"+string(v1.ClusterCreateAction), pod.Name)
	assert.Equal(t, common.PrimusSafeNamespace, pod.Namespace)
	assert.Len(t, pod.Spec.Containers, 1)
	assert.Equal(t, "img:1", pod.Spec.Containers[0].Image)
	assert.Len(t, pod.Spec.HostAliases, 1)
}

func TestGenerateScaleWorkerPod(t *testing.T) {
	cluster := testCluster("c1")
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	pod := generateScaleWorkerPod(v1.ClusterScaleUpAction, cluster, node, "root", "cmd", "img:1", "cm1", testHostsContent())
	assert.Equal(t, "n1", pod.Labels[v1.ClusterManageNodeLabel])
	assert.True(t, len(pod.OwnerReferences) >= 1)
}

func TestGetAdminCluster(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	got, err := getAdminCluster(context.Background(), cl, "c1")
	assert.NoError(t, err)
	assert.Equal(t, "c1", got.Name)

	// Empty id -> nil, nil.
	got, err = getAdminCluster(context.Background(), cl, "")
	assert.NoError(t, err)
	assert.Nil(t, got)

	// Missing -> error.
	_, err = getAdminCluster(context.Background(), cl, "missing")
	assert.Error(t, err)
}

func TestGenerateHosts(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"node1"}

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	node.Spec.PrivateIP = "10.0.0.1"
	node.Spec.PublicIP = "1.2.3.4"
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.MachineStatus.HostName = "host1"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"username": []byte("root")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(node, secret).Build()
	r := &ClusterBaseReconciler{Client: cl}

	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	assert.NoError(t, err)
	assert.Equal(t, "c1", hosts.ClusterName)
	assert.Len(t, hosts.MasterName, 1)
}

func TestGuaranteeHostsConfigMapCreated(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ClusterBaseReconciler{Client: cl}
	owner := metav1.OwnerReference{APIVersion: "v1", Kind: "Pod", Name: "p1"}
	hosts := &HostTemplateContent{ClusterName: "c1"}
	cm, err := r.guaranteeHostsConfigMapCreated(context.Background(), "c1", owner, hosts)
	assert.NoError(t, err)
	assert.Equal(t, "c1", cm.Name)
	// Second call updates the existing configmap.
	cm, err = r.guaranteeHostsConfigMapCreated(context.Background(), "c1", owner, hosts)
	assert.NoError(t, err)
	assert.NotNil(t, cm)
}

func TestClusterGetUsernameFromNodeSecret(t *testing.T) {
	scheme, _ := genMockScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "node-ssh", Namespace: "ns"},
		Data:       map[string][]byte{"username": []byte("nodeuser")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	r := &ClusterBaseReconciler{Client: cl}
	node := &v1.Node{}
	node.Spec.SSHSecret = &corev1.ObjectReference{Name: "node-ssh", Namespace: "ns"}
	cluster := testCluster("c1")
	username, err := r.getUsername(context.Background(), node, cluster)
	assert.NoError(t, err)
	assert.Equal(t, "nodeuser", username)
}

func TestGenerateHostsNodeNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"node1"}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &ClusterBaseReconciler{Client: cl}
	// Node not ready -> error.
	_, err := r.generateHosts(context.Background(), cluster, nil)
	assert.Error(t, err)
}
