/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockdb "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func genMockWorkload(clusterId, workspaceId string) *v1.Workload {
	return &v1.Workload{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.WorkloadKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("workload"),
			Labels: map[string]string{
				v1.WorkspaceIdLabel: workspaceId,
				v1.ClusterIdLabel:   clusterId,
				v1.DisplayNameLabel: "test-workload",
			},
			Annotations: map[string]string{
				v1.MainContainerAnnotation:      "main",
				v1.WorkloadDispatchedAnnotation: "",
			},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace:   workspaceId,
			MaxRetry:    3,
			Priority:    1,
			Images:      []string{"image"},
			EntryPoints: []string{"sh -c test.sh"},
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "kubeflow.org",
				Version: "v1",
				Kind:    "PyTorchJob",
			},
			Resources: []v1.WorkloadResource{{
				Replica: 2,
				CPU:     "16",
				GPU:     "4",
				GPUName: common.AmdGpu,
				Memory:  "1Gi",
			}},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}
}

// Test_modifyWorkload_UpdateResources tests updating workload resources
func Test_modifyWorkload_UpdateResources(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)

	newCPU := "32"
	newMemory := "64Gi"
	newGPU := "8"
	newReplica := 4

	req := &view.PatchWorkloadRequest{
		Resources: &[]v1.WorkloadResource{{
			CPU:     newCPU,
			Memory:  newMemory,
			GPU:     newGPU,
			Replica: newReplica,
		}},
	}

	err := applyWorkloadPatch(workload, req)
	assert.NilError(t, err)
	assert.Equal(t, len(workload.Spec.Resources), 1)
	assert.Equal(t, workload.Spec.Resources[0].CPU, "32")
	assert.Equal(t, workload.Spec.Resources[0].Memory, "64Gi")
	assert.Equal(t, workload.Spec.Resources[0].GPU, "8")
	assert.Equal(t, workload.Spec.Resources[0].Replica, 4)
}

// Test_modifyWorkload_UpdateImage tests updating workload image
func Test_modifyWorkload_UpdateImage(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Images = []string{"old-image:v1"}

	newImage := "new-image:v2"
	req := &view.PatchWorkloadRequest{
		Images: &[]string{newImage},
	}

	err := applyWorkloadPatch(workload, req)
	assert.NilError(t, err)
	assert.Equal(t, len(workload.Spec.Images), 1)
	assert.Equal(t, workload.Spec.Images[0], "new-image:v2")
}

// Test_modifyWorkload_UpdateMultipleFields tests updating multiple fields at once
func Test_modifyWorkload_UpdateMultipleFields(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)

	newPriority := 10
	newCPU := "64"
	newImage := "updated-image:latest"
	newDesc := "updated description"
	newTimeout := 7200

	req := &view.PatchWorkloadRequest{
		Priority: &newPriority,
		Resources: &[]v1.WorkloadResource{{
			CPU: newCPU,
		}},
		Images:      &[]string{newImage},
		Description: &newDesc,
		Timeout:     &newTimeout,
	}

	err := applyWorkloadPatch(workload, req)
	assert.NilError(t, err)
	assert.Equal(t, workload.Spec.Priority, 10)
	assert.Equal(t, len(workload.Spec.Resources), 1)
	assert.Equal(t, workload.Spec.Resources[0].CPU, "64")
	assert.Equal(t, len(workload.Spec.Images), 1)
	assert.Equal(t, workload.Spec.Images[0], "updated-image:latest")
	assert.Equal(t, v1.GetDescription(workload), "updated description")
	assert.Equal(t, *workload.Spec.Timeout, 7200)
}

// Test_modifyWorkload_ReplicaWithSpecifiedNodes tests that replica update fails with specified nodes
func Test_modifyWorkload_ReplicaWithSpecifiedNodes(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.CustomerLabels = map[string]string{
		v1.K8sHostName: "node1 node2",
	}

	newReplica := 5
	req := &view.PatchWorkloadRequest{
		Resources: &[]v1.WorkloadResource{{
			Replica: newReplica,
		}},
	}

	err := applyWorkloadPatch(workload, req)
	assert.Assert(t, err != nil, "Should return error when updating replica with specified nodes")
}

// Test_genCustomerLabelsByNodes tests generating customer labels from node list
func Test_genCustomerLabelsByNodes(t *testing.T) {
	workload := genMockWorkload("cluster1", "workspace1")

	// Test with specified nodes
	nodes := []string{"node1", "node2", "node3"}
	genCustomerLabelsByNodes(workload, nodes, v1.K8sHostName)

	assert.Assert(t, workload.Spec.CustomerLabels != nil)
	assert.Equal(t, workload.Spec.CustomerLabels[v1.K8sHostName], "node1 node2 node3")

	// Test with empty node list
	workload2 := genMockWorkload("cluster1", "workspace1")
	genCustomerLabelsByNodes(workload2, []string{}, v1.K8sHostName)
	assert.Equal(t, len(workload2.Spec.CustomerLabels), 0)

	// Test with excluded nodes
	workload3 := genMockWorkload("cluster1", "workspace1")
	excludedNodes := []string{"bad-node1", "bad-node2"}
	genCustomerLabelsByNodes(workload3, excludedNodes, common.ExcludedNodes)
	assert.Equal(t, workload3.Spec.CustomerLabels[common.ExcludedNodes], "bad-node1 bad-node2")
}

// Test_deleteWorkload tests deleting a workload through handler
func Test_deleteWorkload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-delete"

	user := genMockUser()
	role := genMockRole()

	// Create running workload in etcd
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context with workload name set in context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, workloadId) // deleteWorkload gets name from context
	c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workloads/%s", workloadId), nil)

	// Call deleteWorkload handler (it will call getAndSetUsername and GetRoles internally)
	result, err := h.deleteWorkload(c)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result == nil, "Delete should return nil")

	// Verify workload was deleted from etcd
	deletedWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: workloadId}, deletedWorkload)
	assert.Assert(t, err != nil, "Workload should be deleted from etcd after delete")
}

// Test_updateWorkloadPhase tests updating workload phase
func Test_updateWorkloadPhase(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Status.Phase = v1.WorkloadPending
	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Update phase to running
	err := h.updateWorkloadPhase(ctx, workload, v1.WorkloadRunning, nil)
	assert.NilError(t, err)
	assert.Equal(t, workload.Status.Phase, v1.WorkloadRunning)

	// Update phase to succeeded
	err = h.updateWorkloadPhase(ctx, workload, v1.WorkloadSucceeded, nil)
	assert.NilError(t, err)
	assert.Equal(t, workload.Status.Phase, v1.WorkloadSucceeded)
}

