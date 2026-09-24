/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	testifyassert "github.com/stretchr/testify/assert"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

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

func wsForHelper(name string) *v1.Workspace {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	ws.Spec.Cluster = "c1"
	return ws
}

func TestServiceAccountLifecycleFull(t *testing.T) {
	commonconfig.SetValue("cicd.enable", "true")
	commonconfig.SetValue("monarch.enable", "true")
	commonconfig.SetValue("monarch.client_role", "monarch-sa")
	defer commonconfig.SetValue("cicd.enable", "")
	defer commonconfig.SetValue("monarch.enable", "")
	defer commonconfig.SetValue("monarch.client_role", "")

	cs := k8sfake.NewSimpleClientset()
	ws := wsForHelper("ws1")
	ctx := context.Background()

	testifyassert.NoError(t, createCICDServiceAccount(ctx, ws, cs))
	testifyassert.NoError(t, createMonarchServiceAccount(ctx, ws, cs))
	// idempotent second call
	testifyassert.NoError(t, createMonarchServiceAccount(ctx, ws, cs))

	rb, err := cs.RbacV1().RoleBindings("ws1").Get(ctx, "monarch-sa", metav1.GetOptions{})
	testifyassert.NoError(t, err)
	assert.Equal(t, "monarch-sa", rb.Name)

	testifyassert.NoError(t, deleteMonarchServiceAccount(ctx, ws, cs))
	testifyassert.NoError(t, deleteCICDServiceAccount(ctx, ws, cs))
}

func TestGithubRunnerServiceAccountLifecycle(t *testing.T) {
	commonconfig.SetValue("cicd.enable", "true")
	defer commonconfig.SetValue("cicd.enable", "")

	cs := k8sfake.NewSimpleClientset()
	ws := wsForHelper("ws1")
	ctx := context.Background()

	testifyassert.NoError(t, createGithubRunnerServiceAccount(ctx, ws, cs))
	testifyassert.NoError(t, createGithubRunnerServiceAccount(ctx, ws, cs))

	_, err := cs.CoreV1().ServiceAccounts("ws1").Get(ctx, common.GithubRunnerServiceAccount, metav1.GetOptions{})
	testifyassert.NoError(t, err)
	rb, err := cs.RbacV1().RoleBindings("ws1").Get(ctx, common.GithubRunnerServiceAccount, metav1.GetOptions{})
	testifyassert.NoError(t, err)
	assert.Equal(t, common.GithubRunnerServiceAccount, rb.RoleRef.Name)
	assert.Equal(t, common.ClusterRoleKind, rb.RoleRef.Kind)

	testifyassert.NoError(t, deleteGithubRunnerServiceAccount(ctx, ws, cs))
}

func TestGetPvTemplateAndCreateDataPlanePv(t *testing.T) {
	pvYaml := `apiVersion: v1
kind: PersistentVolume
metadata:
  name: placeholder
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  hostPath:
    path: /data
`
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        common.PrimusPvmName,
			Namespace:   common.PrimusSafeNamespace,
			Labels:      map[string]string{v1.DisplayNameLabel: "pvtmpl"},
			Annotations: map[string]string{"primus-safe.workspace.auto-create-pv": v1.TrueStr},
		},
		Data: map[string]string{"template": pvYaml},
	}
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	ws := wsForHelper("ws1")
	v1.SetLabel(ws, v1.DisplayNameLabel, "wsdisp")
	ctx := context.Background()

	tmpl, err := getPvTemplate(ctx, cl, ws)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, tmpl)

	cs := k8sfake.NewSimpleClientset()
	testifyassert.NoError(t, createDataPlanePv(ctx, ws, cl, cs))
	pvs, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	testifyassert.NoError(t, err)
	testifyassert.Len(t, pvs.Items, 1)
}

func TestGetPvTemplateNoConfigMap(t *testing.T) {
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	tmpl, err := getPvTemplate(context.Background(), cl, wsForHelper("ws1"))
	testifyassert.NoError(t, err)
	testifyassert.Nil(t, tmpl)
}

func TestCreateAndDeleteServiceAccount(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	testifyassert.NoError(t, createServiceAccount(context.Background(), ws, cs, "sa1"))
	// Created.
	_, err := cs.CoreV1().ServiceAccounts("ws1").Get(context.Background(), "sa1", metav1.GetOptions{})
	testifyassert.NoError(t, err)
	// Idempotent create.
	testifyassert.NoError(t, createServiceAccount(context.Background(), ws, cs, "sa1"))
	// Delete.
	testifyassert.NoError(t, deleteServiceAccount(context.Background(), ws, cs, "sa1"))
	// Delete again -> not found ignored.
	testifyassert.NoError(t, deleteServiceAccount(context.Background(), ws, cs, "sa1"))
}

func TestCreateAndDeleteMonarchRoleBinding(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	testifyassert.NoError(t, createMonarchRoleBinding(context.Background(), ws, cs, "role1"))
	_, err := cs.RbacV1().RoleBindings("ws1").Get(context.Background(), "role1", metav1.GetOptions{})
	testifyassert.NoError(t, err)
	// Idempotent.
	testifyassert.NoError(t, createMonarchRoleBinding(context.Background(), ws, cs, "role1"))
	// Delete.
	testifyassert.NoError(t, deleteRoleBinding(context.Background(), ws, cs, "role1"))
	testifyassert.NoError(t, deleteRoleBinding(context.Background(), ws, cs, "role1"))
}

func TestCICDAndMonarchServiceAccountGated(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	// CI/CD and Monarch are disabled by default -> these return nil without touching cluster.
	testifyassert.NoError(t, createCICDServiceAccount(context.Background(), ws, cs))
	testifyassert.NoError(t, deleteCICDServiceAccount(context.Background(), ws, cs))
	testifyassert.NoError(t, createGithubRunnerServiceAccount(context.Background(), ws, cs))
	testifyassert.NoError(t, deleteGithubRunnerServiceAccount(context.Background(), ws, cs))
	testifyassert.NoError(t, createMonarchServiceAccount(context.Background(), ws, cs))
	testifyassert.NoError(t, deleteMonarchServiceAccount(context.Background(), ws, cs))
}
