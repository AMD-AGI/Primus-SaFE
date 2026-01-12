/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newMockModelReconciler creates a mock ModelReconciler for testing
func newMockModelReconciler(adminClient client.Client) *ModelReconciler {
	return &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: adminClient,
		},
	}
}

// genMockModel generates a mock Model for testing
func genMockModel(name string, accessMode v1.AccessMode, workspace string) *v1.Model {
	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
			Description: "Test model for unit tests",
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: accessMode,
			},
			Workspace: workspace,
		},
		Status: v1.ModelStatus{},
	}
	return model
}

// genMockRemoteAPIModel generates a mock remote API Model
func genMockRemoteAPIModel(name string) *v1.Model {
	model := genMockModel(name, v1.AccessModeRemoteAPI, "")
	model.Spec.Source.URL = "https://api.openai.com"
	model.Spec.Source.ModelName = "gpt-4"
	return model
}

// genMockLocalModel generates a mock local Model
func genMockLocalModel(name string, workspace string) *v1.Model {
	model := genMockModel(name, v1.AccessModeLocal, workspace)
	model.Spec.Source.URL = "https://huggingface.co/meta-llama/Llama-2-7b"
	return model
}

// genMockWorkspaceForModel generates a mock Workspace for model testing
func genMockWorkspaceForModel(name, clusterName, pfsPath string) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.ClusterIdLabel: clusterName,
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster: clusterName,
			Volumes: []v1.WorkspaceVolume{
				{
					Id:        1,
					Type:      v1.PFS,
					MountPath: pfsPath,
				},
			},
		},
		Status: v1.WorkspaceStatus{
			Phase: v1.WorkspaceRunning,
		},
	}
}

// genMockOpsJob generates a mock OpsJob for testing
func genMockOpsJob(name string, phase v1.OpsJobPhase) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobDownloadType,
		},
		Status: v1.OpsJobStatus{
			Phase: phase,
		},
	}
}

// genMockBatchJob generates a mock batch Job for testing
func genMockBatchJob(name, namespace string, succeeded, failed int32) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: batchv1.JobStatus{
			Succeeded: succeeded,
			Failed:    failed,
		},
	}
}

// TestNeedsCleanup tests the needsCleanup function
func TestNeedsCleanup(t *testing.T) {
	tests := []struct {
		name       string
		accessMode v1.AccessMode
		expected   bool
	}{
		{
			name:       "Local model needs cleanup",
			accessMode: v1.AccessModeLocal,
			expected:   true,
		},
		{
			name:       "Remote API model does not need cleanup",
			accessMode: v1.AccessModeRemoteAPI,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := genMockModel("test-model", tt.accessMode, "")
			r := newMockModelReconciler(nil)
			result := r.needsCleanup(model)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestExtractHFRepoId tests the extractHFRepoId function
func TestExtractHFRepoId(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Standard HuggingFace URL",
			url:      "https://huggingface.co/meta-llama/Llama-2-7b",
			expected: "meta-llama/Llama-2-7b",
		},
		{
			name:     "HuggingFace URL with trailing slash",
			url:      "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct/",
			expected: "Qwen/Qwen2.5-0.5B-Instruct",
		},
		{
			name:     "Direct repo ID",
			url:      "meta-llama/Llama-2-7b",
			expected: "meta-llama/Llama-2-7b",
		},
		{
			name:     "HTTP HuggingFace URL",
			url:      "http://huggingface.co/test/model",
			expected: "test/model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHFRepoId(tt.url)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestGetPFSPathFromWorkspace tests the getPFSPathFromWorkspace function
func TestGetPFSPathFromWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		volumes  []v1.WorkspaceVolume
		expected string
	}{
		{
			name: "Workspace with PFS volume",
			volumes: []v1.WorkspaceVolume{
				{Type: v1.PFS, MountPath: "/apps"},
			},
			expected: "/apps",
		},
		{
			name: "Workspace with hostpath volume",
			volumes: []v1.WorkspaceVolume{
				{Type: v1.HOSTPATH, MountPath: "/data"},
			},
			expected: "/data",
		},
		{
			name: "Workspace with PFS and hostpath volumes",
			volumes: []v1.WorkspaceVolume{
				{Type: v1.HOSTPATH, MountPath: "/data"},
				{Type: v1.PFS, MountPath: "/apps"},
			},
			expected: "/apps",
		},
		{
			name:     "Workspace with no volumes",
			volumes:  []v1.WorkspaceVolume{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: tt.volumes,
				},
			}
			result := getPFSPathFromWorkspace(ws)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestModelReconcile_RemoteAPIModel tests reconciliation of remote API models
func TestModelReconcile_RemoteAPIModel(t *testing.T) {
	model := genMockRemoteAPIModel("test-remote-model")

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	// First reconcile - should initialize status
	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: model.Name},
	}
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)

	// Verify model status
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: model.Name}, model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseReady)
	assert.Equal(t, model.Status.Message, "Remote API model is ready")
}