// Test_updateWorkloadPhase_WithCondition tests updating workload phase with condition
func Test_updateWorkloadPhase_WithCondition(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Status.Phase = v1.WorkloadRunning
	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Update phase with condition
	cond := &metav1.Condition{
		Type:               string(v1.AdminStopped),
		Status:             metav1.ConditionTrue,
		Message:            "Stopped by admin",
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "AdminAction",
	}

	err := h.updateWorkloadPhase(ctx, workload, v1.WorkloadStopped, cond)
	assert.NilError(t, err)
	assert.Equal(t, workload.Status.Phase, v1.WorkloadStopped)
	assert.Equal(t, len(workload.Status.Conditions), 1)
	assert.Equal(t, workload.Status.Conditions[0].Type, string(v1.AdminStopped))
	assert.Assert(t, workload.Status.EndTime != nil, "EndTime should be set for stopped workload")
}

// Test_modifyWorkload_EmptyValues tests that empty values are ignored
func Test_modifyWorkload_EmptyValues(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Images = []string{"original-image:v1"}
	workload.Spec.EntryPoints = []string{"original-script.sh"}

	// Try to update with empty strings (should be ignored)
	req := &view.PatchWorkloadRequest{
		Images:      &[]string{},
		EntryPoints: &[]string{},
	}

	err := applyWorkloadPatch(workload, req)
	assert.NilError(t, err)
	// Empty values should be ignored, original values should remain
	assert.Equal(t, workload.Spec.Images[0], "original-image:v1")
	assert.Equal(t, workload.Spec.EntryPoints[0], "original-script.sh")
}

// Test_createPreheatWorkload tests creating preheat workload
func Test_createPreheatWorkload(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	mainWorkload := genMockWorkload(clusterId, workspaceId)
	mainWorkload.Spec.Images = []string{"main-image:v1"}
	mainWorkload.Spec.EntryPoints = []string{"main-script.sh"}
	mainWorkload.Spec.IsSupervised = true
	mainWorkload.Spec.MaxRetry = 5

	user := genMockUser()
	role := genMockRole()
	workspace := genMockWorkspace(clusterId, "")
	workspace.Name = workspaceId
	workspace.Status.AvailableReplica = 3

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(mainWorkload, user, role, workspace).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workload{}).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)

	mainQuery := &view.CreateWorkloadRequest{
		DisplayName: "test-workload",
	}
	roles := []*v1.Role{role}

	preHeatWorkload, err := h.createPreheatWorkload(c, mainWorkload, mainQuery, "test-image", user, roles)
	assert.NilError(t, err)
	assert.Assert(t, preHeatWorkload != nil, "Preheat workload should be created")

	// Verify preheat workload properties
	assert.Assert(t, preHeatWorkload.Name != mainWorkload.Name, "Preheat workload should have different name")
	assert.Equal(t, v1.GetDisplayName(preHeatWorkload), "preheat-"+v1.GetDisplayName(mainWorkload))
	assert.Equal(t, preHeatWorkload.Spec.IsSupervised, false)
	assert.Equal(t, preHeatWorkload.Spec.MaxRetry, 0)
	assert.Equal(t, *preHeatWorkload.Spec.TTLSecondsAfterFinished, 10)
	assert.Assert(t, preHeatWorkload.Spec.CronJobs == nil, "CronJobs should be nil")
	assert.Assert(t, preHeatWorkload.Spec.Dependencies == nil, "Dependencies should be nil")

	// Verify resource requirements are minimal
	assert.Equal(t, len(preHeatWorkload.Spec.Resources), 1)
	assert.Equal(t, preHeatWorkload.Spec.Resources[0].CPU, "1")
	assert.Equal(t, preHeatWorkload.Spec.Resources[0].Memory, "8Gi")
	assert.Equal(t, preHeatWorkload.Spec.Resources[0].EphemeralStorage, "50Gi")
	assert.Equal(t, preHeatWorkload.Spec.Resources[0].Replica, 3) // From workspace.Status.AvailableReplica
}

// Test_createWorkloadImpl tests creating workload implementation
func Test_createWorkloadImpl(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Priority = 5
	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)

	roles := []*v1.Role{role}
	resp, err := h.createWorkloadImpl(c, workload, user, roles)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, resp != nil, "Response should not be nil")
	assert.Equal(t, resp.WorkloadId, workload.Name)

	// Verify workload was created
	createdWorkload := &v1.Workload{}
	err = h.Get(ctx, client.ObjectKey{Name: workload.Name}, createdWorkload)
	assert.NilError(t, err)
	assert.Equal(t, createdWorkload.Name, workload.Name)
	assert.Equal(t, v1.GetUserId(createdWorkload), user.Name)
	assert.Equal(t, v1.GetUserName(createdWorkload), v1.GetUserName(user))

	// Verify phase is set to pending
	assert.Equal(t, createdWorkload.Status.Phase, v1.WorkloadPending)
}

// Test_createWorkloadImpl_WithSecrets tests creating workload with secrets
func Test_createWorkloadImpl_WithSecrets(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()

	// Create a secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(v1.SecretGeneral),
				v1.UserIdLabel:     user.Name,
			},
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Add secret reference to workload
	workload.Spec.Secrets = []v1.SecretEntity{
		{
			Id:   "test-secret",
			Type: v1.SecretGeneral,
		},
	}

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	// Create fake kubernetes clientset with the old secret
	fakeClientSet := k8sfake.NewSimpleClientset(secret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)

	roles := []*v1.Role{role}
	resp, err := h.createWorkloadImpl(c, workload, user, roles)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, resp != nil, "Response should not be nil")

	// Verify workload was created with secret type set
	createdWorkload := &v1.Workload{}
	err = h.Get(ctx, client.ObjectKey{Name: workload.Name}, createdWorkload)
	assert.NilError(t, err)
	assert.Equal(t, len(createdWorkload.Spec.Secrets), 1)
	assert.Equal(t, createdWorkload.Spec.Secrets[0].Id, "test-secret")
	assert.Equal(t, string(createdWorkload.Spec.Secrets[0].Type), string(v1.SecretGeneral))
}

// Test_listWorkload tests basic workload listing
func Test_listWorkload(t *testing.T) {
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"

	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Mock database response
	now := time.Now()
	mockWorkloads := []*dbclient.Workload{
		{
			WorkloadId:   "workload-1",
			Workspace:    workspaceId,
			Cluster:      clusterId,
			DisplayName:  "Test Workload 1",
			Phase:        sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
			Priority:     5,
			CreationTime: pq.NullTime{Time: now, Valid: true},
		},
		{
			WorkloadId:   "workload-2",
			Workspace:    workspaceId,
			Cluster:      clusterId,
			DisplayName:  "Test Workload 2",
			Phase:        sql.NullString{String: string(v1.WorkloadPending), Valid: true},
			Priority:     3,
			CreationTime: pq.NullTime{Time: now, Valid: true},
		},
	}

	mockDBClient.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockWorkloads, nil).AnyTimes()
	mockDBClient.EXPECT().CountWorkloads(gomock.Any(), gomock.Any()).Return(2, nil).AnyTimes()
	mockDBClient.EXPECT().GetWorkloadStatisticByWorkloadID(gomock.Any(), "workload-1").Return(nil, nil).AnyTimes()
	mockDBClient.EXPECT().GetWorkloadStatisticByWorkloadID(gomock.Any(), "workload-2").Return(nil, nil).AnyTimes()

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workloads?workspaceId=%s", workspaceId), nil)

	// Call listWorkload
	result, err := h.listWorkload(c)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	listResp, ok := result.(*view.ListWorkloadResponse)
	assert.Equal(t, ok, true)
	assert.Equal(t, listResp.TotalCount, 2)
	assert.Equal(t, len(listResp.Items), 2)
	assert.Equal(t, listResp.Items[0].WorkloadId, "workload-1")
	assert.Equal(t, listResp.Items[0].Phase, string(v1.WorkloadRunning))
	assert.Equal(t, listResp.Items[0].AvgGpuUsage, float64(-1)) // No statistics
	assert.Equal(t, listResp.Items[1].WorkloadId, "workload-2")
}

