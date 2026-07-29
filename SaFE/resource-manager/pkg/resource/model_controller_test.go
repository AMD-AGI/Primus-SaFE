/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
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

	workspaces, err := r.listWorkspaces(context.Background(), "")
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

	info, err := r.getWorkspace(context.Background(), "ws1", "")
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

	_, err := r.getWorkspace(context.Background(), "non-existent", "")
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

// --- merged from model_gomonkey_test.go ---

// patchS3Config patches the S3-related config getters so the construct* helpers succeed.
func patchS3Config(t *testing.T) *gomonkey.Patches {
	t.Helper()
	p := gomonkey.NewPatches()
	p.ApplyFunc(commonconfig.IsS3Enable, func() bool { return true })
	p.ApplyFunc(commonconfig.GetS3Endpoint, func() string { return "https://minio:9000" })
	p.ApplyFunc(commonconfig.GetS3AccessKey, func() string { return "ak" })
	p.ApplyFunc(commonconfig.GetS3SecretKey, func() string { return "sk" })
	p.ApplyFunc(commonconfig.GetS3Bucket, func() string { return "bucket" })
	p.ApplyFunc(commonconfig.GetModelDownloaderImage, func() string { return "downloader:1" })
	p.ApplyFunc(commonconfig.GetDownloadJoImage, func() string { return "download:1" })
	return p
}

func TestConstructDownloadJobFull(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	model.Spec.Source.URL = "hf://org/repo"
	r := newMockModelReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	job, err := r.constructDownloadJob(model)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, job)
}

func TestConstructCleanupJobFull(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	r := newMockModelReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	job, err := r.constructCleanupJob(model)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, job)
}

func TestConstructLocalDownloadOpsJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	model.Status.S3Path = "models/m1"
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(workspace).Build()
	r := newMockModelReconciler(cl)

	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: "models/m1"}
	job, err := r.constructLocalDownloadOpsJob(context.Background(), model, lp)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.OpsJobDownloadType, job.Spec.Type)
}

func TestModelHandleDeleteCreatesCleanupJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m-del", v1.AccessModeLocal, "ws1")
	now := metav1.Now()
	model.DeletionTimestamp = &now
	model.Finalizers = []string{ModelFinalizer}
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(model).
		WithObjects(model).
		Build()
	r := newMockModelReconciler(cl)
	res, err := r.handleDelete(context.Background(), model)
	testifyassert.NoError(t, err)
	testifyassert.True(t, res.RequeueAfter > 0)
}

func TestModelHandlePendingCreatesJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m-pend", v1.AccessModeLocal, "ws1")
	model.Spec.Source.URL = "hf://org/repo"
	model.Status.Phase = v1.ModelPhasePending
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(model).
		WithObjects(model).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.handlePending(context.Background(), model)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseUploading, model.Status.Phase)
}

func TestModelHandlePendingS3Import(t *testing.T) {
	model := genMockModel("m-s3", v1.AccessModeLocal, "ws1")
	model.Labels = map[string]string{v1.ModelS3ImportLabel: v1.TrueStr}
	model.Status.Phase = v1.ModelPhasePending
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(model).
		WithObjects(model, ws).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.handlePending(context.Background(), model)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseDownloading, model.Status.Phase)
}

func TestModelHandleUploadingJobLost(t *testing.T) {
	model := genMockModel("m-up", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	// No upload job -> phase Failed.
	_, err := r.handleUploading(context.Background(), model)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, model.Status.Phase)
}

func TestModelHandleUploadingSucceeded(t *testing.T) {
	model := genMockModel("m-up3", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "m-up3", Namespace: "primus-safe"},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model, job, ws).Build()
	r := newMockModelReconciler(cl)
	_, err := r.handleUploading(context.Background(), model)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseDownloading, model.Status.Phase)
}

func TestModelHandleUploadingFailed(t *testing.T) {
	model := genMockModel("m-up4", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "m-up4", Namespace: "primus-safe"},
		Status:     batchv1.JobStatus{Failed: 1, Active: 0},
	}
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model, job).Build()
	r := newMockModelReconciler(cl)
	_, err := r.handleUploading(context.Background(), model)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, model.Status.Phase)
}

func TestModelHandleDownloadingCreatesOpsJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m-dl", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/m-dl"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: "ws1", Path: "/data/models/m-dl", Status: v1.LocalPathStatusPending},
	}
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	cl := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(model).
		WithObjects(model, workspace).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.handleDownloading(context.Background(), model)
	testifyassert.NoError(t, err)
}

func TestModelHandleUploadingInProgress(t *testing.T) {
	model := genMockModel("m-up2", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	jobName := "m-up2"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "primus-safe"},
		Status:     batchv1.JobStatus{Active: 1},
	}
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model, job).Build()
	r := newMockModelReconciler(cl)
	res, err := r.handleUploading(context.Background(), model)
	testifyassert.NoError(t, err)
	testifyassert.True(t, res.RequeueAfter > 0)
}

func TestModelHandleDeleteNoFinalizer(t *testing.T) {
	model := genMockModel("m-del", v1.AccessModeLocal, "ws1")
	now := metav1.Now()
	model.DeletionTimestamp = &now
	model.Finalizers = []string{"other-finalizer"}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	// No model finalizer -> nothing to do.
	res, err := r.handleDelete(context.Background(), model)
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestConstructLocalDownloadOpsJobNoCluster(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(workspace).Build()
	r := newMockModelReconciler(cl)
	lp := &v1.ModelLocalPath{Workspace: "ws1"}
	_, err := r.constructLocalDownloadOpsJob(context.Background(), model, lp)
	testifyassert.Error(t, err)
}

// --- merged from model_handle_full_test.go ---

func TestHandlePendingLocalModelFull(t *testing.T) {
	model := genMockLocalModel("local-pending", "")
	model.Status.Phase = v1.ModelPhasePending

	mockScheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, batchv1.AddToScheme(mockScheme))
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)

	_, err = r.handlePending(context.Background(), model)
	testifyassert.NoError(t, err)
	// Local model either starts uploading or fails when the download job can't be built.
	testifyassert.Contains(t, []v1.ModelPhase{v1.ModelPhaseUploading, v1.ModelPhaseFailed}, model.Status.Phase)
}

func TestHandleDeleteLocalModelFull(t *testing.T) {
	model := genMockLocalModel("local-delete", "")
	controllerutil.AddFinalizer(model, ModelFinalizer)

	mockScheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, batchv1.AddToScheme(mockScheme))
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)

	_, err = r.handleDelete(context.Background(), model)
	testifyassert.NoError(t, err)
}

func TestModelFailoverHelpers(t *testing.T) {
	r := &ModelReconciler{}
	assert.Equal(t, "/wekafs", r.extractBasePath("/wekafs/models/llama"))
	assert.Equal(t, "", r.extractBasePath("/no-models-here"))

	model := genMockLocalModel("m-tried", "")
	// initially empty
	testifyassert.Empty(t, r.getTriedWorkspaces(model, "/wekafs"))
	// set then read back
	r.setTriedWorkspaces(model, "/wekafs", []string{"ws1", "ws2"})
	got := r.getTriedWorkspaces(model, "/wekafs")
	assert.Equal(t, []string{"ws1", "ws2"}, got)
}

func TestTryFailoverFull(t *testing.T) {
	model := genMockLocalModel("failover", "")
	mockScheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)
	ctx := context.Background()

	// empty path -> cannot determine base path
	testifyassert.False(t, r.tryFailover(ctx, model, &v1.ModelLocalPath{Workspace: "ws1", Path: ""}))

	// valid path but no other workspaces sharing it -> no candidates
	testifyassert.False(t, r.tryFailover(ctx, model, &v1.ModelLocalPath{Workspace: "ws1", Path: "/wekafs/models/failover"}))
}

func TestHandleDeleteNoFinalizer(t *testing.T) {
	model := genMockLocalModel("local-nofin", "")
	mockScheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)

	_, err = r.handleDelete(context.Background(), model)
	testifyassert.NoError(t, err)
}

// --- merged from model_helpers_test.go ---

func batchv1AddToScheme(s *runtime.Scheme) error { return batchv1.AddToScheme(s) }

