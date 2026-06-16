/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestParsePreflightReport(t *testing.T) {
	input := `2026-03-13 15:05:38
[GPU6] Step [11/12] Loss: 7.6475 LR: 5.00e-05 Grad Norm: 3.1557 ETA: 0:06
2026-03-13 15:05:38
[GPU6] Training ended at step 12 [GPU 6]
================================================================================
                    PrimusBench Node Check Report
================================================================================
Generated at: 2026-03-13 01:49:10
================================================================================

Failed Nodes (Node Check) - 2 nodes
--------------------------------------------------------------------------------
  uswslocpm2m-106-1792 (10.158.173.117): [uswslocpm2m-106-1792] [NODE-53] [NODE] [ERROR]: ERROR: Could not find a version that satisfies the requirement tensorboard (from versions: none)ERROR: No matching distribution found for tensorboard[2026-03-13 01:19:12] ERROR: Failed to install dependencies 
  uswslocpm2m-106-1647 (10.158.162.130): [babel_stream_memory.sh] failed to clone babel_stream  

Failed Nodes (Network Check) - 2 nodes
--------------------------------------------------------------------------------
  uswslocpm2m-106-1177 (10.158.160.198)
  uswslocpm2m-106-1909 (10.158.175.187)

Healthy Nodes (Passed All Checks) - 2 nodes
--------------------------------------------------------------------------------
  uswslocpm2m-106-1625 (10.158.160.255)
  uswslocpm2m-106-1724 (10.158.160.237)

================================================================================

Summary: 2 healthy nodes out of 6 total nodes checked

================================================================================`

	report := parsePreflightReport([]byte(input))
	assert.NotNil(t, report, "parsePreflightReport should return non-nil when report format is found")

	expectedFailed := []string{
		"uswslocpm2m-106-1792",
		"uswslocpm2m-106-1647",
		"uswslocpm2m-106-1177",
		"uswslocpm2m-106-1909",
	}
	expectedHealthy := []string{
		"uswslocpm2m-106-1625",
		"uswslocpm2m-106-1724",
	}

	assert.Equal(t, expectedFailed, report.FailedNodes, "FailedNodes should match expected")
	assert.Equal(t, expectedHealthy, report.HealthyNodes, "HealthyNodes should match expected")
}

func TestParsePreflightReport_NoReport(t *testing.T) {
	input := `2026-03-13 15:05:38
[GPU6] Step [11/12] Loss: 7.6475 LR: 5.00e-05
Some random log without PrimusBench report`
	report := parsePreflightReport([]byte(input))
	assert.Nil(t, report, "parsePreflightReport should return nil when report format is not found")
}

// TestIsPreflightWorkload verifies detection of preflight workloads from labels.
func TestIsPreflightWorkload(t *testing.T) {
	t.Run("preflight when ops job id and type labels match", func(t *testing.T) {
		w := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					v1.OpsJobIdLabel:   "job-1",
					v1.OpsJobTypeLabel: string(v1.OpsJobPreflightType),
				},
			},
		}
		assert.True(t, isPreflightWorkload(w))
	})
	t.Run("not preflight when ops job id empty", func(t *testing.T) {
		w := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					v1.OpsJobTypeLabel: string(v1.OpsJobPreflightType),
				},
			},
		}
		assert.False(t, isPreflightWorkload(w))
	})
	t.Run("not preflight when type is not preflight", func(t *testing.T) {
		w := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					v1.OpsJobIdLabel:   "job-1",
					v1.OpsJobTypeLabel: string(v1.OpsJobAddonType),
				},
			},
		}
		assert.False(t, isPreflightWorkload(w))
	})
}

// TestPreflightHandle drives the handle entry for both the pending-init and
// workload-creation branches.
func TestPreflightHandle(t *testing.T) {
	img := "registry.example/preflight:latest"
	entry := "ZWNobyBoZWxsbw=="
	res := &v1.WorkloadResource{Replica: 1, CPU: "4", Memory: "8Gi"}
	node, nf := testNodeAndFlavor(t, "n1", "flavor-1", false)

	newJob := func(name string) *v1.OpsJob {
		return &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.UserIdLabel: "u1"},
			},
			Spec: v1.OpsJobSpec{
				Type:       v1.OpsJobPreflightType,
				Resource:   res,
				Image:      &img,
				EntryPoint: &entry,
				Inputs:     []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
			},
		}
	}

	t.Run("pending init patches status and requeues", func(t *testing.T) {
		job := newJob("pj-pending")
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
			WithStatusSubresource(&v1.OpsJob{}).WithObjects(job, node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		_, err := r.handle(context.Background(), job)
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: "pj-pending"}, updated))
		assert.Equal(t, v1.OpsJobPending, updated.Status.Phase)
	})

	t.Run("running creates preflight workload", func(t *testing.T) {
		job := newJob("pj-run")
		job.Status.Phase = v1.OpsJobRunning
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
			WithStatusSubresource(&v1.OpsJob{}).WithObjects(job, node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		_, err := r.handle(context.Background(), job)
		assert.NoError(t, err)
		wl := &v1.Workload{}
		assert.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: "pj-run"}, wl))
	})
}