// Test_getWorkload tests getting a single workload by ID
func Test_getWorkload(t *testing.T) {
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-123"

	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Mock database workload response
	now := time.Now()
	gvkJSON := `{"group":"kubeflow.org","version":"v1","kind":"PyTorchJob"}`
	resourceJSON := `[{"replica":2,"cpu":"16","gpu":"4","memory":"64Gi"}]`
	mockDBWorkload := &dbclient.Workload{
		WorkloadId:   workloadId,
		Workspace:    workspaceId,
		Cluster:      clusterId,
		DisplayName:  "Test Workload",
		Description:  sql.NullString{String: "Test workload description", Valid: true},
		Phase:        sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
		UserId:       sql.NullString{String: user.Name, Valid: true},
		UserName:     sql.NullString{String: "TestUser", Valid: true},
		Priority:     5,
		Images:       sql.NullString{String: "[\"test-image:v1\"]", Valid: true},
		EntryPoints:  sql.NullString{String: "[\"echo 'test'\"]", Valid: true},
		GVK:          gvkJSON,
		Resources:    sql.NullString{String: resourceJSON, Valid: true},
		IsSupervised: true,
		MaxRetry:     3,
		CreationTime: pq.NullTime{Time: now, Valid: true},
		StartTime:    pq.NullTime{Time: now.Add(time.Minute), Valid: true},
	}

	mockDBClient.EXPECT().GetWorkload(gomock.Any(), workloadId).Return(mockDBWorkload, nil).AnyTimes()

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, workloadId)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workloads/%s", workloadId), nil)

	// Call getWorkload
	result, err := h.getWorkload(c)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	getResp, ok := result.(*view.GetWorkloadResponse)
	assert.Equal(t, ok, true)
	assert.Equal(t, getResp.WorkloadId, workloadId)
	assert.Equal(t, getResp.Phase, string(v1.WorkloadRunning))
	assert.Equal(t, getResp.DisplayName, "Test Workload")
	assert.Equal(t, getResp.Description, "Test workload description")
	assert.Equal(t, getResp.UserId, user.Name)
	assert.Equal(t, getResp.UserName, "TestUser")
	assert.Equal(t, getResp.Priority, 5)
	assert.Equal(t, getResp.Images[0], "test-image:v1")
	assert.Equal(t, getResp.IsSupervised, true)
	assert.Equal(t, getResp.MaxRetry, 3)
	assert.Equal(t, getResp.GroupVersionKind.Kind, "PyTorchJob")
	assert.Equal(t, len(getResp.Resources), 1)
	assert.Equal(t, getResp.Resources[0].Replica, 2)
	assert.Equal(t, getResp.Resources[0].CPU, "16")
	assert.Equal(t, getResp.Resources[0].GPU, "4")
	assert.Equal(t, getResp.Resources[0].Memory, "64Gi")
}

// Test_deleteWorkloadImpl tests deleting a workload
func Test_deleteWorkloadImpl(t *testing.T) {
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-delete"

	user := genMockUser()
	role := genMockRole()

	// Create workload in etcd
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Mock database workload
	mockDBWorkload := &dbclient.Workload{
		WorkloadId: workloadId,
		Workspace:  workspaceId,
		Cluster:    clusterId,
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Phase:      sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
	}

	// Expect database calls
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), workloadId).Return(mockDBWorkload, nil).AnyTimes()
	mockDBClient.EXPECT().SetWorkloadDeleted(gomock.Any(), workloadId).Return(nil).AnyTimes()

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workloads/%s", workloadId), nil)

	roles := []*v1.Role{role}

	// Call deleteWorkloadImpl
	result, err := h.deleteWorkloadImpl(c, workloadId, user, roles)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result == nil, "Delete should return nil")

	// Verify workload was deleted from etcd
	deletedWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: workloadId}, deletedWorkload)
	assert.Assert(t, err != nil, "Workload should be deleted from etcd")

	// Verify phase was updated to stopped before deletion
	updatedWorkload := &v1.Workload{}
	h.Get(context.Background(), client.ObjectKey{Name: workloadId}, updatedWorkload)
	// The workload should be deleted, so we can't check the phase
	// But the test verifies the deletion logic executed successfully
}

// Test_stopWorkload tests stopping a running workload through handler
func Test_stopWorkload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-stop"

	user := genMockUser()
	role := genMockRole()

	// Create running workload in etcd
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context with workload name set in context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, workloadId) // stopWorkload gets name from context
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workloads/%s/stop", workloadId), nil)

	// Call stopWorkload handler (it will call getAndSetUsername and GetRoles internally)
	result, err := h.stopWorkload(c)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result == nil, "Stop should return nil")

	// Verify workload was deleted from etcd (stopped workloads are deleted)
	stoppedWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: workloadId}, stoppedWorkload)
	assert.Assert(t, err != nil, "Workload should be deleted from etcd after stop")
}

// Test_stopWorkloadImpl_OnlyInDatabase tests stopping a workload that only exists in database
// This scenario happens when workload was accidentally deleted from etcd but still exists in DB
func Test_stopWorkloadImpl_OnlyInDatabase(t *testing.T) {
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-db-only"
	userId := "test-user"

	user := genMockUser()
	role := genMockRole()

	// DO NOT create workload in etcd - simulating accidental deletion
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, userId)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workloads/%s/stop", workloadId), nil)

	// Mock database workload - still in Running phase
	dbWorkload := &dbclient.Workload{
		WorkloadId: workloadId,
		Workspace:  workspaceId,
		Cluster:    clusterId,
		UserId:     sql.NullString{String: userId, Valid: true},
		Phase:      sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
	}

	// Expect GetWorkload to return the database workload
	mockDBClient.EXPECT().
		GetWorkload(gomock.Any(), workloadId).
		Return(dbWorkload, nil)

	// Expect SetWorkloadStopped to be called
	mockDBClient.EXPECT().
		SetWorkloadStopped(gomock.Any(), workloadId).
		Return(nil)

	// Call stopWorkloadImpl directly
	result, err := h.stopWorkloadImpl(c, workloadId, user, []*v1.Role{role})

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result == nil, "Stop should return nil")

	// Verify workload does not exist in etcd (was never there)
	etcdWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: workloadId}, etcdWorkload)
	assert.Assert(t, err != nil, "Workload should not exist in etcd")
}

