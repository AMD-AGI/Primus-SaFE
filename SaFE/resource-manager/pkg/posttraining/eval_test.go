/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// loadChartRegistry makes the chart's default registry the reconciler's current one.
func loadChartRegistry(t *testing.T, r *RunReconciler) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "charts", "primus-safe", "values.yaml"))
	require.NoError(t, err)
	var values map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &values))
	reg, err := yaml.Marshal(values["post_training"].(map[string]interface{})["registry"])
	require.NoError(t, err)
	require.NoError(t, r.registry.store.Load(reg, nil))
	r.registry.loadedAt = time.Now()
}

// evalFixtures returns a succeeded run at stage, its workspace, node flavor and Ready output model.
func evalFixtures(stage string) (*dbclient.PostTrainingRun, []client.Object) {
	run := testRunRow("Succeeded", stage)
	run.Framework, run.Image, run.Priority, run.OutputModelID = "slime", "primussafe/slime:test", 1, "m-out"
	run.EvalEnabled = true
	run.Recipe = `{"output":{"base":"` + testRunDir + `"},"evaluation":{"enabled":true,"benchmarks":["gsm8k"]}}`
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{NodeFlavor: "nf1", Cluster: "c1"}}
	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "nf1"}}
	nf.Spec.Cpu.Quantity = resource.MustParse("128")
	nf.Spec.Memory = resource.MustParse("1024Gi")
	nf.Spec.Gpu = &v1.GpuChip{Product: "MI355X", Quantity: resource.MustParse("8")}
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-out", Labels: map[string]string{v1.PostTrainingRunIdLabel: testRun}}}
	model.Spec.Source = v1.ModelSource{AccessMode: v1.AccessModeLocalPath, LocalPath: testRunDir + "/final_model"}
	model.Status.Phase = v1.ModelPhaseReady
	model.Status.ModelConfig = &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, ModelType: "qwen3",
		NumHiddenLayers: 36, HiddenSize: 4096, NumAttentionHeads: 32, NumKeyValueHeads: 8, RopeTheta: "1000000"}
	return run, []client.Object{ws, nf, model}
}

// evalServices lists the run's evaluation inference services.
func evalServices(t *testing.T, r *RunReconciler) []v1.Workload {
	list := &v1.WorkloadList{}
	require.NoError(t, r.List(context.Background(), list, client.MatchingLabels{v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval}))
	return list.Items
}

