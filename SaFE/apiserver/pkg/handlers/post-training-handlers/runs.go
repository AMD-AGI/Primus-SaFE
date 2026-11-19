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
	"slices"
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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
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
	Total int64      `json:"total"`
	Items []listItem `json:"items"`
}

// listItem is a run of the list with how many runs of its recipe hash succeeded on the platform and the latest of
// them in the list's workspace.
type listItem struct {
	*dbclient.PostTrainingRun
	VerifiedRuns      int    `json:"verifiedRuns,omitempty"`
	LatestVerifiedRun string `json:"latestVerifiedRun,omitempty"`
}

// runDetail is a run with its parsed recipe, stage history, output model and evaluations.
type runDetail struct {
	*dbclient.PostTrainingRun
	Recipe       json.RawMessage            `json:"recipe,omitempty"`
	StageHistory json.RawMessage            `json:"stageHistory,omitempty"`
	Summary      json.RawMessage            `json:"metricSummary,omitempty"`
	OutputModel  *outputModel               `json:"outputModel,omitempty"`
	Evaluations  []*dbclient.EvaluationTask `json:"evaluations"`
	Exports      []exportItem               `json:"exports"`
	// ImageListed is whether the registry still lists the run's image, which a copy of the run then keeps.
	ImageListed bool `json:"imageListed"`
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
	filter.Limit, filter.Offset = pageLimit(filter.Limit, defaultListLimit), max(filter.Offset, 0)
	runs, total, err := h.db.ListPostTrainingRuns(c.Request.Context(), filter)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list runs: "+err.Error()))
		return
	}
	hashes := make([]string, 0, len(runs))
	for _, r := range runs {
		r.Recipe = ""
		if r.RecipeHash != "" {
			hashes = append(hashes, r.RecipeHash)
		}
	}
	records, err := h.db.ListPostTrainingRecipeRecords(c.Request.Context(), q.Workspace, hashes)
	if err != nil {
		klog.ErrorS(err, "failed to count the successful runs of the listed recipes")
	}
	items := make([]listItem, 0, len(runs))
	for _, r := range runs {
		rec := records[r.RecipeHash]
		items = append(items, listItem{PostTrainingRun: r, VerifiedRuns: rec.Succeeded, LatestVerifiedRun: rec.LatestRun})
	}
	c.JSON(http.StatusOK, listResponse{Total: total, Items: items})
}

// pageLimit returns a list's page size: limit, def when limit is not positive, and at most maxListLimit.
func pageLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	return min(limit, maxListLimit)
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
	reg, _, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	detail := &runDetail{PostTrainingRun: run, Recipe: rawJSON(run.Recipe), StageHistory: rawJSON(run.StageHistory),
		Summary: rawJSON(run.MetricSummary), Evaluations: []*dbclient.EvaluationTask{},
		ImageListed: reg != nil && reg.ImageByName(run.Image) != nil}
	if run.OutputModelID != "" {
		m := &v1.Model{}
		if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: run.OutputModelID}, m); err == nil {
			detail.OutputModel = &outputModel{ID: m.Name, DisplayName: baseModelName(m), Phase: string(m.Status.Phase)}
		}
	}
	if detail.Exports, err = h.listExports(ctx, run.RunID); err != nil {
		klog.ErrorS(err, "failed to list the run's checkpoint exports", "run", run.RunID)
		detail.Exports = []exportItem{}
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
	message := fmt.Sprintf("Stopped by %s", c.GetString(common.UserName))
	if _, err := h.stopRun(c.Request.Context(), run, message); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("stop post training run %s by user %s", run.RunID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"runId": run.RunID})
}

// cleanupBody is the body of POST /runs/:id/checkpoints/cleanup.
type cleanupBody struct {
	// Keep is "best" to keep the checkpoint with the lowest validation loss, or "none".
	Keep string `json:"keep"`
}

// checkpointCleanup is the cleanup_request column: the directories the controller removes, the step kept and, for
// a cleanup that keeps one, the steps removed.
type checkpointCleanup struct {
	Keep         string   `json:"keep"`
	Paths        []string `json:"paths"`
	KeptStep     *int     `json:"keptStep,omitempty"`
	CleanedSteps *[]int   `json:"cleanedSteps,omitempty"`
}