// TestModelReconcile_LocalModel_InitializeStatus tests local model status initialization
func TestModelReconcile_LocalModel_InitializeStatus(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	// Pre-add finalizer to skip the finalizer-adding step in first reconcile
	controllerutil.AddFinalizer(model, ModelFinalizer)

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: model.Name},
	}
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)

	err = adminClient.Get(context.Background(), client.ObjectKey{Name: model.Name}, model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhasePending)
}

// TestModelReconcile_NotFound tests reconciliation when model is not found
func TestModelReconcile_NotFound(t *testing.T) {
	adminClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: "non-existent-model"},
	}
	result, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)
	assert.Equal(t, result.Requeue, false)
}

// TestModelReconcile_AddFinalizer tests finalizer addition for local models
func TestModelReconcile_AddFinalizer(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhasePending

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: model.Name},
	}
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)

	err = adminClient.Get(context.Background(), client.ObjectKey{Name: model.Name}, model)
	assert.NilError(t, err)
	assert.Equal(t, controllerutil.ContainsFinalizer(model, ModelFinalizer), true)
}

// TestModelDelete_RemoteAPIModel tests deletion of remote API model
func TestModelDelete_RemoteAPIModel(t *testing.T) {
	model := genMockRemoteAPIModel("test-remote-model")
	model.Status.Phase = v1.ModelPhaseReady
	now := metav1.Now()
	model.DeletionTimestamp = &now
	controllerutil.AddFinalizer(model, ModelFinalizer)

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	result, err := r.handleDelete(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, result.Requeue, false)
	assert.Equal(t, controllerutil.ContainsFinalizer(model, ModelFinalizer), false)
}

// TestModelDelete_NoFinalizer tests deletion when no finalizer is present
func TestModelDelete_NoFinalizer(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	// Add a dummy finalizer first (required by fake client when DeletionTimestamp is set)
	controllerutil.AddFinalizer(model, "test-finalizer")
	now := metav1.Now()
	model.DeletionTimestamp = &now

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	// Remove the finalizer to simulate no finalizer scenario
	controllerutil.RemoveFinalizer(model, "test-finalizer")

	result, err := r.handleDelete(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, result.Requeue, false)
}

// TestHandlePending_RemoteAPIModel tests handlePending for remote API model
func TestHandlePending_RemoteAPIModel(t *testing.T) {
	model := genMockRemoteAPIModel("test-remote-model")
	model.Status.Phase = v1.ModelPhasePending

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.handlePending(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseReady)
}

// TestHandleUploading_JobSucceeded tests handleUploading when job succeeds
func TestHandleUploading_JobSucceeded(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseUploading
	model.Status.S3Path = "models/test-model"

	job := genMockBatchJob("test-local-model", common.PrimusSafeNamespace, 1, 0)

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	err = batchv1.AddToScheme(mockScheme)
	assert.NilError(t, err)

	adminClient := fake.NewClientBuilder().
		WithObjects(model, job).
		WithStatusSubresource(model).
		WithScheme(mockScheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err = r.handleUploading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseDownloading)
}

// TestHandleUploading_JobFailed tests handleUploading when job fails
func TestHandleUploading_JobFailed(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseUploading
	model.Status.S3Path = "models/test-model"

	job := genMockBatchJob("test-local-model", common.PrimusSafeNamespace, 0, 3)
	job.Status.Active = 0

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	err = batchv1.AddToScheme(mockScheme)
	assert.NilError(t, err)

	adminClient := fake.NewClientBuilder().
		WithObjects(model, job).
		WithStatusSubresource(model).
		WithScheme(mockScheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err = r.handleUploading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseFailed)
}

