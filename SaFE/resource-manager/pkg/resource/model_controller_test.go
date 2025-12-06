/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestExtractHFRepoId tests the extractHFRepoId helper function
func TestExtractHFRepoId(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "full URL with org/model",
			url:      "https://huggingface.co/microsoft/phi-2",
			expected: "microsoft/phi-2",
		},
		{
			name:     "full URL with single model name",
			url:      "https://huggingface.co/gpt2",
			expected: "gpt2",
		},
		{
			name:     "full URL with trailing slash",
			url:      "https://huggingface.co/microsoft/phi-2/",
			expected: "microsoft/phi-2",
		},
		{
			name:     "repo_id format",
			url:      "microsoft/phi-2",
			expected: "microsoft/phi-2",
		},
		{
			name:     "single model name",
			url:      "gpt2",
			expected: "gpt2",
		},
		{
			name:     "URL with extra path",
			url:      "https://huggingface.co/meta-llama/Llama-2-7b-hf",
			expected: "meta-llama/Llama-2-7b-hf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHFRepoId(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNeedsCleanup tests the needsCleanup method
func TestNeedsCleanup(t *testing.T) {
	r := &ModelReconciler{}

	tests := []struct {
		name       string
		model      *v1.Model
		needsClean bool
	}{
		{
			name: "local mode needs cleanup",
			model: &v1.Model{
				Spec: v1.ModelSpec{
					Source: v1.ModelSource{
						AccessMode: v1.AccessModeLocal,
					},
				},
			},
			needsClean: true,
		},
		{
			name: "remote_api mode does not need cleanup",
			model: &v1.Model{
				Spec: v1.ModelSpec{
					Source: v1.ModelSource{
						AccessMode: v1.AccessModeRemoteAPI,
					},
				},
			},
			needsClean: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.needsCleanup(tt.model)
			assert.Equal(t, tt.needsClean, result)
		})
	}
}

// TestModelReconciler_NewReconciler tests creating a new ModelReconciler
func TestModelReconciler_NewReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	assert.NotNil(t, r)
	assert.NotNil(t, r.Client)
}

// TestModelReconciler_HandleDelete_NoFinalizer tests deletion without finalizer
func TestModelReconciler_HandleDelete_NoFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-model",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			// No finalizer
		},
		Spec: v1.ModelSpec{
			Source: v1.ModelSource{
				AccessMode: v1.AccessModeLocal,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handleDelete(context.Background(), model)
	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestModelReconciler_HandleDelete_RemoteAPI tests deletion for remote_api mode
func TestModelReconciler_HandleDelete_RemoteAPI(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-api-model",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{ModelFinalizer},
		},
		Spec: v1.ModelSpec{
			Source: v1.ModelSource{
				AccessMode: v1.AccessModeRemoteAPI,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handleDelete(context.Background(), model)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify finalizer was removed
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-api-model"}, updatedModel)
	require.NoError(t, err)
	assert.NotContains(t, updatedModel.Finalizers, ModelFinalizer)
}

// TestExtractJobFailureReason tests extracting failure reasons from Job
func TestExtractJobFailureReason(t *testing.T) {
	r := &ModelReconciler{}

	tests := []struct {
		name     string
		job      *batchv1.Job
		contains string
	}{
		{
			name: "job with failed condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed: 3,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job reached backoff limit",
						},
					},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: func() *int32 { i := int32(3); return &i }(),
				},
			},
			contains: "BackoffLimitExceeded",
		},
		{
			name: "job exceeded backoff limit",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed: 3,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: func() *int32 { i := int32(3); return &i }(),
				},
			},
			contains: "Maximum retry attempts",
		},
		{
			name: "job with unknown failure",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed: 1,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: func() *int32 { i := int32(3); return &i }(),
				},
			},
			contains: "Unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.extractJobFailureReason(tt.job)
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestModelReconciler_HandlePending_RemoteAPI tests handlePending for remote_api mode
func TestModelReconciler_HandlePending_RemoteAPI(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-api-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test API Model",
			Source: v1.ModelSource{
				URL:        "https://api.openai.com",
				AccessMode: v1.AccessModeRemoteAPI,
			},
		},
		Status: v1.ModelStatus{
			Phase: v1.ModelPhasePending,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handlePending(context.Background(), model)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify model status is updated to Ready
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-api-model"}, updatedModel)
	require.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseReady, updatedModel.Status.Phase)
}

