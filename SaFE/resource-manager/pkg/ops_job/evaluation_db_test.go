/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func evalWorkload(taskId string, phase v1.WorkloadPhase) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "wl1",
			Labels: map[string]string{dbclient.EvaluationTaskIdLabel: taskId, v1.OpsJobIdLabel: "j1"},
		},
		Status: v1.WorkloadStatus{Phase: phase},
	}
}

func TestEvalUpdateDBStatusNilClient(t *testing.T) {
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	// Nil dbClient -> no-op, no panic.
	r.updateDBStatus(context.Background(), evalWorkload("t1", v1.WorkloadRunning), dbclient.EvaluationTaskStatusRunning)
}

func TestEvalUpdateDBStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "t1", dbclient.EvaluationTaskStatusRunning).Return(nil)

	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), dbClient: db}
	r.updateDBStatus(context.Background(), evalWorkload("t1", v1.WorkloadRunning), dbclient.EvaluationTaskStatusRunning)
}

func TestEvalUpdateDBStatusNoTaskId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), dbClient: db}
	// No task id label -> no db call.
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}}
	r.updateDBStatus(context.Background(), wl, dbclient.EvaluationTaskStatusRunning)
}

func TestEvalUpdateReportPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpdateEvaluationTaskResult(gomock.Any(), "t1", "{}", gomock.Any()).Return(nil)
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), dbClient: db}
	r.updateReportPath(context.Background(), "t1", evalWorkload("t1", v1.WorkloadSucceeded))
}

func TestEvalHandleWorkloadEventImplRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpdateEvaluationTaskStartTime(gomock.Any(), "t1").Return(nil)
	db.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "t1", dbclient.EvaluationTaskStatusRunning).Return(nil)

	job := newTestOpsJob("j1")
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), dbClient: db}
	r.handleWorkloadEventImpl(context.Background(), evalWorkload("t1", v1.WorkloadRunning))
}

func TestEvalHandleWorkloadEventImplFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetEvaluationTask(gomock.Any(), "t1").Return(&dbclient.EvaluationTask{Status: dbclient.EvaluationTaskStatusRunning}, nil)
	db.EXPECT().SetEvaluationTaskFailed(gomock.Any(), "t1", gomock.Any()).Return(nil)

	job := newTestOpsJob("j1")
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), dbClient: db}
	r.handleWorkloadEventImpl(context.Background(), evalWorkload("t1", v1.WorkloadFailed))
}
