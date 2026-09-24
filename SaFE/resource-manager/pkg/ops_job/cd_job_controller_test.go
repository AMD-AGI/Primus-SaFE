/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func cdJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobCDType},
	}
}

func TestGetParameterValue(t *testing.T) {
	tests := []struct {
		name         string
		job          *v1.OpsJob
		paramName    string
		defaultValue string
		expected     string
	}{
		{
			name: "get existing parameter",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{
						{Name: "test_param", Value: "test_value"},
					},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "test_value",
		},
		{
			name: "get non-existing parameter returns default",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{
						{Name: "other_param", Value: "other_value"},
					},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "empty inputs returns default",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getParameterValue(tt.job, tt.paramName, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCDWorkload(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			v1.OpsJobIdLabel:   "j1",
			v1.OpsJobTypeLabel: string(v1.OpsJobCDType),
		},
	}}
	assert.True(t, isCDWorkload(wl))
	assert.False(t, isCDWorkload(&v1.Workload{}))
}

func TestCDObserveFilter(t *testing.T) {
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := cdJob("j1")
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestCDObserveEnded(t *testing.T) {
	// Ended job -> observe returns quit, Reconcile completes without creating workload.
	job := cdJob("j1")
	job.Status.FinishedAt = &metav1.Time{Time: time.Now()}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, quit)
}

func TestCDGenerateSafeAndLensWorkload(t *testing.T) {
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := cdJob("j1")
	safe := r.generateSafeCDWorkload(job, "c1", "main")
	assert.Equal(t, "j1", safe.Name)
	assert.Equal(t, PrimusSaFERepoURL, safe.Spec.Env["REPO_URL"])

	lens := r.generateLensCDWorkload(job, "c1", "dev")
	assert.Equal(t, "j1", lens.Name)
	assert.Equal(t, "dev", lens.Spec.Env["DEPLOY_BRANCH"])
}

func TestCDGenerateCDWorkloadNoControlPlane(t *testing.T) {
	job := cdJob("j1")
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.generateCDWorkload(context.Background(), job)
	assert.Error(t, err)
}

func TestCDGenerateCDWorkloadWithControlPlane(t *testing.T) {
	job := cdJob("j1")
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "ctrl",
		Labels: map[string]string{v1.ClusterControlPlaneLabel: ""},
	}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, cluster)}
	wl, err := r.generateCDWorkload(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, "j1", wl.Name)
	assert.NotNil(t, wl.Spec.Timeout)
}

func TestCDHandleWorkloadExists(t *testing.T) {
	job := cdJob("j1")
	job.Status.Phase = v1.OpsJobPending
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
}

func TestCDHandleGeneratesWorkload(t *testing.T) {
	job := cdJob("j1")
	job.Status.Phase = v1.OpsJobPending
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "ctrl",
		Labels: map[string]string{v1.ClusterControlPlaneLabel: ""},
	}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, cluster)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	wl := &v1.Workload{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, wl))
}

func TestCDHandleWorkloadEvent(t *testing.T) {
	job := newTestOpsJob("j1")
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	h := r.handleWorkloadEvent().(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	runWorkloadEventHandler(t, h, endedWorkload(v1.OpsJobCDType))
	assert.NotNil(t, r)
}

func TestCDHandleWorkloadEventImpl(t *testing.T) {
	job := newTestOpsJob("j1")
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.OpsJobIdLabel: "j1"}},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	r.handleWorkloadEventImpl(context.Background(), wl)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
}

func TestCDReconcileEntry(t *testing.T) {
	job := cdJob("j1")
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "ctrl",
		Labels: map[string]string{v1.ClusterControlPlaneLabel: ""},
	}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, cluster)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestCDCleanupJobRelatedInfo(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	assert.NoError(t, r.cleanupJobRelatedInfo(context.Background(), job))
}
