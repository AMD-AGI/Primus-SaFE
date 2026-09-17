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

func TestJobTTLReconcileNotFound(t *testing.T) {
	r := &JobTTLController{Client: newBaseWithObjs(t).Client}
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestJobTTLReconcileNotEnded(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{TTLSecondsAfterFinished: 10}}
	r := &JobTTLController{Client: newBaseWithObjs(t, job).Client}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestJobTTLDeleteExpired(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{TTLSecondsAfterFinished: 1},
		Status:     v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now().Add(-time.Hour)}},
	}
	r := &JobTTLController{Client: newBaseWithObjs(t, job).Client}
	res, err := r.deleteExpiredJob(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	// Job should be deleted.
	err = r.Get(context.Background(), client.ObjectKey{Name: "j1"}, &v1.OpsJob{})
	assert.Error(t, err)
}

func TestJobTTLDeleteNotYetExpired(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{TTLSecondsAfterFinished: 3600},
		Status:     v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now()}},
	}
	r := &JobTTLController{Client: newBaseWithObjs(t, job).Client}
	res, err := r.deleteExpiredJob(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestJobTTLRelevantChangePredicate(t *testing.T) {
	r := &JobTTLController{Client: newBaseWithObjs(t).Client}
	p := r.relevantChangePredicate()
	oldJob := &v1.OpsJob{}
	newJob := &v1.OpsJob{
		Spec:   v1.OpsJobSpec{TTLSecondsAfterFinished: 10},
		Status: v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now()}},
	}
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: oldJob}))
}
