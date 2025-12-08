/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/constvar"
)

// TestGetWorkspaceMountPath tests the getWorkspaceMountPath helper function
func TestGetWorkspaceMountPath(t *testing.T) {
	tests := []struct {
		name      string
		workspace *v1.Workspace
		expected  string
	}{
		{
			name: "workspace with PFS volume",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: v1.PFS, MountPath: "/wekafs"},
						{Type: v1.HOSTPATH, MountPath: "/apps"},
					},
				},
			},
			expected: "/wekafs",
		},
		{
			name: "workspace with only hostpath volume",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: v1.HOSTPATH, MountPath: "/apps"},
					},
				},
			},
			expected: "/apps",
		},
		{
			name: "workspace with no volumes",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{},
				},
			},
			expected: "",
		},
		{
			name: "workspace with volume without mount path",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: v1.PFS, MountPath: ""},
					},
				},
			},
			expected: "",
		},
		{
			name: "workspace with multiple volumes, PFS first",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: v1.PFS, MountPath: "/wekafs1"},
						{Type: v1.PFS, MountPath: "/wekafs2"},
					},
				},
			},
			expected: "/wekafs1", // Should return first PFS
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWorkspaceMountPath(tt.workspace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestInferenceReconciler_NewReconciler tests creating a new InferenceReconciler
func TestInferenceReconciler_NewReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	assert.NotNil(t, r)
	assert.NotNil(t, r.Client)
}

// TestInferenceReconciler_AddFinalizerIfNeeded tests adding finalizer
func TestInferenceReconciler_AddFinalizerIfNeeded(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result := r.addFinalizerIfNeeded(context.Background(), inference)
	assert.True(t, result)
}

// TestInferenceReconciler_AddFinalizerIfNeeded_AlreadyExists tests when finalizer already exists
func TestInferenceReconciler_AddFinalizerIfNeeded_AlreadyExists(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-inference",
			Finalizers: []string{v1.InferenceFinalizer},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result := r.addFinalizerIfNeeded(context.Background(), inference)
	assert.True(t, result)
}

// TestInferenceReconciler_Reconcile_NotFound tests reconcile when inference is not found
func TestInferenceReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	req := ctrl.Request{NamespacedName: client.ObjectKey{Name: "non-existent"}}
	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestInferenceReconciler_Reconcile_APIInference tests reconcile for API inference
func TestInferenceReconciler_Reconcile_APIInference(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-inference",
		},
		Spec: v1.InferenceSpec{
			ModelForm: constvar.InferenceModelFormAPI,
		},
		Status: v1.InferenceStatus{
			Phase: constvar.InferencePhaseRunning,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	req := ctrl.Request{NamespacedName: client.ObjectKey{Name: "api-inference"}}
	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestInferenceReconciler_HandleStopped tests handling stopped inference
func TestInferenceReconciler_HandleStopped(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "stopped-inference",
		},
		Status: v1.InferenceStatus{
			Phase: constvar.InferencePhaseStopped,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handleStopped(context.Background(), inference)
	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestInferenceReconciler_HandleTerminalState tests handling terminal state
func TestInferenceReconciler_HandleTerminalState(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "failed-inference",
		},
		Status: v1.InferenceStatus{
			Phase: constvar.InferencePhaseFailure,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handleTerminalState(context.Background(), inference)
	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestInferenceReconciler_HandleRunning_NoWorkloadID tests handling running inference without workload ID
func TestInferenceReconciler_HandleRunning_NoWorkloadID(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "running-inference",
		},
		Spec: v1.InferenceSpec{
			ModelForm: constvar.InferenceModelFormModelSquare,
			Instance: v1.InferenceInstance{
				WorkloadID: "", // No workload ID
			},
		},
		Status: v1.InferenceStatus{
			Phase: constvar.InferencePhaseRunning,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		WithStatusSubresource(&v1.Inference{}).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handleRunning(context.Background(), inference)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify inference status is updated to Failure
	updatedInference := &v1.Inference{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "running-inference"}, updatedInference)
	require.NoError(t, err)
	assert.Equal(t, constvar.InferencePhaseFailure, updatedInference.Status.Phase)
}

// TestInferenceReconciler_UpdatePhase tests updating inference phase
func TestInferenceReconciler_UpdatePhase(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Status: v1.InferenceStatus{
			Phase:   constvar.InferencePhasePending,
			Message: "Initial",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		WithStatusSubresource(&v1.Inference{}).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.updatePhase(context.Background(), inference, constvar.InferencePhaseRunning, "Now running")
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify phase is updated
	updatedInference := &v1.Inference{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-inference"}, updatedInference)
	require.NoError(t, err)
	assert.Equal(t, constvar.InferencePhaseRunning, updatedInference.Status.Phase)
	assert.Equal(t, "Now running", updatedInference.Status.Message)
}

// TestInferenceReconciler_UpdatePhase_NoChange tests updatePhase when no change needed
func TestInferenceReconciler_UpdatePhase_NoChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Status: v1.InferenceStatus{
			Phase:   constvar.InferencePhaseRunning,
			Message: "Running",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference).
		WithStatusSubresource(&v1.Inference{}).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	// Call with same phase and message
	result, err := r.updatePhase(context.Background(), inference, constvar.InferencePhaseRunning, "Running")
	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

// TestInferenceReconciler_SyncWorkloadStatus tests syncing workload status
func TestInferenceReconciler_SyncWorkloadStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	tests := []struct {
		name          string
		workloadPhase v1.WorkloadPhase
		expectedPhase constvar.InferencePhaseType
	}{
		{
			name:          "workload pending",
			workloadPhase: v1.WorkloadPending,
			expectedPhase: constvar.InferencePhasePending,
		},
		{
			name:          "workload running",
			workloadPhase: v1.WorkloadRunning,
			expectedPhase: constvar.InferencePhaseRunning,
		},
		{
			name:          "workload failed",
			workloadPhase: v1.WorkloadFailed,
			expectedPhase: constvar.InferencePhaseFailure,
		},
		{
			name:          "workload stopped",
			workloadPhase: v1.WorkloadStopped,
			expectedPhase: constvar.InferencePhaseStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inference := &v1.Inference{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inference",
				},
				Spec: v1.InferenceSpec{
					Instance: v1.InferenceInstance{
						WorkloadID: "test-workload",
					},
				},
				Status: v1.InferenceStatus{
					Phase: constvar.InferencePhasePending,
				},
			}

			workload := &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workload",
				},
				Status: v1.WorkloadStatus{
					Phase: tt.workloadPhase,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(inference, workload).
				WithStatusSubresource(&v1.Inference{}).
				Build()

			r := &InferenceReconciler{
				ClusterBaseReconciler: &ClusterBaseReconciler{
					Client: k8sClient,
				},
			}

			_, err := r.syncWorkloadStatus(context.Background(), inference, workload)
			require.NoError(t, err)

			// Verify inference phase
			updatedInference := &v1.Inference{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-inference"}, updatedInference)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedPhase, updatedInference.Status.Phase)
		})
	}
}

// TestInferenceReconciler_Delete_WithApiKeySecret tests deletion with ApiKey secret
func TestInferenceReconciler_Delete_WithApiKeySecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "api-inference",
			Finalizers: []string{v1.InferenceFinalizer},
		},
		Spec: v1.InferenceSpec{
			ModelForm: constvar.InferenceModelFormAPI,
			Instance: v1.InferenceInstance{
				ApiKey: &corev1.LocalObjectReference{Name: "api-key-secret"},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-key-secret",
			Namespace: "primus-safe",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference, secret).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	err := r.delete(context.Background(), inference)
	require.NoError(t, err)
}

// TestInferenceReconciler_Delete_ModelSquare tests deletion for ModelSquare inference
func TestInferenceReconciler_Delete_ModelSquare(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
		},
	}

	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workspace",
		},
		Spec: v1.WorkspaceSpec{
			Volumes: []v1.WorkspaceVolume{
				{Type: v1.PFS, MountPath: "/wekafs"},
			},
		},
	}

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "model-inference",
			Finalizers: []string{v1.InferenceFinalizer},
		},
		Spec: v1.InferenceSpec{
			ModelForm: constvar.InferenceModelFormModelSquare,
			ModelName: "test-model",
			Resource: v1.InferenceResource{
				Workspace: "test-workspace",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference, model, workspace).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	err := r.delete(context.Background(), inference)
	require.NoError(t, err)
}