// CleanupCheckpoints removes the checkpoints of a finished run, keeping the best one or none. The run controller
// runs the cleanup jobs and updates the run's disk usage; the run, its curves and its output model stay.
func (h *Handler) CleanupCheckpoints(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	var body cleanupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	run, err := h.getRun(c, v1.DeleteVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	plan, err := h.planCleanup(ctx, run, body.Keep)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	// The request and the look for an export go together: a caller that leaves between them would leave the
	// request unchecked.
	act, cancel := detached(ctx)
	defer cancel()
	if err = h.requestCleanup(act, run.RunID, plan); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if err = h.withdrawCleanupIfExporting(ctx, run.RunID); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("clean up the checkpoints of post training run %s (keep %s) by user %s", run.RunID, body.Keep,
		c.GetString(common.UserName))
	c.JSON(http.StatusAccepted, gin.H{"runId": run.RunID, "keep": body.Keep})
}

// exportRunning answers a conflict while an export job of the run has not ended, as it reads a checkpoint. The
// jobs are read from reader: the client's cache, or the API server for a job made a moment ago.
func (h *Handler) exportRunning(ctx context.Context, reader ctrlclient.Reader, runID string) error {
	jobs := &v1.WorkloadList{}
	if err := reader.List(ctx, jobs, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: runID,
		v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport}); err != nil {
		return commonerrors.NewInternalError("failed to list the run's exports: " + err.Error())
	}
	for i := range jobs.Items {
		// A job that is being deleted counts as over, as for the controller: its export was refused or dropped.
		w := &jobs.Items[i]
		if w.IsEnd() {
			continue
		}
		if step := w.Labels[v1.PostTrainingExportStepLabel]; step != "" {
			return commonerrors.NewConflict(fmt.Sprintf("step %s of run %s is being exported", step, runID))
		}
		return commonerrors.NewConflict(fmt.Sprintf("export job %s of run %s is running", w.Name, runID))
	}
	return nil
}

// withdrawCleanupIfExporting takes a cleanup request back when an export started between its plan and its
// request, and answers the conflict: the export and the cleanup each look for the other after they acted, so the
// one that acted second gives way.
func (h *Handler) withdrawCleanupIfExporting(ctx context.Context, runID string) error {
	// Apart from the request, and from the API server: a caller that left must not leave the request behind
	// unchecked, and the cache may not show an export job made a moment ago.
	check, cancel := recheck(ctx)
	found := h.exportRunning(check, h.reader, runID)
	cancel()
	if found == nil {
		return nil
	}
	// An export runs, or the exports cannot be listed to tell: either way the cleanup must not go ahead. The
	// withdrawal has time of its own, as the look may have used up its time.
	undo, cancel := detached(ctx)
	defer cancel()
	if err := h.db.UpdatePostTrainingRun(undo, runID, map[string]interface{}{"cleanup_request": ""}); err != nil {
		return commonerrors.NewInternalError(fmt.Sprintf("failed to withdraw the checkpoint cleanup of run %s, "+
			"which an export may run against: %v", runID, err))
	}
	return found
}

// requestCleanup records a checkpoint cleanup unless the run got one since it was read.
func (h *Handler) requestCleanup(ctx context.Context, runID, plan string) error {
	taken, err := h.db.RequestPostTrainingRunCleanup(ctx, runID, plan)
	if err != nil {
		return commonerrors.NewInternalError("failed to request the cleanup: " + err.Error())
	}
	if !taken {
		return commonerrors.NewConflict(fmt.Sprintf("a checkpoint cleanup of run %s is in progress", runID))
	}
	return nil
}

// planCleanup checks that the checkpoints of run can be cleaned up and returns the cleanup_request value: both
// checkpoint directories for "none", every remaining checkpoint but the best one for "best".
func (h *Handler) planCleanup(ctx context.Context, run *dbclient.PostTrainingRun, keep string) (string, error) {
	switch v1.WorkloadPhase(run.Status) {
	case v1.WorkloadSucceeded, v1.WorkloadFailed, v1.WorkloadStopped:
	default:
		return "", commonerrors.NewConflict(fmt.Sprintf("run %s has not finished", run.RunID))
	}
	if run.CleanupRequest != "" {
		return "", commonerrors.NewConflict(fmt.Sprintf("a checkpoint cleanup of run %s is in progress", run.RunID))
	}
	if err := h.exportRunning(ctx, h.client, run.RunID); err != nil {
		return "", err
	}
	var rec recipe.Recipe
	if err := json.Unmarshal([]byte(run.Recipe), &rec); err != nil || rec.Output.Base == "" {
		return "", commonerrors.NewInternalError(fmt.Sprintf("run %s has no run directory", run.RunID))
	}
	base := rec.Output.Base
	plan := checkpointCleanup{Keep: keep}
	switch keep {
	case "none":
		plan.Paths = []string{base + "/checkpoints", base + "/critic_checkpoints"}
	case "best":
		m, err := h.checkpointMarkers(ctx, run)
		if err != nil {
			return "", err
		}
		if m.BestCheckpoint == nil {
			return "", commonerrors.NewBadRequest("the run has no checkpoint with a validation loss to keep")
		}
		// The list of steps is there when it is empty: it tells the event of this cleanup from an older one.
		steps := []int{}
		for _, cp := range m.Checkpoints {
			switch {
			case cp.Step == m.BestCheckpoint.Step && cp.Deleted:
				// Retention or an earlier cleanup removed it: what is left is not the best one.
				return "", commonerrors.NewBadRequest("the best checkpoint of the run is already deleted")
			case !cp.Deleted && cp.Step != m.BestCheckpoint.Step:
				plan.Paths = append(plan.Paths, fmt.Sprintf("%s/checkpoints/iter_%07d", base, checkpointIteration(cp.Step)))
				steps = append(steps, cp.Step)
			}
		}
		kept := m.BestCheckpoint.Step
		plan.KeptStep, plan.CleanedSteps = &kept, &steps
	default:
		return "", commonerrors.NewBadRequest(`keep must be "best" or "none"`)
	}
	data, _ := json.Marshal(plan)
	return string(data), nil
}

