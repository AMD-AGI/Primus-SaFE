/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
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

// opsNodeFlavor builds a node flavor for ops job tests.
func opsNodeFlavor(name string) *v1.NodeFlavor {
	return &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
			ExtendResources: corev1.ResourceList{
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
		},
	}
}

// TestOpsJobRemoveDuplicates verifies deduplication of job inputs.
func TestOpsJobRemoveDuplicates(t *testing.T) {
	m := &OpsJobMutator{}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterNode, Value: "n1"},
		{Name: v1.ParameterNode, Value: "n1"},
		{Name: v1.ParameterNode, Value: ""},
	}}}
	m.removeDuplicates(job)
	assert.Len(t, job.Spec.Inputs, 1)
}

// TestOpsJobHasDuplicateInput verifies duplicate parameter detection.
func TestOpsJobHasDuplicateInput(t *testing.T) {
	v := &OpsJobValidator{}
	a := []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}
	b := []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}
	assert.True(t, v.hasDuplicateInput(a, b, v1.ParameterNode))
	c := []v1.Parameter{{Name: v1.ParameterNode, Value: "n2"}}
	assert.False(t, v.hasDuplicateInput(a, c, v1.ParameterNode))
}

// TestOpsJobMutateMeta verifies labels and finalizer assignment.
func TestOpsJobMutateMeta(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "Job1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType},
	}
	assert.True(t, m.mutateMeta(context.Background(), job))
	assert.Equal(t, "job1", job.Name)
	assert.Equal(t, string(v1.OpsJobDownloadType), v1.GetLabel(job, v1.OpsJobTypeLabel))
}

// TestOpsJobMutateJobSpec verifies default TTL/timeout and input normalization.
func TestOpsJobMutateJobSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: "Foo"}}}}
	m.mutateJobSpec(context.Background(), job)
	assert.True(t, job.Spec.TTLSecondsAfterFinished > 0)
}

// TestOpsJobGenerateAddonTemplates verifies addon templates appended from node template.
func TestOpsJobGenerateAddonTemplates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	nt := &v1.NodeTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "nt1"},
		Spec:       v1.NodeTemplateSpec{AddOnTemplates: []string{"addon1"}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nt).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterNodeTemplate, Value: "nt1"},
	}}}
	m.generateAddonTemplates(context.Background(), job)
	assert.Equal(t, job.GetParameter(v1.ParameterAddonTemplate).Value, "addon1")
}

// TestOpsJobFilterUnhealthyNodes verifies unhealthy node filtering.
func TestOpsJobFilterUnhealthyNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	healthy := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status:     v1.NodeStatus{MachineStatus: v1.MachineStatus{Phase: v1.NodeReady}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(healthy).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{
		Type:          v1.OpsJobPreflightType,
		IsTolerateAll: true,
		Inputs:        []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}, {Name: v1.ParameterNode, Value: "missing"}},
	}}
	m.filterUnhealthyNodes(context.Background(), job)
	assert.Len(t, job.GetParameters(v1.ParameterNode), 1)
}

// TestOpsJobMutateOnCreation verifies the full create mutation path.
func TestOpsJobMutateOnCreation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: "x"}}},
	}
	assert.NoError(t, m.mutateOnCreation(context.Background(), job))
}

// TestOpsJobMutatorHandle verifies the ops job mutator admission handler.
func TestOpsJobMutatorHandle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &OpsJobMutator{Client: k8sClient, decoder: newDecoder(t)}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: "x"}}},
	}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, job, nil))
	assert.True(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, job, nil))
	assert.True(t, resp.Allowed)
}

// opsJobWithDisplayName builds a job with a display name label.
func opsJobWithDisplayName(name string, jobType v1.OpsJobType) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.OpsJobSpec{Type: jobType, Inputs: []v1.Parameter{{Name: "x", Value: "y"}}},
	}
	v1.SetLabel(job, v1.DisplayNameLabel, "my-job")
	return job
}

// TestOpsJobValidateRequiredParams verifies required parameter validation.
func TestOpsJobValidateRequiredParams(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NoError(t, v.validateRequiredParams(context.Background(), opsJobWithDisplayName("job1", v1.OpsJobDownloadType)))
	assert.Error(t, v.validateRequiredParams(context.Background(), &v1.OpsJob{}))
}