func reconcileReq(name string) ctrlruntime.Request {
	return ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func clientKey(name string) client.ObjectKey { return client.ObjectKey{Name: name} }

func TestBuildLocalModelPath(t *testing.T) {
	assert.Equal(t, "/root/models/m1", buildLocalModelPath("/root/", "", "m1"))
	assert.Equal(t, "/root/sub/models/m1", buildLocalModelPath("/root", "/sub/", "m1"))
}

func TestModelSetAndGetTriedWorkspaces(t *testing.T) {
	r := newMockModelReconciler(nil)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	// Empty -> nil.
	testifyassert.Nil(t, r.getTriedWorkspaces(model, "/base"))
	r.setTriedWorkspaces(model, "/base", []string{"ws1", "ws2"})
	got := r.getTriedWorkspaces(model, "/base")
	assert.Equal(t, []string{"ws1", "ws2"}, got)
	// Different base -> nil.
	testifyassert.Nil(t, r.getTriedWorkspaces(model, "/other"))
}

func TestAppendUniqueAndContains(t *testing.T) {
	s := appendUnique(nil, "a")
	s = appendUnique(s, "a")
	s = appendUnique(s, "b")
	assert.Equal(t, []string{"a", "b"}, s)
	testifyassert.True(t, containsString(s, "a"))
	testifyassert.False(t, containsString(s, "z"))
}

func TestModelGetWorkspace(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	info, err := r.getWorkspace(context.Background(), "ws1", "")
	testifyassert.NoError(t, err)
	assert.Equal(t, "ws1", info.ID)

	_, err = r.getWorkspace(context.Background(), "missing", "")
	testifyassert.Error(t, err)
}

func TestModelListWorkspaces(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	list, err := r.listWorkspaces(context.Background(), "")
	testifyassert.NoError(t, err)
	testifyassert.Len(t, list, 1)
}

func modelSchemeWithBatch(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	testifyassert.NoError(t, batchv1AddToScheme(s))
	return s
}

func TestModelReconcileUploadingDispatch(t *testing.T) {
	model := genMockModel("m-u", v1.AccessModeLocal, "ws1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseUploading
	s := modelSchemeWithBatch(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	// Dispatches to handleUploading; no job -> Failed.
	_, err := r.Reconcile(context.Background(), reconcileReq("m-u"))
	testifyassert.NoError(t, err)
}

func TestModelReconcileReadyNoop(t *testing.T) {
	model := genMockModel("m-r", v1.AccessModeLocal, "ws1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseReady
	s := modelSchemeWithBatch(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	res, err := r.Reconcile(context.Background(), reconcileReq("m-r"))
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestModelReconcileLocalPathMode(t *testing.T) {
	model := genMockModel("m-lp", v1.AccessModeLocalPath, "ws1")
	model.Spec.Source.LocalPath = "/data/models/m-lp"
	cl := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(model).
		WithObjects(model).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.Reconcile(context.Background(), reconcileReq("m-lp"))
	testifyassert.NoError(t, err)
	updated := &v1.Model{}
	testifyassert.NoError(t, cl.Get(context.Background(), clientKey("m-lp"), updated))
	assert.Equal(t, v1.ModelPhaseReady, updated.Status.Phase)
	testifyassert.Len(t, updated.Status.LocalPaths, 1)
}

func TestModelExtractBasePath(t *testing.T) {
	r := newMockModelReconciler(nil)
	assert.Equal(t, "/wekafs", r.extractBasePath("/wekafs/models/llama"))
	assert.Equal(t, "", r.extractBasePath("/nomatch"))
}

func TestModelTryFailoverNoBasePath(t *testing.T) {
	r := newMockModelReconciler(nil)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: "/nobase"}
	testifyassert.False(t, r.tryFailover(context.Background(), model, lp))
}

func TestModelTryFailoverNoCandidates(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: "/wekafs/models/m1"}
	// Only ws1 exists and it's the failed one -> no candidates.
	testifyassert.False(t, r.tryFailover(context.Background(), model, lp))
}

func TestModelInitializeLocalPathsPrivate(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	paths := r.initializeLocalPaths(context.Background(), model)
	testifyassert.Len(t, paths, 1)
	assert.Equal(t, "ws1", paths[0].Workspace)
}
