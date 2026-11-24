/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
)

func TestModelPrewarmReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobModelPrewarmType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterModelPath, Value: "/models/glm"},
				{Name: v1.ParameterNode, Value: "node-1"},
			},
		},
	}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

// runPrewarmPod runs the entry script of a prewarm pod on this host and returns the pod phase and the
// termination message the script wrote.
func runPrewarmPod(t *testing.T, modelPath string) (v1.WorkloadPhase, string) {
	t.Helper()
	terminationLog := filepath.Join(t.TempDir(), "termination-log")
	script := modelprewarm.PodEntryScript(modelPath, modelprewarm.DefaultGlob, 2)
	script = strings.ReplaceAll(script, "/dev/termination-log", terminationLog)
	phase := v1.WorkloadSucceeded
	if err := exec.Command("bash", "-c", script).Run(); err != nil {
		phase = v1.WorkloadFailed
	}
	message, err := os.ReadFile(terminationLog)
	require.NoError(t, err)
	return phase, string(message)
}

// externalPrewarmResult runs the prewarm pod of a simulated external node and reads back its result.
func externalPrewarmResult(t *testing.T, modelPath string) *modelprewarm.Result {
	t.Helper()
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	phase, message := runPrewarmPod(t, modelPath)
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:        externalWorkloadName(job.Name, "admin-1"),
			Annotations: map[string]string{v1.MainContainerAnnotation: "main"},
		},
		Status: v1.WorkloadStatus{
			Phase: phase,
			Pods: []v1.WorkloadPod{{
				PodId:      "pod-1",
				Containers: []v1.Container{{Name: "main", Message: message}},
			}},
		},
	}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workload)}
	result := r.externalWorkloadResult(context.Background(), job, workload, nil)
	require.NotNil(t, result)
	return result
}

func writeWeightFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("w"), size), 0o600))
}

func TestExternalPrewarmReportsBytesReadOnSuccess(t *testing.T) {
	dir := t.TempDir()
	writeWeightFile(t, filepath.Join(dir, "a.safetensors"), 1000)
	writeWeightFile(t, filepath.Join(dir, "b.safetensors"), 2345)

	result := externalPrewarmResult(t, dir)
	assert.Equal(t, modelprewarm.PhaseSucceeded, result.Phase)
	assert.Equal(t, int64(3345), result.BytesRead)
}

func TestExternalPrewarmFailsWhenNoWeightBytesRead(t *testing.T) {
	dir := t.TempDir()
	writeWeightFile(t, filepath.Join(dir, "a.safetensors"), 0)

	result := externalPrewarmResult(t, dir)
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Contains(t, result.Message, "no weight bytes")
}

func TestExternalPrewarmFailsWithReasonOnMissingModelPath(t *testing.T) {
	result := externalPrewarmResult(t, filepath.Join(t.TempDir(), "missing"))
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Contains(t, result.Message, "model path not found")
}

func TestPrewarmHostPathMountsDeclaredRoot(t *testing.T) {
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/data", MountPath: "/data"},
		}},
	}
	workload := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: "Job"},
	}}

	path, err := prewarmHostPath(workspace, workload, "/data/models/glm")
	require.NoError(t, err)
	assert.Equal(t, "/data", path)

	path, err = prewarmHostPath(workspace, workload, "/data")
	require.NoError(t, err)
	assert.Equal(t, "/data", path)

	_, err = prewarmHostPath(workspace, workload, "/other/models")
	assert.Error(t, err)
}