// An automatic evaluation serves the output model, evaluates it and ends the run with its scores.
func TestAutoEvaluationLifecycle(t *testing.T) {
	run, objs := evalFixtures("eval")
	r, db, updates := newTestReconciler(t, run, objs...)
	loadChartRegistry(t, r)
	ctx := context.Background()

	require.Equal(t, evalRequeue, reconcileRun(t, r).RequeueAfter)
	services := evalServices(t, r)
	require.Len(t, services, 1)
	s := services[0]
	require.Equal(t, "Deployment", s.Spec.Kind)
	require.Equal(t, []string{"primussafe/slime:test"}, s.Spec.Images, "without a default image the run's image serves")
	cmd, err := base64.StdEncoding.DecodeString(s.Spec.EntryPoints[0])
	require.NoError(t, err)
	require.Contains(t, string(cmd), "--model-path "+testRunDir+"/final_model --served-model-name m-out --tp 1")
	require.Equal(t, "1", s.Spec.Resources[0].GPU)
	require.Equal(t, "16", s.Spec.Resources[0].CPU, "one of eight GPUs takes an eighth of the node")
	require.Equal(t, &v1.HealthCheck{Path: "/v1/models", Port: 8000, InitialDelaySeconds: 30, PeriodSeconds: 10,
		FailureThreshold: 3}, s.Spec.Readiness)
	require.Equal(t, 8000, s.Spec.Service.Port)
	require.Equal(t, "m-out", s.Spec.Env["PRIMUS_SOURCE_MODEL"])
	require.Equal(t, v1.TrueStr, s.Annotations[evalAutoAnnotation])
	require.Contains(t, (*updates)[len(*updates)-1]["stage_history"], `"event":"evaluate"`)

	command := base64.StdEncoding.EncodeToString(cmd)
	db.EXPECT().GetDataset(gomock.Any(), "gsm8k").Return(&dbclient.Dataset{DatasetType: "evaluation", DisplayName: "gsm8k"}, nil)
	db.EXPECT().GetWorkload(gomock.Any(), s.Name).Return(&dbclient.Workload{WorkloadId: s.Name, DisplayName: s.Name,
		Cluster: "c1", Workspace: "ws1", EntryPoint: command}, nil)
	db.EXPECT().UpsertEvaluationTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, task *dbclient.EvaluationTask) error {
			require.Equal(t, testRun, task.PostTrainingRunId.String)
			require.Equal(t, "m-out", task.ModelId.String)
			return nil
		})
	db.EXPECT().UpdateEvaluationTaskOpsJobId(gomock.Any(), gomock.Any(), evalJobName(s.Name)).Return(nil)
	s.Status.Phase = v1.WorkloadRunning
	require.NoError(t, r.Status().Update(ctx, &s))
	reconcileRun(t, r)
	job := &v1.OpsJob{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: evalJobName(s.Name)}, job))
	require.Equal(t, v1.OpsJobEvaluationType, job.Spec.Type)
	require.Equal(t, testRun, job.Labels[v1.PostTrainingRunIdLabel])
	require.Equal(t, "m-out", job.GetParameter(v1.ParameterModelName).Value)
	require.Contains(t, job.GetParameter(v1.ParameterEvalProbes).Value, "Paris")

	job.Status.Phase = v1.OpsJobSucceeded
	require.NoError(t, r.Update(ctx, job))
	reconcileRun(t, r)
	require.Empty(t, evalServices(t, r), "the service goes once the evaluation is over")
	require.Equal(t, "done", (*updates)[len(*updates)-1]["stage"])
}

// The evaluation service of a run gets a DNS display name from the run's name.
func TestEvaluationServiceHasAValidDisplayName(t *testing.T) {
	run, objs := evalFixtures("eval")
	run.DisplayName = "Qwen3 SFT demo"
	r, _, _ := newTestReconciler(t, run, objs...)
	loadChartRegistry(t, r)
	reconcileRun(t, r)
	services := evalServices(t, r)
	require.Len(t, services, 1)
	name := services[0].Labels[v1.DisplayNameLabel]
	require.Regexp(t, fmt.Sprintf("^[a-z][-a-z0-9]{0,%d}[a-z0-9]$", commonutils.MaxDeploymentNameLen), name,
		"the workload webhook refuses a Deployment without a DNS display name")
	require.Equal(t, "eval-qwen3-sft-demo", name)
}

// The workloads and jobs of a deleted run are removed, except a kept service that is done.
func TestTheChildrenOfADeletedRunAreRemoved(t *testing.T) {
	labels := func(role string) map[string]string {
		return map[string]string{v1.PostTrainingRunIdLabel: "pt-gone", v1.PostTrainingRoleLabel: role}
	}
	service := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-gone-eval", Labels: labels(v1.PostTrainingRoleEval)}}
	kept := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-gone-kept", Labels: labels(v1.PostTrainingRoleEval),
		Annotations: map[string]string{v1.PostTrainingEvalDoneAnnotation: v1.TrueStr, evalKeepAnnotation: v1.TrueStr}}}
	// Marked done and then not removed, as when its delete failed, and not asked to be kept.
	left := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-gone-left", Labels: labels(v1.PostTrainingRoleEval),
		Annotations: map[string]string{v1.PostTrainingEvalDoneAnnotation: v1.TrueStr}}}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "pt-gone-eval-job", Labels: labels("")},
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterEvalTaskId, Value: "task-9"}}}}
	r, db, _ := newTestReconciler(t, testRunRow("Succeeded", "done"), service, kept, left, job)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-gone").Return(nil, gorm.ErrRecordNotFound).Times(2)
	ctx := context.Background()
	db.EXPECT().IsPostTrainingRunDeleted(gomock.Any(), "pt-gone").Return(false, nil)
	_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: "pt-gone"}})
	require.NoError(t, err)
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(service), &v1.Workload{}),
		"a run this database has no row of, as after a restore, is not a deleted one: what runs under it stays")

	db.EXPECT().IsPostTrainingRunDeleted(gomock.Any(), "pt-gone").Return(true, nil)
	db.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "task-9", dbclient.EvaluationTaskStatusCancelled).Return(nil)
	_, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: "pt-gone"}})
	require.NoError(t, err)
	require.Error(t, r.Get(ctx, client.ObjectKeyFromObject(service), &v1.Workload{}),
		"an evaluation service of a deleted run holds GPUs nothing else frees")
	require.Error(t, r.Get(ctx, client.ObjectKeyFromObject(job), &v1.OpsJob{}))
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(kept), &v1.Workload{}), "a kept service goes on serving")
	require.Error(t, r.Get(ctx, client.ObjectKeyFromObject(left), &v1.Workload{}),
		"a done service that was not to be kept only stayed because its delete failed")
}

