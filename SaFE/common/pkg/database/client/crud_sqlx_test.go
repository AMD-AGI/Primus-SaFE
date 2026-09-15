/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	sqrl "github.com/Masterminds/squirrel"

	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func TestDatasetCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"is_deleted": false}

	_ = c.UpsertDataset(ctx, &Dataset{DatasetId: "d1"})
	_, _ = c.SelectDatasets(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountDatasets(ctx, q)
	_, _ = c.GetDataset(ctx, "d1")
	_, _ = c.CheckDatasetNameExists(ctx, "name")
	_ = c.SetDatasetDeleted(ctx, "d1")
	_ = c.UpdateDatasetStatus(ctx, "d1", DatasetStatus("ready"), "msg")
	_ = c.UpdateDatasetFileInfo(ctx, "d1", 100, 5)
	_ = c.UpdateDatasetLocalPath(ctx, "d1", "ws", DatasetStatus("ready"), "msg")
}

func TestCdCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"id": 1}

	_, _ = c.CreateDeploymentRequest(ctx, &DeploymentRequest{})
	_, _ = c.GetDeploymentRequest(ctx, 1)
	_, _ = c.ListDeploymentRequests(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountDeploymentRequests(ctx, q)
	_ = c.UpdateDeploymentRequest(ctx, &DeploymentRequest{Id: 1})
	_, _ = c.CreateEnvironmentSnapshot(ctx, &EnvironmentSnapshot{})
	_, _ = c.GetEnvironmentSnapshot(ctx, 1)
	_, _ = c.GetEnvironmentSnapshotByRequestId(ctx, 1)
	_, _ = c.ListEnvironmentSnapshots(ctx, q, []string{"id"}, 10, 0)
}

func TestEvaluationTaskCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"deleted": false}

	_ = c.UpsertEvaluationTask(ctx, &EvaluationTask{TaskId: "t1"})
	_, _ = c.SelectEvaluationTasks(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountEvaluationTasks(ctx, q)
	_, _ = c.GetEvaluationTask(ctx, "t1")
	_ = c.SetEvaluationTaskDeleted(ctx, "t1")
	_ = c.UpdateEvaluationTaskStatus(ctx, "t1", EvaluationTaskStatus("running"))
	_ = c.UpdateEvaluationTaskOpsJobId(ctx, "t1", "job1")
	_ = c.UpdateEvaluationTaskResult(ctx, "t1", "summary", "/report")
	_ = c.UpdateEvaluationTaskStartTime(ctx, "t1")
	_ = c.SetEvaluationTaskFailed(ctx, "t1", "boom")
}

func TestWorkloadCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"deleted": false}

	_ = c.UpsertWorkload(ctx, &Workload{})
	_, _ = c.SelectWorkloads(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountWorkloads(ctx, q)
	_ = c.SetWorkloadDeleted(ctx, "w1")
	_ = c.SetWorkloadStopped(ctx, "w1")
	_ = c.SetWorkloadDescription(ctx, "w1", "desc")
	_, _ = c.GetWorkload(ctx, "w1")
}

func TestWorkloadPodCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertWorkloadPod(ctx, &WorkloadPod{WorkloadId: "w1", PodId: "p1"})
	_ = c.BatchUpsertWorkloadPods(ctx, []*WorkloadPod{
		{WorkloadId: "w1", PodId: "p1"},
		{WorkloadId: "w1", PodId: "p2"},
	})
	_, _ = c.ListWorkloadPods(ctx, "w1")
	_ = c.DeleteWorkloadPods(ctx, "w1")
	_ = c.DeleteWorkloadPodsNotIn(ctx, "w1", []string{"p1", "p2"})
	_ = c.DeleteWorkloadPodsNotIn(ctx, "w1", nil)
}

func TestWorkloadDispatchNodeCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertWorkloadDispatchNode(ctx, &WorkloadDispatchNode{WorkloadId: "w1", DispatchIndex: 0})
	_, _ = c.ListWorkloadDispatchNodes(ctx, "w1")
	_ = c.DeleteWorkloadDispatchNodes(ctx, "w1")
}

func TestFaultCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"id": 1}

	_ = c.UpsertFault(ctx, &Fault{})
	_, _ = c.SelectFaults(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountFaults(ctx, q)
	_, _ = c.GetFault(ctx, "uid1")
	_ = c.DeleteFault(ctx, "uid1")
}

func TestOpsJobCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"deleted": false}

	_ = c.UpsertJob(ctx, &OpsJob{})
	_, _ = c.SelectJobs(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountJobs(ctx, q)
	_, _ = c.GetOpsJob(ctx, "j1")
	_ = c.SetOpsJobDeleted(ctx, "j1")
}

func TestModelCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertModel(ctx, &Model{})
	_, _ = c.GetModelByID(ctx, "m1")
	_, _ = c.GetModelByModelName(ctx, "name")
	_, _ = c.ListModels(ctx, "", "", false)
	_ = c.DeleteModel(ctx, "m1")
}

func TestImageCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertImage(ctx, &dbmodel.Image{})
	_, _ = c.GetImageByTag(ctx, "tag")
	_, _, _ = c.SelectImages(ctx, &ImageFilter{})
	_, _ = c.GetImage(ctx, 1)
	_ = c.DeleteImage(ctx, 1, "user")
}