// Test_patchWorkload tests patching a workload through handler
func Test_patchWorkload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-patch"

	user := genMockUser()
	role := genMockRole()

	// Create running workload in etcd
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	workload.Spec.Priority = 0
	workload.Spec.Images = []string{"old-image:v1"}
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Prepare patch request - update priority and image
	newPriority := 1
	newImage := "new-image:v2"
	patchReq := view.PatchWorkloadRequest{
		Priority: &newPriority,
		Images:   &[]string{newImage},
	}
	reqBody, _ := json.Marshal(patchReq)

	// Create gin context with workload name set in context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, workloadId) // patchWorkload gets name from context
	c.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/workloads/%s", workloadId), bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call patchWorkload handler
	result, err := h.patchWorkload(c)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result == nil, "Patch should return nil")

	// Verify workload was updated in etcd
	updatedWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: workloadId}, updatedWorkload)
	assert.NilError(t, err)
	assert.Equal(t, updatedWorkload.Spec.Priority, newPriority, "Priority should be updated")
	assert.Equal(t, len(updatedWorkload.Spec.Images), 1)
	assert.Equal(t, updatedWorkload.Spec.Images[0], newImage, "Image should be updated")
}

// Test_getWorkloadPodLog tests getting pod logs
func Test_getWorkloadPodLog(t *testing.T) {
	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-log"
	podName := "test-pod-123"

	user := genMockUser()
	role := genMockRole()
	otherUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-user",
		},
	}

	// Create workload in etcd owned by a different user
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload, v1.UserIdLabel, otherUser.Name) // Owned by other user

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, otherUser, workload).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create clientManager with cluster's clientset
	clientManager := commonutils.NewObjectManager()
	clientFactory := k8sclient.NewClientFactoryWithOnlyClient(context.Background(), clusterId, fakeClientSet)
	clientManager.Add(clusterId, clientFactory)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		clientManager:    clientManager,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context with workload name and pod ID
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name) // Current user
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, workloadId)
	c.Params = gin.Params{
		{Key: common.PodId, Value: podName},
	}
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/workloads/%s/pods/%s/logs", workloadId, podName), nil)

	// Call getWorkloadPodLog handler
	resp, err := h.getWorkloadPodLog(c)
	assert.NilError(t, err)
	podlog, ok := resp.(*view.GetWorkloadPodLogResponse)
	assert.Equal(t, ok, true)
	assert.Equal(t, podlog.WorkloadId, "test-workload-log")
	assert.Equal(t, podlog.PodId, "test-pod-123")
	assert.Equal(t, podlog.Namespace, "test-workspace")
	assert.Equal(t, len(podlog.Logs), 1)
	assert.Equal(t, podlog.Logs[0], "fake logs")
}

// Test_getWorkloadPodContainers tests getting pod containers for a workload
func Test_getWorkloadPodContainers(t *testing.T) {
	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	workloadId := "test-workload-containers"
	podName := "test-pod-123"

	user := genMockUser()
	role := genMockRole()

	// Create workload in etcd
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = workloadId
	workload.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)
	v1.SetLabel(workload, v1.ClusterIdLabel, clusterId)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		Build()

	// Create fake pod with multiple containers
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: workspaceId,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main-container"},
				{Name: "sidecar-container"},
				{Name: "init-container"},
			},
		},
	}
	fakeClientSet := k8sfake.NewSimpleClientset(pod)

	// Create clientManager with cluster's clientset
	clientManager := commonutils.NewObjectManager()
	clientFactory := k8sclient.NewClientFactoryWithOnlyClient(context.Background(), clusterId, fakeClientSet)
	clientManager.Add(clusterId, clientFactory)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		clientManager:    clientManager,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Set(common.Name, workloadId)
	c.Params = gin.Params{
		{Key: common.PodId, Value: podName},
	}
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/workloads/%s/pods/%s/containers", workloadId, podName), nil)

	// Call getWorkloadPodContainers handler
	result, err := h.getWorkloadPodContainers(c)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result != nil, "Result should not be nil")

	// Verify response structure
	response, ok := result.(*view.GetWorkloadPodContainersResponse)
	assert.Assert(t, ok, "Result should be GetWorkloadPodContainersResponse type")
	assert.Equal(t, len(response.Containers), 3, "Should have 3 containers")
	assert.Equal(t, response.Containers[0].Name, "main-container")
	assert.Equal(t, response.Containers[1].Name, "sidecar-container")
	assert.Equal(t, response.Containers[2].Name, "init-container")
	assert.Equal(t, len(response.Shells), 3, "Should have 3 shells")
}

// Test_cloneWorkloadImpl tests cloning a workload from database
func Test_cloneWorkloadImpl(t *testing.T) {
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"
	sourceWorkloadId := "source-workload-123"

	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workload{}).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Mock source workload in database
	gvkJSON := `{"group":"kubeflow.org","version":"v1","kind":"PyTorchJob"}`
	resourceJSON := `[{"replica":2,"cpu":"16","gpu":"4","memory":"64Gi"}]`
	mockDBWorkload := &dbclient.Workload{
		WorkloadId:   sourceWorkloadId,
		Workspace:    workspaceId,
		Cluster:      clusterId,
		DisplayName:  "Source Workload",
		Description:  sql.NullString{String: "Original workload", Valid: true},
		Phase:        sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
		UserId:       sql.NullString{String: "original-user", Valid: true},
		UserName:     sql.NullString{String: "OriginalUser", Valid: true},
		Priority:     5,
		Images:       sql.NullString{String: "[\"test-image:v1\"]", Valid: true},
		EntryPoints:  sql.NullString{String: "[\"echo 'test'\"]", Valid: true},
		GVK:          gvkJSON,
		Resources:    sql.NullString{String: resourceJSON, Valid: true},
		IsSupervised: true,
		MaxRetry:     3,
	}

	// Expect database call to get source workload
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), sourceWorkloadId).Return(mockDBWorkload, nil).AnyTimes()

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workloads/%s/clone", sourceWorkloadId), nil)

	roles := []*v1.Role{role}

	// Call cloneWorkloadImpl
	result, err := h.cloneWorkloadImpl(c, sourceWorkloadId, user, roles)

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, result != nil, "Clone should return result")

	cloneResp, ok := result.(*view.CreateWorkloadResponse)
	assert.Equal(t, ok, true, "Result should be CreateWorkloadResponse")
	assert.Assert(t, cloneResp.WorkloadId != "", "Cloned workload should have ID")
	assert.Assert(t, cloneResp.WorkloadId != sourceWorkloadId, "Cloned workload should have different ID")

	// Verify cloned workload was created in etcd
	clonedWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: cloneResp.WorkloadId}, clonedWorkload)
	assert.NilError(t, err, "Cloned workload should exist in etcd")

	// Verify cloned workload has new user
	assert.Equal(t, v1.GetUserId(clonedWorkload), user.Name, "Cloned workload should have new user ID")
	assert.Equal(t, v1.GetUserName(clonedWorkload), v1.GetUserName(user), "Cloned workload should have new user name")

	// Verify cloned workload has same specs
	assert.Equal(t, clonedWorkload.Spec.Workspace, workspaceId)
	assert.Equal(t, len(clonedWorkload.Spec.Images), 1)
	assert.Equal(t, clonedWorkload.Spec.Images[0], "test-image:v1")
	assert.Equal(t, clonedWorkload.Spec.Priority, 5)
	assert.Equal(t, clonedWorkload.Spec.IsSupervised, true)
	assert.Equal(t, clonedWorkload.Spec.MaxRetry, 3)
}