// TestModelReconciler_SyncInferencePhase tests syncing inference phase
func TestModelReconciler_SyncInferencePhase(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Status: v1.InferenceStatus{
			Phase: "Running",
		},
	}

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Status: v1.ModelStatus{
			Phase:          v1.ModelPhaseReady,
			InferenceID:    "test-inference",
			InferencePhase: "Pending",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model, inference).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	err := r.syncInferencePhase(context.Background(), model)
	require.NoError(t, err)

	// Verify model's inference phase is updated
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-model"}, updatedModel)
	require.NoError(t, err)
	assert.Equal(t, "Running", updatedModel.Status.InferencePhase)
}

// TestModelReconciler_SyncInferencePhase_InferenceNotFound tests when inference is deleted
func TestModelReconciler_SyncInferencePhase_InferenceNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Status: v1.ModelStatus{
			Phase:          v1.ModelPhaseReady,
			InferenceID:    "non-existent-inference",
			InferencePhase: "Running",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	err := r.syncInferencePhase(context.Background(), model)
	require.NoError(t, err)

	// Verify inference fields are cleared
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-model"}, updatedModel)
	require.NoError(t, err)
	assert.Empty(t, updatedModel.Status.InferenceID)
	assert.Empty(t, updatedModel.Status.InferencePhase)
}

// TestModelReconciler_Reconcile_NotFound tests reconcile when model is not found
func TestModelReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	req := ctrl.Request{NamespacedName: client.ObjectKey{Name: "non-existent"}}
	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestModelReconciler_Reconcile_InitStatus tests initializing status
func TestModelReconciler_Reconcile_InitStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "new-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "New Model",
			Source: v1.ModelSource{
				AccessMode: v1.AccessModeRemoteAPI,
			},
		},
		Status: v1.ModelStatus{
			Phase: "", // Empty phase
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	req := ctrl.Request{NamespacedName: client.ObjectKey{Name: "new-model"}}
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Verify status is initialized
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), req.NamespacedName, updatedModel)
	require.NoError(t, err)
	assert.Equal(t, v1.ModelPhasePending, updatedModel.Status.Phase)
}

// TestModelReconciler_HandlePulling_JobSucceeded tests handlePulling when job succeeds
func TestModelReconciler_HandlePulling_JobSucceeded(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: v1.AccessModeLocal,
			},
		},
		Status: v1.ModelStatus{
			Phase: v1.ModelPhasePulling,
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: common.PrimusSafeNamespace,
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model, job).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handlePulling(context.Background(), model)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify model status is Ready
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-model"}, updatedModel)
	require.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseReady, updatedModel.Status.Phase)
}

// TestModelReconciler_HandlePulling_JobFailed tests handlePulling when job fails
func TestModelReconciler_HandlePulling_JobFailed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: v1.AccessModeLocal,
			},
		},
		Status: v1.ModelStatus{
			Phase: v1.ModelPhasePulling,
		},
	}

	backoffLimit := int32(3)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: common.PrimusSafeNamespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
		},
		Status: batchv1.JobStatus{
			Failed: 3,
			Active: 0,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model, job).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handlePulling(context.Background(), model)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify model status is Failed
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-model"}, updatedModel)
	require.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, updatedModel.Status.Phase)
}

// TestModelReconciler_HandlePulling_JobLost tests handlePulling when job is lost
func TestModelReconciler_HandlePulling_JobLost(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: v1.AccessModeLocal,
			},
		},
		Status: v1.ModelStatus{
			Phase: v1.ModelPhasePulling,
		},
	}

	// No job exists
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model).
		WithStatusSubresource(&v1.Model{}).
		Build()

	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handlePulling(context.Background(), model)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify model status is Failed
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-model"}, updatedModel)
	require.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, updatedModel.Status.Phase)
	assert.Contains(t, updatedModel.Status.Message, "lost")
}

// TestModelFinalizer tests the ModelFinalizer constant
func TestModelFinalizer(t *testing.T) {
	assert.Equal(t, "model.amd.com/finalizer", ModelFinalizer)
}

// TestCleanupJobPrefix tests the CleanupJobPrefix constant
func TestCleanupJobPrefix(t *testing.T) {
	assert.Equal(t, "cleanup-", CleanupJobPrefix)
}

// helper function to convert ObjectKey to ctrl.Request
func toCtrlRequest(key client.ObjectKey) ctrl.Request {
	return ctrl.Request{NamespacedName: key}
}
