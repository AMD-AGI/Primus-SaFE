/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// evalHandler returns a handler over a fake API server holding objs and a database with one evaluation dataset.
func evalHandler(t *testing.T, objs ...ctrlclient.Object) *Handler {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().GetDataset(gomock.Any(), "gsm8k").Return(&dbclient.Dataset{DatasetType: "evaluation"}, nil).AnyTimes()
	db.EXPECT().GetDataset(gomock.Any(), "alpaca").Return(&dbclient.Dataset{DatasetType: "sft"}, nil).AnyTimes()
	return NewHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(), db)
}

// statusCode returns the HTTP status of an API error.
func statusCode(err error) int {
	if status, ok := err.(apierrors.APIStatus); ok {
		return int(status.Status().Code)
	}
	return 0
}

func TestRequestEvaluation(t *testing.T) {
	run := &dbclient.PostTrainingRun{RunID: "pt-1", Stage: "done", OutputModelID: "m-out"}
	out := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-out", Labels: map[string]string{v1.PostTrainingRunIdLabel: "pt-1"}}}
	out.Status.Phase = v1.ModelPhaseReady
	other := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-other"}}
	other.Status.Phase = v1.ModelPhaseReady
	h := evalHandler(t, out, other)
	ctx := context.Background()

	for name, req := range map[string]evaluateRequest{
		"no benchmarks":       {},
		"foreign model":       {ModelID: "m-other", Benchmarks: []string{"gsm8k"}},
		"not an eval dataset": {Benchmarks: []string{"alpaca"}},
		"missing model":       {ModelID: "m-gone", Benchmarks: []string{"gsm8k"}},
	} {
		_, err := h.requestEvaluation(ctx, run, &req)
		require.Equal(t, http.StatusBadRequest, statusCode(err), name)
	}

	modelID, err := h.requestEvaluation(ctx, run, &evaluateRequest{Benchmarks: []string{"gsm8k"}, KeepService: true})
	require.NoError(t, err)
	require.Equal(t, "m-out", modelID, "the output model is evaluated by default")
	got := &v1.Model{}
	require.NoError(t, h.client.Get(ctx, ctrlclient.ObjectKey{Name: "m-out"}, got))
	require.JSONEq(t, `{"benchmarks":["gsm8k"],"keepService":true}`, got.Annotations[v1.PostTrainingEvaluateAnnotation])

	_, err = h.requestEvaluation(ctx, run, &evaluateRequest{Benchmarks: []string{"gsm8k"}})
	require.Equal(t, http.StatusConflict, statusCode(err), "a waiting request blocks another")
	_, err = h.requestEvaluation(ctx, &dbclient.PostTrainingRun{RunID: "pt-1", Stage: "eval", OutputModelID: "m-out"},
		&evaluateRequest{Benchmarks: []string{"gsm8k"}})
	require.Equal(t, http.StatusConflict, statusCode(err), "the automatic evaluation blocks another")
}

func TestEvaluationInProgressIgnoresFinishedServices(t *testing.T) {
	labels := map[string]string{v1.PostTrainingRunIdLabel: "pt-1", v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval}
	kept := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "svc-kept", Labels: labels,
		Annotations: map[string]string{v1.PostTrainingEvalDoneAnnotation: v1.TrueStr}}}
	h := evalHandler(t, kept)
	run := &dbclient.PostTrainingRun{RunID: "pt-1", Stage: "done"}
	busy, err := h.evaluationInProgress(context.Background(), run)
	require.NoError(t, err)
	require.False(t, busy, "a kept service whose evaluation is over does not block a new one")

	running := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "svc-new", Labels: labels}}
	h = evalHandler(t, kept, running)
	busy, err = h.evaluationInProgress(context.Background(), run)
	require.NoError(t, err)
	require.True(t, busy)
}