// Test_createWorkload_NormalWorkload tests creating a normal workload
func Test_createWorkload_NormalWorkload(t *testing.T) {
	workspaceId := "test-workspace"

	user := genMockUser()
	role := genMockRole()
	controlPlaneNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "control-plane-node-1",
			Labels: map[string]string{
				common.KubernetesControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.1.100",
				},
			},
		},
	}
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, controlPlaneNode).
		WithScheme(mockScheme).
		WithStatusSubresource(&v1.Workload{}, controlPlaneNode).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	reqBody := fmt.Sprintf(`{
		"displayName": "Normal Workload",
		"description": "A normal PyTorch job",
		"workspaceId": "%s",
		"images": ["pytorch/pytorch:latest"],
		"entryPoints": ["python train.py"],
		"groupVersionKind": {
			"version": "v1",
			"kind": "PyTorchJob"
		},
		"resources": [{
			"replica": 2,
			"cpu": "16",
			"gpu": "4",
			"memory": "64Gi"
		}],
		"priority": 5,
		"isSupervised": true,
		"maxRetry": 3,
		"env": {
			"NCCL_DEBUG": "INFO",
			"MASTER_PORT": "29500"
		}
	}`, workspaceId)

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", strings.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call createWorkload
	result, err := h.createWorkload(c)

	// Should succeed
	assert.NilError(t, err, "Creating normal workload should succeed")
	assert.Assert(t, result != nil, "Result should not be nil")

	createResp, ok := result.(*view.CreateWorkloadResponse)
	assert.Equal(t, ok, true, "Result should be CreateWorkloadResponse")
	assert.Assert(t, createResp.WorkloadId != "", "Workload ID should be generated")

	// Verify workload was created in etcd
	createdWorkload := &v1.Workload{}
	err = h.Get(context.Background(), client.ObjectKey{Name: createResp.WorkloadId}, createdWorkload)
	assert.NilError(t, err, "Workload should exist in etcd")

	// Verify workload specifications
	assert.Equal(t, v1.GetDisplayName(createdWorkload), "Normal Workload")
	assert.Equal(t, v1.GetDescription(createdWorkload), "A normal PyTorch job")
	assert.Equal(t, createdWorkload.Spec.Workspace, workspaceId)
	assert.Equal(t, len(createdWorkload.Spec.Images), 1)
	assert.Equal(t, createdWorkload.Spec.Images[0], "pytorch/pytorch:latest")
	assert.Equal(t, len(createdWorkload.Spec.EntryPoints), 1)
	assert.Equal(t, createdWorkload.Spec.EntryPoints[0], "python train.py")
	assert.Equal(t, createdWorkload.Spec.Priority, 5)
	assert.Equal(t, createdWorkload.Spec.IsSupervised, true)
	assert.Equal(t, createdWorkload.Spec.MaxRetry, 3)
	assert.Equal(t, createdWorkload.Spec.GroupVersionKind.Kind, "PyTorchJob")
	assert.Equal(t, len(createdWorkload.Spec.Resources), 1)
	assert.Equal(t, createdWorkload.Spec.Resources[0].Replica, 2)
	assert.Equal(t, createdWorkload.Spec.Resources[0].CPU, "16")
	assert.Equal(t, createdWorkload.Spec.Resources[0].GPU, "4")
	assert.Equal(t, createdWorkload.Spec.Resources[0].Memory, "64Gi")

	// Verify user labels are set
	assert.Equal(t, v1.GetUserId(createdWorkload), user.Name)
	assert.Equal(t, v1.GetUserName(createdWorkload), v1.GetUserName(user))

	// Verify phase is set to pending
	assert.Equal(t, createdWorkload.Status.Phase, v1.WorkloadPending)

	// Verify environment variables are set
	assert.Equal(t, createdWorkload.Spec.Env["NCCL_DEBUG"], "INFO")
	assert.Equal(t, createdWorkload.Spec.Env["MASTER_PORT"], "29500")

	// Verify it's not a preheat workload (no dependencies)
	assert.Equal(t, len(createdWorkload.Spec.Dependencies), 0, "Normal workload should have no dependencies")
}

// Test_getAdminControlPlaneIp tests getting control plane IP successfully
func Test_getAdminControlPlaneIp(t *testing.T) {
	ctx := context.Background()

	// Create a control plane node with internal IP
	controlPlaneNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "control-plane-node-1",
			Labels: map[string]string{
				common.KubernetesControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.1.100",
				},
				{
					Type:    corev1.NodeHostName,
					Address: "control-plane-1",
				},
			},
		},
	}
	// Create scheme with Kubernetes core types
	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(controlPlaneNode).
		WithScheme(testScheme).
		Build()

	h := Handler{
		Client: fakeCtrlClient,
	}

	// Call getAdminControlPlaneIp
	ip, err := h.getAdminControlPlaneIp(ctx)

	// Should succeed and return the internal IP
	assert.NilError(t, err, "Should successfully get control plane IP")
	assert.Equal(t, ip, "192.168.1.100", "Should return correct internal IP")
}