// TestHandleUploading_JobNotFound tests handleUploading when job is not found
func TestHandleUploading_JobNotFound(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseUploading
	model.Status.S3Path = "models/test-model"

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	err = batchv1.AddToScheme(mockScheme)
	assert.NilError(t, err)

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(mockScheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err = r.handleUploading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseFailed)
	assert.Equal(t, model.Status.Message, "Download job lost or deleted unexpectedly")
}

// TestHandleDownloading_AllReady tests handleDownloading when all paths are ready
func TestHandleDownloading_AllReady(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/test-model"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusReady,
		},
		{
			Workspace: "ws2",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusReady,
		},
	}

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.handleDownloading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseReady)
	assert.Equal(t, model.Status.Message, "Model is ready in 2 workspaces")
}

// TestHandleDownloading_SomeFailed tests handleDownloading when some paths fail
func TestHandleDownloading_SomeFailed(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/test-model"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusReady,
		},
		{
			Workspace: "ws2",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusFailed,
			Message:   "Download failed",
		},
	}

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.handleDownloading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseReady)
	// When some paths succeed and some fail, model is still ready
	assert.Equal(t, model.Status.Message, "Model is ready in 1/2 workspaces (1 failed)")
}

// TestHandleDownloading_AllFailed tests handleDownloading when all paths fail
func TestHandleDownloading_AllFailed(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/test-model"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusFailed,
			Message:   "Download failed",
		},
		{
			Workspace: "ws2",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusFailed,
			Message:   "Download failed",
		},
	}

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.handleDownloading(context.Background(), model)
	assert.NilError(t, err)
	// When all paths fail, model status is Failed
	assert.Equal(t, model.Status.Phase, v1.ModelPhaseFailed)
	assert.Equal(t, model.Status.Message, "All local downloads failed")
}

// TestHandleDownloading_OpsJobSucceeded tests handleDownloading when OpsJob succeeds
func TestHandleDownloading_OpsJobSucceeded(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/test-model"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusDownloading,
		},
	}

	opsJob := genMockOpsJob("download-test-local-model-ws1", v1.OpsJobSucceeded)

	adminClient := fake.NewClientBuilder().
		WithObjects(model, opsJob).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.handleDownloading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.LocalPaths[0].Status, v1.LocalPathStatusReady)
}

// TestHandleDownloading_OpsJobFailed tests handleDownloading when OpsJob fails
func TestHandleDownloading_OpsJobFailed(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/test-model"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusDownloading,
		},
	}

	opsJob := genMockOpsJob("download-test-local-model-ws1", v1.OpsJobFailed)
	opsJob.Status.Conditions = []metav1.Condition{
		{
			Type:    "Failed",
			Status:  metav1.ConditionTrue,
			Reason:  "DownloadFailed",
			Message: "S3 download failed",
		},
	}

	adminClient := fake.NewClientBuilder().
		WithObjects(model, opsJob).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.handleDownloading(context.Background(), model)
	assert.NilError(t, err)
	assert.Equal(t, model.Status.LocalPaths[0].Status, v1.LocalPathStatusFailed)
}

// TestInitializeLocalPaths_PublicModel tests initializeLocalPaths for public models
func TestInitializeLocalPaths_PublicModel(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Spec.Workspace = "" // Public model

	ws1 := genMockWorkspaceForModel("ws1", "cluster1", "/apps")
	ws2 := genMockWorkspaceForModel("ws2", "cluster1", "/data")

	adminClient := fake.NewClientBuilder().
		WithObjects(model, ws1, ws2).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	paths := r.initializeLocalPaths(context.Background(), model)
	assert.Equal(t, len(paths), 2)
}

// TestInitializeLocalPaths_PrivateModel tests initializeLocalPaths for private models
func TestInitializeLocalPaths_PrivateModel(t *testing.T) {
	model := genMockLocalModel("test-local-model", "ws1")

	ws1 := genMockWorkspaceForModel("ws1", "cluster1", "/apps")
	ws2 := genMockWorkspaceForModel("ws2", "cluster1", "/data")

	adminClient := fake.NewClientBuilder().
		WithObjects(model, ws1, ws2).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	paths := r.initializeLocalPaths(context.Background(), model)
	assert.Equal(t, len(paths), 1)
	assert.Equal(t, paths[0].Workspace, "ws1")
}

