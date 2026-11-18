/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
)

// exportJob is an export job of step 20 of testRun in phase whose recipe names the new model.
func exportJob(name string, phase v1.WorkloadPhase) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name),
		Labels: map[string]string{v1.PostTrainingRunIdLabel: testRun, v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport,
			v1.PostTrainingExportStepLabel: "20"}}}
	w.Spec.Env = map[string]string{exportRecipeEnv: base64.StdEncoding.EncodeToString(
		[]byte(`{"exportOnly":{"step":20,"displayName":"qwen sft step 20"}}`))}
	w.Status.Phase = phase
	return w
}

// exportPods returns the submitter row of an export job that printed result.
func exportPods(w *v1.Workload, result string) []*dbclient.WorkloadPod {
	return []*dbclient.WorkloadPod{dbclient.WorkloadPodFromV1(w.Name, string(w.UID), 1, &v1.WorkloadPod{
		PodId: w.Name + "-sub", Containers: []v1.Container{{Name: SubmitterContainer,
			Message: commonopsjob.FilterRunResultLog([]byte(result), testRun)}}})}
}

// Each succeeded export registers its step's model once, and a failed export none.
func TestReconcileExportsRegistersEachStepOnce(t *testing.T) {
	ok := exportJob("pt-x20-a", v1.WorkloadSucceeded)
	failed := exportJob("pt-x20-b", v1.WorkloadFailed)
	failed.Labels[v1.PostTrainingExportStepLabel] = "10"
	r, db, _ := newTestReconciler(t, testRunRow("Succeeded", "done"), ok, failed)
	db.EXPECT().ListWorkloadPods(gomock.Any(), ok.Name, string(ok.UID)).Return(exportPods(ok,
		`[SUCCESS] {"run":"`+testRun+`","attempt":"x20","pv":1,"stage":"export","output":"`+testRunDir+
			`/exports/step-20"}`), nil).Times(1)
	ctx := context.Background()
	run := testRunRow("Succeeded", "done")
	require.NoError(t, r.reconcileExports(ctx, run))
	require.NoError(t, r.reconcileExports(ctx, run), "a registered step is not registered again")

	models := &v1.ModelList{}
	require.NoError(t, r.List(ctx, models))
	require.Len(t, models.Items, 1, "the failed export of step 10 registers nothing")
	m := models.Items[0]
	require.Equal(t, "20", m.Labels[v1.PostTrainingExportStepLabel])
	require.Equal(t, testRun, m.Spec.PostTrainingRunId)
	require.Equal(t, "qwen sft step 20", m.Spec.DisplayName)
	require.NotContains(t, m.Labels, v1.DisplayNameLabel, "a name that is no label value stays out of the labels")
	require.Equal(t, testRunDir+"/exports/step-20", m.Spec.Source.LocalPath)
}

// An export model the user deleted is not registered again from the kept export job.
func TestADeletedExportModelIsNotRegisteredAgain(t *testing.T) {
	w := exportJob("pt-x20-e", v1.WorkloadSucceeded)
	r, db, _ := newTestReconciler(t, testRunRow("Succeeded", "done"), w)
	db.EXPECT().ListWorkloadPods(gomock.Any(), w.Name, string(w.UID)).Return(exportPods(w,
		`[SUCCESS] {"run":"`+testRun+`","attempt":"x20","pv":1,"stage":"export","output":"`+testRunDir+
			`/exports/step-20"}`), nil).AnyTimes()
	ctx := context.Background()
	run := testRunRow("Succeeded", "done")
	require.NoError(t, r.reconcileExports(ctx, run))
	models := &v1.ModelList{}
	require.NoError(t, r.List(ctx, models))
	require.Len(t, models.Items, 1)
	require.Equal(t, testRun+"-s20", models.Items[0].Name, "a retried registration finds the same name")
	job := &v1.Workload{}
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(w), job))
	require.Equal(t, testRun+"-s20", job.Annotations[exportModelAnnotation], "the job records the model it made")

	require.NoError(t, r.Delete(ctx, &models.Items[0]))
	require.NoError(t, r.reconcileExports(ctx, run))
	require.NoError(t, r.List(ctx, models))
	require.Empty(t, models.Items, "the user deleted the model, so the still kept export job makes no new one")
}