// An evaluation whose task cannot be built records the failure and frees its GPUs.
func TestAnEvaluationWhoseTaskCannotBeBuiltEnds(t *testing.T) {
	run, objs := evalFixtures("eval")
	s := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "svc-2", CreationTimestamp: metav1.Now(),
		Labels:      map[string]string{v1.PostTrainingRunIdLabel: testRun, v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval},
		Annotations: map[string]string{evalAutoAnnotation: v1.TrueStr, evalBenchmarksAnnotation: `["gsm8k"]`}}}
	s.Spec.Env = map[string]string{"PRIMUS_SOURCE_MODEL": "m-out"}
	s.Status.Phase = v1.WorkloadRunning
	r, db, updates := newTestReconciler(t, run, append(objs, s)...)
	loadChartRegistry(t, r)
	db.EXPECT().GetDataset(gomock.Any(), "gsm8k").Return(nil, commonerrors.NewNotFoundWithMessage("gone"))
	db.EXPECT().UpsertEvaluationTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, task *dbclient.EvaluationTask) error {
			require.Equal(t, evalTaskID("svc-2"), task.TaskId, "the failure is recorded on the service's own task")
			return nil
		})
	db.EXPECT().SetEvaluationTaskFailed(gomock.Any(), evalTaskID("svc-2"), gomock.Any()).Return(nil)
	reconcileRun(t, r)
	require.Empty(t, evalServices(t, r), "a benchmark deleted during training ends the evaluation and frees its GPUs")
	require.Equal(t, "done", (*updates)[len(*updates)-1]["stage"])
}

// A job or service the cache does not show yet does not end the evaluation.
func TestAnEvaluationIsNotCancelledForWhatTheCacheDoesNotShowYet(t *testing.T) {
	run, objs := evalFixtures("eval")
	s := autoService("svc-3")
	s.Status.Phase = v1.WorkloadRunning
	s.Annotations[evalJobAnnotation] = evalJobName("svc-3")
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: evalJobName("svc-3"),
		Labels: map[string]string{v1.PostTrainingRunIdLabel: testRun}}}
	job.Status.Phase = v1.OpsJobRunning
	r, _, updates := newTestReconciler(t, run, append(objs, s)...)
	loadChartRegistry(t, r)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	r.reader = ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(job, s.DeepCopy()).Build()
	reconcileRun(t, r)
	require.Len(t, evalServices(t, r), 1, "a job the cache does not show yet was not deleted")

	require.NoError(t, r.Delete(context.Background(), s))
	run.StageHistory = `[{"stage":"eval","time":"2026-01-01T00:00:00Z","event":"evaluate","message":"svc-3"}]`
	reconcileRun(t, r)
	for _, u := range *updates {
		require.NotEqual(t, "done", u["stage"], "the service the evaluation started is still there")
	}
}

