/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGenerateDestPath(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	tests := []struct {
		name              string
		job               *v1.OpsJob
		workspace         *v1.Workspace
		expectedDestValue string
		expectError       bool
	}{
		{
			name: "non-download type job should not modify destPath",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobPreflightType,
					Inputs: []v1.Parameter{
						{Name: v1.ParameterDestPath, Value: "data/file.tar"},
						{Name: v1.ParameterWorkspace, Value: "test-ws"},
					},
				},
			},
			workspace:         nil,
			expectedDestValue: "data/file.tar", // unchanged
			expectError:       false,
		},
		{
			name: "download type job with PFS volume workspace - destPath should be modified",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobDownloadType,
					Inputs: []v1.Parameter{
						{Name: v1.ParameterDestPath, Value: "data/file.tar"},
						{Name: v1.ParameterWorkspace, Value: "test-ws"},
					},
				},
			},
			workspace: &v1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ws"},
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: v1.PFS, MountPath: "/mnt/pfs"},
					},
				},
			},
			expectedDestValue: "/mnt/pfs/data/file.tar",
			expectError:       false,
		},
		{
			name: "download type job with non-PFS volume workspace - uses first volume",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobDownloadType,
					Inputs: []v1.Parameter{
						{Name: v1.ParameterDestPath, Value: "models/llama.bin"},
						{Name: v1.ParameterWorkspace, Value: "test-ws"},
					},
				},
			},
			workspace: &v1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ws"},
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: "hostpath", MountPath: "/data/shared"},
					},
				},
			},
			expectedDestValue: "/data/shared/models/llama.bin",
			expectError:       false,
		},
		{
			name: "download type job with workspace without volumes - should error",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobDownloadType,
					Inputs: []v1.Parameter{
						{Name: v1.ParameterDestPath, Value: "data/file.tar"},
						{Name: v1.ParameterWorkspace, Value: "test-ws"},
					},
				},
			},
			workspace: &v1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ws"},
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{},
				},
			},
			expectedDestValue: "data/file.tar", // unchanged on error
			expectError:       true,
		},
		{
			name: "download type job with multiple volumes - PFS has priority",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobDownloadType,
					Inputs: []v1.Parameter{
						{Name: v1.ParameterDestPath, Value: "output/result.json"},
						{Name: v1.ParameterWorkspace, Value: "test-ws"},
					},
				},
			},
			workspace: &v1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ws"},
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{Type: "hostpath", MountPath: "/data/local"},
						{Type: v1.PFS, MountPath: "/mnt/nfs"},
					},
				},
			},
			expectedDestValue: "/mnt/nfs/output/result.json",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with workspace if provided
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.workspace != nil {
				clientBuilder = clientBuilder.WithObjects(tt.workspace)
			}
			fakeClient := clientBuilder.Build()

			mutator := &OpsJobMutator{
				Client: fakeClient,
			}

			// Get the original destPath value for comparison
			originalDestParam := tt.job.GetParameter(v1.ParameterDestPath)
			originalValue := ""
			if originalDestParam != nil {
				originalValue = originalDestParam.Value
			}

			// Call generateDestPath
			err := mutator.generateDestPath(context.Background(), tt.job)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Re-query the parameter from job to verify modification
			destParam := tt.job.GetParameter(v1.ParameterDestPath)
			if destParam != nil {
				assert.Equal(t, tt.expectedDestValue, destParam.Value,
					"destPath should be modified from '%s' to '%s'", originalValue, tt.expectedDestValue)
			} else {
				assert.Equal(t, "", tt.expectedDestValue, "no destPath param expected")
			}
		})
	}
}