// TestInitializeLocalPaths_DeduplicatePaths tests path deduplication
func TestInitializeLocalPaths_DeduplicatePaths(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Spec.Workspace = "" // Public model

	// Two workspaces share the same PFS path
	ws1 := genMockWorkspaceForModel("ws1", "cluster1", "/apps")
	ws2 := genMockWorkspaceForModel("ws2", "cluster1", "/apps") // Same path as ws1

	adminClient := fake.NewClientBuilder().
		WithObjects(model, ws1, ws2).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	paths := r.initializeLocalPaths(context.Background(), model)
	// Should deduplicate to 1 path since both workspaces share the same PFS
	assert.Equal(t, len(paths), 1)
}

// TestListWorkspaces tests the listWorkspaces function
func TestListWorkspaces(t *testing.T) {
	ws1 := genMockWorkspaceForModel("ws1", "cluster1", "/apps")
	ws2 := genMockWorkspaceForModel("ws2", "cluster1", "/data")

	adminClient := fake.NewClientBuilder().
		WithObjects(ws1, ws2).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	workspaces, err := r.listWorkspaces(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(workspaces), 2)
}

// TestGetWorkspace tests the getWorkspace function
func TestGetWorkspace(t *testing.T) {
	ws := genMockWorkspaceForModel("ws1", "cluster1", "/apps")

	adminClient := fake.NewClientBuilder().
		WithObjects(ws).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	info, err := r.getWorkspace(context.Background(), "ws1")
	assert.NilError(t, err)
	assert.Equal(t, info.ID, "ws1")
	assert.Equal(t, info.PFSPath, "/apps")
}

// TestGetWorkspace_NotFound tests getWorkspace when workspace doesn't exist
func TestGetWorkspace_NotFound(t *testing.T) {
	adminClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	_, err := r.getWorkspace(context.Background(), "non-existent")
	assert.ErrorContains(t, err, "not found")
}

// TestExtractOpsJobFailureReason tests extractOpsJobFailureReason
func TestExtractOpsJobFailureReason(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		expected   string
	}{
		{
			name: "With failure condition",
			conditions: []metav1.Condition{
				{
					Type:    "Failed",
					Status:  metav1.ConditionTrue,
					Reason:  "DownloadFailed",
					Message: "S3 download failed",
				},
			},
			expected: "DownloadFailed: S3 download failed",
		},
		{
			name:       "Without failure condition",
			conditions: []metav1.Condition{},
			expected:   "Unknown error during download",
		},
		{
			name: "Failure condition with false status",
			conditions: []metav1.Condition{
				{
					Type:   "Failed",
					Status: metav1.ConditionFalse,
				},
			},
			expected: "Unknown error during download",
		},
	}

	r := newMockModelReconciler(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opsJob := &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: tt.conditions,
				},
			}
			result := r.extractOpsJobFailureReason(opsJob)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestExtractJobFailureReason tests extractJobFailureReason