// Test_handleBatchWorkloads_BatchDelete tests batch delete operation
func Test_handleBatchWorkloads_BatchDelete(t *testing.T) {
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceId := "test-workspace"
	clusterId := "test-cluster"

	user := genMockUser()
	role := genMockRole()

	// Create multiple workloads to delete
	workload1 := genMockWorkload(clusterId, workspaceId)
	workload1.Name = "workload-to-delete-1"
	workload1.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload1, v1.UserIdLabel, user.Name)

	workload2 := genMockWorkload(clusterId, workspaceId)
	workload2.Name = "workload-to-delete-2"
	workload2.Status.Phase = v1.WorkloadRunning
	v1.SetLabel(workload2, v1.UserIdLabel, user.Name)

	workload3 := genMockWorkload(clusterId, workspaceId)
	workload3.Name = "workload-to-delete-3"
	workload3.Status.Phase = v1.WorkloadPending
	v1.SetLabel(workload3, v1.UserIdLabel, user.Name)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload1, workload2, workload3).
		WithScheme(scheme.Scheme).
		WithStatusSubresource(workload1, workload2, workload3).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	// Create mock database client
	mockDBClient := mockdb.NewMockInterface(ctrl)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		dbClient:         mockDBClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Mock database workloads
	mockDBWorkload1 := &dbclient.Workload{
		WorkloadId: "workload-to-delete-1",
		Workspace:  workspaceId,
		Cluster:    clusterId,
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Phase:      sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
	}
	mockDBWorkload2 := &dbclient.Workload{
		WorkloadId: "workload-to-delete-2",
		Workspace:  workspaceId,
		Cluster:    clusterId,
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Phase:      sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
	}
	mockDBWorkload3 := &dbclient.Workload{
		WorkloadId: "workload-to-delete-3",
		Workspace:  workspaceId,
		Cluster:    clusterId,
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Phase:      sql.NullString{String: string(v1.WorkloadPending), Valid: true},
	}

	// Expect database calls for all workloads
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), "workload-to-delete-1").Return(mockDBWorkload1, nil).AnyTimes()
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), "workload-to-delete-2").Return(mockDBWorkload2, nil).AnyTimes()
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), "workload-to-delete-3").Return(mockDBWorkload3, nil).AnyTimes()
	mockDBClient.EXPECT().SetWorkloadDeleted(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Create batch delete request
	reqBody := `{
		"workloadIds": [
			"workload-to-delete-1",
			"workload-to-delete-2",
			"workload-to-delete-3"
		]
	}`

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.UserId, user.Name)
	c.Set(common.UserName, v1.GetUserName(user))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads/batch-delete", strings.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call handleBatchWorkloads with BatchDelete action
	result, err := h.handleBatchWorkloads(c, BatchDelete)

	// Should succeed
	assert.NilError(t, err, "Batch delete should succeed")
	assert.Assert(t, result == nil, "Result should be nil for successful batch operation")

	// Verify all workloads were deleted from etcd
	for _, name := range []string{"workload-to-delete-1", "workload-to-delete-2", "workload-to-delete-3"} {
		deletedWorkload := &v1.Workload{}
		err = h.Get(context.Background(), client.ObjectKey{Name: name}, deletedWorkload)
		assert.Assert(t, err != nil, "Workload %s should be deleted from etcd", name)
	}
}

// Test_cvtToListWorkloadSql tests converting list workload query to SQL
func Test_cvtToListWorkloadSql(t *testing.T) {
	// Test basic query with multiple filters
	query := &view.ListWorkloadRequest{
		ClusterId:   "test-cluster",
		WorkspaceId: "test-workspace",
		Phase:       "running,pending",
		Description: "test workload",
		UserName:    "testuser",
		UserId:      "user-123",
		Kind:        "PyTorchJob,TFJob",
		WorkloadId:  "workload-123",
		SortBy:      "CreationTime",
		Order:       "desc",
	}

	dbSql, orderBy := cvtToListWorkloadSql(query)

	// Verify SQL query is generated
	assert.Assert(t, dbSql != nil, "SQL query should be generated")
	assert.Assert(t, len(orderBy) > 0, "OrderBy should be generated")

	// Convert to SQL string to verify structure
	sqlStr, args, err := dbSql.ToSql()
	assert.NilError(t, err, "Should convert to SQL successfully")

	// Verify key conditions are in SQL structure (with placeholders)
	assert.Assert(t, strings.Contains(sqlStr, "is_deleted"), "Should include IsDeleted condition")
	assert.Assert(t, strings.Contains(sqlStr, "LIKE"), "Should use LIKE for description and other fields")

	// Verify parameter values are in args array
	argsStr := fmt.Sprintf("%v", args)
	assert.Assert(t, strings.Contains(argsStr, "false"), "Args should contain false for is_deleted")
	assert.Assert(t, strings.Contains(argsStr, "test-cluster"), "Args should contain cluster value")
	assert.Assert(t, strings.Contains(argsStr, "test-workspace"), "Args should contain workspace value")
	assert.Assert(t, strings.Contains(argsStr, "user-123"), "Args should contain userId value")
	assert.Assert(t, strings.Contains(argsStr, "%test workload%"), "Args should contain LIKE pattern for description")

	// Verify orderBy contains sorting
	assert.Assert(t, len(orderBy) > 0, "Should have order by clauses")
	orderByStr := strings.Join(orderBy, " ")
	assert.Assert(t, strings.Contains(strings.ToLower(orderByStr), "desc"), "Should include DESC order")
}

// Test_cvtToListWorkloadSql_EmptyQuery tests with empty query
func Test_cvtToListWorkloadSql_EmptyQuery(t *testing.T) {
	// Test empty query (only default filters)
	query := &view.ListWorkloadRequest{}

	dbSql, orderBy := cvtToListWorkloadSql(query)

	// Verify SQL query is generated with defaults
	assert.Assert(t, dbSql != nil, "SQL query should be generated")
	assert.Assert(t, len(orderBy) > 0, "OrderBy should be generated with defaults")

	// Convert to SQL string
	sqlStr, _, err := dbSql.ToSql()
	assert.NilError(t, err, "Should convert to SQL successfully")

	// Verify default conditions
	assert.Assert(t, strings.Contains(sqlStr, "is_deleted"), "Should include IsDeleted condition by default")
}

