/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func downloadJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.WorkspaceIdLabel: "ws1",
				v1.ClusterIdLabel:   "c1",
			},
		},
		Spec: v1.OpsJobSpec{
			Type:  v1.OpsJobDownloadType,
			Image: pointer.String("img:latest"),
			Inputs: []v1.Parameter{
				{Name: v1.ParameterSecret, Value: "sec"},
				{Name: v1.ParameterEndpoint, Value: "http://x"},
				{Name: v1.ParameterDestPath, Value: "/data"},
			},
		},
	}
}

func TestIsDownloadWorkload(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			v1.OpsJobIdLabel:   "j1",
			v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType),
		},
	}}
	assert.True(t, isDownloadWorkload(wl))
	assert.False(t, isDownloadWorkload(&v1.Workload{}))
}

func TestDownloadObserveFilter(t *testing.T) {
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := downloadJob("j1")
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))

	other := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}
	assert.True(t, r.filter(context.Background(), other))
}

func TestDownloadGenerateWorkload(t *testing.T) {
	job := downloadJob("j1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	wl, err := r.generateDownloadWorkload(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, "j1", wl.Name)
	assert.Equal(t, "http://x", wl.Spec.Env["INPUT_URL"])
	assert.Equal(t, "/data", wl.Spec.Env["DEST_PATH"])
}

func TestDownloadGenerateWorkloadNoWorkspaceId(t *testing.T) {
	job := downloadJob("j1")
	job.Labels = nil
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.generateDownloadWorkload(context.Background(), job)
	assert.Error(t, err)
}

func TestDownloadHandleSetsPending(t *testing.T) {
	job := downloadJob("j1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	// First handle: phase empty -> set Pending and requeue.
	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobPending, job.Status.Phase)
	_ = res
}

func TestDownloadHandleCreatesWorkload(t *testing.T) {
	job := downloadJob("j1")
	job.Status.Phase = v1.OpsJobPending
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	// Workload should now exist.
	wl := &v1.Workload{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, wl))
}

func TestDownloadHandleWorkloadEvent(t *testing.T) {
	job := newTestOpsJob("j1")
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	h := r.handleWorkloadEvent().(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	runWorkloadEventHandler(t, h, endedWorkload(v1.OpsJobDownloadType))
	assert.NotNil(t, r)
}

func TestDownloadReconcileEntry(t *testing.T) {
	job := downloadJob("j1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestDownloadCleanupJobRelatedInfo(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	assert.NoError(t, r.cleanupJobRelatedInfo(context.Background(), job))
}