// TestPreflightJobReconciler_filter verifies skip vs reconcile gating by job type.
func TestPreflightJobReconciler_filter(t *testing.T) {
	r := &PreflightJobReconciler{}
	ctx := context.Background()

	t.Run("skip non-preflight jobs", func(t *testing.T) {
		job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType}}
		assert.True(t, r.filter(ctx, job))
	})
	t.Run("process preflight jobs", func(t *testing.T) {
		job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobPreflightType}}
		assert.False(t, r.filter(ctx, job))
	})
}

// TestPreflightJobReconciler_observe verifies observe returns quit when job has ended.
func TestPreflightJobReconciler_observe(t *testing.T) {
	r := &PreflightJobReconciler{}
	ctx := context.Background()

	now := metav1.Now()
	ended := &v1.OpsJob{Status: v1.OpsJobStatus{Phase: v1.OpsJobSucceeded, FinishedAt: &now}}
	quit, err := r.observe(ctx, ended)
	require.NoError(t, err)
	assert.True(t, quit)

	running := &v1.OpsJob{Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning}}
	quit, err = r.observe(ctx, running)
	require.NoError(t, err)
	assert.False(t, quit)
}

func testNodeAndFlavor(t *testing.T, nodeName, flavorName string, withGPU bool) (*v1.Node, *v1.NodeFlavor) {
	t.Helper()
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: flavorName},
		Spec: v1.NodeFlavorSpec{
			Cpu: v1.CpuChip{
				Quantity: resource.MustParse("4"),
			},
			Memory: resource.MustParse("8Gi"),
		},
	}
	if withGPU {
		nf.Spec.Gpu = &v1.GpuChip{
			Product:      v1.GpuProduct("MI300X"),
			ResourceName: "amd.com/gpu",
			Quantity:     resource.MustParse("8"),
		}
	}
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				v1.NodeFlavorIdLabel: flavorName,
			},
		},
	}
	return node, nf
}

