/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// exportRequest asks to export one saved checkpoint of a finished run as a new model.
type exportRequest struct {
	Step        int    `json:"step"`
	DisplayName string `json:"displayName,omitempty"`
	PeftAdapter bool   `json:"peftAdapter,omitempty"`
}

// exportOnly is the part of an export job's recipe that turns the launcher into a one-checkpoint export; the
// controller reads the display name of the new model from it.
type exportOnly struct {
	Step        int    `json:"step"`
	Dir         string `json:"dir"`
	DisplayName string `json:"displayName"`
	PeftAdapter bool   `json:"peftAdapter,omitempty"`
}

// ExportCheckpoint exports one saved checkpoint of a finished run as a new model, in a one-node job that uses the
// run's image so the checkpoint format matches. The run's status and outputs do not change.
func (h *Handler) ExportCheckpoint(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	var req exportRequest
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
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if err = h.authorize(c, v1.CreateVerb, run.Workspace, "", "", run.Priority); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	w, err := h.exportWorkload(c, reg, run, &req)
	if err == nil {
		err = h.checkExportConflict(c, run.RunID, req.Step)
	}
	if err == nil {
		err = h.client.Create(c.Request.Context(), w)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("export step %d of post training run %s by user %s", req.Step, run.RunID, c.GetString(common.UserName))
	c.JSON(http.StatusAccepted, gin.H{"runId": run.RunID, "step": req.Step, "workloadId": w.Name})
}

// checkExportConflict refuses a second export of a checkpoint while an earlier one runs or after it succeeded; a
// failed export can be retried.
func (h *Handler) checkExportConflict(c *gin.Context, runID string, step int) error {
	list := &v1.WorkloadList{}
	if err := h.client.List(c.Request.Context(), list, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: runID,
		v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport, v1.PostTrainingExportStepLabel: strconv.Itoa(step)}); err != nil {
		return commonerrors.NewInternalError("failed to list the run's exports: " + err.Error())
	}
	for i := range list.Items {
		if w := &list.Items[i]; !w.IsEnd() || w.Status.Phase == v1.WorkloadSucceeded {
			return commonerrors.NewConflict(fmt.Sprintf("step %d is already exported or being exported", step))
		}
	}
	return nil
}

