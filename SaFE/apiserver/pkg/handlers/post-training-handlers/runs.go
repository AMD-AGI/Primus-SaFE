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

// listItem is a run of the list with how many runs of its recipe hash succeeded on the platform, and for an imported
// run what the list shows of its import.
type listItem struct {
	*dbclient.PostTrainingRun
	VerifiedRuns      int         `json:"verifiedRuns,omitempty"`
	LatestVerifiedRun string      `json:"latestVerifiedRun,omitempty"`
	Import            *listImport `json:"import,omitempty"`
}

// listImport is where and when an imported run ran and was imported, its failed gate and its full --load path.
type listImport struct {
	SlurmJobID    string      `json:"slurmJobId,omitempty"`
	SourcePath    string      `json:"sourcePath"`
	Via           string      `json:"via"`
	Verdict       string      `json:"verdict,omitempty"`
	ImportedAt    *time.Time  `json:"importedAt,omitempty"`
	FailedGate    *importGate `json:"failedGate,omitempty"`
	BaseModelPath string      `json:"baseModelPath,omitempty"`
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
	hashes := make([]string, 0, len(runs))
	for _, r := range runs {
		r.Recipe = ""
		if r.RecipeHash != "" {
			hashes = append(hashes, r.RecipeHash)
		}
	}
	records, err := h.db.CountPostTrainingRunsByRecipe(c.Request.Context(), hashes)
	if err != nil {
		klog.ErrorS(err, "failed to count the successful runs of the listed recipes")
	}
	items := make([]listItem, 0, len(runs))
	for _, r := range runs {
		rec := records[r.RecipeHash]
		items = append(items, listItem{PostTrainingRun: r, VerifiedRuns: rec.Succeeded, LatestVerifiedRun: rec.LatestRun,
			Import: h.listImportOf(c.Request.Context(), r)})
	}
	c.JSON(http.StatusOK, listResponse{Total: total, Items: items})
}

// listImportOf reads what the list shows of an imported run's import, or nil for a SaFE run or an unreadable record.
func (h *Handler) listImportOf(ctx context.Context, run *dbclient.PostTrainingRun) *listImport {
	if !imported(run) {
		return nil
	}
	imp, err := h.db.GetPostTrainingImportByRun(ctx, run.RunID)
	if err != nil {
		klog.ErrorS(err, "failed to read the import of a run", "run", run.RunID)
		return nil
	}
	out := &listImport{SlurmJobID: imp.SlurmJobID, SourcePath: imp.SourcePath, Via: imp.Via, Verdict: imp.Verdict}
	if imp.CreatedAt.Valid {
		out.ImportedAt = &imp.CreatedAt.Time
	}
	s := &importSummary{}
	if json.Unmarshal([]byte(imp.Summary), s) == nil {
		out.FailedGate = previewOf(s).FailedGate
		_ = json.Unmarshal(s.Arguments.Kept["load"], &out.BaseModelPath)
	}
	return out
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

// getWritableRun is getRun for the endpoints that change a run or start work from it, which an imported run refuses:
// SaFE has no recipe, workload or output model for it.
func (h *Handler) getWritableRun(c *gin.Context, verb v1.RoleVerb) (*dbclient.PostTrainingRun, error) {
	run, err := h.getRun(c, verb)
	if err != nil {
		return nil, err
	}
	if imported(run) {
		return nil, commonerrors.NewBadRequest("Imported runs are read-only.")
	}
	return run, nil
}

// imported tells whether a run was imported rather than trained on the platform.
func imported(run *dbclient.PostTrainingRun) bool {
	return run.Source != "" && run.Source != dbclient.PostTrainingSourceSafe
}

// runImportResponse is GET /post-training/runs/:id/import: the parsed data of an imported run that its row and
// metric points do not hold, as the import recorded it.
type runImportResponse struct {
	ImportID      string          `json:"importId"`
	SourcePath    string          `json:"sourcePath"`
	Format        string          `json:"format"`
	SlurmJobID    string          `json:"slurmJobId,omitempty"`
	Via           string          `json:"via"`
	Verdict       string          `json:"verdict,omitempty"`
	ParserVersion string          `json:"parserVersion,omitempty"`
	ImportedAt    *time.Time      `json:"importedAt,omitempty"`
	ImportedBy    string          `json:"importedBy,omitempty"`
	Summary       json.RawMessage `json:"summary"`
	Resources     json.RawMessage `json:"resources"`
	Warnings      json.RawMessage `json:"warnings"`
}

// GetRunImport returns the gates, markers, check output, resource samples, checkpoints and kept arguments of an
// imported run.
func (h *Handler) GetRunImport(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if !imported(run) {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("run %s is not imported",
			run.RunID)))
		return
	}
	imp, err := h.db.GetPostTrainingImportByRun(c.Request.Context(), run.RunID)
	if err != nil {
		apiutils.AbortWithApiError(c, notFoundOr(err, "the import of run "+run.RunID))
		return
	}
	resp := runImportResponse{ImportID: imp.ImportID, SourcePath: imp.SourcePath, Format: imp.Format,
		SlurmJobID: imp.SlurmJobID, Via: imp.Via, Verdict: imp.Verdict, ParserVersion: imp.ParserVersion,
		ImportedBy: imp.UserName, Summary: rawOrNull(imp.Summary), Resources: rawOrNull(imp.Resources),
		Warnings: rawOrNull(imp.Warnings)}
	if imp.CreatedAt.Valid {
		resp.ImportedAt = &imp.CreatedAt.Time
	}
	c.JSON(http.StatusOK, resp)
}

