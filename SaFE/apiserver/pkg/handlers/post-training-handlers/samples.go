/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// sampleView is a stored sample, with the run directory file of the untruncated sample when it was truncated.
type sampleView struct {
	*dbclient.PostTrainingRunSample
	File string `json:"file,omitempty"`
}

// samplesResponse is one step of a run's samples and the steps that have samples.
type samplesResponse struct {
	Kind    string                               `json:"kind"`
	Step    *int                                 `json:"step"`
	Dataset string                               `json:"dataset"`
	Steps   []dbclient.PostTrainingRunSampleStep `json:"steps"`
	Samples []sampleView                         `json:"samples"`
	// Capped says the run stored as many samples as it may; later ones are only in the run directory.
	Capped bool `json:"capped"`
}

// GetSamples returns the RL samples of one step of a run: kind rollout or eval, the latest step unless step is
// given, and for evaluations one dataset, the first unless dataset is given.
func (h *Handler) GetSamples(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	kind := c.DefaultQuery("kind", "rollout")
	if kind != "rollout" && kind != "eval" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("kind must be rollout or eval"))
		return
	}
	ctx := c.Request.Context()
	steps, err := h.db.ListPostTrainingRunSampleSteps(ctx, run.RunID, kind)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list the run's samples: "+err.Error()))
		return
	}
	resp := samplesResponse{Kind: kind, Steps: steps, Samples: []sampleView{}, Capped: samplesCapped(run)}
	if resp.Steps == nil {
		resp.Steps = []dbclient.PostTrainingRunSampleStep{}
	}
	step, dataset, ok := pickSampleStep(steps, c.Query("step"), c.Query("dataset"))
	if !ok {
		c.JSON(http.StatusOK, resp)
		return
	}
	resp.Step, resp.Dataset = &step, dataset
	rows, err := h.db.ListPostTrainingRunSamples(ctx, dbclient.PostTrainingRunSampleFilter{RunID: run.RunID, Kind: kind,
		Step: step, Dataset: dataset})
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to read the run's samples: "+err.Error()))
		return
	}
	var rec recipe.Recipe
	_ = json.Unmarshal([]byte(run.Recipe), &rec)
	file := fmt.Sprintf("%s/samples/%s-%d.jsonl", rec.Output.Base, kind, step)
	for _, row := range rows {
		v := sampleView{PostTrainingRunSample: row}
		if len([]rune(row.Prompt)) < row.PromptChars || len([]rune(row.Response)) < row.ResponseChars {
			v.File = file
		}
		resp.Samples = append(resp.Samples, v)
	}
	c.JSON(http.StatusOK, resp)
}

// pickSampleStep chooses the step and dataset to show: the requested ones, or the latest step and its first
// dataset. It reports false when the run has no such samples.
func pickSampleStep(steps []dbclient.PostTrainingRunSampleStep, stepQuery, datasetQuery string) (int, string, bool) {
	if len(steps) == 0 {
		return 0, "", false
	}
	step := steps[0].Step
	if stepQuery != "" {
		v, err := strconv.Atoi(stepQuery)
		if err != nil {
			return 0, "", false
		}
		step = v
	}
	for _, s := range steps {
		if s.Step == step && (datasetQuery == "" || s.Dataset == datasetQuery) {
			return step, s.Dataset, true
		}
	}
	return 0, "", false
}

// samplesCapped reads from the run's metric summary whether it reached its cap on stored samples.
func samplesCapped(run *dbclient.PostTrainingRun) bool {
	var sum struct {
		SamplesCapped bool `json:"samplesCapped"`
	}
	_ = json.Unmarshal([]byte(run.MetricSummary), &sum)
	return sum.SamplesCapped
}