// checkpointMarkers returns the run's checkpoints with the best one and the deleted ones marked, from its main
// metric and validation loss.
func (h *Handler) checkpointMarkers(ctx context.Context, run *dbclient.PostTrainingRun) (markers, error) {
	main := "train/loss"
	if run.TrainType == registry.TrainTypeRL {
		main = "rollout/raw_reward"
	}
	rows, err := h.db.ListPostTrainingRunMetrics(ctx, run.RunID, []string{main, valLossMetric})
	if err != nil {
		return markers{}, commonerrors.NewInternalError("failed to read the run's metrics: " + err.Error())
	}
	points := map[string][]*dbclient.PostTrainingRunMetric{}
	for _, row := range rows {
		points[row.Name] = append(points[row.Name], row)
	}
	return buildMarkers(run, points), nil
}

// authorizeModelDelete checks the caller may delete a model the way the model API does: its owner, or a role that
// grants model deletion; a missing model needs no check.
func (h *Handler) authorizeModelDelete(c *gin.Context, name string) error {
	m := &v1.Model{}
	if err := h.client.Get(c.Request.Context(), ctrlclient.ObjectKey{Name: name}, m); err != nil {
		return ctrlclient.IgnoreNotFound(err)
	}
	in := authority.AccessInput{Context: c.Request.Context(), ResourceKind: v1.ModelKind, Verb: v1.DeleteVerb,
		UserId: c.GetString(common.UserId), ResourceOwner: v1.GetUserId(m)}
	if m.Spec.Workspace != "" {
		in.Workspaces = []string{m.Spec.Workspace}
	}
	return h.access.Authorize(in)
}

// sourceModelEnv is the environment variable by which a workload names the model it runs on.
const sourceModelEnv = "PRIMUS_SOURCE_MODEL"

// checkModelUnused refuses to delete the run's output model while a running or pending workload other than the
// run's own still runs on it, the way the model API does.
func (h *Handler) checkModelUnused(ctx context.Context, run *dbclient.PostTrainingRun) error {
	workloads := &v1.WorkloadList{}
	if err := h.client.List(ctx, workloads); err != nil {
		return commonerrors.NewInternalError("failed to check the workloads of the model: " + err.Error())
	}
	var inUse []string
	for i := range workloads.Items {
		w := &workloads.Items[i]
		if w.GetEnv(sourceModelEnv) != run.OutputModelID || v1.GetLabel(w, v1.PostTrainingRunIdLabel) == run.RunID {
			continue
		}
		if w.Status.Phase == v1.WorkloadRunning || w.Status.Phase == v1.WorkloadPending {
			inUse = append(inUse, w.Name)
		}
	}
	// A run that takes the model as its base reads the model's weights, and does not name it the way a workload
	// that serves it does.
	based, _, err := h.db.ListPostTrainingRuns(ctx, dbclient.PostTrainingRunFilter{BaseModelID: run.OutputModelID,
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}})
	if err != nil {
		return commonerrors.NewInternalError("failed to check the runs of the model: " + err.Error())
	}
	for _, r := range based {
		if r.RunID != run.RunID {
			inUse = append(inUse, r.RunID)
		}
	}
	if len(inUse) > 0 {
		return commonerrors.NewBadRequest(fmt.Sprintf("cannot delete model %s: running/pending workloads exist: %v. "+
			"Stop or delete these workloads first, or delete the run without its model", run.OutputModelID, inUse))
	}
	return nil
}

