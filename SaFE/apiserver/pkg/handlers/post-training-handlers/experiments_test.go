/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// baselineRequest is the baseline of an RL experiment with one benchmark.
func baselineRequest() experimentRequest {
	return experimentRequest{DisplayName: "grpo-ab", Baseline: recipe.SubmitRequest{Workspace: "ws", TrainType: "rl",
		Method: "full", ModelID: "m1", DatasetID: "d1", Choices: recipe.Choices{Lr: 1e-6, NumRollout: 10},
		Evaluation: recipe.EvaluationRequest{Benchmarks: []string{"aime"}}}}
}

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
}

func TestMergeChanges(t *testing.T) {
	dst := map[string]interface{}{"a": map[string]interface{}{"x": 1, "y": 2}, "b": 1, "c": 3}
	mergeChanges(dst, map[string]interface{}{"a": map[string]interface{}{"y": 5}, "b": nil, "d": "new"})
	require.Equal(t, map[string]interface{}{"a": map[string]interface{}{"x": 1, "y": 5}, "c": 3, "d": "new"}, dst)
}

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