// TestOpsJobValidateImmutableFields verifies immutable field checks.
func TestOpsJobValidateImmutableFields(t *testing.T) {
	v := &OpsJobValidator{}
	oldJob := opsJobWithDisplayName("job1", v1.OpsJobDownloadType)
	assert.NoError(t, v.validateImmutableFields(opsJobWithDisplayName("job1", v1.OpsJobDownloadType), oldJob))

	changed := opsJobWithDisplayName("job1", v1.OpsJobAddonType)
	assert.Error(t, v.validateImmutableFields(changed, oldJob))
}

// TestOpsJobValidateAddon verifies addon job validation.
func TestOpsJobValidateAddon(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{
		Type:   v1.OpsJobAddonType,
		Inputs: []v1.Parameter{{Name: v1.ParameterScript, Value: "ZWNobw=="}},
	}}
	assert.NoError(t, v.validateAddon(context.Background(), job))

	noParam := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType}}
	assert.Error(t, v.validateAddon(context.Background(), noParam))
}

// TestOpsJobValidateDownload verifies download job validation.
func TestOpsJobValidateDownload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	assert.Error(t, v.validateDownload(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType}}))

	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{
		{Name: v1.ParameterEndpoint, Value: "http://x"},
		{Name: v1.ParameterDestPath, Value: "/data"},
		{Name: v1.ParameterSecret, Value: "secret"},
		{Name: v1.ParameterWorkspace, Value: "ws1"},
	}}}
	assert.NoError(t, v.validateDownload(context.Background(), job))
}

// TestOpsJobValidateDumpling verifies dumplog job validation.
func TestOpsJobValidateDumpling(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDumpLogType,
		Inputs: []v1.Parameter{{Name: v1.ParameterWorkload, Value: "w1"}}}}
	assert.NoError(t, v.validateDumpling(context.Background(), job))
}

// TestOpsJobListRelatedRunningJobs verifies listing running jobs.
func TestOpsJobListRelatedRunningJobs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	jobs, err := v.listRelatedRunningJobs(context.Background(), "cluster1", []string{string(v1.OpsJobPreflightType)})
	assert.NoError(t, err)
	assert.Len(t, jobs, 0)
}

// TestOpsJobValidateNodeDuplicated verifies duplicate node detection across jobs.
func TestOpsJobValidateNodeDuplicated(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobPreflightType}}
	assert.NoError(t, v.validateNodeDuplicated(context.Background(), job))
}

// TestOpsJobValidateNodes verifies node cluster/flavor consistency validation.
func TestOpsJobValidateNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "flavor1"}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	v := &OpsJobValidator{Client: k8sClient}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{
		Type:   v1.OpsJobPreflightType,
		Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
	}}
	assert.NoError(t, v.validateNodes(context.Background(), job))
}

// TestOpsJobValidatePreflight verifies preflight job validation.
func TestOpsJobValidatePreflight(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "flavor1"}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(node, opsNodeFlavor("flavor1")).Build()
	v := &OpsJobValidator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor1",
		}},
		Spec: v1.OpsJobSpec{
			Type:       v1.OpsJobPreflightType,
			Inputs:     []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
			Resource:   &v1.WorkloadResource{CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi", Replica: 1},
			Image:      pointer.String("img"),
			EntryPoint: pointer.String("ZWNobw=="),
		},
	}
	assert.NoError(t, v.validatePreflight(context.Background(), job))

	noResource := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobPreflightType}}
	assert.Error(t, v.validatePreflight(context.Background(), noResource))
}

// TestOpsJobValidateOnCreation verifies the full create validation path.
func TestOpsJobValidateOnCreation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NoError(t, v.validateOnCreation(context.Background(), opsJobWithDisplayName("job1", v1.OpsJobCDType)))
}

// TestOpsJobValidateOnUpdate verifies the update validation path.
func TestOpsJobValidateOnUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	job := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	assert.NoError(t, v.validateOnUpdate(context.Background(), job, job))
}

// TestOpsJobValidatorHandle verifies the ops job validator admission handler.
func TestOpsJobValidatorHandle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	job := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, job, nil))
	assert.True(t, resp.Allowed)
}
