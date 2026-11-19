/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// baselineRequest is the baseline of an RL experiment with one benchmark.
func baselineRequest() experimentRequest {
	return experimentRequest{DisplayName: "grpo-ab", Baseline: recipe.SubmitRequest{Workspace: "ws", TrainType: "rl",
		Method: "full", ModelID: "m1", DatasetID: "d1", Choices: recipe.Choices{Lr: 1e-6, NumRollout: 10},
		Evaluation: recipe.EvaluationRequest{Benchmarks: []string{"aime"}}}}
}

// Each variant's request is the baseline with its changes and a name of experiment and variant.
func TestVariantRequests(t *testing.T) {
	req := baselineRequest()
	req.Variants = []experimentVariant{
		{Name: "lr2e-6", Changes: map[string]interface{}{"choices": map[string]interface{}{"lr": 2e-6}}},
		{Name: "full-data", Changes: map[string]interface{}{"datasetId": "d2", "workspace": "other",
			"evaluation": map[string]interface{}{"benchmarks": []interface{}{"gsm8k"}}}},
	}
	reqs, names, err := variantRequests(&req, 42)
	require.NoError(t, err)
	require.Equal(t, []string{"baseline", "lr2e-6", "full-data"}, names)
	require.Equal(t, "grpo-ab-baseline", reqs[0].DisplayName)
	require.Equal(t, 1e-6, reqs[0].Choices.Lr)
	require.Equal(t, 2e-6, reqs[1].Choices.Lr)
	require.Equal(t, 10, reqs[1].Choices.NumRollout, "fields a variant leaves alone stay the baseline's")
	require.Equal(t, "d2", reqs[2].DatasetID)
	for _, r := range reqs {
		require.Equal(t, 42, r.Seed, "every variant shares the seed")
		require.Equal(t, "ws", r.Workspace, "a variant stays in the baseline's workspace")
		require.Equal(t, []string{"aime"}, r.Evaluation.Benchmarks, "every variant runs the baseline's benchmarks")
		require.True(t, r.Evaluation.Enabled)
	}

	for _, variants := range [][]experimentVariant{
		nil,
		{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}},
		{{Name: "Bad Name"}},
		{{Name: "baseline"}},
		{{Name: "a"}, {Name: "a"}},
		{{Name: "sft", Changes: map[string]interface{}{"trainType": "sft"}}},
	} {
		req.Variants = variants
		_, _, err = variantRequests(&req, 1)
		require.Equal(t, http.StatusBadRequest, statusCode(err), "%+v", variants)
	}

	req.Variants = []experimentVariant{{Name: "lr2e-6"}}
	req.DisplayName = strings.Repeat("e", 120)
	_, _, err = variantRequests(&req, 1)
	require.Equal(t, http.StatusBadRequest, statusCode(err), "a variant's run name holds at most 128 characters")
}

// Changes merge into nested fields, and a null removes the field.
func TestMergeChanges(t *testing.T) {
	dst := map[string]interface{}{"a": map[string]interface{}{"x": 1, "y": 2}, "b": 1, "c": 3}
	mergeChanges(dst, map[string]interface{}{"a": map[string]interface{}{"y": 5}, "b": nil, "d": "new"})
	require.Equal(t, map[string]interface{}{"a": map[string]interface{}{"x": 1, "y": 5}, "c": 3, "d": "new"}, dst)
}

// An experiment lists its runs in variant order, without their recipes.
func TestExperimentRunsInVariantOrder(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), dbclient.PostTrainingRunFilter{ExperimentID: "exp-1", Limit: 5}).
		Return([]*dbclient.PostTrainingRun{
			{RunID: "r3", VariantName: "b", Recipe: "{}"},
			{RunID: "r1", VariantName: "baseline"},
			{RunID: "r2", VariantName: "a"},
		}, int64(3), nil)
	h := NewHandler(nil, db)
	runs, err := h.experimentRuns(testContext(), &dbclient.PostTrainingExperiment{ExperimentID: "exp-1",
		BaselineVariant: "baseline", Variants: `[{"name":"a"},{"name":"b"}]`})
	require.NoError(t, err)
	require.Equal(t, []string{"r1", "r2", "r3"}, []string{runs[0].RunID, runs[1].RunID, runs[2].RunID})
	require.Empty(t, runs[2].Recipe, "the list leaves the recipes out")
}

// An experiment is not deleted in part: a model that one of its runs may not lose refuses the delete before any
// run is deleted, with the reason and not as a server error.
func TestDeleteExperimentChecksEveryModelBeforeItDeletesARun(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := templateHandler(t, db)
	db.EXPECT().GetPostTrainingExperiment(gomock.Any(), "exp-1").Return(&dbclient.PostTrainingExperiment{
		ExperimentID: "exp-1", Workspace: "ws1", UserID: "u1", BaselineVariant: "baseline"}, nil)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return([]*dbclient.PostTrainingRun{
		{RunID: "pt-a", WorkloadID: "pt-a", Workspace: "ws1", UserID: "u1", VariantName: "baseline", Status: "Succeeded"},
		{RunID: "pt-b", WorkloadID: "pt-b", Workspace: "ws1", UserID: "u1", VariantName: "lr", Status: "Succeeded",
			OutputModelID: "m-b"},
	}, int64(2), nil).AnyTimes()
	require.NoError(t, h.client.Create(context.Background(), &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "infer-1"},
		Spec:   v1.WorkloadSpec{Env: map[string]string{sourceModelEnv: "m-b"}},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/?deleteModel=true", nil)
	c.Params = gin.Params{{Key: "id", Value: "exp-1"}}
	c.Set(common.UserId, "u1")
	c.Set(common.UserName, "alice")
	h.DeleteExperiment(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "infer-1")
}

// An experiment whose variants did not all start is rolled back although its request has ended, as when the caller
// gave up waiting: the runs that started would otherwise go on.
func TestAnExperimentIsRolledBackAfterItsRequestEnded(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := templateHandler(t, db)
	live := func(ctx context.Context) { require.NoError(t, ctx.Err(), "the rollback outlives the request") }
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), "pt-a").DoAndReturn(
		func(ctx context.Context, _ string) error { live(ctx); return nil })
	db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), "pt-a").Return(nil)
	db.EXPECT().DeletePostTrainingRunSamples(gomock.Any(), "pt-a", gomock.Any()).Return(nil)
	db.EXPECT().SetPostTrainingExperimentDeleted(gomock.Any(), "exp-1").DoAndReturn(
		func(ctx context.Context, _ string) error { live(ctx); return nil })

	ended, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ended)
	h.rollBackExperiment(c, &dbclient.PostTrainingExperiment{ExperimentID: "exp-1"},
		[]*preparedRun{{row: &dbclient.PostTrainingRun{RunID: "pt-a", WorkloadID: "pt-a"}}})
	require.Error(t, c.Request.Context().Err(), "the request keeps its own context")
}