func TestExtractJobFailureReason(t *testing.T) {
	backoffLimit := int32(3)

	tests := []struct {
		name       string
		conditions []batchv1.JobCondition
		failed     int32
		backoff    *int32
		expected   string
	}{
		{
			name: "With failure condition",
			conditions: []batchv1.JobCondition{
				{
					Type:    batchv1.JobFailed,
					Status:  corev1.ConditionTrue,
					Reason:  "BackoffLimitExceeded",
					Message: "Job has reached the specified backoff limit",
				},
			},
			failed:   3,
			backoff:  &backoffLimit,
			expected: "BackoffLimitExceeded: Job has reached the specified backoff limit",
		},
		{
			name:       "Without failure condition but backoff exceeded",
			conditions: []batchv1.JobCondition{},
			failed:     3,
			backoff:    &backoffLimit,
			expected:   "Maximum retry attempts exceeded",
		},
		{
			name:       "Unknown error",
			conditions: []batchv1.JobCondition{},
			failed:     1,
			backoff:    &backoffLimit,
			expected:   "Unknown error during download",
		},
	}

	r := newMockModelReconciler(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &batchv1.Job{
				Spec: batchv1.JobSpec{
					BackoffLimit: tt.backoff,
				},
				Status: batchv1.JobStatus{
					Conditions: tt.conditions,
					Failed:     tt.failed,
				},
			}
			result := r.extractJobFailureReason(job)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestModelMethods tests Model type methods
func TestModelMethods(t *testing.T) {
	t.Run("IsRemoteAPI", func(t *testing.T) {
		model := genMockRemoteAPIModel("test")
		assert.Equal(t, model.IsRemoteAPI(), true)
		assert.Equal(t, model.IsLocal(), false)
	})

	t.Run("IsLocal", func(t *testing.T) {
		model := genMockLocalModel("test", "")
		assert.Equal(t, model.IsLocal(), true)
		assert.Equal(t, model.IsRemoteAPI(), false)
	})

	t.Run("IsPublic", func(t *testing.T) {
		publicModel := genMockLocalModel("test", "")
		assert.Equal(t, publicModel.IsPublic(), true)

		privateModel := genMockLocalModel("test", "workspace1")
		assert.Equal(t, privateModel.IsPublic(), false)
	})

	t.Run("GetModelName", func(t *testing.T) {
		model := genMockRemoteAPIModel("test")
		assert.Equal(t, model.GetModelName(), "gpt-4")

		model2 := genMockLocalModel("test", "")
		model2.Spec.Source.ModelName = ""
		assert.Equal(t, model2.GetModelName(), "Test-Model") // Falls back to display name
	})

	t.Run("GetS3Path", func(t *testing.T) {
		model := genMockLocalModel("test", "")
		model.Status.S3Path = "models/custom-path"
		assert.Equal(t, model.GetS3Path(), "models/custom-path")

		model2 := genMockLocalModel("test2", "")
		model2.Status.S3Path = ""
		assert.Equal(t, model2.GetS3Path(), "models/Test-Model")
	})

	t.Run("GetSafeDisplayName", func(t *testing.T) {
		model := genMockLocalModel("test", "")
		model.Spec.DisplayName = "Qwen/Qwen2.5-0.5B"
		assert.Equal(t, model.GetSafeDisplayName(), "Qwen-Qwen2.5-0.5B")
	})
}

// TestModelPhaseTransitions tests model phase transitions
func TestModelPhaseTransitions(t *testing.T) {
	model := genMockLocalModel("test-model", "")

	t.Run("IsPending", func(t *testing.T) {
		model.Status.Phase = ""
		assert.Equal(t, model.IsPending(), true)
		model.Status.Phase = v1.ModelPhasePending
		assert.Equal(t, model.IsPending(), true)
	})

	t.Run("IsUploading", func(t *testing.T) {
		model.Status.Phase = v1.ModelPhaseUploading
		assert.Equal(t, model.IsUploading(), true)
	})

	t.Run("IsDownloading", func(t *testing.T) {
		model.Status.Phase = v1.ModelPhaseDownloading
		assert.Equal(t, model.IsDownloading(), true)
	})

	t.Run("IsReady", func(t *testing.T) {
		model.Status.Phase = v1.ModelPhaseReady
		assert.Equal(t, model.IsReady(), true)
	})

	t.Run("IsFailed", func(t *testing.T) {
		model.Status.Phase = v1.ModelPhaseFailed
		assert.Equal(t, model.IsFailed(), true)
	})
}

// TestGetLocalPathForWorkspace tests GetLocalPathForWorkspace method
func TestGetLocalPathForWorkspace(t *testing.T) {
	model := genMockLocalModel("test-model", "")
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test",
			Status:    v1.LocalPathStatusReady,
		},
		{
			Workspace: "ws2",
			Path:      "/data/models/test",
			Status:    v1.LocalPathStatusFailed,
		},
	}

	t.Run("Existing workspace", func(t *testing.T) {
		lp := model.GetLocalPathForWorkspace("ws1")
		assert.Assert(t, lp != nil)
		assert.Equal(t, lp.Path, "/apps/models/test")
	})

	t.Run("Non-existing workspace", func(t *testing.T) {
		lp := model.GetLocalPathForWorkspace("ws3")
		assert.Assert(t, lp == nil)
	})
}

// TestIsReadyInWorkspace tests IsReadyInWorkspace method
func TestIsReadyInWorkspace(t *testing.T) {
	model := genMockLocalModel("test-model", "")
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test",
			Status:    v1.LocalPathStatusReady,
		},
		{
			Workspace: "ws2",
			Path:      "/data/models/test",
			Status:    v1.LocalPathStatusFailed,
		},
	}

	assert.Equal(t, model.IsReadyInWorkspace("ws1"), true)
	assert.Equal(t, model.IsReadyInWorkspace("ws2"), false)
	assert.Equal(t, model.IsReadyInWorkspace("ws3"), false)
}

