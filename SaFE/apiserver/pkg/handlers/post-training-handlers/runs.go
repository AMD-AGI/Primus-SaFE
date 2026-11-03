/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

const (
	defaultListLimit = 50
	maxListLimit     = 500
)

// listQuery holds the filters of GET /post-training/runs.
type listQuery struct {
	Workspace   string `form:"workspace"`
	Framework   string `form:"framework"`
	TrainType   string `form:"trainType"`
	Method      string `form:"method"`
	Status      string `form:"status"`
	Stage       string `form:"stage"`
	BaseModelID string `form:"baseModelId"`
	Owner       string `form:"owner"`
	Search      string `form:"search"`
	Limit       int    `form:"limit"`
	Offset      int    `form:"offset"`
}

// listResponse is one page of runs.
type listResponse struct {
	Total int64                       `json:"total"`
	Items []*dbclient.PostTrainingRun `json:"items"`
}

// runDetail is a run with its parsed recipe, stage history, output model and evaluations.
type runDetail struct {
	*dbclient.PostTrainingRun
	Recipe       json.RawMessage            `json:"recipe,omitempty"`
	StageHistory json.RawMessage            `json:"stageHistory,omitempty"`
	Summary      json.RawMessage            `json:"metricSummary,omitempty"`
	OutputModel  *outputModel               `json:"outputModel,omitempty"`
	Evaluations  []*dbclient.EvaluationTask `json:"evaluations"`
}

// outputModel summarizes the model a run registered.
type outputModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Phase       string `json:"phase"`
}

// ListRuns returns a workspace's runs, newest first.
func (h *Handler) ListRuns(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	var q listQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid query: "+err.Error()))
		return
	}
	if q.Workspace == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("workspace is required"))
		return
	}
	if err := h.authorize(c, v1.ListVerb, q.Workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	filter := dbclient.PostTrainingRunFilter{Workspace: q.Workspace, Framework: q.Framework, TrainType: q.TrainType,
		Method: q.Method, Stage: q.Stage, BaseModelID: q.BaseModelID, UserID: q.Owner, Search: q.Search,
		Limit: q.Limit, Offset: q.Offset}
	if q.Status != "" {
		filter.Statuses = strings.Split(q.Status, ",")
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultListLimit
	}
	if filter.Limit > maxListLimit {
		filter.Limit = maxListLimit
	}
	runs, total, err := h.db.ListPostTrainingRuns(c.Request.Context(), filter)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list runs: "+err.Error()))
		return
	}
	for _, r := range runs {
		r.Recipe = ""
	}
	c.JSON(http.StatusOK, listResponse{Total: total, Items: runs})
}

// getRun loads a run and checks the caller may act on it with verb.
func (h *Handler) getRun(c *gin.Context, verb v1.RoleVerb) (*dbclient.PostTrainingRun, error) {
	run, err := h.db.GetPostTrainingRun(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("run %s not found", c.Param("id")))
		}
		return nil, commonerrors.NewInternalError("failed to read the run: " + err.Error())
	}
	if err := h.authorize(c, verb, run.Workspace, run.UserID, run.WorkloadID, 0); err != nil {
		return nil, err
	}
	return run, nil
}

// GetRun returns a run with its recipe snapshot, stage history, output model and evaluations.
func (h *Handler) GetRun(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	detail := &runDetail{PostTrainingRun: run, Recipe: rawJSON(run.Recipe), StageHistory: rawJSON(run.StageHistory),
		Summary: rawJSON(run.MetricSummary), Evaluations: []*dbclient.EvaluationTask{}}
	if run.OutputModelID != "" {
		m := &v1.Model{}
		if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: run.OutputModelID}, m); err == nil {
			detail.OutputModel = &outputModel{ID: m.Name, DisplayName: baseModelName(m), Phase: string(m.Status.Phase)}
		}
	}
	tasks, err := h.db.SelectEvaluationTasks(ctx,
		sqrl.Eq{"post_training_run_id": run.RunID, "is_deleted": false}, []string{"creation_time DESC"}, 100, 0)
	if err != nil {
		klog.ErrorS(err, "failed to list the run's evaluations", "run", run.RunID)
	} else if tasks != nil {
		detail.Evaluations = tasks
	}
	c.JSON(http.StatusOK, detail)
}

// rawJSON returns s as raw JSON, or nil when it is empty or not JSON.
func rawJSON(s string) json.RawMessage {
	if s == "" || !json.Valid([]byte(s)) {
		return nil
	}
	return json.RawMessage(s)
}

// StopRun stops the run's training workload; the controller then records the run as Stopped.
func (h *Handler) StopRun(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.DeleteVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	message := fmt.Sprintf("the run is stopped by %s", c.GetString(common.UserName))
	if err := h.stopWorkload(c.Request.Context(), run.WorkloadID, message); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("stop post training run %s by user %s", run.RunID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"runId": run.RunID})
}

// stopWorkload marks a workload Stopped and deletes it, the way the workload stop API does; a missing or finished
// workload is left alone.
func (h *Handler) stopWorkload(ctx context.Context, name, message string) error {
	w := &v1.Workload{}
	if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: name}, w); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if w.IsEnd() {
		return nil
	}
	patch := ctrlclient.MergeFrom(w.DeepCopy())
	now := metav1.NewTime(time.Now().UTC())
	w.Status.Phase = v1.WorkloadStopped
	if w.Status.EndTime == nil {
		w.Status.EndTime = &now
	}
	meta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{
		Type: string(v1.AdminStopped), Status: metav1.ConditionTrue, Message: message, LastTransitionTime: now,
		Reason: commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(w)),
	})
	if err := h.client.Status().Patch(ctx, w, patch); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := h.client.Delete(ctx, w); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// DeleteRun stops a running run, then soft-deletes it and its metric points; deleteModel=true also deletes the model
// it registered, whose weights the model controller removes.
func (h *Handler) DeleteRun(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.DeleteVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	message := fmt.Sprintf("the run is deleted by %s", c.GetString(common.UserName))
	if err := h.stopWorkload(ctx, run.WorkloadID, message); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if c.Query("deleteModel") == "true" && run.OutputModelID != "" {
		m := &v1.Model{}
		m.Name = run.OutputModelID
		if err := h.client.Delete(ctx, m); err != nil && !apierrors.IsNotFound(err) {
			apiutils.AbortWithApiError(c, err)
			return
		}
	}
	if err := h.db.SetPostTrainingRunDeleted(ctx, run.RunID); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to delete the run: "+err.Error()))
		return
	}
	if err := h.db.DeletePostTrainingRunMetrics(ctx, run.RunID); err != nil {
		klog.ErrorS(err, "failed to delete the run's metric points", "run", run.RunID)
	}
	klog.Infof("delete post training run %s by user %s", run.RunID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"runId": run.RunID})
}