// A service not ready within the wait fails its evaluation task.
func TestEvaluationServiceNotReadyInTime(t *testing.T) {
	run, objs := evalFixtures("eval")
	s := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "svc-1", CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Hour)),
		Labels:      map[string]string{v1.PostTrainingRunIdLabel: testRun, v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval},
		Annotations: map[string]string{evalAutoAnnotation: v1.TrueStr, evalBenchmarksAnnotation: `["gsm8k"]`}}}
	s.Spec.Env = map[string]string{"PRIMUS_SOURCE_MODEL": "m-out"}
	r, db, updates := newTestReconciler(t, run, append(objs, s)...)
	loadChartRegistry(t, r)
	db.EXPECT().UpsertEvaluationTask(gomock.Any(), gomock.Any()).Return(nil)
	db.EXPECT().SetEvaluationTaskFailed(gomock.Any(), gomock.Any(), "The inference service was not ready within 2h0m0s").Return(nil)
	reconcileRun(t, r)
	require.Empty(t, evalServices(t, r))
	require.Equal(t, "done", (*updates)[len(*updates)-1]["stage"])
}

// A requested evaluation is taken once, keeps its service and leaves the run's stage alone.
func TestRequestedEvaluationKeepsItsService(t *testing.T) {
	run, objs := evalFixtures("done")
	model := objs[2].(*v1.Model)
	model.Annotations = map[string]string{v1.PostTrainingEvaluateAnnotation: `{"benchmarks":["gsm8k"],"keepService":true}`}
	r, _, _ := newTestReconciler(t, run, objs...)
	loadChartRegistry(t, r)
	reconcileRun(t, r)
	services := evalServices(t, r)
	require.Len(t, services, 1)
	require.Equal(t, v1.TrueStr, services[0].Annotations[evalKeepAnnotation])
	require.Empty(t, services[0].Annotations[evalAutoAnnotation], "a requested evaluation leaves the stage alone")
	got := &v1.Model{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "m-out"}, got))
	require.NotContains(t, got.Annotations, v1.PostTrainingEvaluateAnnotation, "the request is taken once")
}

// An evaluation job whose service is gone is deleted and its task cancelled.
func TestDeletedServiceCancelsItsJob(t *testing.T) {
	run, objs := evalFixtures("done")
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "pt-eval-x", Labels: map[string]string{
		v1.PostTrainingRunIdLabel: testRun, v1.OpsJobTypeLabel: string(v1.OpsJobEvaluationType)}}}
	job.Spec.Type = v1.OpsJobEvaluationType
	job.Spec.Inputs = []v1.Parameter{{Name: v1.ParameterEvalServiceId, Value: "gone"}, {Name: v1.ParameterEvalTaskId, Value: "t1"}}
	job.Status.Phase = v1.OpsJobRunning
	r, db, _ := newTestReconciler(t, run, append(objs, job)...)
	db.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "t1", dbclient.EvaluationTaskStatusCancelled).Return(nil)
	reconcileRun(t, r)
	require.Error(t, r.Get(context.Background(), client.ObjectKey{Name: "pt-eval-x"}, &v1.OpsJob{}))
}

// The evaluation service asks for its GPUs with the CPU and memory shares that go with them.
func TestServiceResource(t *testing.T) {
	_, objs := evalFixtures("eval")
	nf := objs[1].(*v1.NodeFlavor)
	whole := serviceResource(nf, 8)
	require.Equal(t, "128", whole.CPU)
	require.Equal(t, "1Ti", whole.Memory)
	require.Equal(t, "512Gi", whole.SharedMemory)
	require.Equal(t, v1.WorkloadResource{Replica: 1, GPU: "2", CPU: "16", Memory: "64Gi", SharedMemory: "32Gi",
		EphemeralStorage: "50Gi"}, serviceResource(nil, 2), "without a flavor the defaults of the deploy page apply")

	viper.Set("workspace.ephemeral_store_reserve_percent", 0.05)
	viper.Set("workload.max_ephemeral_store_percent", 0.6)
	t.Cleanup(func() {
		viper.Set("workspace.ephemeral_store_reserve_percent", nil)
		viper.Set("workload.max_ephemeral_store_percent", nil)
	})
	nf.Spec.ExtendResources = corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("1000Gi")}
	require.Equal(t, "600Gi", serviceResource(nf, 8).EphemeralStorage,
		"the ephemeral storage stays within the per-workload maximum, not the 950Gi left after the reserve")
	require.Equal(t, "75Gi", serviceResource(nf, 1).EphemeralStorage, "the maximum is shared by the GPUs")
}