// Test_cvtDBWorkloadToResponseItem tests converting database workload to response item
func Test_cvtDBWorkloadToResponseItem(t *testing.T) {
	ctx := context.Background()

	// Create test workload in etcd for pending phase message retrieval
	clusterId := "test-cluster"
	workspaceId := "test-workspace"
	workloadId := "test-workload-123"

	user := genMockUser()
	role := genMockRole()

	// Create workload in etcd (for pending message test)
	etcdWorkload := genMockWorkload(clusterId, workspaceId)
	etcdWorkload.Name = workloadId
	etcdWorkload.Status.Phase = v1.WorkloadPending
	etcdWorkload.Status.Message = "Waiting for resources"

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, etcdWorkload).
		WithScheme(scheme.Scheme).
		Build()

	h := Handler{
		Client: fakeCtrlClient,
	}

	// Create database workload with comprehensive data
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	gvkJSON := `{"group":"kubeflow.org","version":"v1","kind":"PyTorchJob"}`
	resourceJSON := `[{"replica":2,"cpu":"16","gpu":"4","memory":"64Gi"}]`

	dbWorkload := &dbclient.Workload{
		WorkloadId:     workloadId,
		Workspace:      workspaceId,
		Cluster:        clusterId,
		DisplayName:    "Test Workload",
		Description:    sql.NullString{String: "Test description", Valid: true},
		Phase:          sql.NullString{String: string(v1.WorkloadPending), Valid: true},
		UserId:         sql.NullString{String: "user-123", Valid: true},
		UserName:       sql.NullString{String: "TestUser", Valid: true},
		Priority:       5,
		IsTolerateAll:  true,
		WorkloadUId:    sql.NullString{String: "uid-123", Valid: true},
		ScaleRunnerSet: sql.NullString{String: "runner-set-1", Valid: true},
		ScaleRunnerId:  sql.NullString{String: "runner-id-1", Valid: true},
		QueuePosition:  3,
		DispatchCount:  2,
		CreationTime:   pq.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
		StartTime:      pq.NullTime{Time: startTime, Valid: true},
		EndTime:        pq.NullTime{Time: now, Valid: true},
		Timeout:        3600, // 1 hour timeout
		GVK:            gvkJSON,
		Resources:      sql.NullString{String: resourceJSON, Valid: true},
	}

	// Call cvtDBWorkloadToResponseItem
	result := h.cvtDBWorkloadToResponseItem(ctx, dbWorkload)

	// Verify basic fields
	assert.Equal(t, result.WorkloadId, workloadId)
	assert.Equal(t, result.WorkspaceId, workspaceId)
	assert.Equal(t, result.ClusterId, clusterId)
	assert.Equal(t, result.DisplayName, "Test Workload")
	assert.Equal(t, result.Description, "Test description")
	assert.Equal(t, result.Phase, string(v1.WorkloadPending))
	assert.Equal(t, result.UserId, "user-123")
	assert.Equal(t, result.UserName, "TestUser")
	assert.Equal(t, result.Priority, 5)
	assert.Equal(t, result.IsTolerateAll, true)
	assert.Equal(t, result.WorkloadUid, "uid-123")
	assert.Equal(t, result.ScaleRunnerSet, "runner-set-1")
	assert.Equal(t, result.ScaleRunnerId, "runner-id-1")
	assert.Equal(t, result.QueuePosition, 3)
	assert.Equal(t, result.DispatchCount, 2)

	// Verify AvgGpuUsage default value
	assert.Equal(t, result.AvgGpuUsage, float64(-1))

	// Verify time fields are not empty
	assert.Assert(t, result.CreationTime != "", "CreationTime should be set")
	assert.Assert(t, result.StartTime != "", "StartTime should be set")
	assert.Assert(t, result.EndTime != "", "EndTime should be set")

	// Verify duration is calculated
	assert.Assert(t, result.Duration != "0s", "Duration should be calculated")
	assert.Assert(t, strings.Contains(result.Duration, "h") || strings.Contains(result.Duration, "m") ||
		strings.Contains(result.Duration, "s"), "Duration should have time units")

	// Verify GVK is parsed
	assert.Equal(t, result.GroupVersionKind.Kind, "PyTorchJob")
	assert.Equal(t, result.GroupVersionKind.Group, "kubeflow.org")
	assert.Equal(t, result.GroupVersionKind.Version, "v1")

	// Verify resource is parsed
	assert.Equal(t, len(result.Resources), 1)
	assert.Equal(t, result.Resources[0].Replica, 2)
	assert.Equal(t, result.Resources[0].CPU, "16")
	assert.Equal(t, result.Resources[0].GPU, "4")
	assert.Equal(t, result.Resources[0].Memory, "64Gi")

	// Verify timeout fields
	assert.Assert(t, result.Timeout != nil, "Timeout should be set")
	assert.Equal(t, *result.Timeout, 3600)
	assert.Assert(t, result.SecondsUntilTimeout >= 0 || result.SecondsUntilTimeout == -1,
		"SecondsUntilTimeout should be valid")

	// Verify message is retrieved for pending phase
	assert.Equal(t, result.Message, "Waiting for resources", "Should retrieve message from etcd for pending workload")
}

