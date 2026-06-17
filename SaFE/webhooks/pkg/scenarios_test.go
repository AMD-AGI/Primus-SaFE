/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestClusterValidatorHandleBranches covers update and decode-error handler branches.
func TestClusterValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient, decoder: newDecoder(t)}

	oldCluster := validControlPlaneCluster()
	newCluster := validControlPlaneCluster()
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, newCluster, oldCluster))
	assert.Assert(t, resp.Allowed)

	// decode error path
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestNodeHandleBranches covers node handler update and decode-error branches.
func TestNodeHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, validNode(), validNode()))
	assert.Assert(t, resp.Allowed)

	v := &NodeValidator{Client: k8sClient, decoder: newDecoder(t)}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, validNode(), validNode()))
	assert.Assert(t, resp.Allowed)
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestOpsJobValidatorHandleBranches covers ops job handler update and decode-error branches.
func TestOpsJobValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &OpsJobValidator{Client: k8sClient, decoder: newDecoder(t)}
	job := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, job, job))
	assert.Assert(t, resp.Allowed)
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestWorkspaceHandleDecodeError covers workspace handler decode-error branches.
func TestWorkspaceHandleDecodeError(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkspaceMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// richWorkspace builds a workspace that exercises gpu/manager/default mutation branches.
func richWorkspace() *v1.Workspace {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{
			Cluster:     "cluster1",
			NodeFlavor:  "flavor1",
			Replica:     2,
			QueuePolicy: v1.QueueFifoPolicy,
			IsDefault:   true,
			Managers:    []string{"u1"},
			Volumes: []v1.WorkspaceVolume{
				{Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
			},
		},
	}
	v1.SetLabel(ws, v1.ClusterIdLabel, "cluster1")
	v1.SetLabel(ws, v1.DisplayNameLabel, "my-ws")
	return ws
}

// TestWorkspaceRichMutateAndValidate covers gpu/manager/default workspace branches.
func TestWorkspaceRichMutateAndValidate(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	flavor := gpuFlavor("flavor1")
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, flavor, user).Build()

	m := &WorkspaceMutator{Client: k8sClient}
	assert.NilError(t, m.mutateOnCreation(context.Background(), richWorkspace()))

	v := &WorkspaceValidator{Client: k8sClient}
	assert.NilError(t, v.validateOnCreation(context.Background(), richWorkspace()))
}

// TestNodeRichMutate covers node label/subnet/flavor mutation branches.
func TestNodeRichMutate(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
		Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{
			KubePodsSubnet: pointer.String("10.0.0.0/16"),
		}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient}
	node := validNode()
	node.Spec.Cluster = pointer.String("cluster1")
	v1.SetLabel(node, v1.NodeFlavorIdLabel, "flavor1")
	assert.Assert(t, m.mutateOnCreation(context.Background(), node))
	assert.Equal(t, v1.GetGpuResourceName(node), common.AmdGpu)
}

// TestWorkloadValidateServiceNodePort covers nodePort service validation branches.
func TestWorkloadValidateServiceNodePort(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, NodePort: 30080,
		Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeNodePort,
	}}}
	assert.NilError(t, v.validateService(context.Background(), w))

	badType := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: corev1.ProtocolTCP, ServiceType: "Bad",
	}}}
	assert.Assert(t, v.validateService(context.Background(), badType) != nil)
}

// TestWorkloadValidateWorkspaceQuota covers the quota-insufficient branch.
func TestWorkloadValidateWorkspaceQuota(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 0}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkloadValidator{Client: k8sClient}
	w := validWorkload()
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 5, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}}
	assert.Assert(t, v.validateWorkspace(context.Background(), w) != nil)
}

// TestWorkloadValidateResourceEnoughWithFlavor covers per-node resource validation with a flavor.
func TestWorkloadValidateResourceEnoughWithFlavor(t *testing.T) {
	scheme := newScheme(t)
	flavor := gpuFlavor("flavor1")
	flavor.Spec.Gpu = nil
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	v := &WorkloadValidator{Client: k8sClient}
	w := validWorkload()
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "2Gi", SharedMemory: "1Gi", EphemeralStorage: "3Gi"}}
	assert.NilError(t, v.validateResourceEnough(context.Background(), w))
}

// TestWorkloadMutateSecretsClusterSecret covers the secret dedup branch.
func TestWorkloadMutateSecretsClusterSecret(t *testing.T) {
	scheme := newScheme(t)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: common.PrimusSafeNamespace}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Secrets: []v1.SecretEntity{
		{Id: "sec1"}, {Id: "sec1"}, {Id: "missing"},
	}}}
	m.mutateSecrets(context.Background(), w, nil)
	assert.Equal(t, len(w.Spec.Secrets), 1)
}

// TestWorkloadMutateRdmaResourceWithFlavor covers the rdma loop over gpu resources.
func TestWorkloadMutateRdmaResourceWithFlavor(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), w)
	assert.Equal(t, w.Spec.Resources[0].RdmaResource, "")
}

// TestWorkspaceMutateScaleDownActual covers the scale-down node selection error branch.
func TestWorkspaceMutateScaleDownActual(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 3}}
	oldWs.Status.AvailableReplica = 3
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 1}}
	// not enough nodes available -> error
	assert.Assert(t, m.mutateScaleDown(context.Background(), oldWs, newWs) != nil)
}