// TestGetReadyWorkspaces tests GetReadyWorkspaces method
func TestGetReadyWorkspaces(t *testing.T) {
	model := genMockLocalModel("test-model", "")
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test",
			Status:    v1.LocalPathStatusReady,
		},
		{
			Workspace: "ws2",
			Path:      "/data/models/test",
			Status:    v1.LocalPathStatusFailed,
		},
		{
			Workspace: "ws3",
			Path:      "/storage/models/test",
			Status:    v1.LocalPathStatusReady,
		},
	}

	workspaces := model.GetReadyWorkspaces()
	assert.Equal(t, len(workspaces), 2)
	assert.Equal(t, workspaces[0], "ws1")
	assert.Equal(t, workspaces[1], "ws3")
}

// TestWorkspaceInfo tests WorkspaceInfo struct
func TestWorkspaceInfo(t *testing.T) {
	info := WorkspaceInfo{
		ID:      "test-workspace",
		PFSPath: "/apps",
	}
	assert.Equal(t, info.ID, "test-workspace")
	assert.Equal(t, info.PFSPath, "/apps")
}

// TestModelReconcile_Deletion tests full deletion flow
func TestModelReconcile_Deletion(t *testing.T) {
	model := genMockLocalModel("test-local-model", "")
	model.Status.Phase = v1.ModelPhaseReady
	now := metav1.Now()
	model.DeletionTimestamp = &now
	controllerutil.AddFinalizer(model, ModelFinalizer)

	// Register batchv1 scheme for Job resource
	testScheme := scheme.Scheme
	_ = batchv1.AddToScheme(testScheme)

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(testScheme).
		Build()

	r := newMockModelReconciler(adminClient)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: model.Name},
	}

	// Since S3 is not enabled in tests, the cleanup should still proceed
	// and remove the finalizer
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)
}

// TestModelStatusUpdateTime tests that UpdateTime is set properly
func TestModelStatusUpdateTime(t *testing.T) {
	model := genMockRemoteAPIModel("test-model")

	adminClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	r := newMockModelReconciler(adminClient)

	req := ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: model.Name},
	}
	_, err := r.Reconcile(context.Background(), req)
	assert.NilError(t, err)

	err = adminClient.Get(context.Background(), client.ObjectKey{Name: model.Name}, model)
	assert.NilError(t, err)
	assert.Assert(t, model.Status.UpdateTime != nil)
	// Check that the time is recent (within last minute)
	assert.Assert(t, time.Since(model.Status.UpdateTime.Time) < time.Minute)
}

// TestModelConstants tests model-related constants
func TestModelConstants(t *testing.T) {
	assert.Equal(t, ModelFinalizer, "model.amd.com/finalizer")
	assert.Equal(t, CleanupJobPrefix, "cleanup-")
	assert.Equal(t, DownloadJobPrefix, "download-")
}

// TestModelKindConstant tests Model kind constant
func TestModelKindConstant(t *testing.T) {
	assert.Equal(t, v1.ModelKind, "Model")
}

// TestSourceModelLabel tests SourceModelLabel constant
func TestSourceModelLabel(t *testing.T) {
	assert.Equal(t, v1.SourceModelLabel, "primus-safe/source-model")
}

// TestAccessModeConstants tests access mode constants
func TestAccessModeConstants(t *testing.T) {
	assert.Equal(t, string(v1.AccessModeRemoteAPI), "remote_api")
	assert.Equal(t, string(v1.AccessModeLocal), "local")
}

// TestModelPhaseConstants tests model phase constants
func TestModelPhaseConstants(t *testing.T) {
	assert.Equal(t, string(v1.ModelPhasePending), "Pending")
	assert.Equal(t, string(v1.ModelPhaseUploading), "Uploading")
	assert.Equal(t, string(v1.ModelPhaseDownloading), "Downloading")
	assert.Equal(t, string(v1.ModelPhaseReady), "Ready")
	assert.Equal(t, string(v1.ModelPhaseFailed), "Failed")
}

// TestLocalPathStatusConstants tests local path status constants
func TestLocalPathStatusConstants(t *testing.T) {
	assert.Equal(t, string(v1.LocalPathStatusPending), "Pending")
	assert.Equal(t, string(v1.LocalPathStatusDownloading), "Downloading")
	assert.Equal(t, string(v1.LocalPathStatusReady), "Ready")
	assert.Equal(t, string(v1.LocalPathStatusFailed), "Failed")
}