// TestInferenceReconciler_UpdateInferenceInstance tests updating inference instance
func TestInferenceReconciler_UpdateInferenceInstance(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Spec: v1.InferenceSpec{
			Instance: v1.InferenceInstance{
				BaseUrl: "",
			},
		},
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload",
		},
		Spec: v1.WorkloadSpec{
			Workspace: "test-ns",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference, workload).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	err := r.updateInferenceInstance(context.Background(), inference, workload)
	require.NoError(t, err)

	// Verify baseUrl is updated
	updatedInference := &v1.Inference{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-inference"}, updatedInference)
	require.NoError(t, err)
	assert.Contains(t, updatedInference.Spec.Instance.BaseUrl, "test-workload")
}

// TestInferenceReconciler_HandlePending_WithExistingWorkload tests pending with existing workload
func TestInferenceReconciler_HandlePending_WithExistingWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-workload",
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Spec: v1.InferenceSpec{
			ModelForm: constvar.InferenceModelFormModelSquare,
			Instance: v1.InferenceInstance{
				WorkloadID: "existing-workload",
			},
		},
		Status: v1.InferenceStatus{
			Phase: constvar.InferencePhasePending,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference, workload).
		WithStatusSubresource(&v1.Inference{}).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	result, err := r.handlePending(context.Background(), inference)
	require.NoError(t, err)

	// Should sync workload status
	updatedInference := &v1.Inference{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-inference"}, updatedInference)
	require.NoError(t, err)
	assert.Equal(t, constvar.InferencePhaseRunning, updatedInference.Status.Phase)
	assert.True(t, result.RequeueAfter > 0)
}

// TestSyncInterval tests the SyncInterval constant
func TestSyncInterval(t *testing.T) {
	assert.Equal(t, 10*time.Minute, SyncInterval)
}

// TestInferenceDownloadJobPrefix tests the job prefix constant
func TestInferenceDownloadJobPrefix(t *testing.T) {
	assert.Equal(t, "inference-download-", InferenceDownloadJobPrefix)
}

// TestInferenceCleanupJobPrefix tests the cleanup job prefix constant
func TestInferenceCleanupJobPrefix(t *testing.T) {
	assert.Equal(t, "inference-cleanup-", InferenceCleanupJobPrefix)
}

// TestInferenceReconciler_CreateWorkload_ExistingWorkload tests createWorkload when workload exists
func TestInferenceReconciler_CreateWorkload_ExistingWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	existingWorkload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-workload",
			Labels: map[string]string{
				v1.InferenceIdLabel: "test-inference",
			},
		},
	}

	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Spec: v1.InferenceSpec{
			DisplayName: "Test Inference",
			UserID:      "user-123",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inference, existingWorkload).
		Build()

	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: k8sClient,
		},
	}

	workload, err := r.createWorkload(context.Background(), inference)
	require.NoError(t, err)
	assert.NotNil(t, workload)
	assert.Equal(t, "existing-workload", workload.Name)
}