// checkModelDelete checks that the caller may delete the output model of a run together with the run, and that no
// other workload runs on that model; a run with no model of its own to delete passes.
func (h *Handler) checkModelDelete(c *gin.Context, run *dbclient.PostTrainingRun) error {
	if run.OutputModelID == "" {
		return nil
	}
	if err := h.authorizeModelDelete(c, run.OutputModelID); err != nil {
		return err
	}
	return h.checkModelUnused(c.Request.Context(), run)
}

// stopRun stops the run's training workload, its evaluation services except kept ones whose evaluation is over,
// and its checkpoint exports; the controller then cancels the evaluation jobs and records the run. It reports
// whether anything was still running.
func (h *Handler) stopRun(ctx context.Context, run *dbclient.PostTrainingRun, message string) (bool, error) {
	return h.endRun(ctx, run, message, false, false)
}

// endRun stops what stopRun stops; deleting the run also stops its backfill and, when its model goes too, its
// kept services, and deletes its unfinished OpsJobs, which the controller no longer follows once the run is
// deleted. It reports whether anything was still running.
func (h *Handler) endRun(ctx context.Context, run *dbclient.PostTrainingRun, message string,
	deleting, deleteModel bool) (bool, error) {
	workloads := &v1.WorkloadList{}
	if err := h.client.List(ctx, workloads, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
		return false, err
	}
	names := []string{run.WorkloadID}
	for i := range workloads.Items {
		w := &workloads.Items[i]
		switch v1.GetLabel(w, v1.PostTrainingRoleLabel) {
		case v1.PostTrainingRoleEval:
			if v1.GetAnnotation(w, v1.PostTrainingEvalDoneAnnotation) == v1.TrueStr && !deleteModel {
				continue
			}
		case v1.PostTrainingRoleExport:
		case v1.PostTrainingRoleBackfill:
			if !deleting {
				continue
			}
		default:
			continue
		}
		names = append(names, w.Name)
	}
	stopped := false
	for i := range workloads.Items {
		if w := &workloads.Items[i]; !w.IsEnd() && slices.Contains(names, w.Name) {
			stopped = true
		}
	}
	for _, name := range names {
		if err := h.stopWorkload(ctx, name, message); err != nil {
			return stopped, err
		}
	}
	if !deleting {
		return stopped, nil
	}
	jobs := &v1.OpsJobList{}
	if err := h.client.List(ctx, jobs, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
		return stopped, err
	}
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if job.IsEnd() || job.Status.Phase == v1.OpsJobSucceeded || job.Status.Phase == v1.OpsJobFailed {
			continue
		}
		if task := job.GetParameter(v1.ParameterEvalTaskId); task != nil {
			if err := h.db.UpdateEvaluationTaskStatus(ctx, task.Value, dbclient.EvaluationTaskStatusCancelled); err != nil {
				klog.ErrorS(err, "failed to cancel an evaluation task", "task", task.Value)
			}
		}
		if err := h.client.Delete(ctx, job); err != nil && !apierrors.IsNotFound(err) {
			return stopped, err
		}
		stopped = true
	}
	return stopped, nil
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
	if err := h.deleteRun(c, run, c.Query("deleteModel") == "true"); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"runId": run.RunID})
}

// deleteRun stops the run's workload, deletes its output model when asked and soft-deletes the run with its metric
// points.
func (h *Handler) deleteRun(c *gin.Context, run *dbclient.PostTrainingRun, deleteModel bool) error {
	ctx := c.Request.Context()
	if deleteModel {
		if err := h.checkModelDelete(c, run); err != nil {
			return err
		}
	}
	if _, err := h.endRun(ctx, run, fmt.Sprintf("Deleted by %s", c.GetString(common.UserName)), true,
		deleteModel); err != nil {
		return err
	}
	if deleteModel && run.OutputModelID != "" {
		m := &v1.Model{}
		m.Name = run.OutputModelID
		if err := h.client.Delete(ctx, m); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err := h.db.SetPostTrainingRunDeleted(ctx, run.RunID); err != nil {
		return commonerrors.NewInternalError("failed to delete the run: " + err.Error())
	}
	if err := h.db.DeletePostTrainingRunMetrics(ctx, run.RunID); err != nil {
		klog.ErrorS(err, "failed to delete the run's metric points", "run", run.RunID)
	}
	if err := h.db.DeletePostTrainingRunSamples(ctx, run.RunID, nil); err != nil {
		klog.ErrorS(err, "failed to delete the run's samples", "run", run.RunID)
	}
	klog.Infof("delete post training run %s by user %s", run.RunID, c.GetString(common.UserName))
	return nil
}
