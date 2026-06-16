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
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func opsWorkQueue() v1.RequestWorkQueue {
	return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
}

func endedWorkload(opsType v1.OpsJobType) *v1.Workload {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wl1",
			Labels: map[string]string{
				v1.OpsJobIdLabel:   "j1",
				v1.OpsJobTypeLabel: string(opsType),
			},
		},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadSucceeded},
	}
	wl.Status.EndTime = &metav1.Time{Time: metav1.Now().Time}
	return wl
}

func runWorkloadEventHandler(t *testing.T, h interface {
	Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
	Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
}, wl *v1.Workload) {
	t.Helper()
	q := opsWorkQueue()
	defer q.ShutDown()
	h.Create(context.Background(), event.CreateEvent{Object: wl}, q)
	old := wl.DeepCopy()
	old.Status.Phase = v1.WorkloadRunning
	old.Status.EndTime = nil
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: old, ObjectNew: wl}, q)
}

func TestDatasetPredicates(t *testing.T) {
	p := datasetOpsJobPredicate()
	withLabel := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{dbclient.DatasetIdLabel: "d1"}}}
	assert.True(t, p.Create(event.CreateEvent{Object: withLabel}))
	assert.False(t, p.Create(event.CreateEvent{Object: &v1.OpsJob{}}))

	pp := opsJobPhaseChangedPredicate()
	assert.True(t, pp.Create(event.CreateEvent{Object: &v1.OpsJob{}}))
	oldJob := &v1.OpsJob{}
	newJob := &v1.OpsJob{Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning}}
	assert.True(t, pp.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}))
	assert.False(t, pp.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: oldJob.DeepCopy()}))
	assert.False(t, pp.Delete(event.DeleteEvent{Object: &v1.OpsJob{}}))
}

func TestAddonHandleWorkloadEvent(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{
			v1.ClusterIdLabel:  "c1",
			v1.OpsJobTypeLabel: string(v1.OpsJobAddonType),
		}},
	}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), allJobs: map[string]*AddonJob{}}
	h := r.handleWorkloadEvent().(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	q := opsWorkQueue()
	defer q.ShutDown()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	h.Create(context.Background(), event.CreateEvent{Object: wl}, q)
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: wl, ObjectNew: wl.DeepCopy()}, q)
}

func TestAddonHandleNodeEvent(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{
			v1.ClusterIdLabel:  "c1",
			v1.OpsJobTypeLabel: string(v1.OpsJobAddonType),
		}},
	}
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t, job),
		allJobs: map[string]*AddonJob{
			"j1": {nodes: map[string]AddonJobPhase{"n1": {Phase: v1.OpsJobRunning}}, maxFailCount: 1},
		},
	}
	h := r.handleNodeEvent().(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	q := opsWorkQueue()
	defer q.ShutDown()
	oldNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	oldNode.Spec.Cluster = ptrStr("c1")
	newNode := oldNode.DeepCopy()
	newNode.Spec.Cluster = nil
	// Node unmanaged -> handleNodeRemovedEvent path.
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)
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

func TestPreflightHandleWorkloadEvent(t *testing.T) {
	job := newTestOpsJob("j1")
	r := &PreflightJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	h := r.handleWorkloadEvent().(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	runWorkloadEventHandler(t, h, endedWorkload(v1.OpsJobPreflightType))
	assert.NotNil(t, r)
}

func TestEvaluationHandleWorkloadEvent(t *testing.T) {
	job := newTestOpsJob("j1")
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	h := r.handleWorkloadEvent().(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	runWorkloadEventHandler(t, h, endedWorkload(v1.OpsJobEvaluationType))
	assert.NotNil(t, r)
}
