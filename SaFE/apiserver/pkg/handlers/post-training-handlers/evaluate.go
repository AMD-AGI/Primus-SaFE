/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// evaluateRequest is the body of POST /runs/:id/evaluate.
type evaluateRequest struct {
	// ModelID is the run's output model by default, or a model exported from the run.
	ModelID     string   `json:"modelId,omitempty"`
	Benchmarks  []string `json:"benchmarks"`
	KeepService bool     `json:"keepService,omitempty"`
}

// EvaluateRun evaluates a model of the run again. The run controller does the work; the run's status and stage
// do not change.
func (h *Handler) EvaluateRun(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	var req evaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	if regErr != nil {
		apiutils.AbortWithApiError(c, apierrors.NewServiceUnavailable("Post Training registry is invalid: "+regErr.Error()))
		return
	}
	if err := pausedError(reg); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	run, err := h.getWritableRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if err = h.authorize(c, v1.CreateVerb, run.Workspace, "", "", run.Priority); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	modelID, err := h.requestEvaluation(c.Request.Context(), run, &req)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("evaluate model %s of post training run %s by user %s", modelID, run.RunID, c.GetString(common.UserName))
	c.JSON(http.StatusAccepted, gin.H{"runId": run.RunID, "modelId": modelID})
}

// requestEvaluation checks an evaluation request of run and records it on the model, where the run controller
// takes it. It returns the evaluated model id.
func (h *Handler) requestEvaluation(ctx context.Context, run *dbclient.PostTrainingRun, req *evaluateRequest) (string, error) {
	if len(req.Benchmarks) == 0 {
		return "", commonerrors.NewBadRequest("benchmarks is required")
	}
	modelID := req.ModelID
	if modelID == "" {
		modelID = run.OutputModelID
	}
	if modelID == "" {
		return "", commonerrors.NewBadRequest("the run has no output model to evaluate")
	}
	model := &v1.Model{}
	if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: modelID}, model); err != nil {
		if apierrors.IsNotFound(err) {
			return "", commonerrors.NewBadRequest(fmt.Sprintf("model %s not found", modelID))
		}
		return "", commonerrors.NewInternalError("failed to read the model: " + err.Error())
	}
	if v1.GetLabel(model, v1.PostTrainingRunIdLabel) != run.RunID {
		return "", commonerrors.NewBadRequest(fmt.Sprintf("model %s is not an output of run %s", modelID, run.RunID))
	}
	if !model.IsReady() {
		return "", commonerrors.NewBadRequest(fmt.Sprintf("model %s is not ready", modelID))
	}
	for _, id := range req.Benchmarks {
		dataset, err := h.db.GetDataset(ctx, id)
		if err != nil || dataset == nil {
			return "", commonerrors.NewBadRequest(fmt.Sprintf("dataset %s not found", id))
		}
		if dataset.DatasetType != "evaluation" {
			return "", commonerrors.NewBadRequest(fmt.Sprintf("dataset %s is not an evaluation dataset", id))
		}
	}
	busy, err := h.evaluationInProgress(ctx, run)
	if err != nil {
		return "", commonerrors.NewInternalError("failed to check the run's evaluations: " + err.Error())
	}
	if busy {
		return "", commonerrors.NewConflict(fmt.Sprintf("run %s is being evaluated", run.RunID))
	}
	data, _ := json.Marshal(map[string]interface{}{"benchmarks": req.Benchmarks, "keepService": req.KeepService})
	patch := ctrlclient.MergeFrom(model.DeepCopy())
	v1.SetAnnotation(model, v1.PostTrainingEvaluateAnnotation, string(data))
	if err = h.client.Patch(ctx, model, patch); err != nil {
		return "", commonerrors.NewInternalError("failed to request the evaluation: " + err.Error())
	}
	return modelID, nil
}

// evaluationInProgress reports whether the run is being evaluated: its automatic evaluation is running, a
// request waits on one of its models, or one of its inference services has not finished its evaluation.
func (h *Handler) evaluationInProgress(ctx context.Context, run *dbclient.PostTrainingRun) (bool, error) {
	if run.Stage == "eval" {
		return true, nil
	}
	models := &v1.ModelList{}
	if err := h.client.List(ctx, models, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
		return false, err
	}
	for i := range models.Items {
		if v1.GetAnnotation(&models.Items[i], v1.PostTrainingEvaluateAnnotation) != "" {
			return true, nil
		}
	}
	services := &v1.WorkloadList{}
	if err := h.client.List(ctx, services, ctrlclient.MatchingLabels{
		v1.PostTrainingRunIdLabel: run.RunID, v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval}); err != nil {
		return false, err
	}
	for i := range services.Items {
		s := &services.Items[i]
		if !s.IsEnd() && v1.GetAnnotation(s, v1.PostTrainingEvalDoneAnnotation) != v1.TrueStr {
			return true, nil
		}
	}
	return false, nil
}