// rawOrNull passes stored JSON through, or null when the column is empty.
func rawOrNull(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(s)
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
	run, err := h.getWritableRun(c, v1.DeleteVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	message := fmt.Sprintf("Stopped by %s", c.GetString(common.UserName))
	if err := h.stopWorkload(c.Request.Context(), run.WorkloadID, message); err != nil {
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

// checkpointCleanup is the cleanup_request column: the directories the controller removes and the step kept.
type checkpointCleanup struct {
	Keep     string   `json:"keep"`
	Paths    []string `json:"paths"`
	KeptStep *int     `json:"keptStep,omitempty"`
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
	run, err := h.getWritableRun(c, v1.DeleteVerb)
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
	if err = h.db.UpdatePostTrainingRun(ctx, run.RunID, map[string]interface{}{"cleanup_request": plan}); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to request the cleanup: "+err.Error()))
		return
	}
	klog.Infof("clean up the checkpoints of post training run %s (keep %s) by user %s", run.RunID, body.Keep,
		c.GetString(common.UserName))
	c.JSON(http.StatusAccepted, gin.H{"runId": run.RunID, "keep": body.Keep})
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
	exports, err := h.listExports(ctx, run.RunID)
	if err != nil {
		return "", commonerrors.NewInternalError("failed to list the run's exports: " + err.Error())
	}
	for _, e := range exports {
		switch v1.WorkloadPhase(e.Status) {
		case v1.WorkloadSucceeded, v1.WorkloadFailed, v1.WorkloadStopped:
		default:
			if e.WorkloadID != "" {
				return "", commonerrors.NewConflict(fmt.Sprintf("step %d of run %s is being exported", e.Step, run.RunID))
			}
		}
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
		for _, cp := range m.Checkpoints {
			if !cp.Deleted && cp.Step != m.BestCheckpoint.Step {
				plan.Paths = append(plan.Paths, fmt.Sprintf("%s/checkpoints/iter_%07d", base, cp.Step))
			}
		}
		kept := m.BestCheckpoint.Step
		plan.KeptStep = &kept
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
	if imported(run) {
		// Only SaFE's record goes: the run directory on Spur is not touched, and the directory can be imported again.
		if err := h.db.DeletePostTrainingImportOfRun(ctx, run.RunID); err != nil {
			return commonerrors.NewInternalError("failed to delete the import record: " + err.Error())
		}
		deleteModel = false
	} else if err := h.stopWorkload(ctx, run.WorkloadID, fmt.Sprintf("Deleted by %s",
		c.GetString(common.UserName))); err != nil {
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