// exportWorkload checks an export request against the run and its kept checkpoints and builds the export job.
func (h *Handler) exportWorkload(c *gin.Context, reg *registry.Registry, run *dbclient.PostTrainingRun,
	req *exportRequest) (*v1.Workload, error) {
	ctx := c.Request.Context()
	switch v1.WorkloadPhase(run.Status) {
	case v1.WorkloadSucceeded, v1.WorkloadFailed, v1.WorkloadStopped:
	default:
		return nil, commonerrors.NewConflict("export becomes available when the run ends")
	}
	if req.PeftAdapter && run.Method != registry.MethodLora {
		return nil, commonerrors.NewBadRequest("only LoRA runs have a PEFT adapter to export")
	}
	var rec recipe.Recipe
	if err := json.Unmarshal([]byte(run.Recipe), &rec); err != nil || rec.Output.Base == "" {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("run %s has no run directory", run.RunID))
	}
	m, err := h.checkpointMarkers(ctx, run)
	if err != nil {
		return nil, err
	}
	var found *checkpointMark
	for i := range m.Checkpoints {
		if m.Checkpoints[i].Step == req.Step {
			found = &m.Checkpoints[i]
		}
	}
	switch {
	case found == nil:
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("step %d has no checkpoint", req.Step))
	case found.Deleted:
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("the checkpoint of step %d was deleted", req.Step))
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = fmt.Sprintf("%s-step%d", run.DisplayName, req.Step)
	}
	snapshot := map[string]interface{}{}
	if err = json.Unmarshal([]byte(run.Recipe), &snapshot); err != nil {
		return nil, commonerrors.NewInternalError("failed to read the recipe snapshot: " + err.Error())
	}
	snapshot["exportOnly"] = exportOnly{Step: req.Step, Dir: fmt.Sprintf("%s/exports/step-%d", rec.Output.Base, req.Step),
		DisplayName: name, PeftAdapter: req.PeftAdapter}
	recipeJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to encode the export recipe: " + err.Error())
	}
	ws, err := h.getWorkspace(ctx, run.Workspace)
	if err != nil {
		return nil, err
	}
	if ws.flavor == nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("workspace %s has no node flavor", ws.ID))
	}
	env := map[string]string{
		common.RayJobEntrypoint: base64.StdEncoding.EncodeToString([]byte(launcherEntrypoint)),
		recipeEnv:               base64.StdEncoding.EncodeToString(recipeJSON),
		"PYTHONUNBUFFERED":      "1",
	}
	if img := reg.ImageByName(rec.Provenance.Image); img != nil {
		for k, v := range img.Env {
			env[k] = v
		}
	}
	head := nodeResource(ws.flavor, rec.Layout.GpusPerNode)
	head.Replica = 1
	ttl := runTTLSeconds
	w := &v1.Workload{}
	w.Name = commonutils.GenerateName(fmt.Sprintf("%s-x%d", run.RunID, req.Step))
	w.Labels = map[string]string{
		v1.UserIdLabel:                 c.GetString(common.UserId),
		v1.PostTrainingRunIdLabel:      run.RunID,
		v1.PostTrainingRoleLabel:       v1.PostTrainingRoleExport,
		v1.PostTrainingExportStepLabel: strconv.Itoa(req.Step),
	}
	w.Annotations = map[string]string{v1.UserNameAnnotation: c.GetString(common.UserName)}
	w.Spec = v1.WorkloadSpec{
		GroupVersionKind:        v1.GroupVersionKind{Kind: common.RayJobKind, Version: common.DefaultVersion},
		Images:                  []string{rec.Provenance.Image},
		EntryPoints:             []string{base64.StdEncoding.EncodeToString([]byte(nodeInitScript))},
		Resources:               []v1.WorkloadResource{head},
		Env:                     env,
		Priority:                run.Priority,
		Workspace:               run.Workspace,
		TTLSecondsAfterFinished: &ttl,
	}
	return w, nil
}

// exportItem is one exported checkpoint of a run: its latest export job and, once registered, its model.
type exportItem struct {
	Step        int    `json:"step"`
	WorkloadID  string `json:"workloadId,omitempty"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	ModelID     string `json:"modelId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	ModelPhase  string `json:"modelPhase,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

// listExports returns the run's checkpoint exports by step, newest step first.
func (h *Handler) listExports(ctx context.Context, runID string) ([]exportItem, error) {
	byStep := map[int]*exportItem{}
	item := func(label string) *exportItem {
		step, err := strconv.Atoi(label)
		if err != nil {
			return nil
		}
		if byStep[step] == nil {
			byStep[step] = &exportItem{Step: step}
		}
		return byStep[step]
	}
	jobs := &v1.WorkloadList{}
	if err := h.client.List(ctx, jobs, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: runID,
		v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport}); err != nil {
		return nil, err
	}
	sort.Slice(jobs.Items, func(i, j int) bool {
		return jobs.Items[i].CreationTimestamp.Before(&jobs.Items[j].CreationTimestamp)
	})
	for i := range jobs.Items {
		w := &jobs.Items[i]
		if it := item(v1.GetLabel(w, v1.PostTrainingExportStepLabel)); it != nil {
			it.WorkloadID, it.Status, it.Message = w.Name, string(w.Status.Phase), w.Status.Message
			it.CreatedAt = w.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	models := &v1.ModelList{}
	if err := h.client.List(ctx, models, ctrlclient.MatchingLabels{v1.PostTrainingRunIdLabel: runID}); err != nil {
		return nil, err
	}
	for i := range models.Items {
		m := &models.Items[i]
		if !v1.HasLabel(m, v1.PostTrainingExportStepLabel) {
			continue
		}
		if it := item(v1.GetLabel(m, v1.PostTrainingExportStepLabel)); it != nil {
			it.ModelID, it.DisplayName, it.ModelPhase = m.Name, baseModelName(m), string(m.Status.Phase)
		}
	}
	out := make([]exportItem, 0, len(byStep))
	for _, it := range byStep {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Step > out[j].Step })
	return out, nil
}