// An evaluation service that was marked done and whose delete then failed is removed by the next reconcile, which
// also ends the automatic evaluation.
func TestAnEvaluationServiceWhoseDeleteFailedIsRemovedNext(t *testing.T) {
	run, objs := evalFixtures("eval")
	s := autoService("svc-9")
	s.Status.Phase = v1.WorkloadRunning
	s.Annotations[evalJobAnnotation] = evalJobName("svc-9")
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: evalJobName("svc-9"),
		Labels: map[string]string{v1.PostTrainingRunIdLabel: testRun}}}
	job.Status.Phase = v1.OpsJobSucceeded
	r, _, updates := newTestReconciler(t, run, append(objs, s, job)...)
	loadChartRegistry(t, r)
	failDelete := true
	r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*v1.Workload); ok && failDelete {
				return apierrors.NewServiceUnavailable("the API server is busy")
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	ctx := context.Background()
	_, err := r.reconcileEvaluations(ctx, run)
	require.Error(t, err, "the service is marked done and its delete fails")

	failDelete = false
	_, err = r.reconcileEvaluations(ctx, run)
	require.NoError(t, err)
	for _, left := range evalServices(t, r) {
		require.False(t, left.DeletionTimestamp.IsZero(), "%s no longer holds its GPUs", left.Name)
	}
	moved := false
	for _, u := range *updates {
		moved = moved || u["stage"] == "done"
	}
	require.True(t, moved, "the automatic evaluation ends the run")
}

// autoService is a finished automatic evaluation service of testRun.
func autoService(name string) *v1.Workload {
	return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
		v1.PostTrainingRunIdLabel: testRun, v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval},
		Annotations: map[string]string{evalAutoAnnotation: v1.TrueStr}}}
}

// Finishing an earlier service once more, as when its delete failed, does not end the evaluation that a later
// service runs: only the service of the latest automatic evaluation moves the run on.
func TestFinishingAnEarlierServiceAgainDoesNotEndTheRun(t *testing.T) {
	run, objs := evalFixtures("eval")
	run.StageHistory = `[{"stage":"eval","event":"evaluate","message":"pt-eval-2"}]`
	first := autoService("pt-eval-1")
	r, _, updates := newTestReconciler(t, run, append(objs, first, autoService("pt-eval-2"))...)
	loadChartRegistry(t, r)
	require.NoError(t, r.finishEvaluation(context.Background(), run, first, true))
	for _, u := range *updates {
		require.NotEqual(t, "done", u["stage"], "pt-eval-2 still evaluates")
	}
	services := evalServices(t, r)
	require.Len(t, services, 1, "the first service is removed all the same")
	require.Equal(t, "pt-eval-2", services[0].Name)
}

// A service ends the run when the service of the latest automatic evaluation in the history evaluates no more.
// Here the history names the kept first service, whose evaluation ended: the save of the second one's start
// failed.
func TestAServiceEndsTheRunWhenTheLatestOneInTheHistoryEvaluatesNoMore(t *testing.T) {
	run, objs := evalFixtures("eval")
	run.StageHistory = `[{"stage":"eval","event":"evaluate","message":"pt-eval-1"}]`
	first, second := autoService("pt-eval-1"), autoService("pt-eval-2")
	first.Annotations[evalKeepAnnotation] = v1.TrueStr
	first.Annotations[v1.PostTrainingEvalDoneAnnotation] = v1.TrueStr
	r, _, updates := newTestReconciler(t, run, append(objs, first, second)...)
	require.NoError(t, r.finishEvaluation(context.Background(), run, second, true))
	require.NotEmpty(t, *updates, "the run is saved")
	require.Equal(t, "done", (*updates)[len(*updates)-1]["stage"])
}
