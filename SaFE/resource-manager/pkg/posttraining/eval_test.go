/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
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

// autoService is a finished automatic evaluation service of testRun.
func autoService(name string) *v1.Workload {
	return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
		v1.PostTrainingRunIdLabel: testRun, v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval},
		Annotations: map[string]string{evalAutoAnnotation: v1.TrueStr}}}
}

func TestBaselineIsEvaluatedTwice(t *testing.T) {
	run, objs := evalFixtures("eval")
	run.ExperimentID, run.VariantName = "exp-1", "baseline"
	first := autoService("pt-eval-1")
	r, db, updates := newTestReconciler(t, run, append(objs, first)...)
	loadChartRegistry(t, r)
	db.EXPECT().GetPostTrainingExperiment(gomock.Any(), "exp-1").Return(
		&dbclient.PostTrainingExperiment{ExperimentID: "exp-1", BaselineVariant: "baseline"}, nil).Times(1)
	ctx := context.Background()

	require.NoError(t, r.finishEvaluation(ctx, run, first, true))
	last := (*updates)[len(*updates)-1]
	require.NotContains(t, last, "stage", "the run waits for the second evaluation")
	require.Contains(t, last["stage_history"], `"event":"evaluate-repeat"`)
	services := evalServices(t, r)
	require.Len(t, services, 1, "the first service is gone and the second one serves")
	require.Equal(t, v1.TrueStr, services[0].Annotations[evalAutoAnnotation])

	run.StageHistory = last["stage_history"].(string)
	require.NoError(t, r.finishEvaluation(ctx, run, &services[0], true))
	require.Equal(t, "done", (*updates)[len(*updates)-1]["stage"], "the baseline is evaluated twice, not more")
}

func TestVariantIsEvaluatedOnce(t *testing.T) {
	run, objs := evalFixtures("eval")
	run.ExperimentID, run.VariantName = "exp-1", "tis"
	s := autoService("pt-eval-2")
	r, db, updates := newTestReconciler(t, run, append(objs, s)...)
	db.EXPECT().GetPostTrainingExperiment(gomock.Any(), "exp-1").Return(
		&dbclient.PostTrainingExperiment{ExperimentID: "exp-1", BaselineVariant: "baseline"}, nil)
	require.NoError(t, r.finishEvaluation(context.Background(), run, s, true))
	require.Equal(t, "done", (*updates)[len(*updates)-1]["stage"])
	require.Empty(t, evalServices(t, r))
}