// An export model the cache does not show yet is not created twice and inherits once visible.
func TestAnExportModelThatTheCacheDoesNotShowYetIsCreatedOnce(t *testing.T) {
	w := exportJob("pt-x20-f", v1.WorkloadSucceeded)
	base := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-base"}}
	base.Status.ModelConfig = &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, NumHiddenLayers: 36}
	r, db, _ := newTestReconciler(t, testRunRow("Succeeded", "done"), w, base)
	db.EXPECT().ListWorkloadPods(gomock.Any(), w.Name, string(w.UID)).Return(exportPods(w,
		`[SUCCESS] {"run":"`+testRun+`","attempt":"x20","pv":1,"stage":"export","output":"`+testRunDir+
			`/exports/step-20"}`), nil).AnyTimes()
	lag := true
	inner := r.Client
	r.Client = interceptor.NewClient(inner.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object,
			opts ...client.GetOption) error {
			if _, ok := obj.(*v1.Model); ok && lag && key.Name != "m-base" {
				return apierrors.NewNotFound(v1.SchemeGroupVersion.WithResource("models").GroupResource(), key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1.ModelList); ok && lag {
				return nil
			}
			return c.List(ctx, list, opts...)
		},
	})
	ctx := context.Background()
	run := testRunRow("Succeeded", "done")
	run.BaseModelID = "m-base"
	require.NoError(t, r.reconcileExports(ctx, run), "the new Model is not read back before the cache has it")
	require.NoError(t, r.reconcileExports(ctx, run))
	models := &v1.ModelList{}
	require.NoError(t, inner.List(ctx, models, client.MatchingLabels{v1.PostTrainingRunIdLabel: testRun}))
	require.Len(t, models.Items, 1, "a registration the cache does not show yet is not made twice")

	lag = false
	require.NoError(t, r.reconcileExports(ctx, run))
	m := &v1.Model{}
	require.NoError(t, inner.Get(ctx, client.ObjectKey{Name: testRun + "-s20"}, m))
	require.NotNil(t, m.Status.ModelConfig, "the visible Model inherits the base model's structure summary")
}

// An export whose output is outside the run directory registers no model.
func TestExportOutsideTheRunDirectoryIsIgnored(t *testing.T) {
	w := exportJob("pt-x20-c", v1.WorkloadSucceeded)
	r, db, _ := newTestReconciler(t, testRunRow("Succeeded", "done"), w)
	db.EXPECT().ListWorkloadPods(gomock.Any(), w.Name, string(w.UID)).Return(exportPods(w,
		`[SUCCESS] {"run":"`+testRun+`","attempt":"x20","pv":1,"stage":"export","output":"/elsewhere"}`), nil)
	require.NoError(t, r.reconcileExports(context.Background(), testRunRow("Succeeded", "done")))
	models := &v1.ModelList{}
	require.NoError(t, r.List(context.Background(), models))
	require.Empty(t, models.Items)
}

// The exports of a stopped run are registered.
func TestReconcileTakesExportsOfFinishedRuns(t *testing.T) {
	w := exportJob("pt-x20-d", v1.WorkloadSucceeded)
	r, db, _ := newTestReconciler(t, testRunRow("Stopped", "train"), w)
	db.EXPECT().ListWorkloadPods(gomock.Any(), w.Name, string(w.UID)).Return(exportPods(w,
		`[SUCCESS] {"run":"`+testRun+`","attempt":"x20","pv":1,"stage":"export","output":"`+testRunDir+
			`/exports/step-20"}`), nil)
	reconcileRun(t, r)
	models := &v1.ModelList{}
	require.NoError(t, r.List(context.Background(), models))
	require.Len(t, models.Items, 1)
}