// Test_cvtDBWorkloadToGetResponse tests converting database workload to detailed response
func Test_cvtDBWorkloadToGetResponse(t *testing.T) {
	ctx := context.Background()

	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(scheme.Scheme).
		Build()

	h := Handler{
		Client:           fakeCtrlClient,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create comprehensive database workload
	now := time.Now()
	gvkJSON := `{"group":"kubeflow.org","version":"v1","kind":"PyTorchJob"}`
	resourceJSON := `[{"replica":2,"cpu":"16","gpu":"4","memory":"64Gi"}]`
	conditionsJSON := `[{"type":"Ready","status":"True","message":"Workload is ready"}]`
	podsJSON := `[{"podId":"pod-1","phase":"Running"}]`
	nodesJSON := `[["node-1","node-2"]]`
	ranksJSON := `[["0","1"]]`
	customerLabelsJSON := `{"kubernetes.io/hostname":"node1 node2","custom-key":"custom-value"}`
	livenessJSON := `{"httpGet":{"path":"/healthz","port":8080}}`
	readinessJSON := `{"httpGet":{"path":"/ready","port":8080}}`
	serviceJSON := `{"type":"ClusterIP","port":8080}`
	envJSON := `{"ENV_VAR":"value","ANOTHER_VAR":"another_value"}`
	cronJobsJSON := `[{"schedule":"0 0 * * *","command":"backup"}]`
	secretsJSON := `[{"id":"secret-1","type":"general"}]`

	dbWorkload := &dbclient.Workload{
		WorkloadId:     "test-workload-get",
		Workspace:      "test-workspace",
		Cluster:        "test-cluster",
		DisplayName:    "Test Workload",
		Description:    sql.NullString{String: "Test description", Valid: true},
		Phase:          sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
		UserId:         sql.NullString{String: user.Name, Valid: true},
		UserName:       sql.NullString{String: "TestUser", Valid: true},
		Priority:       5,
		Images:         sql.NullString{String: "[\"test-image:v1\"]", Valid: true},
		EntryPoints:    sql.NullString{String: fmt.Sprintf("[\"%s\"]", stringutil.Base64Encode("python train.py")), Valid: true},
		IsSupervised:   true,
		MaxRetry:       3,
		TTLSecond:      300,
		CreationTime:   pq.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
		StartTime:      pq.NullTime{Time: now.Add(-30 * time.Minute), Valid: true},
		GVK:            gvkJSON,
		Resources:      sql.NullString{String: resourceJSON, Valid: true},
		Conditions:     sql.NullString{String: conditionsJSON, Valid: true},
		Pods:           sql.NullString{String: podsJSON, Valid: true},
		Nodes:          sql.NullString{String: nodesJSON, Valid: true},
		Ranks:          sql.NullString{String: ranksJSON, Valid: true},
		CustomerLabels: sql.NullString{String: customerLabelsJSON, Valid: true},
		Liveness:       sql.NullString{String: livenessJSON, Valid: true},
		Readiness:      sql.NullString{String: readinessJSON, Valid: true},
		Service:        sql.NullString{String: serviceJSON, Valid: true},
		Env:            sql.NullString{String: envJSON, Valid: true},
		CronJobs:       sql.NullString{String: cronJobsJSON, Valid: true},
		Secrets:        sql.NullString{String: secretsJSON, Valid: true},
	}

	roles := []*v1.Role{role}

	// Call cvtDBWorkloadToGetResponse
	result := h.cvtDBWorkloadToGetResponse(ctx, user, roles, dbWorkload)

	// Verify basic fields from WorkloadResponseItem
	assert.Equal(t, result.WorkloadId, "test-workload-get")
	assert.Equal(t, result.WorkspaceId, "test-workspace")
	assert.Equal(t, result.DisplayName, "Test Workload")

	// Verify additional fields
	assert.Equal(t, len(result.Images), 1)
	assert.Equal(t, result.Images[0], "test-image:v1")
	assert.Equal(t, result.IsSupervised, true)
	assert.Equal(t, result.MaxRetry, 3)

	// Verify EntryPoint is Base64 decoded
	assert.Equal(t, len(result.EntryPoints), 1)
	assert.Equal(t, result.EntryPoints[0], "python train.py", "EntryPoint should be Base64 decoded")

	// Verify TTLSecondsAfterFinished
	assert.Assert(t, result.TTLSecondsAfterFinished != nil, "TTLSecondsAfterFinished should be set")
	assert.Equal(t, *result.TTLSecondsAfterFinished, 300)

	// Verify Conditions are parsed
	assert.Equal(t, len(result.Conditions), 1)
	assert.Equal(t, result.Conditions[0].Type, "Ready")

	// Verify Pods are parsed
	assert.Equal(t, len(result.Pods), 1)
	assert.Equal(t, result.Pods[0].PodId, "pod-1")

	// Verify Nodes are parsed (2D array: retry attempts × nodes)
	assert.Equal(t, len(result.Nodes), 1)
	assert.Equal(t, len(result.Nodes[0]), 2)
	assert.Equal(t, result.Nodes[0][0], "node-1")
	assert.Equal(t, result.Nodes[0][1], "node-2")

	// Verify Ranks are parsed (2D array: retry attempts × ranks)
	assert.Equal(t, len(result.Ranks), 1)
	assert.Equal(t, len(result.Ranks[0]), 2)
	assert.Equal(t, result.Ranks[0][0], "0")
	assert.Equal(t, result.Ranks[0][1], "1")

	// Verify CustomerLabels are parsed and separated
	assert.Assert(t, result.CustomerLabels != nil, "CustomerLabels should be set")
	assert.Equal(t, result.CustomerLabels["custom-key"], "custom-value")
	assert.Equal(t, len(result.SpecifiedNodes), 2, "SpecifiedNodes should be parsed from hostname label")

	// Verify Liveness is parsed
	assert.Assert(t, result.Liveness != nil, "Liveness should be set")

	// Verify Readiness is parsed
	assert.Assert(t, result.Readiness != nil, "Readiness should be set")

	// Verify Service is parsed
	assert.Assert(t, result.Service != nil, "Service should be set")

	// Verify Env is parsed
	assert.Equal(t, len(result.Env), 2)
	assert.Equal(t, result.Env["ENV_VAR"], "value")
	assert.Equal(t, result.Env["ANOTHER_VAR"], "another_value")

	// Verify CronJobs are parsed
	assert.Equal(t, len(result.CronJobs), 1)

	// Verify Secrets are parsed (user is owner, so should have access)
	assert.Equal(t, len(result.Secrets), 1)
	assert.Equal(t, result.Secrets[0].Id, "secret-1")
}

// Test_buildSSHCommand tests building SSH command
func Test_buildSSHCommand(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		sshEnable      string
		subDomain      string
		domain         string
		sshPort        string
		userId         string
		workspace      string
		pod            *v1.WorkloadPod
		gvk            v1.GroupVersionKind
		mainContainer  string
		expectedResult string
	}{
		{
			name:      "SSH disabled returns empty",
			sshEnable: "false",
			pod: &v1.WorkloadPod{
				PodId: "test-pod",
				Phase: corev1.PodRunning,
			},
			gvk:            v1.GroupVersionKind{Group: "kubeflow.org", Version: "v1", Kind: "PyTorchJob"},
			expectedResult: "",
		},
		{
			name:      "Pod not running returns empty",
			sshEnable: "true",
			subDomain: "ssh",
			domain:    "example.com",
			sshPort:   "2222",
			pod: &v1.WorkloadPod{
				PodId: "test-pod",
				Phase: corev1.PodPending,
			},
			gvk:            v1.GroupVersionKind{Group: "kubeflow.org", Version: "v1", Kind: "PyTorchJob"},
			expectedResult: "",
		},
		{
			name:          "Valid SSH address with all parameters",
			sshEnable:     "true",
			subDomain:     "ssh",
			domain:        "example.com",
			sshPort:       "2222",
			userId:        "test-user",
			workspace:     "test-workspace",
			mainContainer: "pytorch",
			pod: &v1.WorkloadPod{
				PodId: "workload-abc123-master-0",
				Phase: corev1.PodRunning,
			},
			gvk:            v1.GroupVersionKind{Group: "kubeflow.org", Version: "v1", Kind: "PyTorchJob"},
			expectedResult: "ssh -o ServerAliveInterval=60 test-user.workload-abc123-master-0.pytorch.sh.test-workspace@ssh.example.com -p 2222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			commonconfig.SetValue("ssh.enable", tt.sshEnable)
			commonconfig.SetValue("global.sub_domain", tt.subDomain)
			commonconfig.SetValue("global.domain", tt.domain)
			commonconfig.SetValue("ssh.server_port", tt.sshPort)
			defer func() {
				commonconfig.SetValue("ssh.enable", "")
				commonconfig.SetValue("global.sub_domain", "")
				commonconfig.SetValue("global.domain", "")
				commonconfig.SetValue("ssh.server_port", "")
			}()

			// Create workload template ConfigMap
			templateCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workload-template-pytorchjob",
					Namespace: common.PrimusSafeNamespace,
					Labels: map[string]string{
						v1.WorkloadVersionLabel: tt.gvk.Version,
						v1.WorkloadKindLabel:    tt.gvk.Kind,
					},
					Annotations: map[string]string{
						v1.MainContainerAnnotation: tt.mainContainer,
					},
				},
			}

			testScheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
			utilruntime.Must(scheme.AddToScheme(testScheme))

			fakeClient := ctrlruntimefake.NewClientBuilder().
				WithScheme(testScheme).
				WithObjects(templateCM).
				Build()

			h := Handler{
				Client: fakeClient,
			}

			// Call buildSSHCommand
			result := h.buildSSHCommand(ctx, tt.pod, tt.userId, tt.workspace, tt.gvk)

			assert.Equal(t, result, tt.expectedResult)
		})
	}
}

// Test_buildSSHCommand_NoTemplate tests buildSSHCommand when workload template is not found
func Test_buildSSHCommand_NoTemplate(t *testing.T) {
	ctx := context.Background()

	// Setup config
	commonconfig.SetValue("ssh.enable", "true")
	commonconfig.SetValue("global.sub_domain", "ssh")
	commonconfig.SetValue("global.domain", "example.com")
	commonconfig.SetValue("ssh.server_port", "2222")
	defer func() {
		commonconfig.SetValue("ssh.enable", "")
		commonconfig.SetValue("global.sub_domain", "")
		commonconfig.SetValue("global.domain", "")
		commonconfig.SetValue("ssh.server_port", "")
	}()

	pod := &v1.WorkloadPod{
		PodId: "test-pod",
		Phase: corev1.PodRunning,
	}
	gvk := v1.GroupVersionKind{Group: "kubeflow.org", Version: "v1", Kind: "NonExistentJob"}

	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(scheme.AddToScheme(testScheme))

	fakeClient := ctrlruntimefake.NewClientBuilder().
		WithScheme(testScheme).
		Build()

	h := Handler{
		Client: fakeClient,
	}

	// Call buildSSHCommand - should return empty string when template not found
	result := h.buildSSHCommand(ctx, pod, "test-user", "test-workspace", gvk)

	assert.Equal(t, result, "")
}