// TestGeneratePreflightWorkload verifies workload generation and validation errors.
func TestGeneratePreflightWorkload(t *testing.T) {
	img := "registry.example/preflight:latest"
	entry := "ZWNobyBoZWxsbw=="
	res := &v1.WorkloadResource{Replica: 1, CPU: "4", Memory: "8Gi"}

	t.Run("error when resource nil", func(t *testing.T) {
		node, nf := testNodeAndFlavor(t, "n1", "flavor-1", false)
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{Name: "pj-1"},
			Spec: v1.OpsJobSpec{
				Type:       v1.OpsJobPreflightType,
				Image:      &img,
				EntryPoint: &entry,
				Inputs:     []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
			},
		}
		_, err := r.generatePreflightWorkload(context.Background(), job)
		require.Error(t, err)
	})

	t.Run("error when node parameters empty", func(t *testing.T) {
		node, nf := testNodeAndFlavor(t, "n1", "flavor-1", false)
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{Name: "pj-1"},
			Spec: v1.OpsJobSpec{
				Type:       v1.OpsJobPreflightType,
				Resource:   res,
				Image:      &img,
				EntryPoint: &entry,
				Inputs:     []v1.Parameter{},
			},
		}
		_, err := r.generatePreflightWorkload(context.Background(), job)
		require.Error(t, err)
	})

	t.Run("error when node object missing", func(t *testing.T) {
		_, nf := testNodeAndFlavor(t, "n1", "flavor-1", false)
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{Name: "pj-1"},
			Spec: v1.OpsJobSpec{
				Type:       v1.OpsJobPreflightType,
				Resource:   res,
				Image:      &img,
				EntryPoint: &entry,
				Inputs:     []v1.Parameter{{Name: v1.ParameterNode, Value: "missing-node"}},
			},
		}
		_, err := r.generatePreflightWorkload(context.Background(), job)
		require.Error(t, err)
	})

	t.Run("success without GPU flavor sets workspace default and hostnames", func(t *testing.T) {
		node, nf := testNodeAndFlavor(t, "n1", "flavor-cpu", false)
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pj-cpu",
				Labels: map[string]string{
					v1.ClusterIdLabel:  "c1",
					v1.UserIdLabel:     "u1",
					v1.WorkspaceIdLabel: "",
				},
				Annotations: map[string]string{
					v1.UserNameAnnotation: "alice",
				},
			},
			Spec: v1.OpsJobSpec{
				Type:                    v1.OpsJobPreflightType,
				Resource:                res,
				Image:                   &img,
				EntryPoint:              &entry,
				TimeoutSecond:           3600,
				TTLSecondsAfterFinished: 300,
				Inputs: []v1.Parameter{
					{Name: v1.ParameterNode, Value: "n1"},
					{Name: v1.ParameterNode, Value: "n2"},
				},
			},
		}
		wl, err := r.generatePreflightWorkload(context.Background(), job)
		require.NoError(t, err)
		require.NotNil(t, wl)
		assert.Equal(t, job.Name, wl.Name)
		assert.Equal(t, "n1 n2", wl.Spec.CustomerLabels[v1.K8sHostName])
		assert.Equal(t, corev1.NamespaceDefault, wl.Spec.Workspace)
		require.NotNil(t, wl.Spec.Timeout)
		assert.Equal(t, 3600, *wl.Spec.Timeout)
		require.NotNil(t, wl.Spec.TTLSecondsAfterFinished)
		assert.Equal(t, 300, *wl.Spec.TTLSecondsAfterFinished)
		require.Len(t, wl.OwnerReferences, 1)
		assert.Equal(t, job.Name, wl.OwnerReferences[0].Name)
		_, hasGPUProduct := wl.Spec.Env[common.GPU_PRODUCT]
		assert.False(t, hasGPUProduct)
	})

	t.Run("success with GPU flavor injects GPU_PRODUCT", func(t *testing.T) {
		node, nf := testNodeAndFlavor(t, "gpu-node", "flavor-gpu", true)
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pj-gpu",
				Labels: map[string]string{
					v1.ClusterIdLabel:   "c1",
					v1.UserIdLabel:      "u1",
					v1.WorkspaceIdLabel: "ws-1",
				},
				Annotations: map[string]string{v1.UserNameAnnotation: "bob"},
			},
			Spec: v1.OpsJobSpec{
				Type:       v1.OpsJobPreflightType,
				Resource:   res,
				Image:      &img,
				EntryPoint: &entry,
				Inputs:     []v1.Parameter{{Name: v1.ParameterNode, Value: "gpu-node"}},
				Env:        map[string]string{"EXISTING": "1"},
			},
		}
		wl, err := r.generatePreflightWorkload(context.Background(), job)
		require.NoError(t, err)
		assert.Equal(t, "ws-1", wl.Spec.Workspace)
		assert.Equal(t, "1", wl.Spec.Env["EXISTING"])
		assert.Equal(t, string(nf.Spec.Gpu.Product), wl.Spec.Env[common.GPU_PRODUCT])
	})

	t.Run("GPU_PRODUCT not overwritten when already set on job env", func(t *testing.T) {
		node, nf := testNodeAndFlavor(t, "gpu-node", "flavor-gpu", true)
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node, nf).Build()
		r := &PreflightJobReconciler{OpsJobBaseReconciler: &OpsJobBaseReconciler{Client: cl}}
		job := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "pj-gpu-override",
				Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.UserIdLabel: "u1"},
				Annotations: map[string]string{
					v1.UserNameAnnotation: "bob",
				},
			},
			Spec: v1.OpsJobSpec{
				Type:       v1.OpsJobPreflightType,
				Resource:   res,
				Image:      &img,
				EntryPoint: &entry,
				Inputs:     []v1.Parameter{{Name: v1.ParameterNode, Value: "gpu-node"}},
				Env: map[string]string{
					common.GPU_PRODUCT: "CUSTOM_GPU",
				},
			},
		}
		wl, err := r.generatePreflightWorkload(context.Background(), job)
		require.NoError(t, err)
		assert.Equal(t, "CUSTOM_GPU", wl.Spec.Env[common.GPU_PRODUCT])
	})
}
