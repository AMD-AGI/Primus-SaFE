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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// TestWorkloadMutateSecretsClusterConfig covers cluster default image secret injection.
func TestWorkloadMutateSecretsClusterConfig(t *testing.T) {
	commonconfig.SetValue("global.image_secret", "imgsec")
	defer commonconfig.SetValue("global.image_secret", "")
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := validWorkload()
	m.mutateSecrets(context.Background(), w, nil)
	assert.Equal(t, len(w.Spec.Secrets), 1)
}

// TestWorkloadMutateRdmaResourceEnabled covers the rdma assignment branch.
func TestWorkloadMutateRdmaResourceEnabled(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	scheme := newScheme(t)
	flavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "flavor1"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
			Gpu:    &v1.GpuChip{ResourceName: common.AmdGpu, Quantity: resource.MustParse("8")},
			ExtendResources: corev1.ResourceList{
				"rdma/hca": resource.MustParse("4"),
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), w)
	assert.Equal(t, w.Spec.Resources[0].RdmaResource, "4")
}

// TestWorkloadValidateResourceEnoughEphemeral covers the ephemeral storage limit branch.
func TestWorkloadValidateResourceEnoughEphemeral(t *testing.T) {
	commonconfig.SetValue("workload.max_ephemeral_store_percent", "0.99")
	defer commonconfig.SetValue("workload.max_ephemeral_store_percent", "0")
	nf := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
		ExtendResources: corev1.ResourceList{
			corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
		},
	}}
	res := &v1.WorkloadResource{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}
	assert.NilError(t, validateResourceEnough(nf, res))
}

// TestClusterValidateControlPlaneErrors covers control plane field error branches.
func TestClusterValidateControlPlaneErrors(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient}

	noSubnet := validControlPlaneCluster()
	noSubnet.Spec.ControlPlane.KubePodsSubnet = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noSubnet) != nil)

	noSvc := validControlPlaneCluster()
	noSvc.Spec.ControlPlane.KubeServiceAddress = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noSvc) != nil)

	noDNS := validControlPlaneCluster()
	noDNS.Spec.ControlPlane.NodeLocalDNSIP = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noDNS) != nil)

	noImg := validControlPlaneCluster()
	noImg.Spec.ControlPlane.KubeSprayImage = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noImg) != nil)
}

// TestWorkspaceValidateVolumesErrors covers volume validation error branches.
func TestWorkspaceValidateVolumesErrors(t *testing.T) {
	v := &WorkspaceValidator{}
	badType := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: "invalid", MountPath: "/x"},
	}}}
	assert.Assert(t, v.validateVolumes(badType, nil) != nil)

	noStorage := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.PFS, MountPath: "/x"},
	}}}
	assert.Assert(t, v.validateVolumes(noStorage, nil) != nil)

	noCapacity := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.PFS, MountPath: "/x", StorageClass: "sc"},
	}}}
	assert.Assert(t, v.validateVolumes(noCapacity, nil) != nil)
}

// TestNodeFlavorValidateCommonDisks covers disk and extend-resource validation branches.
func TestNodeFlavorValidateCommonDisks(t *testing.T) {
	v := &NodeFlavorValidator{}
	base := func() *v1.NodeFlavor {
		return &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
		}}
	}
	badRoot := base()
	badRoot.Spec.RootDisk = &v1.DiskFlavor{Count: 0}
	assert.Assert(t, v.validateCommon(badRoot) != nil)

	badData := base()
	badData.Spec.DataDisk = &v1.DiskFlavor{Count: 0}
	assert.Assert(t, v.validateCommon(badData) != nil)

	badEph := base()
	badEph.Spec.ExtendResources = corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("0")}
	assert.Assert(t, v.validateCommon(badEph) != nil)

	okDisk := base()
	okDisk.Spec.RootDisk = &v1.DiskFlavor{Count: 1, Quantity: resource.MustParse("100Gi")}
	okDisk.Spec.DataDisk = &v1.DiskFlavor{Count: 2, Quantity: resource.MustParse("200Gi")}
	assert.NilError(t, v.validateCommon(okDisk))
}

// TestWorkloadValidateServiceErrors covers service validation error branches.
func TestWorkloadValidateServiceErrors(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	badProto := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: "ICMP", ServiceType: corev1.ServiceTypeClusterIP,
	}}}
	assert.Assert(t, v.validateService(context.Background(), badProto) != nil)

	nodePortMissing := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeNodePort,
	}}}
	assert.Assert(t, v.validateService(context.Background(), nodePortMissing) != nil)
}

// TestOpsJobValidateOnCreationTypes covers the create validation switch arms.
func TestOpsJobValidateOnCreationTypes(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	dumplog := opsJobWithDisplayName("job1", v1.OpsJobDumpLogType)
	dumplog.Spec.Inputs = []v1.Parameter{{Name: v1.ParameterWorkload, Value: "w1"}}
	assert.NilError(t, v.validateOnCreation(context.Background(), dumplog))

	download := opsJobWithDisplayName("job2", v1.OpsJobDownloadType)
	download.Spec.Inputs = []v1.Parameter{
		{Name: v1.ParameterEndpoint, Value: "http://x"},
		{Name: v1.ParameterDestPath, Value: "/data"},
		{Name: v1.ParameterSecret, Value: "secret"},
		{Name: v1.ParameterWorkspace, Value: "ws1"},
	}
	assert.NilError(t, v.validateOnCreation(context.Background(), download))
}

// TestFaultMutateOnCreationWithOwner covers owner reference assignment from node.
func TestFaultMutateOnCreationWithOwner(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", UID: "uid-1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &FaultMutator{Client: k8sClient}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "fault1"},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Node:      &v1.FaultNode{ClusterName: "cluster1", AdminName: "node1"},
		},
	}
	m.mutateOnCreation(context.Background(), fault)
	assert.Assert(t, len(fault.OwnerReferences) > 0)
}

// TestUserValidateOnUpdateAccessRemoved covers the update validation chain.
func TestUserValidateOnUpdateAccessRemoved(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultRole()).Build()
	v := &UserValidator{Client: k8sClient}
	oldUser := validUser("u1")
	newUser := validUser("u1")
	assert.NilError(t, v.validateOnUpdate(context.Background(), newUser, oldUser))
}

// TestNodeMutatorHandleUpdateChanged covers the node mutator update patch path.
func TestNodeMutatorHandleUpdateChanged(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient, decoder: newDecoder(t)}
	oldNode := validNode()
	newNode := validNode()
	v1.SetLabel(newNode, v1.NodeGpuCountLabel, "8")
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, newNode, oldNode))
	assert.Assert(t, resp.Allowed)
}

// TestUserMutatorHandleUpdate covers the user mutator update patch path.
func TestUserMutatorHandleUpdate(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &UserMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, validUser("u1"), validUser("u1")))
	assert.Assert(t, resp.Allowed)
}

// TestFaultValidatorHandleDecodeError covers fault validator decode-error path.
func TestFaultValidatorHandleDecodeError(t *testing.T) {
	v := &FaultValidator{decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestNodeValidateNodeWorkspace covers node workspace existence validation.
func TestNodeValidateNodeWorkspace(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &NodeValidator{Client: k8sClient}
	node := validNode()
	node.Spec.Workspace = pointer.String("ws1")
	assert.NilError(t, v.validateNodeWorkspace(context.Background(), node))

	missing := validNode()
	missing.Spec.Workspace = pointer.String("missing")
	assert.Assert(t, v.validateNodeWorkspace(context.Background(), missing) != nil)
}
